package socketcanv3

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"unsafe"

	canopen "github.com/samsamfire/gocanopen"
	can "github.com/samsamfire/gocanopen/pkg/can"
	"golang.org/x/sys/unix"
)

const (
	SocketCANFrameSize = 16
)

func init() {
	can.RegisterInterface("socketcanv3", NewBus)
}

type CANframe struct {
	id   uint32
	dlc  uint8
	pad  uint8
	res0 uint8
	res1 uint8
	data [8]uint8
}

type Bus struct {
	fd         int
	rxCallback canopen.FrameListener
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	logger     *slog.Logger
}

const (
	canFrameSize = 16
	// The maximum number of CAN frames to read at once (batch size)
	msgBatchSize = 64
)

var defaultTimeSpec = unix.Timespec{}
var defaultTimeVal = unix.Timeval{}

func init() {
	// Let go infer the type on startup
	// because these values are architecture dependent
	defaultTimeSpec.Nsec = 100_000_000 // 100 ms
	defaultTimeVal.Usec = 100_000      // 100 ms
}

// CANFrame represents the structure of a CAN frame, matching the C layout.
type CANFrame struct {
	ID   uint32
	Len  uint8
	_    [3]uint8 // Padding
	Data [8]uint8
}

// Create a new SocketCAN bus. This expects the CAN channel to be up.
// e.g. running "ip a" should show can0 or something similar.
func NewBus(channel string) (canopen.Bus, error) {
	iface, err := net.InterfaceByName(channel)
	if err != nil {
		return nil, err
	}

	fd, err := unix.Socket(unix.AF_CAN, unix.SOCK_RAW, unix.CAN_RAW)
	if err != nil {
		return nil, fmt.Errorf("failed to create CAN socket : %v", err)
	}
	err = unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &defaultTimeVal)
	if err != nil {
		return nil, fmt.Errorf("failed to set read timeout %v", err)
	}
	addr := &unix.SockaddrCAN{Ifindex: iface.Index}
	if err := unix.Bind(fd, addr); err != nil {
		return nil, err
	}
	socketcan := &Bus{fd: fd, logger: slog.Default()}
	return socketcan, nil
}

// "Connect" implementation of Bus interface
func (b *Bus) Connect(...any) error {
	var ctx context.Context
	ctx, b.cancel = context.WithCancel(context.Background())
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		b.processIncoming(ctx)
	}()
	return nil
}

// "Disconnect" implementation of Bus interface
func (b *Bus) Disconnect() error {
	if b.cancel == nil {
		return nil
	}
	b.cancel()
	b.wg.Wait()
	//b.f.Close()
	return nil
}

// "Send" implementation of Bus interface
func (b *Bus) Send(frame canopen.Frame) error {
	canFrame := &CANframe{}
	canFrame.id = frame.ID
	canFrame.dlc = frame.DLC
	canFrame.pad = frame.Flags
	canFrame.data = frame.Data

	rawData := (*(*[16]byte)(unsafe.Pointer(canFrame)))[:]
	n, err := unix.Write(b.fd, rawData)
	if n != 16 || err != nil {
		return err
	}
	return nil
}
func (b *Bus) processIncoming(ctx context.Context) {

	if err := unix.SetNonblock(b.fd, false); err != nil {
		b.logger.Error("failed to set blocking mode", "err", err)
		return
	}

	canopenFrame := canopen.Frame{}
	frames := make([]CANFrame, msgBatchSize)
	iovecs := make([]unix.Iovec, msgBatchSize)
	mmsgs := make([]Mmsghdr, msgBatchSize)

	for i := range msgBatchSize {
		iovecs[i].Base = (*byte)(unsafe.Pointer(&frames[i]))
		iovecs[i].SetLen(canFrameSize)
		mmsgs[i].Hdr.Iov = &iovecs[i]
		mmsgs[i].Hdr.Iovlen = 1
	}

	for {
		select {
		case <-ctx.Done():
			b.logger.Info("exiting CAN bus reception, closed")
			return
		default:

			ts := unix.Timespec{
				Nsec: 10_000_000, // 10ms
			}

			n, _, errno := unix.Syscall6(
				unix.SYS_RECVMMSG,
				uintptr(b.fd),
				uintptr(unsafe.Pointer(&mmsgs[0])),
				uintptr(msgBatchSize),
				0, // Flags: 0 means "Wait for full batch or timeout"
				uintptr(unsafe.Pointer(&ts)),
				0,
			)

			if errno != 0 {
				if errno == unix.EAGAIN || errno == unix.EWOULDBLOCK || errno == unix.EINTR {
					continue
				}
				b.logger.Error("syscall error", "err", errno)
				return
			}

			nbMsg := int(n)
			if nbMsg == 0 {
				b.logger.Info("socket closed")
				return
			}

			for i := range nbMsg {
				frame := frames[i]
				canopenFrame.ID = frame.ID
				canopenFrame.DLC = frame.Len
				canopenFrame.Data = frame.Data
				if b.rxCallback != nil {
					b.rxCallback.Handle(canopenFrame)
				}
			}
		}
	}
}

// "Subscribe" implementation of Bus interface
func (b *Bus) Subscribe(rxCallback canopen.FrameListener) error {
	b.rxCallback = rxCallback
	return nil
}

// Enable own reception on the bus. CAN be useful when testing for example
func (b *Bus) SetReceiveOwn(enabled bool) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	b.logger.Info("setting option 'CAN_RAW_RECV_OWN_MSGS'", "fd", b.fd, "enabled", enabled)
	return unix.SetsockoptInt(b.fd, unix.SOL_CAN_RAW, unix.CAN_RAW_RECV_OWN_MSGS, enabledInt)
}

// Add some filtering to CAN bus
func (b *Bus) SetFilters(filters []unix.CanFilter) error {
	b.logger.Info("setting option 'CAN_RAW_FILTER'", "fd", b.fd, "filters", filters)
	return unix.SetsockoptCanRawFilter(b.fd, unix.SOL_CAN_RAW, unix.CAN_RAW_FILTER, filters)
}
