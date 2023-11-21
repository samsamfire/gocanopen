package main

import (
	"canopen"

	"github.com/brutella/can"
)

// Basic implementation with socketcan (this is the implementation used by brutella/can)
// This is a wrapper around brutella/can as Bus implementation example
// Adding a custom driver would be possible by changing the bellow implementations
type SocketcanBus struct {
	bus          *can.Bus
	frameHandler canopen.FrameListener
}

func (socketcan *SocketcanBus) Subscribe(framehandler canopen.FrameListener) {
	// Socketcan will forward messages to frameHandler via Handle method
	socketcan.frameHandler = framehandler
	// brutella/can defines a "Handle" interface for handling received CAN frames
	socketcan.bus.Subscribe(socketcan)
}

func (socketcan *SocketcanBus) Handle(frame can.Frame) {
	// Convert brutella frame to canopen frame
	socketcan.frameHandler.Handle(canopen.Frame{ID: frame.ID, DLC: frame.Length, Flags: frame.Flags, Data: frame.Data})
}

func (socketcan *SocketcanBus) Send(frame canopen.BufferTxFrame) error {
	new_frame := can.Frame{ID: frame.Ident, Length: frame.DLC, Flags: 0, Res0: 0, Res1: 0, Data: frame.Data}
	return socketcan.bus.Publish(new_frame)
}

func NewSocketcanBus(name string) (SocketcanBus, error) {
	bus, err := can.NewBusForInterfaceWithName(name)
	return SocketcanBus{bus: bus}, err
}

func (socketcan *SocketcanBus) Connect(...any) error {
	go socketcan.bus.ConnectAndPublish()
	return nil
}
