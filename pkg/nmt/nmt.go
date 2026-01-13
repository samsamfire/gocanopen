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
	bm                     *canopen.BusManager
	logger                 *slog.Logger
	mu                     sync.Mutex
	emcy                   *emergency.EMCY
	operatingState         uint8
	operatingStatePrev     uint8
	internalCommand        Command
	nodeId                 uint8
	control                uint16
	hearbeatProducerTimeUs uint32
	timer                  *time.Timer
	resetCommand           uint8
	nmtTxBuff              canopen.Frame
	hbTxBuff               canopen.Frame
	callbacks              map[uint64]func(nmtState uint8)
	callbackNextId         uint64
	rxCancel               func()
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
		fmt.Println("processing via handle")
		nmt.processCommand(command)
	}
}

func (nmt *NMT) processCommand(command Command) {
	nmtStateCopy := nmt.operatingState

	switch command {
	case CommandEnterOperational:
		nmtStateCopy = StateOperational

	case CommandEnterStopped:
		nmtStateCopy = StateStopped

	case CommandEnterPreOperational:
		nmtStateCopy = StatePreOperational

	case CommandResetNode:
		nmt.resetCommand = ResetApp

	case CommandResetCommunication:
		nmt.resetCommand = ResetComm
	}

	if nmt.resetCommand != ResetNot {
		nmt.logger.Debug("this reset command should be handled by user", "command", CommandDescription[command])
	}

	fmt.Println("prev", stateMap[nmt.operatingState], "new", stateMap[nmtStateCopy])

	if nmtStateCopy != nmt.operatingState {
		nmt.setState(nmtStateCopy)
	}
}

func (nmt *NMT) setState(newState uint8) {
	if newState != nmt.operatingState {
		prev := stateMap[nmt.operatingState]
		nmt.logger.Info("nmt state changed", "previous", prev, "new", stateMap[newState])
		nmt.operatingState = newState

		// Heartbeat is sent on three events :
		// - a hearbeat producer timeout (cyclic)
		// - state has changed
		// - startup
		nmt.sendHeartbeat()

		for _, callback := range nmt.callbacks {
			callback(newState)
		}
	}
}

// Send a hearbeat with the current nmt state
// this will trigger an automatic reschedule if hearbeat producer is active
func (nmt *NMT) sendHeartbeat() {
	nmt.hbTxBuff.Data[0] = nmt.operatingState
	_ = nmt.send(nmt.hbTxBuff)

	fmt.Println("sending heartbeat", "period", nmt.hearbeatProducerTimeUs, "state", stateMap[nmt.operatingState])

	// Reset timer
	if nmt.hearbeatProducerTimeUs > 0 {
		if nmt.timer == nil {
			nmt.timer = time.AfterFunc(time.Duration(nmt.hearbeatProducerTimeUs)*time.Microsecond, nmt.heartbeatTimeout)
		} else {
			nmt.timer.Reset(time.Duration(nmt.hearbeatProducerTimeUs) * time.Microsecond)
		}
	}
}

func (nmt *NMT) heartbeatTimeout() {
	nmt.mu.Lock()
	defer nmt.mu.Unlock()
	// Heartbeat is sent on three events :
	// - a hearbeat producer timeout (cyclic)
	// - state has changed
	// - startup
	nmt.sendHeartbeat()
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
	fmt.Println("oparting state", "state", stateMap[nmt.operatingState])
	return nmt.operatingState
}

// Get and clear pending reset command
func (nmt *NMT) GetPendingReset() uint8 {
	nmt.mu.Lock()
	defer nmt.mu.Unlock()
	cmd := nmt.resetCommand
	nmt.resetCommand = ResetNot
	return cmd
}

// Reset internal NMT state machine
func (nmt *NMT) Reset() {
	nmt.mu.Lock()
	nmt.operatingState = StateInitializing
	nmt.mu.Unlock()
	nmt.Start()
}

// Stop NMT processing
func (nmt *NMT) Stop() {
	nmt.mu.Lock()
	defer nmt.mu.Unlock()

	if nmt.timer != nil {
		nmt.timer.Stop()
	}
	// Remove any callbacks
	nmt.callbacks = make(map[uint64]func(nmtState uint8))
	nmt.callbackNextId = 1
}

// Start NMT processing (this will trigger sending a heartbeat because equivalent to bootup)
func (nmt *NMT) Start() {
	nmt.mu.Lock()
	defer nmt.mu.Unlock()

	// Heartbeat is sent on three events :
	// - a hearbeat producer timeout (cyclic)
	// - state has changed
	// - startup
	nmt.sendHeartbeat()
	if nmt.operatingState == StateInitializing {
		if nmt.control&StartupToOperational != 0 {
			nmt.operatingState = StateOperational
		} else {
			nmt.operatingState = StatePreOperational
		}
	}
}

// Send NMT command to self, don't send on network
func (nmt *NMT) SendInternalCommand(command uint8) {
	nmt.mu.Lock()
	defer nmt.mu.Unlock()

	nmt.processCommand(Command(command))
}

// Send an NMT command to the network
func (nmt *NMT) SendCommand(command Command, nodeId uint8) error {
	nmt.mu.Lock()
	defer nmt.mu.Unlock()

	// Also apply to node if concerned
	if nodeId == 0 || nodeId == nmt.nodeId {
		nmt.processCommand(command)
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
	firstHbTimeMs uint16,
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
	nmt.operatingStatePrev = nmt.operatingState
	nmt.nodeId = nodeId
	nmt.control = control
	nmt.emcy = emergency
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

	// Configure NMT specific tx/rx buffers
	rxCancel, err := nmt.bm.Subscribe(uint32(canIdNmtRx), 0x7FF, false, nmt)
	nmt.rxCancel = rxCancel
	if err != nil {
		return nil, err
	}
	nmt.nmtTxBuff = canopen.NewFrame(uint32(canIdNmtTx), 0, 2)
	nmt.hbTxBuff = canopen.NewFrame(uint32(canIdHbTx), 0, 1)

	// Start heartbeat
	nmt.Start()

	return nmt, nil
}
