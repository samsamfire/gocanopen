package nmt

import (
	"sync"

	canopen "github.com/samsamfire/gocanopen"
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

const ServiceId = 0

// Possible NMT states
const (
	StateInitializing   uint8 = 0
	StatePreOperational uint8 = 127
	StateOperational    uint8 = 5
	StateStopped        uint8 = 4
	StateUnknown        uint8 = 255
)

var stateMap = map[uint8]string{
	StateInitializing:   "INITIALIZING",
	StatePreOperational: "PRE-OPERATIONAL",
	StateOperational:    "OPERATIONAL",
	StateStopped:        "STOPPED",
	StateUnknown:        "UNKNOWN",
}

// Global node state to be used
const (
	ResetNot  uint8 = 0
	ResetComm uint8 = 1
	ResetApp  uint8 = 2
	ResetQuit uint8 = 3
)

// Available NMT commands
// They can be broadcasted to all nodes or to individual nodes
type Command uint8

const (
	CommandEmpty               Command = 0
	CommandEnterOperational    Command = 1
	CommandEnterStopped        Command = 2
	CommandEnterPreOperational Command = 128
	CommandResetNode           Command = 129
	CommandResetCommunication  Command = 130
)

var CommandDescription = map[Command]string{
	CommandEnterOperational:    "ENTER-OPERATIONAL",
	CommandEnterStopped:        "ENTER-STOPPED",
	CommandEnterPreOperational: "ENTER-PREOPERATIONAL",
	CommandResetNode:           "RESET-NODE",
	CommandResetCommunication:  "RESET-COMMUNICATION",
}

// NMT object for processing NMT behaviour, slave or master
type NMT struct {
	*canopen.BusManager
	mu                     sync.Mutex
	emcy                   *emergency.EMCY
	operatingState         uint8
	operatingStatePrev     uint8
	internalCommand        Command
	nodeId                 uint8
	control                uint16
	hearbeatProducerTimeUs uint32
	hearbeatProducerTimer  uint32
	nmtTxBuff              canopen.Frame
	hbTxBuff               canopen.Frame
	callback               func(nmtState uint8)
}

func (nmt *NMT) Handle(frame canopen.Frame) {
	nmt.mu.Lock()
	defer nmt.mu.Unlock()

	data := frame.Data
	if frame.DLC != 2 {
		return
	}
	command := Command(data[0])
	nodeId := data[1]
	if nodeId == 0 || nodeId == nmt.nodeId {
		nmt.internalCommand = command
	}
}

// Process NMT related tasks. This returns the global requested node state that
// can be used by application
func (nmt *NMT) Process(internalState *uint8, timeDifferenceUs uint32, timerNextUs *uint32) uint8 {
	nmt.mu.Lock()
	defer nmt.mu.Unlock()

	nmtStateCopy := nmt.operatingState
	resetCommand := ResetNot
	nmtInit := nmtStateCopy == StateInitializing
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
		nmt.mu.Unlock()
		nmt.Send(nmt.hbTxBuff)
		nmt.mu.Lock()
		if nmtStateCopy == StateInitializing {
			if nmt.control&StartupToOperational != 0 {
				nmtStateCopy = StateOperational
			} else {
				nmtStateCopy = StatePreOperational
			}
		} else {
			nmt.hearbeatProducerTimer = nmt.hearbeatProducerTimeUs

		}
	}
	nmt.operatingStatePrev = nmtStateCopy

	// Process internal NMT commands either from RX buffer or nmt send command
	if nmt.internalCommand != CommandEmpty {
		switch nmt.internalCommand {
		case CommandEnterOperational:
			nmtStateCopy = StateOperational

		case CommandEnterStopped:
			nmtStateCopy = StateStopped

		case CommandEnterPreOperational:
			nmtStateCopy = StatePreOperational

		case CommandResetNode:
			resetCommand = ResetApp

		case CommandResetCommunication:
			resetCommand = ResetComm

		}
		if resetCommand != ResetNot {
			log.Debugf("[NMT] received reset command %v this should be handled by user", CommandDescription[nmt.internalCommand])
		}
		nmt.internalCommand = CommandEmpty
	}

	busOff_HB := nmt.control&nmtErrOnBusOffHb != 0 &&
		(nmt.emcy.IsError(emergency.EmCanTXBusPassive) ||
			nmt.emcy.IsError(emergency.EmHeartbeatConsumer) ||
			nmt.emcy.IsError(emergency.EmHBConsumerRemoteReset))

	errRegMasked := (nmt.control&nmtErrOnErrReg != 0) &&
		((nmt.emcy.GetErrorRegister() & byte(nmt.control)) != 0)

	if nmtStateCopy == StateOperational && (busOff_HB || errRegMasked) {
		if nmt.control&nmtErrToStopped != 0 {
			nmtStateCopy = StateStopped
		} else {
			nmtStateCopy = StatePreOperational
		}
	} else if (nmt.control&nmtErrFreeToOperational) != 0 &&
		nmtStateCopy == StatePreOperational &&
		!busOff_HB &&
		!errRegMasked {

		nmtStateCopy = StateOperational
	}

	// Callback on change
	if nmt.operatingStatePrev != nmtStateCopy || nmtInit {
		if nmtInit {
			log.Debugf("[NMT] state changed | INITIALIZING ==> %v", stateMap[nmtStateCopy])
		} else {
			log.Debugf("[NMT] state changed | %v ==> %v", stateMap[nmt.operatingStatePrev], stateMap[nmtStateCopy])
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
	nmt.mu.Lock()
	defer nmt.mu.Unlock()

	if nmt == nil {
		return StateInitializing
	} else {
		return nmt.operatingState
	}
}

// Send NMT command to self, don't send on network
func (nmt *NMT) SendInternalCommand(command uint8) {
	nmt.mu.Lock()
	defer nmt.mu.Unlock()

	nmt.internalCommand = Command(command)
}

// Send an NMT command to the network
func (nmt *NMT) SendCommand(command Command, nodeId uint8) error {
	nmt.mu.Lock()
	defer nmt.mu.Unlock()

	// Also apply to node if concerned
	if nodeId == 0 || nodeId == nmt.nodeId {
		nmt.internalCommand = command
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

	nmt.operatingState = StateInitializing
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
	nmt.nmtTxBuff = canopen.NewFrame(uint32(canIdNmtTx), 0, 2)
	nmt.hbTxBuff = canopen.NewFrame(uint32(canIdHbTx), 0, 1)
	return nmt, nil
}
