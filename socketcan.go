package canopen

import (
	"github.com/brutella/can"
)

// Basic wrapper for socketcan (this is the implementation used by brutella/can)
// This is a wrapper around brutella/can as Bus implementation example
// Adding a custom driver would be possible by changing the bellow implementations

type SocketcanBus struct {
	bus        *can.Bus
	rxCallback FrameListener
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
func (socketcan *SocketcanBus) Send(frame Frame) error {
	return socketcan.bus.Publish(
		can.Frame{
			ID:     frame.ID,
			Length: frame.DLC,
			Flags:  frame.Flags,
			Res0:   0,
			Res1:   0,
			Data:   frame.Data,
		})
}

// "Subscribe" implementation of Bus interface
func (socketcan *SocketcanBus) Subscribe(rxCallback FrameListener) error {
	socketcan.rxCallback = rxCallback
	// brutella/can defines a "Handle" interface for handling received CAN frames
	socketcan.bus.Subscribe(socketcan)
	return nil
}

// brutella/can specific "Handle" implementation
func (socketcan *SocketcanBus) Handle(frame can.Frame) {
	// Convert brutella frame to canopen frame
	socketcan.rxCallback.Handle(Frame{ID: frame.ID, DLC: frame.Length, Flags: frame.Flags, Data: frame.Data})
}

func NewSocketCanBus(name string) (*SocketcanBus, error) {
	bus, err := can.NewBusForInterfaceWithName(name)
	return &SocketcanBus{bus: bus}, err
}
