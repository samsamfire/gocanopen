package socketcanv2

import (
	"context"
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
	SocketCANFrameSize  = 16
	DefaultRcvTimeoutUs = 100000
)

func init() {
	can.RegisterInterface("socketcanv2", NewSocketCanBus)
}

type CANframe struct {
	id   uint32
	dlc  uint8
	pad  uint8
	res0 uint8
	res1 uint8
	data [8]uint8
}

type SocketcanBus struct {
	f          *os.File
	fd         int
	rxCallback canopen.FrameListener
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	logger     *slog.Logger
}

// Create a new SocketCAN bus. This expects the CAN channel to be up.
// e.g. running "ip a" should show can0 or something similar.
func NewSocketCanBus(channel string) (canopen.Bus, error) {
	iface, err := net.InterfaceByName(channel)
	if err != nil {
		return nil, err
	}

	fd, err := syscall.Socket(syscall.AF_CAN, syscall.SOCK_RAW, unix.CAN_RAW)
	if err != nil {
		return nil, fmt.Errorf("failed to create CAN socket : %v", err)
	}
	tv := syscall.Timeval{
		Sec:  0,
		Usec: int64(DefaultRcvTimeoutUs),
	}
	err = syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv)
	if err != nil {
		return nil, fmt.Errorf("failed to set read timeout %v", err)
	}
	addr := &unix.SockaddrCAN{Ifindex: iface.Index}
	if err := unix.Bind(fd, addr); err != nil {
		return nil, err
	}
	socketcan := &SocketcanBus{fd: fd, logger: slog.Default()}
	return socketcan, nil
}

// "Connect" implementation of Bus interface
func (s *SocketcanBus) Connect(...any) error {
	var ctx context.Context
	ctx, s.cancel = context.WithCancel(context.Background())
	s.f = os.NewFile(uintptr(s.fd), fmt.Sprintf("fd %d", s.fd))
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.processIncoming(ctx)
	}()
	return nil
}

// "Disconnect" implementation of Bus interface
func (s *SocketcanBus) Disconnect() error {
	if s.cancel == nil {
		return nil
	}
	s.cancel()
	s.wg.Wait()
	s.f.Close()
	return nil
}

// "Send" implementation of Bus interface
func (s *SocketcanBus) Send(frame canopen.Frame) error {
	canFrame := &CANframe{}
	canFrame.id = frame.ID
	canFrame.dlc = frame.DLC
	canFrame.pad = frame.Flags
	canFrame.data = frame.Data

	rawData := (*(*[16]byte)(unsafe.Pointer(canFrame)))[:]
	n, err := s.f.Write(rawData)
	if n != 16 || err != nil {
		return err
	}
	return nil
}

// process incoming frames. This is meant to be run inside of a goroutine
func (s *SocketcanBus) processIncoming(ctx context.Context) {
	frame := &CANframe{}
	canopenFrame := canopen.Frame{}
	rxFrame := make([]byte, SocketCANFrameSize)
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("exiting CAN bus reception, closed")
			return
		default:
			n, err := s.f.Read(rxFrame)
			if n != 16 || err != nil {
				s.logger.Info("exiting CAN bus reception")
				return
			}
			// Direct translation in CANFrame
			frame = (*CANframe)(unsafe.Pointer(&rxFrame[0]))
			// Copy into canopen.Frame structure
			canopenFrame.ID = frame.id
			canopenFrame.DLC = frame.dlc
			canopenFrame.Flags = frame.pad
			canopenFrame.Data = frame.data
			if s.rxCallback != nil {
				s.rxCallback.Handle(canopenFrame)
			}
		}
	}
}

// "Subscribe" implementation of Bus interface
func (s *SocketcanBus) Subscribe(rxCallback canopen.FrameListener) error {
	s.rxCallback = rxCallback
	return nil
}

// Enable own reception on the bus. CAN be useful when testing for example
func (s *SocketcanBus) SetReceiveOwn(enabled bool) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	s.logger.Info("setting option 'CAN_RAW_RECV_OWN_MSGS'", "fd", s.fd, "enabled", enabled)
	return unix.SetsockoptInt(s.fd, unix.SOL_CAN_RAW, unix.CAN_RAW_RECV_OWN_MSGS, enabledInt)
}

// Add some filtering to CAN bus
func (s *SocketcanBus) SetFilters(filters []unix.CanFilter) error {
	s.logger.Info("setting option 'CAN_RAW_FILTER'", "fd", s.fd, "filters", filters)
	return unix.SetsockoptCanRawFilter(s.fd, unix.SOL_CAN_RAW, unix.CAN_RAW_FILTER, filters)
}
