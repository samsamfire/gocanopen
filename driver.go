package canopen

import (
	"github.com/brutella/can"
)

// A CAN Bus interface that implements sending
type Bus interface {
	Send(frame BufferTxFrame) error    // Send a frame on the bus
	Subscribe(subscriber FrameHandler) // Subscribe to can frames
	Connect(...any) error
}

// Interface used for handling a CAN frame, implementation depends on the CAN object type
type FrameHandler interface {
	Handle(frame can.Frame)
}

// Basic implementation with socketcan (this is the implementation used by brutella/can)
// This is a "fake" wrapper around brutella/can as Bus implementation example
// Adding a custom driver would be possible by changing the bellow implementations
type SocketcanBus struct {
	bus *can.Bus
}

func (socketcan *SocketcanBus) Subscribe(framehandler FrameHandler) {
	socketcan.bus.Subscribe(framehandler)
}

func (socketcan *SocketcanBus) Send(frame BufferTxFrame) error {
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
