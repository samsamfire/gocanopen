package nmt

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/emergency"
	"github.com/samsamfire/gocanopen/pkg/od"
)

const (
	StartupToOperational uint16 = 0x0100
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
	CommandEmpty:               "",
	CommandEnterOperational:    "ENTER-OPERATIONAL",
	CommandEnterStopped:        "ENTER-STOPPED",
	CommandEnterPreOperational: "ENTER-PREOPERATIONAL",
	CommandResetNode:           "RESET-NODE",
	CommandResetCommunication:  "RESET-COMMUNICATION",
}

// NMT object for processing NMT behaviour, slave or master
type NMT struct {
	bm                 *canopen.BusManager
	logger             *slog.Logger
	mu                 sync.Mutex
	emcy               *emergency.EMCY
	operatingState     uint8
	operatingStatePrev uint8
	internalCommand    Command
	nodeId             uint8
	control            uint16
	timerProducer      *time.Timer
	periodProducer     time.Duration
	firstHbTime        time.Duration
	canIdNmtRx         uint16
	nmtTxBuff          canopen.Frame
	hbTxBuff           canopen.Frame
	callbacks          map[uint64]func(nmtState uint8)
	callbackNextId     uint64
	rxCancel           func()
	reset              uint8
	stopped            bool
}

// Handle [NMT] related RX CAN frames
func (nmt *NMT) Handle(frame canopen.Frame) {
	nmt.mu.Lock()
	nodeId := nmt.nodeId
	nmt.mu.Unlock()

	data := frame.Data
	if frame.DLC != 2 {
		return
	}
	command := Command(data[0])
	targetNodeId := data[1]

	// Reject any commands that are not for us
	if targetNodeId != 0 && targetNodeId != nodeId {
		return
	}
	nmt.processCommand(command)
}

// Heartbeat is sent on three events :
// - a hearbeat producer timeout (cyclic)
// - state has changed
// - startup
func (nmt *NMT) restartTimerProducer(duration time.Duration) {
	nmt.mu.Lock()
	defer nmt.mu.Unlock()

	if nmt.stopped || nmt.periodProducer == 0 {
		return
	}

	if nmt.timerProducer == nil {
		nmt.timerProducer = time.AfterFunc(duration, nmt.producerHandler)
	} else {
		nmt.timerProducer.Reset(duration)
	}
}

func (nmt *NMT) producerHandler() {
	nmt.mu.Lock()
	nmtState := nmt.operatingState
	nmtStatePrev := nmt.operatingStatePrev
	nmtInit := nmtState == StateInitializing
	nmt.hbTxBuff.Data[0] = nmtState
	nmt.mu.Unlock()

	_ = nmt.send(nmt.hbTxBuff)

	nmt.mu.Lock()
	if nmtStatePrev != nmtState || nmtInit {
		nmt.logger.Info("nmt state changed", "previous", stateMap[nmtStatePrev], "new", stateMap[nmtState])
		for _, callback := range nmt.callbacks {
			callback(nmt.operatingState)
		}
	}

	if nmtInit {
		if nmt.control&StartupToOperational != 0 {
			nmt.operatingState = StateOperational
		} else {
			nmt.operatingState = StatePreOperational
		}
		nmt.logger.Info("automatically transitioning based on configuration", "state", stateMap[nmt.operatingState], "firstHeartbeat", nmt.firstHbTime)
		nmt.mu.Unlock()
		// Restart timer straight away to send new state
		nmt.restartTimerProducer(0)
		return
	}

	nmt.operatingStatePrev = nmtState
	nmt.mu.Unlock()
	nmt.restartTimerProducer(nmt.periodProducer)
}

func (nmt *NMT) send(frame canopen.Frame) error {
	err := nmt.bm.Send(frame)
	if err != nil {
		nmt.logger.Error("failed to send", "err", err)
	}
	return err
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

func (nmt *NMT) Start() {
	nmt.mu.Lock()
	nmt.stopped = false
	if nmt.rxCancel != nil {
		nmt.rxCancel()
	}
	// Configure NMT specific tx/rx buffers
	rxCancel, _ := nmt.bm.Subscribe(uint32(nmt.canIdNmtRx), 0x7FF, false, nmt)
	nmt.rxCancel = rxCancel
	nmt.mu.Unlock()
	nmt.restartTimerProducer(nmt.firstHbTime)
}

// Reset internal NMT state machine
func (nmt *NMT) Reset() {
	nmt.mu.Lock()
	nmt.operatingState = StateInitializing
	nmt.operatingStatePrev = StateInitializing
	nmt.reset = ResetNot
	nmt.mu.Unlock()
	nmt.restartTimerProducer(nmt.firstHbTime)
}

func (nmt *NMT) CheckResetAndClear() uint8 {
	nmt.mu.Lock()
	defer nmt.mu.Unlock()
	ret := nmt.reset
	nmt.reset = ResetNot
	return ret
}

// Stop NMT processing
func (nmt *NMT) Stop() {
	nmt.mu.Lock()
	defer nmt.mu.Unlock()
	nmt.stopped = true
	// Remove any callbacks
	nmt.callbacks = make(map[uint64]func(nmtState uint8))
	nmt.callbackNextId = 1
	if nmt.rxCancel != nil {
		nmt.rxCancel()
		nmt.rxCancel = nil
	}
	if nmt.timerProducer != nil {
		nmt.timerProducer.Stop()
		nmt.timerProducer = nil
	}
}

// Send NMT command to self, don't send on network
func (nmt *NMT) SendInternalCommand(command uint8) {
	cmd := Command(command)
	nmt.processCommand(cmd)
}

func (nmt *NMT) processCommand(command Command) {
	nmt.mu.Lock()
	defer nmt.mu.Unlock()

	newState := nmt.operatingState

	nmt.logger.Info("got new command", "command", CommandDescription[command])

	switch command {
	case CommandEmpty:
		return
	case CommandEnterOperational:
		newState = StateOperational

	case CommandEnterStopped:
		newState = StateStopped

	case CommandEnterPreOperational:
		newState = StatePreOperational

	case CommandResetNode:
		nmt.reset = ResetApp
		return

	case CommandResetCommunication:
		nmt.reset = ResetComm
		return
	}

	if newState != nmt.operatingState {
		nmt.operatingState = newState
		nmt.mu.Unlock()
		// state change, so should send straight away
		nmt.restartTimerProducer(0)
		nmt.mu.Lock()
	}
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
	return nmt.send(nmt.nmtTxBuff)
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
	firstHbTime time.Duration,
	canIdNmtTx uint16,
	canIdNmtRx uint16,
	canIdHbTx uint16,
	entry1017 *od.Entry,
) (*NMT, error) {

	if logger == nil {
		logger = slog.Default()
	}

	nmt := &NMT{bm: bm, logger: logger.With("service", "[NMT]")}
	if entry1017 == nil || bm == nil {
		return nil, canopen.ErrIllegalArgument
	}

	nmt.operatingState = StateInitializing
	nmt.nodeId = nodeId
	nmt.control = control
	nmt.emcy = emergency
	nmt.reset = ResetNot
	nmt.callbacks = make(map[uint64]func(nmtState uint8))
	nmt.callbackNextId = 1
	nmt.firstHbTime = firstHbTime

	hbProdTimeMs, err := entry1017.Uint16(0)
	if err != nil {
		nmt.logger.Error("reading producer heartbeat failed",
			"index", fmt.Sprintf("x%x", 0x1017),
			"subindex", 0,
			"error", err,
		)
		return nil, canopen.ErrOdParameters
	}
	nmt.periodProducer = time.Duration(uint32(hbProdTimeMs)) * time.Millisecond
	// Extension needs to be initialized
	entry1017.AddExtension(nmt, od.ReadEntryDefault, writeEntry1017)

	nmt.nmtTxBuff = canopen.NewFrame(uint32(canIdNmtTx), 0, 2)
	nmt.hbTxBuff = canopen.NewFrame(uint32(canIdHbTx), 0, 1)
	return nmt, nil
}
