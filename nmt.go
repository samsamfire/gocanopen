package canopen

import "fmt"

const (
	CO_NMT_ERR_REG_MASK            uint16 = 0x00FF
	CO_NMT_STARTUP_TO_OPERATIONAL  uint16 = 0x0100
	CO_NMT_ERR_ON_BUSOFF_HB        uint16 = 0x1000
	CO_NMT_ERR_ON_ERR_REG          uint16 = 0x2000
	CO_NMT_ERR_TO_STOPPED          uint16 = 0x4000
	CO_NMT_ERR_FREE_TO_OPERATIONAL uint16 = 0x8000
)

const (
	CO_RESET_NOT  uint8 = 0
	CO_RESET_COMM uint8 = 1
	CO_RESET_APP  uint8 = 2
	CO_RESET_QUIT uint8 = 3
)

const (
	CO_NMT_UNKNOWN         = -1
	CO_NMT_INITIALIZING    = 0
	CO_NMT_PRE_OPERATIONAL = 127
	CO_NMT_OPERATIONAL     = 5
	CO_NMT_STOPPED         = 4
)

const (
	CO_NMT_NO_COMMAND            = 0
	CO_NMT_ENTER_OPERATIONAL     = 1
	CO_NMT_ENTER_STOPPED         = 2
	CO_NMT_ENTER_PRE_OPERATIONAL = 128
	CO_NMT_RESET_NODE            = 129
	CO_NMT_RESET_COMMUNICATION   = 130
)

// NMT object
type NMT struct {
	OperatingState         uint8
	OperatingStatePrev     uint8
	InternalCommand        uint8
	NodeId                 uint8
	Control                uint16
	HearbeatProducerTimeUs uint32
	HearbeatProducerTimer  uint32
	Entry1017              Extension
	Emergency              *EM
	CANModule              *CANModule
	NMTTxBuff              *BufferTxFrame
	HBTxBuff               *BufferTxFrame
	Callback               func(nmtState uint8)
	// Optionally add callback functions
}

func (nmt *NMT) Init(
	OD_1017_ProducerHbTime *Entry,
	emergency *EM,
	node_id uint8,
	control uint16,
	first_hb_time_ms uint16,
	can_module *CANModule,
	nmt_tx_index uint16,
	nmt_rx_index uint16,
	hb_tx_index uint16,
	can_id_nmt_tx uint16,
	can_id_nmt_rx uint16,
	can_id_hb_tx uint16,
) error {
	if OD_1017_ProducerHbTime == nil || em == nil || can_module == nil {
		return CANopenError(CO_ERROR_ILLEGAL_ARGUMENT)
	}

	nmt.OperatingState = CO_NMT_INITIALIZING
	nmt.OperatingStatePrev = nmt.OperatingState
	nmt.NodeId = node_id
	nmt.Control = control
	nmt.Emergency = emergency
	nmt.HearbeatProducerTimer = uint32(first_hb_time_ms * 1000)

	/* get and verify required "Producer heartbeat time" from Object Dict. */

	return nil

}

// void CO_NMT_initCallbackPre(CO_NMT_t *NMT,
//    void *object,
//    void (*pFunctSignal)(void *object));

// void CO_NMT_initCallbackChanged(CO_NMT_t *NMT,
// 	   void (*pFunctNMT)(CO_NMT_internalState_t state));
// #endif

// Called cyclically
func (nmt *NMT) Process(internal_state *uint8, time_difference_us uint32, timer_next_us *uint32) uint8 {
	nmtStateCopy := nmt.OperatingState
	resetCommand := CO_RESET_NOT
	nmtInit := nmtStateCopy == CO_NMT_INITIALIZING
	if nmt.HearbeatProducerTimer > time_difference_us {
		nmt.HearbeatProducerTimer = nmt.HearbeatProducerTimer - time_difference_us
	} else {
		nmt.HearbeatProducerTimer = 0
	}
	/* Send Hearbeat if:
	* - First start (send bootup message)
	* - Hearbeat producer enabled and timer expired or the operating state has changed
	 */
	if nmtInit || (nmt.HearbeatProducerTimeUs != 0 && (nmt.HearbeatProducerTimer == 0 || nmtStateCopy != nmt.OperatingStatePrev)) {
		nmt.HBTxBuff.Data[0] = nmtStateCopy
		nmt.CANModule.Send(*nmt.HBTxBuff)
		if nmtStateCopy == CO_NMT_INITIALIZING {
			/* NMT slave is self starting */
			if nmt.Control&CO_NMT_STARTUP_TO_OPERATIONAL != 0 {
				nmtStateCopy = CO_NMT_OPERATIONAL
			} else {
				nmtStateCopy = CO_NMT_PRE_OPERATIONAL
			}
		} else {
			/* Start timer from the beginning. If OS is slow, time sliding may
			* occur. However, heartbeat is not for synchronization, it is for
			* health report. In case of initializing, timer is set in the
			* CO_NMT_init() function with pre-defined value. */
			nmt.HearbeatProducerTimer = nmt.HearbeatProducerTimeUs

		}
	}
	nmt.OperatingStatePrev = nmtStateCopy

	/* Process internal NMT commands either from RX buffer or nmt Send COmmand */
	if nmt.InternalCommand != CO_NMT_NO_COMMAND {
		switch nmt.InternalCommand {

		case CO_NMT_ENTER_OPERATIONAL:
			nmtStateCopy = CO_NMT_OPERATIONAL

		case CO_NMT_ENTER_STOPPED:
			nmtStateCopy = CO_NMT_STOPPED

		case CO_NMT_ENTER_PRE_OPERATIONAL:
			nmtStateCopy = CO_NMT_PRE_OPERATIONAL

		case CO_NMT_RESET_NODE:
			resetCommand = CO_RESET_APP

		case CO_NMT_RESET_COMMUNICATION:
			resetCommand = CO_RESET_COMM

		default:

		}
		nmt.InternalCommand = CO_NMT_NO_COMMAND
	}

	/* verify NMT transitions based on error register */
	// TODO, don't have Emergency implementation yet

	/* Callback on operating state change */
	if nmt.OperatingStatePrev != nmtStateCopy || nmtInit {
		if nmt.Callback != nil {
			nmt.Callback(nmtStateCopy)
		}
	}

	/* Calculate, when next Heartbeat needs to be send */
	if nmt.HearbeatProducerTimeUs != 0 && timer_next_us != nil {
		if nmt.OperatingStatePrev != nmtStateCopy {
			*timer_next_us = 0
		} else if *timer_next_us > nmt.HearbeatProducerTimer {
			*timer_next_us = nmt.HearbeatProducerTimer
		}
	}

	nmt.OperatingState = nmtStateCopy
	*internal_state = nmtStateCopy

	return resetCommand

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
	nmt.CANModule.Send((*nmt.NMTTxBuff))
	return nil

}
