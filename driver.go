package canopen

type COResult int8

const CAN_RTR_FLAG uint32 = 0x40000000
const CAN_SFF_MASK uint32 = 0x000007FF
const CAN_EFF_FLAG uint32 = 0x80000000

type CANopenError int8

func (error CANopenError) Error() string {
	error_str, ok := CANOPEN_ERRORS[int8(error)]
	if ok {
		return error_str
	}
	return "Unknown error"
}

const (
	CO_ERROR_NO                       = 0
	CO_ERROR_ILLEGAL_ARGUMENT         = -1
	CO_ERROR_OUT_OF_MEMORY            = -2
	CO_ERROR_TIMEOUT                  = -3
	CO_ERROR_ILLEGAL_BAUDRATE         = -4
	CO_ERROR_RX_OVERFLOW              = -5
	CO_ERROR_RX_PDO_OVERFLOW          = -6
	CO_ERROR_RX_MSG_LENGTH            = -7
	CO_ERROR_RX_PDO_LENGTH            = -8
	CO_ERROR_TX_OVERFLOW              = -9
	CO_ERROR_TX_PDO_WINDOW            = -10
	CO_ERROR_TX_UNCONFIGURED          = -11
	CO_ERROR_OD_PARAMETERS            = -12
	CO_ERROR_DATA_CORRUPT             = -13
	CO_ERROR_CRC                      = -14
	CO_ERROR_TX_BUSY                  = -15
	CO_ERROR_WRONG_NMT_STATE          = -16
	CO_ERROR_SYSCALL                  = -17
	CO_ERROR_INVALID_STATE            = -18
	CO_ERROR_NODE_ID_UNCONFIGURED_LSS = -19
)

// A map between the errors and the description
var CANOPEN_ERRORS = map[int8]string{
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

/* Received message object */
type CANrxMsg struct {
	Ident      uint32
	Mask       uint32
	Object     any
	Callback   func(object any, message any)
	CANifindex int
}

/* Transmit message object */
type CANtxMsg struct {
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
	RxArray           []CANrxMsg
	TxArray           []CANtxMsg
	CANerrorstatus    uint16
	CANnormal         bool
	UseCANrxFilters   bool
	BufferInhibitFlag bool
	FirstCANtxMessage bool
	CANtxCount        uint32
	ErrOld            uint32
}

/* Create a New CANModule object */
func NewCANModule(bus Bus, rxArray []CANrxMsg, txArray []CANtxMsg) *CANModule {
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

func (canmodule *CANModule) Init(bus Bus, rxArray []CANrxMsg, txArray []CANtxMsg) {
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
		rxBuffer.Callback = nil
		rxBuffer.CANifindex = 0
	}

}

/* Update rx filters in buffer */
func (canmodule *CANModule) SetRxFilters() {
	// TODO
}

/* Send CAN messages in buffer */
// Error handling is very limited right now

func (canmodule *CANModule) Send(buf CANtxMsg) error {

	return canmodule.Bus.Send(buf)
}

func (canmodule *CANModule) ClearSyncPDOs() (result COResult) {
	// TODO
	return 0
}

/* This should be called cyclically */
func (canmodule *CANModule) Process() (result COResult) {
	// TODO
	return 0
}

/* Initialize one transmit buffer element in tx array*/
func (canmodule *CANModule) TxBufferInit(index uint32, ident uint32, rtr bool, length uint8, syncFlag bool) (result COResult, msg *CANtxMsg) {
	var buffer *CANtxMsg = nil
	if canmodule == nil || index >= uint32(len(canmodule.TxArray)) {
		return -1, nil
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
	return CO_ERROR_NO, buffer
}

/* Initialize one receive buffer element in rx array with function callback*/
func (canmodule *CANModule) RxBufferInit(index uint32, ident uint32, mask uint32, rtr bool, object interface{}, callback func(object interface{}, message interface{})) CANopenError {

	ret := CO_ERROR_NO
	if canmodule == nil || object == nil || callback == nil || index >= uint32(len(canmodule.RxArray)) {
		return CO_ERROR_ILLEGAL_ARGUMENT
	}

	/* Configure object variables */
	buffer := &canmodule.RxArray[index]
	buffer.Object = object
	buffer.Callback = callback
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
		ret = CO_ERROR_ILLEGAL_ARGUMENT
	}

	return CANopenError(ret)
}
