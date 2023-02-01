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

// Interface used for handling a CAN frame, implementation depends on the CAN object type
type FrameHandler interface {
	Handle(frame can.Frame)
}

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

/* CANModule */
type CANModule struct {
	Bus               Bus
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

/* Create a New CANModule object */
func NewCANModule(bus Bus, rxArray []BufferRxFrame, txArray []BufferTxFrame) *CANModule {
	canmodule := &CANModule{
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

	return canmodule
}

func (canmodule *CANModule) Init(bus Bus) {
	canmodule.Bus = bus
	canmodule.RxArray = []BufferRxFrame{}
	canmodule.TxArray = []BufferTxFrame{}
	canmodule.CANerrorstatus = 0
	canmodule.CANnormal = false
	canmodule.UseCANrxFilters = false
	canmodule.BufferInhibitFlag = false
	canmodule.FirstCANtxMessage = false
	canmodule.CANtxCount = 0
	canmodule.ErrOld = 0

	// For now ignore rx filters but initialize everything inside rxBuffer
	for _, rxBuffer := range canmodule.RxArray {
		rxBuffer.Ident = 0
		rxBuffer.Mask = 0xFFFFFFFF
		rxBuffer.Object = nil
		rxBuffer.CANifindex = 0
	}

}

/* Update rx filters in buffer */
func (canmodule *CANModule) SetRxFilters() {
	// TODO
}

/* Send CAN messages in buffer */
// Error handling is very limited right now

func (canmodule *CANModule) Send(buf BufferTxFrame) error {
	return canmodule.Bus.Send(buf)
}

func (canmodule *CANModule) ClearSyncPDOs() (result COResult) {
	// TODO
	return 0
}

/* This should be called cyclically to update errors & process unsent messages*/
func (canmodule *CANModule) Process() error {
	// TODO get bus state error
	canmodule.CANerrorstatus = 0
	/*Loop through tx array and send unsent messages*/
	if canmodule.CANtxCount > 0 {
		found := false
		for _, buffer := range canmodule.TxArray {
			if buffer.BufferFull {
				buffer.BufferFull = false
				canmodule.CANtxCount -= 1
				canmodule.Send(buffer)
				found = true
			}
		}
		if !found {
			canmodule.CANtxCount = 0
		}
	}
	return nil

}

// Initialize transmit buffer append it to existing transmit buffer array and return it
// Return buffer, index of buffer
func (canmodule *CANModule) InsertTxBuffer(ident uint32, rtr bool, length uint8, syncFlag bool) (*BufferTxFrame, int, error) {
	// This is specific to socketcan
	ident = ident & CAN_SFF_MASK
	if rtr {
		ident |= CAN_RTR_FLAG
	}
	canmodule.TxArray = append(canmodule.TxArray, NewBufferTxFrame(ident, length, syncFlag, 0))
	return &canmodule.TxArray[len(canmodule.TxArray)-1], len(canmodule.TxArray) - 1, nil
}

// Update an already present buffer instead of appending
func (canmodule *CANModule) UpdateTxBuffer(index int, ident uint32, rtr bool, length uint8, syncFlag bool) (*BufferTxFrame, error) {
	// This is specific to socketcan
	ident = ident & CAN_SFF_MASK
	if rtr {
		ident |= CAN_RTR_FLAG
	}
	canmodule.TxArray[index] = NewBufferTxFrame(ident, length, syncFlag, 0)
	return &canmodule.TxArray[index], nil
}

// Initialize receive buffer append it to existing receive buffer
// Return the index of the added buffer
func (canmodule *CANModule) InsertRxBuffer(ident uint32, mask uint32, rtr bool, object FrameHandler) (int, error) {

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

	canmodule.RxArray = append(canmodule.RxArray, NewBufferRxFrame(ident, mask, object, 0))

	// TODO handle RX filters smhw
	return len(canmodule.RxArray) - 1, nil
}

// Update an already present buffer instead of appending
func (canmodule *CANModule) UpdateRxBuffer(index int, ident uint32, mask uint32, rtr bool, object FrameHandler) error {
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
	canmodule.RxArray[index] = NewBufferRxFrame(ident, mask, object, 0)
	return nil
}

// Implements handle interface i.e processes a can message
func (canmodule *CANModule) Handle(frame can.Frame) {
	// Feed the frame to the correct callback
	// TODO this could probably be quicker if it was sorted or we had a map
	for _, framebBuffer := range canmodule.RxArray {
		if (frame.ID^framebBuffer.Ident)&framebBuffer.Mask == 0 {
			// Callback for the specific CANopen object (PDO, SDO, NMT, HB, etc)
			framebBuffer.Object.Handle(frame)
		}
	}
}
