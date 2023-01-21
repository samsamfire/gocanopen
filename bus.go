package canopen

import (
	"github.com/brutella/can"
)

// The Bus interface should implement the following methods
type Bus interface {
	Send(frame BufferTxFrame) error
	Receive(frame *BufferRxFrame) error
}

// SocketCANBus implements the above interface

type SocketCANBus struct {
	Bus *can.Bus
}

// Send a frame on the bus
func (sbus *SocketCANBus) Send(frame BufferTxFrame) error {
	// Convert frame to brutella struct, TODO change this in the future, rather unnecessary
	new_frame := can.Frame{ID: frame.Ident, Length: frame.DLC, Flags: 0, Res0: 0, Res1: 0, Data: frame.Data}
	return sbus.Bus.Publish(new_frame)

}

func (sbus *SocketCANBus) Receive(frame *BufferRxFrame) error {
	return nil
}
