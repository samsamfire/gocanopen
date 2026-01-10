//go:build linux

package socketcanring

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"unsafe"

	canopen "github.com/samsamfire/gocanopen"
	can "github.com/samsamfire/gocanopen/pkg/can"
	"golang.org/x/sys/unix"
)

// Constants for AF_PACKET Ring Buffer
const (
	ETH_P_CAN        = 0x000C // EtherType for CAN
	TPACKET_V1       = 1
	TP_STATUS_USER   = 1
	TP_STATUS_COPY   = 2
	TP_STATUS_KERNEL = 0
)

func init() {
	can.RegisterInterface("socketcanring", NewBus)
}

// CANFrame represents the standard C struct for a CAN frame (16 bytes)
type CANFrame struct {
	ID   uint32
	Len  uint8
	_    [3]byte // Padding
	Data [8]byte
}

type Bus struct {
	txFd       int
	rxFd       int
	rxCallback canopen.FrameListener
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	logger     *slog.Logger

	ringBuffer []byte
	ringReq    unix.TpacketReq
}

func NewBus(channel string) (canopen.Bus, error) {
	iface, err := net.InterfaceByName(channel)
	if err != nil {
		return nil, err
	}

	// 1. Setup TX Socket (Standard AF_CAN)
	txFd, err := unix.Socket(unix.AF_CAN, unix.SOCK_RAW, unix.CAN_RAW)
	if err != nil {
		return nil, fmt.Errorf("failed to create TX socket: %v", err)
	}
	addrCan := &unix.SockaddrCAN{Ifindex: iface.Index}
	if err := unix.Bind(txFd, addrCan); err != nil {
		unix.Close(txFd)
		return nil, fmt.Errorf("failed to bind TX socket: %v", err)
	}

	// 2. Setup RX Socket (AF_PACKET)
	// Using ETH_P_ALL to ensure protocol setup doesn't interfere
	rxFd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW, int(htons(unix.ETH_P_ALL)))
	if err != nil {
		unix.Close(txFd)
		return nil, fmt.Errorf("failed to create RX socket: %v", err)
	}

	// 3. Set Packet Version to V1 (MATCHING TCPDUMP)
	if err := unix.SetsockoptInt(rxFd, unix.SOL_PACKET, unix.PACKET_VERSION, TPACKET_V1); err != nil {
		unix.Close(rxFd)
		unix.Close(txFd)
		return nil, fmt.Errorf("failed to set TPACKET_V1: %v", err)
	}

	// 4. Set Packet Reserve (MATCHING TCPDUMP)
	// tcpdump reserves 4 bytes. This often fixes alignment issues on ARM.
	if err := unix.SetsockoptInt(rxFd, unix.SOL_PACKET, unix.PACKET_RESERVE, 4); err != nil {
		unix.Close(rxFd)
		unix.Close(txFd)
		return nil, fmt.Errorf("failed to set PACKET_RESERVE: %v", err)
	}

	// 5. Ring Parameters
	// We go back to 4096 blocks because V1 handles small blocks much better than V2/V3.
	// Frame size 256 is safe for V1 (Header is smaller).
	blockSize := 4096
	frameSize := 256
	blockNr := 64

	req := unix.TpacketReq{
		Block_size: uint32(blockSize),
		Block_nr:   uint32(blockNr),
		Frame_size: uint32(frameSize),
		Frame_nr:   uint32((blockSize / frameSize) * blockNr),
	}

	// 6. Request Ring Buffer
	if err := unix.SetsockoptTpacketReq(rxFd, unix.SOL_PACKET, unix.PACKET_RX_RING, &req); err != nil {
		unix.Close(rxFd)
		unix.Close(txFd)
		return nil, fmt.Errorf("failed to set PACKET_RX_RING (req=%+v): %v", req, err)
	}

	// 7. Memory Map
	totalSize := int(req.Block_size * req.Block_nr)
	data, err := unix.Mmap(rxFd, 0, totalSize, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		unix.Close(rxFd)
		unix.Close(txFd)
		return nil, fmt.Errorf("failed to mmap: %v", err)
	}

	// 8. Bind RX Socket
	sll := &unix.SockaddrLinklayer{
		Protocol: htons(ETH_P_CAN),
		Ifindex:  iface.Index,
	}
	if err := unix.Bind(rxFd, sll); err != nil {
		unix.Munmap(data)
		unix.Close(rxFd)
		unix.Close(txFd)
		return nil, fmt.Errorf("failed to bind RX socket: %v", err)
	}

	return &Bus{
		txFd:       txFd,
		rxFd:       rxFd,
		ringBuffer: data,
		ringReq:    req,
		logger:     slog.Default(),
	}, nil
}

func (b *Bus) Connect(args ...any) error {
	var ctx context.Context
	ctx, b.cancel = context.WithCancel(context.Background())
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		b.processIncoming(ctx)
	}()
	return nil
}

func (b *Bus) Disconnect() error {
	if b.cancel == nil {
		return nil
	}
	b.cancel()
	b.wg.Wait()

	unix.Munmap(b.ringBuffer)
	unix.Close(b.rxFd)
	unix.Close(b.txFd)
	return nil
}

func (b *Bus) Send(frame canopen.Frame) error {
	canFrame := &CANFrame{}
	canFrame.ID = frame.ID
	canFrame.Len = frame.DLC
	canFrame.Data = frame.Data

	rawData := (*(*[16]byte)(unsafe.Pointer(canFrame)))[:]
	n, err := unix.Write(b.txFd, rawData)
	if n != 16 || err != nil {
		return err
	}
	return nil
}

func (b *Bus) processIncoming(ctx context.Context) {
	frameIdx := 0
	totalFrames := int(b.ringReq.Frame_nr)
	frameSize := int(b.ringReq.Frame_size)

	pfd := []unix.PollFd{
		{Fd: int32(b.rxFd), Events: unix.POLLIN},
	}

	canopenFrame := canopen.Frame{}

	for {
		select {
		case <-ctx.Done():
			return
		default:
			offset := frameIdx * frameSize
			if offset >= len(b.ringBuffer) {
				frameIdx = 0
				offset = 0
			}

			// READ V1 HEADER (Different struct than V2!)
			// TpacketHdr is the V1 header struct in Go unix package
			headerBuf := b.ringBuffer[offset : offset+int(unsafe.Sizeof(unix.TpacketHdr{}))]
			hdr := (*unix.TpacketHdr)(unsafe.Pointer(&headerBuf[0]))

			if uint64(hdr.Status)&uint64(unix.TP_STATUS_USER) == 0 {
				_, err := unix.Poll(pfd, 100)
				if err != nil && err != unix.EINTR {
					b.logger.Error("poll error", "err", err)
				}
				continue
			}

			// Calculate Data Pointer for V1
			// Mac is the offset to the MAC header (Ethernet/CAN header)
			dataStart := offset + int(hdr.Mac)

			if dataStart+16 <= len(b.ringBuffer) {
				rawFrame := b.ringBuffer[dataStart : dataStart+16]
				canFrame := (*CANFrame)(unsafe.Pointer(&rawFrame[0]))

				canopenFrame.ID = canFrame.ID
				canopenFrame.DLC = canFrame.Len
				canopenFrame.Data = canFrame.Data

				if b.rxCallback != nil {
					b.rxCallback.Handle(canopenFrame)
				}
			}

			// Return ownership to Kernel
			hdr.Status = unix.TP_STATUS_KERNEL
			frameIdx = (frameIdx + 1) % totalFrames
		}
	}
}

func (b *Bus) Subscribe(rxCallback canopen.FrameListener) error {
	b.rxCallback = rxCallback
	return nil
}

func (b *Bus) SetReceiveOwn(enabled bool) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	// Apply to TX socket (AF_CAN)
	return unix.SetsockoptInt(b.txFd, unix.SOL_CAN_RAW, unix.CAN_RAW_RECV_OWN_MSGS, enabledInt)
}

func (b *Bus) SetFilters(filters []unix.CanFilter) error {
	// Apply to TX socket only, does not affect AF_PACKET RX
	return unix.SetsockoptCanRawFilter(b.txFd, unix.SOL_CAN_RAW, unix.CAN_RAW_FILTER, filters)
}

func htons(v uint16) uint16 {
	data := make([]byte, 2)
	binary.BigEndian.PutUint16(data, v)
	return *(*uint16)(unsafe.Pointer(&data[0]))
}
