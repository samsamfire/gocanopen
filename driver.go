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

/* Transmit message object */
type BufferTxFrame struct {
	Ident      uint32
	DLC        uint8
	Data       [8]byte
	BufferFull bool
	SyncFlag   bool
	CANifindex int
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

func (canmodule *CANModule) Init(bus Bus, rxArray []BufferRxFrame, txArray []BufferTxFrame) {
	canmodule.Bus = bus
	canmodule.RxArray = rxArray
	canmodule.TxArray = txArray
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

/* Initialize one transmit buffer element in tx array*/
func (canmodule *CANModule) TxBufferInit(index uint32, ident uint32, rtr bool, length uint8, syncFlag bool) (result error, msg *BufferTxFrame) {
	var buffer *BufferTxFrame = &BufferTxFrame{}
	if canmodule == nil || index >= uint32(len(canmodule.TxArray)) {
		return CO_ERROR_ILLEGAL_ARGUMENT, nil
	}
	buffer.CANifindex = 0
	/* get specific buffer */
	buffer = &canmodule.TxArray[index]
	/* CAN identifier and rtr */
	buffer.Ident = ident & CAN_SFF_MASK
	if rtr {
		buffer.Ident |= CAN_RTR_FLAG
	}
	buffer.DLC = length
	buffer.BufferFull = false
	buffer.SyncFlag = syncFlag
	return nil, buffer
}

/* Initialize one receive buffer element in rx array with function callback*/
func (canmodule *CANModule) RxBufferInit(index uint32, ident uint32, mask uint32, rtr bool, object FrameHandler) error {

	var ret error
	if canmodule == nil || object == nil || index >= uint32(len(canmodule.RxArray)) {
		log.Warn("Some arguments in RX buffer init are nil")
		return CO_ERROR_ILLEGAL_ARGUMENT
	}

	/* Configure object variables */
	buffer := &canmodule.RxArray[index]
	buffer.Object = object
	buffer.CANifindex = 0

	/* CAN identifier and CAN mask, bit aligned with CAN module. Different on different microcontrollers. */
	buffer.Ident = ident & CAN_SFF_MASK
	if rtr {
		buffer.Ident |= CAN_RTR_FLAG
	}
	buffer.Mask = (mask & CAN_SFF_MASK) | CAN_EFF_FLAG | CAN_RTR_FLAG

	/* Set CAN hardware module filter and mask. */
	if canmodule.UseCANrxFilters {
		// pass
	} else {
		//pass
		//ret = CO_ERROR_ILLEGAL_ARGUMENT
	}

	return ret
}

// Implements handle interface i.e processes a can message
func (canmodule *CANModule) Handle(frame can.Frame) {
	log.Debug("Received can frame ", frame)
	// Feed the frame to the correct callback
	// TODO this could probably be quicker if it was sorted or we had a map
	for _, framebBuffer := range canmodule.RxArray {
		if (frame.ID^framebBuffer.Ident)&framebBuffer.Mask == 0 {
			// Callback for the specific CANopen object (PDO, SDO, NMT, HB, etc)
			framebBuffer.Object.Handle(frame)
		}
	}
}
