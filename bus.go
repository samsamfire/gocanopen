package canopen

import (
	"fmt"

	log "github.com/sirupsen/logrus"
)

const canRtrFlag uint32 = 0x40000000
const canSffMask uint32 = 0x000007FF

// CAN bus errors
const (
	canErrorTxWarning   = 0x0001 // CAN transmitter warning
	canErrorTxPassive   = 0x0002 // CAN transmitter passive
	canErrorTxBusOff    = 0x0004 // CAN transmitter bus off
	canErrorTxOverflow  = 0x0008 // CAN transmitter overflow
	canErrorPdoLate     = 0x0080 // TPDO is outside sync window
	canErrorRxWarning   = 0x0100 // CAN receiver warning
	canErrorRxPassive   = 0x0200 // CAN receiver passive
	canErrorRxOverflow  = 0x0800 // CAN receiver overflow */
	canErrorWarnPassive = 0x0303 // Combination
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

// Bus manager is a wrapper around the CAN bus interface
// Used by the CANopen stack to control errors, callbacks for specific IDs, etc.
type busManager struct {
	bus            Bus // Bus interface that can be adapted
	frameListeners map[uint32][]FrameListener
	canError       uint16
}

// Implements the FrameListener interface
// This handles all received CAN frames from Bus
func (bm *busManager) Handle(frame Frame) {
	listeners, ok := bm.frameListeners[frame.ID]
	if !ok {
		return
	}
	for _, listener := range listeners {
		listener.Handle(frame)
	}
}

// Send a CAN message
// Limited error handling
func (bm *busManager) Send(frame Frame) error {
	err := bm.bus.Send(frame)
	if err != nil {
		log.Warnf("[CAN ERROR] %v", err)
	}
	return err
}

// This should be called cyclically to update errors
func (bm *busManager) process() error {
	// TODO get bus state error
	bm.canError = 0
	return nil
}

// Subscribe to a specific CAN ID
func (bm *busManager) Subscribe(ident uint32, mask uint32, rtr bool, callback FrameListener) error {
	ident = ident & canSffMask
	if rtr {
		ident |= canRtrFlag
	}
	_, ok := bm.frameListeners[ident]
	if !ok {
		bm.frameListeners[ident] = []FrameListener{callback}
		return nil
	}
	// TODO add error if callback exists already
	bm.frameListeners[ident] = append(bm.frameListeners[ident], callback)
	return nil
}

func NewBusManager(bus Bus) *busManager {
	bm := &busManager{
		bus:            bus,
		frameListeners: make(map[uint32][]FrameListener),
		canError:       0,
	}
	return bm
}

// Create Bus from local available buses
// Currently supported : socketcan, virtualcan
func createBusInternal(canInterface string, channel string, bitrate int) (Bus, error) {
	var bus Bus
	var err error
	switch canInterface {
	case "socketcan", "":
		bus, err = NewSocketCanBus(channel)
	case "virtualcan":
		bus = NewVirtualCanBus(channel)
	default:
		err = fmt.Errorf("unsupported interface : %v", canInterface)
	}
	if err != nil {
		return nil, err
	}
	return bus, err
}
