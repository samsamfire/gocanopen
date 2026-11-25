//go:build linux

package socketcanring

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	canopen "github.com/samsamfire/gocanopen/v2"
	can "github.com/samsamfire/gocanopen/v2/pkg/can"
	"golang.org/x/sys/unix"
)

// Constants for AF_PACKET Ring Buffer
const (
	ETH_P_CAN        = 0x000C // EtherType for CAN
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

// This implementation uses a kernel ring buffer for reception
// which can significantly reduce CPU usage for large amounts
// of read data, with low requirements on latency.
// It uses regular socket for sending
type Bus struct {
	channel       string
	ifindex       int
	txFd          int // -1 when closed
	rxFd          int // -1 when closed
	rxCallback    canopen.FrameListener
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	logger        *slog.Logger
	pollingPeriod time.Duration
	ringBuffer    []byte
	ringReq       unix.TpacketReq
}

func NewBus(channel string) (canopen.Bus, error) {
	iface, err := net.InterfaceByName(channel)
	if err != nil {
		return nil, err
	}
	bus := &Bus{
		channel:       channel,
		ifindex:       iface.Index,
		txFd:          -1,
		rxFd:          -1,
		logger:        slog.Default(),
		pollingPeriod: 10 * time.Millisecond,
	}
	if err := bus.open(); err != nil {
		return nil, err
	}
	return bus, nil
}

// Create the TX socket and the RX ring buffer
func (b *Bus) open() error {
	// Setup TX Socket (Standard AF_CAN)
	txFd, err := unix.Socket(unix.AF_CAN, unix.SOCK_RAW, unix.CAN_RAW)
	if err != nil {
		return fmt.Errorf("failed to create TX socket: %v", err)
	}
	addrCan := &unix.SockaddrCAN{Ifindex: b.ifindex}
	if err := unix.Bind(txFd, addrCan); err != nil {
		unix.Close(txFd)
		return fmt.Errorf("failed to bind TX socket: %v", err)
	}

	// Setup RX Socket (AF_PACKET)
	// Only CAN frames are received
	rxFd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW, int(htons(unix.ETH_P_CAN)))
	if err != nil {
		unix.Close(txFd)
		return fmt.Errorf("failed to create RX socket: %v", err)
	}

	// Set Packet Version to V2, the frame headers are parsed as unix.Tpacket2Hdr
	if err := unix.SetsockoptInt(rxFd, unix.SOL_PACKET, unix.PACKET_VERSION, unix.TPACKET_V2); err != nil {
		unix.Close(rxFd)
		unix.Close(txFd)
		return fmt.Errorf("failed to set TPACKET_V2: %v", err)
	}

	// Set Packet Reserve (matches tcpdump)
	// tcpdump reserves 4 bytes. This often fixes alignment issues on ARM.
	if err := unix.SetsockoptInt(rxFd, unix.SOL_PACKET, unix.PACKET_RESERVE, 4); err != nil {
		unix.Close(rxFd)
		unix.Close(txFd)
		return fmt.Errorf("failed to set PACKET_RESERVE: %v", err)
	}

	// Ring Parameters
	blockSize := 4096
	frameSize := 256
	blockNr := 64

	req := unix.TpacketReq{
		Block_size: uint32(blockSize),
		Block_nr:   uint32(blockNr),
		Frame_size: uint32(frameSize),
		Frame_nr:   uint32((blockSize / frameSize) * blockNr),
	}

	// Request Ring Buffer
	if err := unix.SetsockoptTpacketReq(rxFd, unix.SOL_PACKET, unix.PACKET_RX_RING, &req); err != nil {
		unix.Close(rxFd)
		unix.Close(txFd)
		return fmt.Errorf("failed to set PACKET_RX_RING (req=%+v): %v", req, err)
	}

	// Memory map
	totalSize := int(req.Block_size * req.Block_nr)
	data, err := unix.Mmap(rxFd, 0, totalSize, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		unix.Close(rxFd)
		unix.Close(txFd)
		return fmt.Errorf("failed to mmap: %v", err)
	}

	// Bind RX Socket
	sll := &unix.SockaddrLinklayer{
		Protocol: htons(ETH_P_CAN),
		Ifindex:  b.ifindex,
	}

	if err := unix.Bind(rxFd, sll); err != nil {
		unix.Munmap(data)
		unix.Close(rxFd)
		unix.Close(txFd)
		return fmt.Errorf("failed to bind RX socket: %v", err)
	}

	b.txFd = txFd
	b.rxFd = rxFd
	b.ringBuffer = data
	b.ringReq = req
	return nil
}

// Close the sockets and release the ring buffer
func (b *Bus) close() error {
	if b.rxFd < 0 {
		return nil
	}
	errUnmap := unix.Munmap(b.ringBuffer)
	errRx := unix.Close(b.rxFd)
	errTx := unix.Close(b.txFd)
	b.ringBuffer = nil
	b.rxFd = -1
	b.txFd = -1
	return errors.Join(errUnmap, errRx, errTx)
}

// The bus can be connected again after a disconnection
func (b *Bus) Connect(args ...any) error {
	if b.rxFd < 0 {
		if err := b.open(); err != nil {
			return err
		}
	}
	var ctx context.Context
	ctx, b.cancel = context.WithCancel(context.Background())
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		b.processIncoming(ctx)
	}()
	return nil
}

// This stops reception and closes the sockets
func (b *Bus) Disconnect() error {
	if b.cancel != nil {
		b.cancel()
		b.wg.Wait()
		b.cancel = nil
	}
	return b.close()
}

func (b *Bus) Send(frame canopen.Frame) error {
	canFrame := &CANFrame{}
	canFrame.ID = frame.ID
	canFrame.Len = frame.DLC
	canFrame.Data = frame.Data

	rawData := (*(*[16]byte)(unsafe.Pointer(canFrame)))[:]
	n, err := unix.Write(b.txFd, rawData)
	if err != nil {
		return err
	}
	if n != len(rawData) {
		return io.ErrShortWrite
	}
	return nil
}

func (b *Bus) processIncoming(ctx context.Context) {

	frameIdx := 0
	totalFrames := int(b.ringReq.Frame_nr)
	frameSize := int(b.ringReq.Frame_size)
	ticker := time.NewTicker(b.pollingPeriod)
	defer ticker.Stop()

	var canopenFrame canopen.Frame

	for {
		// SWEEP: Process ALL frames currently in the buffer
		for {
			// Calculate Offset
			offset := frameIdx * frameSize
			if offset >= len(b.ringBuffer) {
				frameIdx = 0
				offset = 0
			}

			// Read V2 Header
			headerBuf := b.ringBuffer[offset : offset+int(unsafe.Sizeof(unix.Tpacket2Hdr{}))]
			hdr := (*unix.Tpacket2Hdr)(unsafe.Pointer(&headerBuf[0]))

			// If Kernel owns the frame, we are caught up. Stop sweeping.
			// Status is shared with the kernel, access it atomically
			if atomic.LoadUint32(&hdr.Status)&unix.TP_STATUS_USER == 0 {
				break
			}

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
			atomic.StoreUint32(&hdr.Status, unix.TP_STATUS_KERNEL)

			// Move to next frame immediately
			frameIdx = (frameIdx + 1) % totalFrames
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			continue
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
	return unix.SetsockoptCanRawFilter(b.rxFd, unix.SOL_CAN_RAW, unix.CAN_RAW_FILTER, filters)
}

// Update polling period for RX
func (b *Bus) SetPollRx(period time.Duration) {
	b.pollingPeriod = period
}

func htons(v uint16) uint16 {
	data := make([]byte, 2)
	binary.BigEndian.PutUint16(data, v)
	return *(*uint16)(unsafe.Pointer(&data[0]))
}

// State returns the state of the CAN interface, from its up flag only: see
// can.LinkState.
func (b *Bus) State() (canopen.BusState, error) {
	return can.LinkState(b.channel)
}
