package canopen

import (
	"fmt"

	log "github.com/sirupsen/logrus"
)

const CAN_RTR_FLAG uint32 = 0x40000000
const CAN_SFF_MASK uint32 = 0x000007FF
const CAN_EFF_FLAG uint32 = 0x80000000

// CAN bus errors
const (
	CAN_ERRTX_WARNING    = 0x0001 /**< 0x0001, CAN transmitter warning */
	CAN_ERRTX_PASSIVE    = 0x0002 /**< 0x0002, CAN transmitter passive */
	CAN_ERRTX_BUS_OFF    = 0x0004 /**< 0x0004, CAN transmitter bus off */
	CAN_ERRTX_OVERFLOW   = 0x0008 /**< 0x0008, CAN transmitter overflow */
	CAN_ERRTX_PDO_LATE   = 0x0080 /**< 0x0080, TPDO is outside sync window */
	CAN_ERRRX_WARNING    = 0x0100 /**< 0x0100, CAN receiver warning */
	CAN_ERRRX_PASSIVE    = 0x0200 /**< 0x0200, CAN receiver passive */
	CAN_ERRRX_OVERFLOW   = 0x0800 /**< 0x0800, CAN receiver overflow */
	CAN_ERR_WARN_PASSIVE = 0x0303 /**< 0x0303, combination */
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

// Interface used for handling a CAN frame, implementation specific : will depend on the bus type
type FrameListener interface {
	Handle(frame Frame)
}

// A CAN Bus interface
// A custom implementation should implement all these methods
type Bus interface {
	Connect(...any) error                   // Connect to the actual bus
	Disconnect() error                      // Disconnect from bus
	Send(frame Frame) error                 // Send a frame on the bus
	Subscribe(callback FrameListener) error // Subscribe to all can frames
}

// Bus manager is a wrapper around the CAN bus interface
// Used by the CANopen stack to control errors, callbacks for specific IDs, etc.
type BusManager struct {
	Bus               Bus // Bus interface that can be adapted
	frameListeners    map[uint32][]FrameListener
	CANerrorstatus    uint16
	CANnormal         bool
	UseCANrxFilters   bool
	BufferInhibitFlag bool
	FirstCANtxMessage bool
	CANtxCount        uint32
	ErrOld            uint32
}

// Implements the FrameListener interface
// This handles all received CAN frames from Bus
func (busManager *BusManager) Handle(frame Frame) {
	listeners, ok := busManager.frameListeners[frame.ID]
	if !ok {
		return
	}
	for _, listener := range listeners {
		listener.Handle(frame)
	}
}

// Send a CAN message from given buffer
// Limited error handling
func (busManager *BusManager) Send(frame Frame) error {
	err := busManager.Bus.Send(frame)
	if err != nil {
		log.Warnf("[CAN ERROR] %v", err)
	}
	return err
}

// This should be called cyclically to update errors
func (busManager *BusManager) Process() error {
	// TODO get bus state error
	busManager.CANerrorstatus = 0
	return nil
}

// Subscribe to a specific CAN ID
func (busManager *BusManager) Subscribe(ident uint32, mask uint32, rtr bool, callback FrameListener) error {
	ident = ident & CAN_SFF_MASK
	if rtr {
		ident |= CAN_RTR_FLAG
	}
	_, ok := busManager.frameListeners[ident]
	if !ok {
		busManager.frameListeners[ident] = []FrameListener{callback}
		return nil
	}
	// TODO add error if callback exists already
	busManager.frameListeners[ident] = append(busManager.frameListeners[ident], callback)
	return nil
}

// Update rx filters in buffer
func (busManager *BusManager) SetRxFilters() {
	// TODO
}

// Abort pending TPDOs
func (busManager *BusManager) ClearSyncPDOs() error {
	// TODO
	return nil
}

func NewBusManager(bus Bus) *BusManager {
	busManager := &BusManager{
		Bus:               bus,
		frameListeners:    make(map[uint32][]FrameListener),
		CANerrorstatus:    0,
		CANnormal:         false,
		UseCANrxFilters:   false,
		BufferInhibitFlag: false,
		FirstCANtxMessage: false,
		CANtxCount:        0,
		ErrOld:            0,
	}

	return busManager
}

// Create Bus from local available buses
// Currently supported : socketcan, virtualcan
func createBusInternal(canInterface string, channel string, bitrate int) (Bus, error) {
	var bus Bus
	var err error
	switch canInterface {
	case "socketcan", "":
		bus, err = NewSocketcanBus(channel)
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
