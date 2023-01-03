package canopen

type COResult int8

const CAN_RTR_FLAG uint32 = 0x40000000
const CAN_SFF_MASK uint32 = 0x000007FF
const CAN_EFF_FLAG uint32 = 0x80000000

const (
	CO_ERROR_NO               COResult = 0  /**< Operation completed successfully */
	CO_ERROR_ILLEGAL_ARGUMENT COResult = -1 /**< Error in function arguments */
	CO_ERROR_OUT_OF_MEMORY    COResult = -2 /**< Memory allocation failed */
	CO_ERROR_TIMEOUT          COResult = -3 /**< Function timeout */
	CO_ERROR_ILLEGAL_BAUDRATE COResult = -4 /**< Illegal baudrate passed to function
	  CO_CANmodule_init() */
	CO_ERROR_RX_OVERFLOW COResult = -5 /**< Previous message was not processed
	  yet */
	CO_ERROR_RX_PDO_OVERFLOW COResult = -6 /**< previous PDO was not processed yet */
	CO_ERROR_RX_MSG_LENGTH   COResult = -7 /**< Wrong receive message length */
	CO_ERROR_RX_PDO_LENGTH   COResult = -8 /**< Wrong receive PDO length */
	CO_ERROR_TX_OVERFLOW     COResult = -9 /**< Previous message is still waiting,
	  buffer full */
	CO_ERROR_TX_PDO_WINDOW   COResult = -10 /**< Synchronous TPDO is outside window */
	CO_ERROR_TX_UNCONFIGURED COResult = -11 /**< Transmit buffer was not configured
	  properly */
	CO_ERROR_OD_PARAMETERS COResult = -12 /**< Error in Object Dictionary parameters*/
	CO_ERROR_DATA_CORRUPT  COResult = -13 /**< Stored data are corrupt */
	CO_ERROR_CRC           COResult = -14 /**< CRC does not match */
	CO_ERROR_TX_BUSY       COResult = -15 /**< Sending rejected because driver is
	  busy. Try again */
	CO_ERROR_WRONG_NMT_STATE COResult = -16 /**< Command can't be processed in current
	  state */
	CO_ERROR_SYSCALL                  COResult = -17 /**< Syscall failed */
	CO_ERROR_INVALID_STATE            COResult = -18 /**< Driver not ready */
	CO_ERROR_NODE_ID_UNCONFIGURED_LSS COResult = -19 /**< Node-id is in LSS unconfigured
	  state. If objects are handled properly,
	  this may not be an error. */
)

/* Received message object */
type CANrxMsg struct {
	Ident      uint32
	Mask       uint32
	Object     interface{}
	Callback   func(object interface{}, message interface{})
	CANifindex int
}

/* Transmit message object */
type CANtxMsg struct {
	Ident      uint32
	DLC        uint8
	Data       []byte
	BufferFull bool
	SyncFlag   bool
	CANifindex int
}

/* CANModule */
type CANModule struct {
	CAN               interface{}
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
func NewCANModule(can interface{}, rxArray []CANrxMsg, txArray []CANtxMsg) *CANModule {
	canmodule := &CANModule{
		CAN:               can,
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
	// For now ignore rx filters but initialize everything inside rxBuffer
	for _, rxBuffer := range rxArray {
		rxBuffer.Ident = 0
		rxBuffer.Mask = 0xFFFFFFFF
		rxBuffer.Object = nil
		rxBuffer.Callback = nil
		rxBuffer.CANifindex = 0
	}

	return canmodule
}

/* Update rx filters in buffer */
func (canmodule *CANModule) SetRxFilters() {
	// TODO
}

/* Send CAN messages in buffer */
func (canmodule *CANModule) Send(buffer []CANtxMsg) (result COResult) {
	// TODO
	return 0
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
func (canmodule *CANModule) RxBufferInit(index uint32, ident uint32, mask uint32, rtr bool, object interface{}, callback func(object interface{}, message interface{})) (result COResult) {

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

	return ret
}
