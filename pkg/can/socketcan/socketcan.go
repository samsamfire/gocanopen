package socketcan

import (
	sockcan "github.com/brutella/can"
	can "github.com/samsamfire/gocanopen/pkg/can"
)

// Basic wrapper for socketcan it uses the implementation
// that can be found here : https://github.com/brutella/can

func init() {
	can.RegisterInterface("socketcan", NewSocketCanBus)
}

type SocketcanBus struct {
	bus        *sockcan.Bus
	rxCallback can.FrameListener
}

// "Connect" implementation of Bus interface
func (socketcan *SocketcanBus) Connect(...any) error {
	go socketcan.bus.ConnectAndPublish()
	return nil
}

// "Disconnect" implementation of Bus interface
func (socketcan *SocketcanBus) Disconnect() error {
	return socketcan.bus.Disconnect()
}

// "Send" implementation of Bus interface
func (socketcan *SocketcanBus) Send(frame can.Frame) error {
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
func (socketcan *SocketcanBus) Subscribe(rxCallback can.FrameListener) error {
	socketcan.rxCallback = rxCallback
	// brutella/can defines a "Handle" interface for handling received CAN frames
	socketcan.bus.Subscribe(socketcan)
	return nil
}

// brutella/can specific "Handle" implementation
func (socketcan *SocketcanBus) Handle(frame sockcan.Frame) {
	// Convert brutella frame to canopen frame
	socketcan.rxCallback.Handle(can.Frame{ID: frame.ID, DLC: frame.Length, Flags: frame.Flags, Data: frame.Data})
}

func NewSocketCanBus(name string) (can.Bus, error) {
	bus, err := sockcan.NewBusForInterfaceWithName(name)
	return &SocketcanBus{bus: bus}, err
}
