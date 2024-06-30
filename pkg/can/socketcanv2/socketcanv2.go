package socketcanv2

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"unsafe"

	canopen "github.com/samsamfire/gocanopen"
	can "github.com/samsamfire/gocanopen/pkg/can"
	"golang.org/x/sys/unix"
)

type CANframe struct {
	id   uint32
	dlc  uint8
	pad  uint8
	res0 uint8
	res1 uint8
	data [8]uint8
}

func init() {
	can.RegisterInterface("socketcanv2", NewSocketCanBus)
}

type SocketcanBus struct {
	f          *os.File
	rxCallback canopen.FrameListener
}

// "Connect" implementation of Bus interface
func (socketcan *SocketcanBus) Connect(...any) error {
	frame := &CANframe{}
	canopenFrame := canopen.Frame{}
	go func() {
		rxFrame := make([]byte, 16)
		for {
			n, err := socketcan.f.Read(rxFrame)
			if n != 16 || err != nil {
				return
			}
			// Direct translation in CANFrame
			frame = (*CANframe)(unsafe.Pointer(&rxFrame[0]))
			// Copy into canopen.Frame structure
			canopenFrame.ID = frame.id
			canopenFrame.DLC = frame.dlc
			canopenFrame.Flags = frame.pad
			canopenFrame.Data = frame.data
			if socketcan.rxCallback != nil {
				socketcan.rxCallback.Handle(canopenFrame)
			}
		}
	}()
	return nil
}

// "Disconnect" implementation of Bus interface
func (socketcan *SocketcanBus) Disconnect() error {
	return nil
}

// "Send" implementation of Bus interface
func (socketcan *SocketcanBus) Send(frame canopen.Frame) error {
	canFrame := &CANframe{}
	canFrame.id = frame.ID
	canFrame.dlc = frame.DLC
	canFrame.pad = frame.Flags
	canFrame.data = frame.Data
	var rawData []byte = (*(*[16]byte)(unsafe.Pointer(canFrame)))[:]
	n, err := socketcan.f.Write(rawData)
	if n != 16 || err != nil {
		return err
	}
	return nil
}

// "Subscribe" implementation of Bus interface
func (socketcan *SocketcanBus) Subscribe(rxCallback canopen.FrameListener) error {
	socketcan.rxCallback = rxCallback
	return nil
}

func NewSocketCanBus(name string) (canopen.Bus, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, err
	}

	s, _ := syscall.Socket(syscall.AF_CAN, syscall.SOCK_RAW, unix.CAN_RAW)
	addr := &unix.SockaddrCAN{Ifindex: iface.Index}
	if err := unix.Bind(s, addr); err != nil {
		return nil, err
	}

	f := os.NewFile(uintptr(s), fmt.Sprintf("fd %d", s))
	socketcan := &SocketcanBus{f: f}
	return socketcan, nil
}
