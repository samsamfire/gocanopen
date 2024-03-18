package nmt

import (
	canopen "github.com/samsamfire/gocanopen"
	can "github.com/samsamfire/gocanopen/pkg/can"
	"github.com/samsamfire/gocanopen/pkg/emergency"
	"github.com/samsamfire/gocanopen/pkg/od"
	log "github.com/sirupsen/logrus"
)

const (
	nmtErrRegMask           uint16 = 0x00FF
	StartupToOperational    uint16 = 0x0100
	nmtErrOnBusOffHb        uint16 = 0x1000
	nmtErrOnErrReg          uint16 = 0x2000
	nmtErrToStopped         uint16 = 0x4000
	nmtErrFreeToOperational uint16 = 0x8000
)

const SERVICE_ID = 0

const (
	RESET_NOT  uint8 = 0
	RESET_COMM uint8 = 1
	RESET_APP  uint8 = 2
	RESET_QUIT uint8 = 3
)

// Possible NMT states
const (
	NMT_INITIALIZING    uint8 = 0
	NMT_PRE_OPERATIONAL uint8 = 127
	NMT_OPERATIONAL     uint8 = 5
	NMT_STOPPED         uint8 = 4
	NMT_UNKNOWN         uint8 = 255
)

var NMT_STATE_MAP = map[uint8]string{
	NMT_INITIALIZING:    "INITIALIZING",
	NMT_PRE_OPERATIONAL: "PRE-OPERATIONAL",
	NMT_OPERATIONAL:     "OPERATIONAL",
	NMT_STOPPED:         "STOPPED",
	NMT_UNKNOWN:         "UNKNOWN",
}

type NMTCommand uint8

// Available NMT commands
// They can be broadcasted to all nodes or to individual nodes
const (
	NMT_NO_COMMAND            NMTCommand = 0
	NMT_ENTER_OPERATIONAL     NMTCommand = 1
	NMT_ENTER_STOPPED         NMTCommand = 2
	NMT_ENTER_PRE_OPERATIONAL NMTCommand = 128
	NMT_RESET_NODE            NMTCommand = 129
	NMT_RESET_COMMUNICATION   NMTCommand = 130
)

var CommandDescription = map[NMTCommand]string{
	NMT_ENTER_OPERATIONAL:     "ENTER-OPERATIONAL",
	NMT_ENTER_STOPPED:         "ENTER-STOPPED",
	NMT_ENTER_PRE_OPERATIONAL: "ENTER-PREOPERATIONAL",
	NMT_RESET_NODE:            "RESET-NODE",
	NMT_RESET_COMMUNICATION:   "RESET-COMMUNICATION",
}

// NMT object for processing NMT behaviour, slave or master
type NMT struct {
	*canopen.BusManager
	emcy                   *emergency.EMCY
	operatingState         uint8
	operatingStatePrev     uint8
	internalCommand        NMTCommand
	nodeId                 uint8
	control                uint16
	hearbeatProducerTimeUs uint32
	hearbeatProducerTimer  uint32
	nmtTxBuff              can.Frame
	hbTxBuff               can.Frame
	callback               func(nmtState uint8)
}

func (nmt *NMT) Handle(frame can.Frame) {
	dlc := frame.DLC
	data := frame.Data
	if dlc != 2 {
		return
	}
	command := NMTCommand(data[0])
	nodeId := data[1]
	if nodeId == 0 || nodeId == nmt.nodeId {
		nmt.internalCommand = command
	}
}

func (nmt *NMT) Process(internalState *uint8, timeDifferenceUs uint32, timerNextUs *uint32) uint8 {
	nmtStateCopy := nmt.operatingState
	resetCommand := RESET_NOT
	nmtInit := nmtStateCopy == NMT_INITIALIZING
	if nmt.hearbeatProducerTimer > timeDifferenceUs {
		nmt.hearbeatProducerTimer -= timeDifferenceUs
	} else {
		nmt.hearbeatProducerTimer = 0
	}
	// Heartbeat is sent on three events :
	// - a hearbeat producer timeout (cyclic)
	// - state has changed
	// - startup
	if nmtInit || (nmt.hearbeatProducerTimeUs != 0 && (nmt.hearbeatProducerTimer == 0 || nmtStateCopy != nmt.operatingStatePrev)) {
		nmt.hbTxBuff.Data[0] = nmtStateCopy
		nmt.Send(nmt.hbTxBuff)
		if nmtStateCopy == NMT_INITIALIZING {
			if nmt.control&StartupToOperational != 0 {
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
			log.Debugf("[NMT] received reset command %v this should be handled by user", CommandDescription[nmt.internalCommand])
		}
		nmt.internalCommand = NMT_NO_COMMAND
	}

	busOff_HB := nmt.control&nmtErrOnBusOffHb != 0 &&
		(nmt.emcy.IsError(emergency.EmCanTXBusPassive) ||
			nmt.emcy.IsError(emergency.EmHeartbeatConsumer) ||
			nmt.emcy.IsError(emergency.EmHBConsumerRemoteReset))

	errRegMasked := (nmt.control&nmtErrOnErrReg != 0) &&
		((nmt.emcy.GetErrorRegister() & byte(nmt.control)) != 0)

	if nmtStateCopy == NMT_OPERATIONAL && (busOff_HB || errRegMasked) {
		if nmt.control&nmtErrToStopped != 0 {
			nmtStateCopy = NMT_STOPPED
		} else {
			nmtStateCopy = NMT_PRE_OPERATIONAL
		}
	} else if (nmt.control&nmtErrFreeToOperational) != 0 &&
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

// Get a NMT state
func (nmt *NMT) GetInternalState() uint8 {
	if nmt == nil {
		return NMT_INITIALIZING
	} else {
		return nmt.operatingState
	}
}

// Send NMT command to self, don't send on network
func (nmt *NMT) SendInternalCommand(command uint8) {
	nmt.internalCommand = NMTCommand(command)
}

// Send an NMT command to the network
func (nmt *NMT) SendCommand(command NMTCommand, nodeId uint8) error {
	// Also apply to node if concerned
	if nodeId == 0 || nodeId == nmt.nodeId {
		nmt.internalCommand = NMTCommand(command)
	}
	// Send NMT command
	nmt.nmtTxBuff.Data[0] = uint8(command)
	nmt.nmtTxBuff.Data[1] = nodeId
	return nmt.Send(nmt.nmtTxBuff)
}

func NewNMT(
	bm *canopen.BusManager,
	emergency *emergency.EMCY,
	nodeId uint8,
	control uint16,
	firstHbTimeMs uint16,
	canIdNmtTx uint16,
	canIdNmtRx uint16,
	canIdHbTx uint16,
	entry1017 *od.Entry,
) (*NMT, error) {

	nmt := &NMT{BusManager: bm}
	if entry1017 == nil || bm == nil {
		return nil, canopen.ErrIllegalArgument
	}

	nmt.operatingState = NMT_INITIALIZING
	nmt.operatingStatePrev = nmt.operatingState
	nmt.nodeId = nodeId
	nmt.control = control
	nmt.emcy = emergency
	nmt.hearbeatProducerTimer = uint32(firstHbTimeMs * 1000)

	hbProdTimeMs, err := entry1017.Uint16(0)
	if err != nil {
		log.Errorf("[NMT][%x|%x] reading producer heartbeat failed : %v", 0x1017, 0x0, err)
		return nil, canopen.ErrOdParameters
	}
	nmt.hearbeatProducerTimeUs = uint32(hbProdTimeMs) * 1000
	// Extension needs to be initialized
	entry1017.AddExtension(nmt, od.ReadEntryDefault, writeEntry1017)

	if nmt.hearbeatProducerTimer > nmt.hearbeatProducerTimeUs {
		nmt.hearbeatProducerTimer = nmt.hearbeatProducerTimeUs
	}

	// Configure NMT specific tx/rx buffers
	err = nmt.Subscribe(uint32(canIdNmtRx), 0x7FF, false, nmt)
	if err != nil {
		return nil, err
	}
	nmt.nmtTxBuff = can.NewFrame(uint32(canIdNmtTx), 0, 2)
	nmt.hbTxBuff = can.NewFrame(uint32(canIdHbTx), 0, 1)
	return nmt, nil
}
