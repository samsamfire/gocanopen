package socketcanv2

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"syscall"
	"unsafe"

	canopen "github.com/samsamfire/gocanopen"
	can "github.com/samsamfire/gocanopen/pkg/can"
	"golang.org/x/sys/unix"
)

const (
	SocketCANFrameSize = 16
)

func init() {
	can.RegisterInterface("socketcanv2", NewBus)
	can.RegisterInterface("socketcan", NewBus)
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
	f          *os.File
	fd         int
	rxCallback canopen.FrameListener
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	logger     *slog.Logger
}

// Create a new SocketCAN bus. This expects the CAN channel to be up.
// e.g. running "ip a" should show can0 or something similar.
func NewBus(channel string) (canopen.Bus, error) {
	iface, err := net.InterfaceByName(channel)
	if err != nil {
		return nil, err
	}

	fd, err := unix.Socket(unix.AF_CAN, unix.SOCK_RAW, unix.CAN_RAW)
	//fd, err := syscall.Socket(syscall.AF_CAN, syscall.SOCK_RAW, unix.CAN_RAW)
	if err != nil {
		return nil, fmt.Errorf("failed to create CAN socket : %v", err)
	}
	err = unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &DefaultTimeVal)
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
	b.f = os.NewFile(uintptr(b.fd), fmt.Sprintf("fd %d", b.fd))
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
	b.f.Close()
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
	n, err := b.f.Write(rawData)
	if n != 16 || err != nil {
		return err
	}
	return nil
}

// process incoming frames. This is meant to be run inside of a goroutine
func (b *Bus) processIncoming(ctx context.Context) {
	frame := &CANframe{}
	canopenFrame := canopen.Frame{}
	rxFrame := make([]byte, SocketCANFrameSize)
	for {
		select {
		case <-ctx.Done():
			b.logger.Info("exiting CAN bus reception, closed")
			return
		default:
			n, err := b.f.Read(rxFrame)
			if errors.Is(err, syscall.EAGAIN) {
				continue
			}
			if n != 16 || err != nil {
				b.logger.Info("exiting CAN bus reception", "error", err)
				return
			}
			// Direct translation in CANFrame
			frame = (*CANframe)(unsafe.Pointer(&rxFrame[0]))
			// Copy into canopen.Frame structure
			canopenFrame.ID = frame.id
			canopenFrame.DLC = frame.dlc
			canopenFrame.Flags = frame.pad
			canopenFrame.Data = frame.data
			if b.rxCallback != nil {
				b.rxCallback.Handle(canopenFrame)
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
