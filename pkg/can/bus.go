package can

import (
	"fmt"
)

const CanRtrFlag uint32 = 0x40000000
const CanSffMask uint32 = 0x000007FF

// CAN bus errors
const (
	CanErrorTxWarning   = 0x0001 // CAN transmitter warning
	CanErrorTxPassive   = 0x0002 // CAN transmitter passive
	CanErrorTxBusOff    = 0x0004 // CAN transmitter bus off
	CanErrorTxOverflow  = 0x0008 // CAN transmitter overflow
	CanErrorPdoLate     = 0x0080 // TPDO is outside sync window
	CanErrorRxWarning   = 0x0100 // CAN receiver warning
	CanErrorRxPassive   = 0x0200 // CAN receiver passive
	CanErrorRxOverflow  = 0x0800 // CAN receiver overflow
	CanErrorWarnPassive = 0x0303 // Combination
)

// A CAN frame
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

// A CAN Bus interface
type Bus interface {
	Connect(...any) error                   // Connect to the CAN bus
	Disconnect() error                      // Disconnect from CAN bus
	Send(frame Frame) error                 // Send a frame on the bus
	Subscribe(callback FrameListener) error // Subscribe to all received CAN frames
}

// Register a new CAN bus interface type
// This should be called inside an init() function of plugin
func RegisterInterface(interfaceType string, newInterface NewInterfaceFunc) {
	interfaceRegistry[interfaceType] = newInterface
}

type NewInterfaceFunc func(channel string) (Bus, error)

var interfaceRegistry = make(map[string]NewInterfaceFunc)

// Create a new CAN bus with given interface
// Currently supported : socketcan, virtualcan
func NewBus(canInterface string, channel string, bitrate int) (Bus, error) {
	createInterface, ok := interfaceRegistry[canInterface]
	if !ok {
		return nil, fmt.Errorf("unsupported interface : %v", canInterface)
	}
	return createInterface(channel)
}
