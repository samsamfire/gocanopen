package canopen

const (
	CanErrorTxWarning          = 0x0001 // CAN transmitter warning
	CanErrorTxPassive          = 0x0002 // CAN transmitter passive
	CanErrorTxBusOff           = 0x0004 // CAN transmitter bus off
	CanErrorTxOverflow         = 0x0008 // CAN transmitter overflow
	CanErrorPdoLate            = 0x0080 // TPDO is outside sync window
	CanErrorRxWarning          = 0x0100 // CAN receiver warning
	CanErrorRxPassive          = 0x0200 // CAN receiver passive
	CanErrorRxOverflow         = 0x0800 // CAN receiver overflow
	CanErrorWarnPassive        = 0x0303 // Combination
	CanRtrFlag          uint32 = 0x40000000
	CanSffMask          uint32 = 0x000007FF
)

// A CAN Bus interface
type Bus interface {
	Connect(...any) error                   // Connect to the CAN bus
	Disconnect() error                      // Disconnect from CAN bus
	Send(frame Frame) error                 // Send a frame on the bus
	Subscribe(callback FrameListener) error // Subscribe to all received CAN frames
}

// A generic 11bit CAN frame
type Frame struct {
	ID    uint32
	Flags uint8
	DLC   uint8
	Data  [8]byte
}

func NewFrame(id uint32, flags uint8, dlc uint8) Frame {
	return Frame{ID: id, Flags: flags, DLC: dlc}
}

// Interface for handling a received CAN frame
type FrameListener interface {
	Handle(frame Frame)
}
