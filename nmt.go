package canopen

import (
	log "github.com/sirupsen/logrus"
)

const (
	NMT_ERR_REG_MASK            uint16 = 0x00FF
	NMT_STARTUP_TO_OPERATIONAL  uint16 = 0x0100
	NMT_ERR_ON_BUSOFF_HB        uint16 = 0x1000
	NMT_ERR_ON_ERR_REG          uint16 = 0x2000
	NMT_ERR_TO_STOPPED          uint16 = 0x4000
	NMT_ERR_FREE_TO_OPERATIONAL uint16 = 0x8000
)

const (
	RESET_NOT  uint8 = 0
	RESET_COMM uint8 = 1
	RESET_APP  uint8 = 2
	RESET_QUIT uint8 = 3
)

const (
	NMT_INITIALIZING    uint8 = 0
	NMT_PRE_OPERATIONAL uint8 = 127
	NMT_OPERATIONAL     uint8 = 5
	NMT_STOPPED         uint8 = 4
)

var NMT_STATE_MAP = map[uint8]string{
	NMT_INITIALIZING:    "INITIALIZING",
	NMT_PRE_OPERATIONAL: "PRE-OPERATIONAL",
	NMT_OPERATIONAL:     "OPERATIONAL",
	NMT_STOPPED:         "STOPPED",
}

const (
	NMT_NO_COMMAND            uint8 = 0
	NMT_ENTER_OPERATIONAL     uint8 = 1
	NMT_ENTER_STOPPED         uint8 = 2
	NMT_ENTER_PRE_OPERATIONAL uint8 = 128
	NMT_RESET_NODE            uint8 = 129
	NMT_RESET_COMMUNICATION   uint8 = 130
)

var NMT_COMMAND_MAP = map[uint8]string{
	NMT_ENTER_OPERATIONAL:     "ENTER-OPERATIONAL",
	NMT_ENTER_STOPPED:         "ENTER-STOPPED",
	NMT_ENTER_PRE_OPERATIONAL: "ENTER-PREOPERATIONAL",
	NMT_RESET_NODE:            "RESET-NODE",
	NMT_RESET_COMMUNICATION:   "RESET-COMMUNICATION",
}

type NMT struct {
	operatingState         uint8
	operatingStatePrev     uint8
	internalCommand        uint8
	nodeId                 uint8
	control                uint16
	hearbeatProducerTimeUs uint32
	hearbeatProducerTimer  uint32
	extensionEntry1017     Extension
	emergency              *EM
	busManager             *BusManager
	nmtTxBuff              *BufferTxFrame
	hbTxBuff               *BufferTxFrame
	callback               func(nmtState uint8)
}

func (nmt *NMT) Handle(frame Frame) {
	dlc := frame.DLC
	data := frame.Data
	if dlc != 2 {
		return
	}
	command := data[0]
	nodeId := data[1]
	if nodeId == 0 || nodeId == nmt.nodeId {
		nmt.internalCommand = command
	}
}

func (nmt *NMT) Init(
	entry1017 *Entry,
	emergency *EM,
	nodeId uint8,
	control uint16,
	firstHbTimeMs uint16,
	busManager *BusManager,
	canIdNmtTx uint16,
	canIdNmtRx uint16,
	canIdHbTx uint16,
) error {
	if entry1017 == nil || busManager == nil {
		return CO_ERROR_ILLEGAL_ARGUMENT
	}

	nmt.operatingState = NMT_INITIALIZING
	nmt.operatingStatePrev = nmt.operatingState
	nmt.nodeId = nodeId
	nmt.control = control
	nmt.emergency = emergency
	nmt.hearbeatProducerTimer = uint32(firstHbTimeMs * 1000)

	var HBprodTime_ms uint16
	err := entry1017.GetUint16(0, &HBprodTime_ms)
	if err != nil {
		log.Errorf("[NMT][%x|%x] reading producer heartbeat failed : %v", err)
		return CO_ERROR_OD_PARAMETERS
	}
	nmt.hearbeatProducerTimeUs = uint32(HBprodTime_ms) * 1000
	// Extension needs to be initialized
	nmt.extensionEntry1017.Object = nmt
	nmt.extensionEntry1017.Read = ReadEntryOriginal
	nmt.extensionEntry1017.Write = WriteEntry1017
	entry1017.AddExtension(&nmt.extensionEntry1017)

	if nmt.hearbeatProducerTimer > nmt.hearbeatProducerTimeUs {
		nmt.hearbeatProducerTimer = nmt.hearbeatProducerTimeUs
	}

	// Configure NMT specific tx/rx buffers
	nmt.busManager = busManager
	_, err = busManager.InsertRxBuffer(uint32(canIdNmtRx), 0x7FF, false, nmt)
	if err != nil {
		return err
	}
	nmt.nmtTxBuff, _, err = busManager.InsertTxBuffer(uint32(canIdNmtTx), false, 2, false)
	if err != nil {
		return err
	}
	nmt.hbTxBuff, _, err = busManager.InsertTxBuffer(uint32(canIdHbTx), false, 1, false)
	if err != nil {
		return err
	}
	return nil

}

func (nmt *NMT) Process(internalState *uint8, timeDifferenceUs uint32, timerNextUs *uint32) uint8 {
	nmtStateCopy := nmt.operatingState
	resetCommand := RESET_NOT
	nmtInit := nmtStateCopy == NMT_INITIALIZING
	if nmt.hearbeatProducerTimer > timeDifferenceUs {
		nmt.hearbeatProducerTimer = nmt.hearbeatProducerTimer - timeDifferenceUs
	} else {
		nmt.hearbeatProducerTimer = 0
	}
	// Heartbeat is sent on three events :
	// - a hearbeat producer timeout (cyclic)
	// - state has changed
	// - startup
	if nmtInit || (nmt.hearbeatProducerTimeUs != 0 && (nmt.hearbeatProducerTimer == 0 || nmtStateCopy != nmt.operatingStatePrev)) {
		nmt.hbTxBuff.Data[0] = nmtStateCopy
		nmt.busManager.Send(*nmt.hbTxBuff)
		if nmtStateCopy == NMT_INITIALIZING {
			if nmt.control&NMT_STARTUP_TO_OPERATIONAL != 0 {
				nmtStateCopy = NMT_OPERATIONAL
			} else {
				nmtStateCopy = NMT_PRE_OPERATIONAL
			}
		} else {
			nmt.hearbeatProducerTimer = nmt.hearbeatProducerTimeUs

		}
	}
	nmt.operatingStatePrev = nmtStateCopy

	// Process internal NMT commands either from RX buffer or nmt send command
	if nmt.internalCommand != NMT_NO_COMMAND {
		switch nmt.internalCommand {
		case NMT_ENTER_OPERATIONAL:
			nmtStateCopy = NMT_OPERATIONAL

		case NMT_ENTER_STOPPED:
			nmtStateCopy = NMT_STOPPED

		case NMT_ENTER_PRE_OPERATIONAL:
			nmtStateCopy = NMT_PRE_OPERATIONAL

		case NMT_RESET_NODE:
			resetCommand = RESET_APP

		case NMT_RESET_COMMUNICATION:
			resetCommand = RESET_COMM

		}
		if resetCommand != RESET_NOT {
			log.Debugf("[NMT] received reset command %v this should be handled by user", NMT_COMMAND_MAP[nmt.internalCommand])
		}
		nmt.internalCommand = NMT_NO_COMMAND
	}

	busOff_HB := nmt.control&NMT_ERR_ON_BUSOFF_HB != 0 &&
		(nmt.emergency.IsError(CO_EM_CAN_TX_BUS_OFF) ||
			nmt.emergency.IsError(CO_EM_HEARTBEAT_CONSUMER) ||
			nmt.emergency.IsError(CO_EM_HB_CONSUMER_REMOTE_RESET))

	errRegMasked := (nmt.control&NMT_ERR_ON_ERR_REG != 0) &&
		((nmt.emergency.GetErrorRegister() & byte(nmt.control)) != 0)

	if nmtStateCopy == NMT_OPERATIONAL && (busOff_HB || errRegMasked) {
		if nmt.control&NMT_ERR_TO_STOPPED != 0 {
			nmtStateCopy = NMT_STOPPED
		} else {
			nmtStateCopy = NMT_PRE_OPERATIONAL
		}
	} else if (nmt.control&NMT_ERR_FREE_TO_OPERATIONAL) != 0 &&
		nmtStateCopy == NMT_PRE_OPERATIONAL &&
		!busOff_HB &&
		!errRegMasked {

		nmtStateCopy = NMT_OPERATIONAL
	}

	// Callback on change
	if nmt.operatingStatePrev != nmtStateCopy || nmtInit {
		if nmtInit {
			log.Debugf("[NMT] state changed | INITIALIZING ==> %v", NMT_STATE_MAP[nmtStateCopy])
		} else {
			log.Debugf("[NMT] state changed | %v ==> %v", NMT_STATE_MAP[nmt.operatingStatePrev], NMT_STATE_MAP[nmtStateCopy])
		}
		if nmt.callback != nil {
			nmt.callback(nmtStateCopy)
		}
	}

	// Calculate next heartbeat
	if nmt.hearbeatProducerTimeUs != 0 && timerNextUs != nil {
		if nmt.operatingStatePrev != nmtStateCopy {
			*timerNextUs = 0
		} else if *timerNextUs > nmt.hearbeatProducerTimer {
			*timerNextUs = nmt.hearbeatProducerTimer
		}
	}

	nmt.operatingState = nmtStateCopy
	*internalState = nmtStateCopy

	return resetCommand

}

func (nmt *NMT) GetInternalState() uint8 {
	if nmt == nil {
		return NMT_INITIALIZING
	} else {
		return nmt.operatingState
	}
}

// Send NMT command to self, don't send on network
func (nmt *NMT) SendInternalCommand(command uint8) {
	nmt.internalCommand = command
}

// Send an NMT command to the network
func (nmt *NMT) SendCommand(command uint8, node_id uint8) error {
	if nmt == nil {
		return CO_ERROR_ILLEGAL_ARGUMENT
	}
	// Also apply to node if concerned
	if node_id == 0 || node_id == nmt.nodeId {
		nmt.internalCommand = command
	}
	// Send NMT command
	nmt.nmtTxBuff.Data[0] = command
	nmt.nmtTxBuff.Data[1] = node_id
	nmt.busManager.Send((*nmt.nmtTxBuff))
	return nil
}
