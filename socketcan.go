package canopen

import (
	"github.com/brutella/can"
)

// Basic wrapper for socketcan (this is the implementation used by brutella/can)
// This is a wrapper around brutella/can as Bus implementation example
// Adding a custom driver would be possible by changing the bellow implementations

type SocketcanBus struct {
	bus          *can.Bus
	frameHandler FrameHandler
}

// "Send" implementation of Bus interface
func (socketcan *SocketcanBus) Send(frame BufferTxFrame) error {
	new_frame := can.Frame{ID: frame.Ident, Length: frame.DLC, Flags: 0, Res0: 0, Res1: 0, Data: frame.Data}
	return socketcan.bus.Publish(new_frame)
}

// "Subscribe" implementation of Bus interface
func (socketcan *SocketcanBus) Subscribe(framehandler FrameHandler) {
	socketcan.frameHandler = framehandler
	// brutella/can defines a "Handle" interface for handling received CAN frames
	socketcan.bus.Subscribe(socketcan)
}

// "Connect" implementation of Bus interface
func (socketcan *SocketcanBus) Connect(...any) error {
	go socketcan.bus.ConnectAndPublish()
	return nil
}

// brutella/can specific "Handle" implementation
func (socketcan *SocketcanBus) Handle(frame can.Frame) {
	// Convert brutella frame to canopen frame
	socketcan.frameHandler.Handle(Frame{ID: frame.ID, DLC: frame.Length, Flags: frame.Flags, Data: frame.Data})
}

func NewSocketcanBus(name string) (SocketcanBus, error) {
	bus, err := can.NewBusForInterfaceWithName(name)
	return SocketcanBus{bus: bus}, err
}
