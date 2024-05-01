package socketcan

import (
	sockcan "github.com/brutella/can"
	canopen "github.com/samsamfire/gocanopen"
	can "github.com/samsamfire/gocanopen/pkg/can"
)

// Basic wrapper for socketcan it uses the implementation
// that can be found here : https://github.com/brutella/can

func init() {
	can.RegisterInterface("socketcan", NewSocketCanBus)
}

type SocketcanBus struct {
	bus        *sockcan.Bus
	rxCallback canopen.FrameListener
}

// "Connect" implementation of Bus interface
func (socketcan *SocketcanBus) Connect(...any) error {
	go func() {
		err := socketcan.bus.ConnectAndPublish()
		if err != nil {
			return
		}
	}()
	return nil
}

// "Disconnect" implementation of Bus interface
func (socketcan *SocketcanBus) Disconnect() error {
	return socketcan.bus.Disconnect()
}

// "Send" implementation of Bus interface
func (socketcan *SocketcanBus) Send(frame canopen.Frame) error {
	return socketcan.bus.Publish(
		sockcan.Frame{
			ID:     frame.ID,
			Length: frame.DLC,
			Flags:  frame.Flags,
			Res0:   0,
			Res1:   0,
			Data:   frame.Data,
		})
}

// "Subscribe" implementation of Bus interface
func (socketcan *SocketcanBus) Subscribe(rxCallback canopen.FrameListener) error {
	socketcan.rxCallback = rxCallback
	// brutella/can defines a "Handle" interface for handling received CAN frames
	socketcan.bus.Subscribe(socketcan)
	return nil
}

// brutella/can specific "Handle" implementation
func (socketcan *SocketcanBus) Handle(frame sockcan.Frame) {
	// Convert brutella frame to canopen frame
	socketcan.rxCallback.Handle(canopen.Frame{ID: frame.ID, DLC: frame.Length, Flags: frame.Flags, Data: frame.Data})
}

func NewSocketCanBus(name string) (canopen.Bus, error) {
	bus, err := sockcan.NewBusForInterfaceWithName(name)
	return &SocketcanBus{bus: bus}, err
}
