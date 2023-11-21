package canopen

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

// TX Buffer struct for sending specific CAN frame ID
type BufferTxFrame struct {
	Ident      uint32
	DLC        uint8
	Data       [8]byte
	BufferFull bool
	SyncFlag   bool
	CANifindex int
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
	txBuffer          map[uint32]*BufferTxFrame
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
func (busManager *BusManager) Send(buf BufferTxFrame) error {
	return busManager.Bus.Send(Frame{ID: buf.Ident, Flags: 0, DLC: buf.DLC, Data: buf.Data})
}

// This should be called cyclically to update errors & process unsent messages
func (busManager *BusManager) Process() error {
	// TODO get bus state error
	busManager.CANerrorstatus = 0
	// Loop through tx array and send unsent messages
	if busManager.CANtxCount > 0 {
		found := false
		for _, buffer := range busManager.txBuffer {
			if buffer.BufferFull {
				buffer.BufferFull = false
				busManager.CANtxCount -= 1
				busManager.Send(*buffer)
				found = true
			}
		}
		if !found {
			busManager.CANtxCount = 0
		}
	}
	return nil

}

func (busManager *BusManager) InsertTxBuffer(ident uint32, rtr bool, length uint8, syncFlag bool) (*BufferTxFrame, error) {
	// This is specific to socketcan
	ident = ident & CAN_SFF_MASK
	if rtr {
		ident |= CAN_RTR_FLAG
	}
	busManager.txBuffer[ident] = NewBufferTxFrame(ident, length, syncFlag, 0)
	return busManager.txBuffer[ident], nil
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

func NewBufferTxFrame(ident uint32, length uint8, syncFlag bool, CANifindex int) *BufferTxFrame {
	return &BufferTxFrame{Ident: ident, DLC: length, BufferFull: false, SyncFlag: syncFlag, CANifindex: CANifindex}
}

func NewBusManager(bus Bus) *BusManager {
	busManager := &BusManager{
		Bus:               bus,
		frameListeners:    make(map[uint32][]FrameListener),
		txBuffer:          make(map[uint32]*BufferTxFrame),
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
