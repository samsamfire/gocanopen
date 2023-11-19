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
	DLC   uint8
	Data  [8]byte
	Flags uint8
}

// RX Buffer struct for receiving specific CAN frame ID
type BufferRxFrame struct {
	Ident      uint32
	Mask       uint32
	handler    FrameHandler
	CANifindex int
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
type FrameHandler interface {
	Handle(frame Frame)
}

// A CAN Bus interface
type Bus interface {
	Send(frame BufferTxFrame) error    // Send a frame on the bus
	Subscribe(subscriber FrameHandler) // Subscribe to can frames
	Connect(...any) error
}

// Bus manager is responsible for using the Bus
// It has interal buffers etc
type BusManager struct {
	Bus               Bus // Bus interface that can be adapted
	txBuffer          map[uint32]*BufferTxFrame
	rxBuffer          map[uint32]BufferRxFrame
	CANerrorstatus    uint16
	CANnormal         bool
	UseCANrxFilters   bool
	BufferInhibitFlag bool
	FirstCANtxMessage bool
	CANtxCount        uint32
	ErrOld            uint32
}

// Implements the CAN package "Handle" interface for handling a CAN message
// This feeds a CAN frame to a handler
func (busManager *BusManager) Handle(frame Frame) {
	frameBuffer, ok := busManager.rxBuffer[frame.ID]
	if !ok {
		return
	}
	frameBuffer.handler.Handle(frame)
}

// Send CAN message from given buffer
// Error handling is very limited right now

func (busManager *BusManager) Send(buf BufferTxFrame) error {
	return busManager.Bus.Send(buf)
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

func (busManager *BusManager) InsertRxBuffer(ident uint32, mask uint32, rtr bool, object FrameHandler) error {
	// This part is specific to socketcan
	ident = ident & CAN_SFF_MASK
	if rtr {
		ident |= CAN_RTR_FLAG
	}
	mask = (mask & CAN_SFF_MASK) | CAN_EFF_FLAG | CAN_RTR_FLAG
	busManager.rxBuffer[ident] = NewBufferRxFrame(ident, mask, object, 0)
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

func NewBufferRxFrame(ident uint32, mask uint32, object FrameHandler, CANifindex int) BufferRxFrame {
	return BufferRxFrame{Ident: ident, Mask: mask, handler: object, CANifindex: CANifindex}
}

func NewBufferTxFrame(ident uint32, length uint8, syncFlag bool, CANifindex int) *BufferTxFrame {
	return &BufferTxFrame{Ident: ident, DLC: length, BufferFull: false, SyncFlag: syncFlag, CANifindex: CANifindex}
}

func NewBusManager(bus Bus) *BusManager {
	busManager := &BusManager{
		Bus:               bus,
		rxBuffer:          make(map[uint32]BufferRxFrame),
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
