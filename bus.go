package canopen

import (
	"github.com/brutella/can"
)

// The Bus interface should implement the following methods
type Bus interface {
	Send(frame CANtxMsg) error
	Receive(frame *CANrxMsg) error
}

// SocketCANBus implements the above interface

type SocketCANBus struct {
	bus can.Bus
}

// Send a frame on the bus
func (sbus *SocketCANBus) Send(frame CANtxMsg) error {
	// Convert frame to brutella struct, TODO change this in the future, rather unnecessary
	new_frame := can.Frame{ID: frame.Ident, Length: frame.DLC, Flags: 0, Res0: 0, Res1: 0, Data: frame.Data}
	return sbus.bus.Publish(new_frame)

}

// Receive a frame on the bus
func (sbus *SocketCANBus) Receive(frame *CANrxMsg) error {
	return nil
}
