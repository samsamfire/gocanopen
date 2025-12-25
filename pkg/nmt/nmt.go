package nmt

import (
	"fmt"
	"log/slog"
	"sync"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/emergency"
	"github.com/samsamfire/gocanopen/pkg/od"
)

const (
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
	logger                 *slog.Logger
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
	callbacks              map[uint64]func(nmtState uint8)
	callbackNextId         uint64
}

// Handle [NMT] related RX CAN frames
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

// Process [NMT] state machine and TX CAN frames
// It returns a computed global state request for the node
// This should be called periodically
func (nmt *NMT) Process(internalState *uint8, timeDifferenceUs uint32) uint8 {
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
		_ = nmt.Send(nmt.hbTxBuff)
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
			nmt.logger.Debug("this reset command should be handled by user", "command", CommandDescription[nmt.internalCommand])
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
		prev := ""
		if nmtInit {
			prev = stateMap[StateInitializing]
		} else {
			prev = stateMap[nmt.operatingStatePrev]
		}
		nmt.logger.Info("nmt state changed", "previous", prev, "new", stateMap[nmtStateCopy])
		for _, callback := range nmt.callbacks {
			callback(nmtStateCopy)
		}
	}

	nmt.operatingState = nmtStateCopy
	*internalState = nmtStateCopy

	return resetCommand

}

// Get a NMT state
func (nmt *NMT) GetInternalState() uint8 {
	if nmt == nil {
		return StateInitializing
	}
	nmt.mu.Lock()
	defer nmt.mu.Unlock()
	return nmt.operatingState
}

// Reset internal NMT state machine
func (nmt *NMT) Reset() {
	nmt.mu.Lock()
	defer nmt.mu.Unlock()
	nmt.operatingState = StateInitializing
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

// Add a callback func to be called on NMT state change
// It returns a cancel func that can be used to remove the callback
func (nmt *NMT) AddStateChangeCallback(callback func(nmtState uint8)) (cancel func()) {
	nmt.mu.Lock()
	defer nmt.mu.Unlock()

	id := nmt.callbackNextId
	nmt.callbackNextId++
	nmt.callbacks[id] = callback

	// Return a cancel closure func
	return func() {
		nmt.mu.Lock()
		defer nmt.mu.Unlock()
		delete(nmt.callbacks, id)
	}
}

func NewNMT(
	bm *canopen.BusManager,
	logger *slog.Logger,
	emergency *emergency.EMCY,
	nodeId uint8,
	control uint16,
	firstHbTimeMs uint16,
	canIdNmtTx uint16,
	canIdNmtRx uint16,
	canIdHbTx uint16,
	entry1017 *od.Entry,
) (*NMT, error) {

	if logger == nil {
		logger = slog.Default()
	}

	nmt := &NMT{BusManager: bm, logger: logger.With("service", "[NMT]")}
	if entry1017 == nil || bm == nil {
		return nil, canopen.ErrIllegalArgument
	}

	nmt.operatingState = StateInitializing
	nmt.operatingStatePrev = nmt.operatingState
	nmt.nodeId = nodeId
	nmt.control = control
	nmt.emcy = emergency
	nmt.hearbeatProducerTimer = uint32(firstHbTimeMs * 1000)
	nmt.callbacks = make(map[uint64]func(nmtState uint8))
	nmt.callbackNextId = 1

	hbProdTimeMs, err := entry1017.Uint16(0)
	if err != nil {
		nmt.logger.Error("reading producer heartbeat failed",
			"index", fmt.Sprintf("x%x", 0x1017),
			"subindex", 0,
			"error", err,
		)
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
