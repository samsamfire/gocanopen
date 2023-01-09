package canopen

import "fmt"

const (
	CO_NMT_ERR_REG_MASK            = 0x00FF
	CO_NMT_STARTUP_TO_OPERATIONAL  = 0x0100
	CO_NMT_ERR_ON_BUSOFF_HB        = 0x1000
	CO_NMT_ERR_ON_ERR_REG          = 0x2000
	CO_NMT_ERR_TO_STOPPED          = 0x4000
	CO_NMT_ERR_FREE_TO_OPERATIONAL = 0x8000
)

const (
	CO_RESET_NOT  = 0
	CO_RESET_COMM = 1
	CO_RESET_APP  = 2
	CO_RESET_QUIT = 3
)

// NMT object
type NMT struct {
	OperatingState         uint8
	OperatingStatePrev     uint8
	InternalCommand        uint8
	NodeId                 uint8
	Control                uint8
	HearbeatProducerTimeUs uint32
	HearbeatProducerTimer  uint32
	Entry1017              Extension
	Emergency              *EM
	CANModule              *CANModule
	NMTTxBuff              *CANtxMsg
	HBTxBuff               *CANtxMsg
	// Optionally add callback functions
}

func (nmt *NMT) Init(
	entry *Entry,
	em *EM,
	node_id uint8,
	control uint8,
	first_hb_time_ms uint16,
	can_module *CANModule,
	nmt_tx_index uint16,
	nmt_rx_index uint16,
	hb_tx_index uint16,
	can_id_nmt_tx uint16,
	can_id_nmt_rx uint16,
	can_id_hb_tx uint16,
) error {
	return nil
}

// void CO_NMT_initCallbackPre(CO_NMT_t *NMT,
//    void *object,
//    void (*pFunctSignal)(void *object));

// void CO_NMT_initCallbackChanged(CO_NMT_t *NMT,
// 	   void (*pFunctNMT)(CO_NMT_internalState_t state));
// #endif

/**
* Process received NMT and produce Heartbeat messages.
*
* Function must be called cyclically.
*
* @param NMT This object.
* @param [out] NMTstate If not NULL, CANopen NMT internal state is returned.
* @param timeDifference_us Time difference from previous function call in
* microseconds.
* @param [out] timerNext_us info to OS - see CO_process().
*
* @return #CO_NMT_reset_cmd_t
 */

func (nmt *NMT) Process(internal_state uint16, time_difference_us uint32, timer_next_us uint32) uint16 {
	return 0
}

/**
* Query current NMT state
*
* @param NMT This object.
*
* @return @ref CO_NMT_internalState_t
 */
// static inline CO_NMT_internalState_t CO_NMT_getInternalState(CO_NMT_t *NMT) {
// return (NMT == NULL) ? CO_NMT_INITIALIZING : NMT->operatingState;
// }

/**
* Send NMT command to self, without sending NMT message
*
* Internal NMT state will be verified and switched inside @ref CO_NMT_process()
*
* @param NMT This object.
* @param command NMT command
 */

func (nmt *NMT) SendInternalCommand(command uint16) {
	// TODOs
}

/**
* Send NMT master command.
*
* This functionality may only be used from NMT master, as specified by
* standard CiA302-2. Standard provides one exception, where application from
* slave node may send NMT master command: "If CANopen object 0x1F80 has value
* of **0x2**, then NMT slave shall execute the NMT service start remote node
* (CO_NMT_ENTER_OPERATIONAL) with nodeID set to 0."
*
* @param NMT This object.
* @param command NMT command from CO_NMT_command_t.
* @param nodeID Node ID of the remote node. 0 for all nodes including self.
*
* @return CO_ERROR_NO on success or CO_ReturnError_t from CO_CANsend().
 */

func (nmt *NMT) SendCommand(command uint8, node_id uint8) error {

	if nmt == nil {
		return fmt.Errorf(CANOPEN_ERRORS[CO_ERROR_ILLEGAL_ARGUMENT])
	}

	/* Apply NMT command also to this node, if set so. */
	if node_id == 0 || node_id == nmt.NodeId {
		nmt.InternalCommand = command
	}

	/* Send NMT master message. */
	nmt.NMTTxBuff.Data[0] = command
	nmt.NMTTxBuff.Data[1] = node_id

	//TODO finish this

	//return CO_CANsend(NMT->NMT_CANdevTx, NMT->NMT_TXbuff);
	return nil

}
