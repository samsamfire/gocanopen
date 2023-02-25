package canopen

import (
	"github.com/brutella/can"
	log "github.com/sirupsen/logrus"
)

type COResult int8

const CAN_RTR_FLAG uint32 = 0x40000000
const CAN_SFF_MASK uint32 = 0x000007FF
const CAN_EFF_FLAG uint32 = 0x80000000

type CANopenError int8

func (error CANopenError) Error() string {
	error_str, ok := CANOPEN_ERRORS[error]
	if ok {
		return error_str
	}
	return "Unknown error"
}

const (
	CO_ERROR_NO                       CANopenError = 0
	CO_ERROR_ILLEGAL_ARGUMENT         CANopenError = -1
	CO_ERROR_OUT_OF_MEMORY            CANopenError = -2
	CO_ERROR_TIMEOUT                  CANopenError = -3
	CO_ERROR_ILLEGAL_BAUDRATE         CANopenError = -4
	CO_ERROR_RX_OVERFLOW              CANopenError = -5
	CO_ERROR_RX_PDO_OVERFLOW          CANopenError = -6
	CO_ERROR_RX_MSG_LENGTH            CANopenError = -7
	CO_ERROR_RX_PDO_LENGTH            CANopenError = -8
	CO_ERROR_TX_OVERFLOW              CANopenError = -9
	CO_ERROR_TX_PDO_WINDOW            CANopenError = -10
	CO_ERROR_TX_UNCONFIGURED          CANopenError = -11
	CO_ERROR_OD_PARAMETERS            CANopenError = -12
	CO_ERROR_DATA_CORRUPT             CANopenError = -13
	CO_ERROR_CRC                      CANopenError = -14
	CO_ERROR_TX_BUSY                  CANopenError = -15
	CO_ERROR_WRONG_NMT_STATE          CANopenError = -16
	CO_ERROR_SYSCALL                  CANopenError = -17
	CO_ERROR_INVALID_STATE            CANopenError = -18
	CO_ERROR_NODE_ID_UNCONFIGURED_LSS CANopenError = -19
)

// A map between the errors and the description
var CANOPEN_ERRORS = map[CANopenError]string{
	CO_ERROR_NO:                       "Operation completed successfully",
	CO_ERROR_ILLEGAL_ARGUMENT:         "Error in function arguments",
	CO_ERROR_OUT_OF_MEMORY:            "Memory allocation failed",
	CO_ERROR_TIMEOUT:                  "Function timeout",
	CO_ERROR_ILLEGAL_BAUDRATE:         "Illegal baudrate passed to function",
	CO_ERROR_RX_OVERFLOW:              "Previous message was not processed yet",
	CO_ERROR_RX_PDO_OVERFLOW:          "Previous PDO was not processed yet",
	CO_ERROR_RX_MSG_LENGTH:            "Wrong receive message length",
	CO_ERROR_RX_PDO_LENGTH:            "Wrong receive PDO length",
	CO_ERROR_TX_OVERFLOW:              "Previous message is still waiting, buffer full",
	CO_ERROR_TX_PDO_WINDOW:            "Synchronous TPDO is outside window",
	CO_ERROR_TX_UNCONFIGURED:          "Transmit buffer was not configured properly",
	CO_ERROR_OD_PARAMETERS:            "Error in Object Dictionary parameters",
	CO_ERROR_DATA_CORRUPT:             "Stored data are corrupt",
	CO_ERROR_CRC:                      "CRC does not match",
	CO_ERROR_TX_BUSY:                  "Sending rejected because driver is busy. Try again",
	CO_ERROR_WRONG_NMT_STATE:          "Command can't be processed in current state",
	CO_ERROR_SYSCALL:                  "Syscall failed",
	CO_ERROR_INVALID_STATE:            "Driver not ready",
	CO_ERROR_NODE_ID_UNCONFIGURED_LSS: "Node-id is in LSS unconfigured state. If objects are handled properly,this may not be an error.",
}

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

/* Received message object buffer */
type BufferRxFrame struct {
	Ident      uint32
	Mask       uint32
	Object     FrameHandler // Object implements frame handler can be any canopen object type
	CANifindex int
}

func NewBufferRxFrame(ident uint32, mask uint32, object FrameHandler, CANifindex int) BufferRxFrame {
	return BufferRxFrame{Ident: ident, Mask: mask, Object: object, CANifindex: CANifindex}
}

/* Transmit message object */
type BufferTxFrame struct {
	Ident      uint32
	DLC        uint8
	Data       [8]byte
	BufferFull bool
	SyncFlag   bool
	CANifindex int
}

func NewBufferTxFrame(ident uint32, length uint8, syncFlag bool, CANifindex int) BufferTxFrame {
	return BufferTxFrame{Ident: ident, DLC: length, BufferFull: false, SyncFlag: syncFlag, CANifindex: CANifindex}
}

// Bus manager is responsible for using the Bus
// It has interal buffers etc
type BusManager struct {
	Bus               Bus // Bus interface that can be adapted
	RxArray           []BufferRxFrame
	TxArray           []BufferTxFrame
	CANerrorstatus    uint16
	CANnormal         bool
	UseCANrxFilters   bool
	BufferInhibitFlag bool
	FirstCANtxMessage bool
	CANtxCount        uint32
	ErrOld            uint32
}

/* Create a New BusManager object */
func NewBusManager(bus Bus, rxArray []BufferRxFrame, txArray []BufferTxFrame) *BusManager {
	bus_manager := &BusManager{
		Bus:               bus,
		RxArray:           rxArray,
		TxArray:           txArray,
		CANerrorstatus:    0,
		CANnormal:         false,
		UseCANrxFilters:   false,
		BufferInhibitFlag: false,
		FirstCANtxMessage: false,
		CANtxCount:        0,
		ErrOld:            0,
	}

	return bus_manager
}

func (bus_manager *BusManager) Init(bus Bus) {
	bus_manager.Bus = bus
	bus_manager.RxArray = []BufferRxFrame{}
	bus_manager.TxArray = []BufferTxFrame{}
	bus_manager.CANerrorstatus = 0
	bus_manager.CANnormal = false
	bus_manager.UseCANrxFilters = false
	bus_manager.BufferInhibitFlag = false
	bus_manager.FirstCANtxMessage = false
	bus_manager.CANtxCount = 0
	bus_manager.ErrOld = 0

	// For now ignore rx filters but initialize everything inside rxBuffer
	for _, rxBuffer := range bus_manager.RxArray {
		rxBuffer.Ident = 0
		rxBuffer.Mask = 0xFFFFFFFF
		rxBuffer.Object = nil
		rxBuffer.CANifindex = 0
	}

}

// Implements CAN package handle interface for processing a CAN message
func (bus_manager *BusManager) Handle(frame can.Frame) {
	// Feed the frame to the correct callback
	// TODO this could probably be quicker if it was sorted or we had a map
	for _, framebBuffer := range bus_manager.RxArray {
		if (frame.ID^framebBuffer.Ident)&framebBuffer.Mask == 0 {
			// Callback for the specific CANopen object (PDO, SDO, NMT, HB, etc)
			framebBuffer.Object.Handle(frame)
		}
	}
}

/* Update rx filters in buffer */
func (bus_manager *BusManager) SetRxFilters() {
	// TODO
}

/* Send CAN messages in buffer */
// Error handling is very limited right now

func (bus_manager *BusManager) Send(buf BufferTxFrame) error {
	return bus_manager.Bus.Send(buf)
}

func (bus_manager *BusManager) ClearSyncPDOs() (result COResult) {
	// TODO
	return 0
}

/* This should be called cyclically to update errors & process unsent messages*/
func (bus_manager *BusManager) Process() error {
	// TODO get bus state error
	bus_manager.CANerrorstatus = 0
	/*Loop through tx array and send unsent messages*/
	if bus_manager.CANtxCount > 0 {
		found := false
		for _, buffer := range bus_manager.TxArray {
			if buffer.BufferFull {
				buffer.BufferFull = false
				bus_manager.CANtxCount -= 1
				bus_manager.Send(buffer)
				found = true
			}
		}
		if !found {
			bus_manager.CANtxCount = 0
		}
	}
	return nil

}

// Initialize transmit buffer append it to existing transmit buffer array and return it
// Return buffer, index of buffer
func (bus_manager *BusManager) InsertTxBuffer(ident uint32, rtr bool, length uint8, syncFlag bool) (*BufferTxFrame, int, error) {
	// This is specific to socketcan
	ident = ident & CAN_SFF_MASK
	if rtr {
		ident |= CAN_RTR_FLAG
	}
	bus_manager.TxArray = append(bus_manager.TxArray, NewBufferTxFrame(ident, length, syncFlag, 0))
	return &bus_manager.TxArray[len(bus_manager.TxArray)-1], len(bus_manager.TxArray) - 1, nil
}

// Update an already present buffer instead of appending
func (bus_manager *BusManager) UpdateTxBuffer(index int, ident uint32, rtr bool, length uint8, syncFlag bool) (*BufferTxFrame, error) {
	// This is specific to socketcan
	ident = ident & CAN_SFF_MASK
	if rtr {
		ident |= CAN_RTR_FLAG
	}
	bus_manager.TxArray[index] = NewBufferTxFrame(ident, length, syncFlag, 0)
	return &bus_manager.TxArray[index], nil
}

// Initialize receive buffer append it to existing receive buffer
// Return the index of the added buffer
func (bus_manager *BusManager) InsertRxBuffer(ident uint32, mask uint32, rtr bool, object FrameHandler) (int, error) {

	if object == nil {
		log.Error("Rx buffer needs a frame handler")
		return 0, CO_ERROR_ILLEGAL_ARGUMENT
	}
	// This part is specific to socketcan
	ident = ident & CAN_SFF_MASK
	if rtr {
		ident |= CAN_RTR_FLAG
	}
	mask = (mask & CAN_SFF_MASK) | CAN_EFF_FLAG | CAN_RTR_FLAG

	bus_manager.RxArray = append(bus_manager.RxArray, NewBufferRxFrame(ident, mask, object, 0))

	// TODO handle RX filters smhw
	return len(bus_manager.RxArray) - 1, nil
}

// Update an already present buffer instead of appending
func (bus_manager *BusManager) UpdateRxBuffer(index int, ident uint32, mask uint32, rtr bool, object FrameHandler) error {
	if object == nil {
		log.Error("Rx buffer needs a frame handler")
		return CO_ERROR_ILLEGAL_ARGUMENT
	}
	// This part is specific to socketcan
	ident = ident & CAN_SFF_MASK
	if rtr {
		ident |= CAN_RTR_FLAG
	}
	mask = (mask & CAN_SFF_MASK) | CAN_EFF_FLAG | CAN_RTR_FLAG
	// Here, accessing an invalid is worse than panicing
	bus_manager.RxArray[index] = NewBufferRxFrame(ident, mask, object, 0)
	return nil
}
