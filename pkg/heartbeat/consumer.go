package heartbeat

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/emergency"
	"github.com/samsamfire/gocanopen/pkg/nmt"
	"github.com/samsamfire/gocanopen/pkg/od"
)

const (
	HeartbeatUnconfigured = 0x00 // Consumer entry inactive
	HeartbeatUnknown      = 0x01 // Consumer enabled, but no heartbeat received yet
	HeartbeatActive       = 0x02 // Heartbeat received within set time
	HeartbeatTimeout      = 0x03 // No heartbeat received for set time
	ServiceId             = 0x700
)

const (
	EventStarted = 0x01
	EventTimeout = 0x02
	EventChanged = 0x03
	EventBoot    = 0x04
)

// Node specific hearbeat consumer part
type hbConsumerEntry struct {
	mu           sync.Mutex
	nodeId       uint8
	cobId        uint16
	nmtState     uint8
	nmtStatePrev uint8
	hbState      uint8
	timer        *time.Timer
	timeUs       uint32
	rxCancel     func()
	parent       *HBConsumer
	index        int
}

// Hearbeat consumer object for monitoring node hearbeats
type HBConsumer struct {
	bm                        *canopen.BusManager
	logger                    *slog.Logger
	mu                        sync.Mutex
	emcy                      *emergency.EMCY
	entries                   []*hbConsumerEntry
	allMonitoredActive        bool
	allMonitoredOperational   bool
	nmtIsPreOrOperationalPrev bool
	eventCallback             HBEventCallback
	isOperational             bool
}

type HBEventCallback func(event uint8, index uint8, nodeId uint8, nmtState uint8)

// Handle [HBConsumer] related RX CAN frames
func (entry *hbConsumerEntry) Handle(frame canopen.Frame) {
	entry.mu.Lock()

	if frame.DLC != 1 {
		entry.mu.Unlock()
		return
	}
	consumer := entry.parent
	entry.nmtState = frame.Data[0]

	var eventType uint8
	var eventState uint8

	if entry.nmtState == nmt.StateInitializing {
		// Boot up message is an error if previously received (means reboot)
		if entry.hbState == HeartbeatActive {
			consumer.emcy.ErrorReport(emergency.EmHBConsumerRemoteReset, emergency.ErrHeartbeat, uint32(entry.index))
		}
		// Signal reboot
		eventType = EventBoot
		eventState = nmt.StateInitializing
		entry.hbState = HeartbeatUnknown
	} else {
		// Signal Boot-up
		if entry.hbState != HeartbeatActive {
			eventType = EventStarted
			eventState = nmt.StateInitializing
		}
		// Heartbeat message
		entry.hbState = HeartbeatActive
	}

	// Reset timer
	if entry.timer != nil {
		entry.timer.Reset(time.Duration(entry.timeUs) * time.Microsecond)
	} else {
		entry.timer = time.AfterFunc(time.Duration(entry.timeUs)*time.Microsecond, entry.timerHandler)
	}

	nmtChanged := entry.nmtState != entry.nmtStatePrev
	currentNmtState := entry.nmtState

	entry.mu.Unlock()

	// Execute callbacks
	if eventType != 0 && consumer.eventCallback != nil {
		consumer.eventCallback(
			eventType,
			entry.nodeId,
			uint8(entry.index+1),
			eventState,
		)
	}

	if nmtChanged {
		if consumer.eventCallback != nil {
			consumer.eventCallback(
				EventChanged,
				entry.nodeId,
				uint8(entry.index+1),
				currentNmtState,
			)
		}
		entry.mu.Lock()
		entry.nmtStatePrev = currentNmtState
		entry.mu.Unlock()
	}

	consumer.checkAllMonitored()
}

func (entry *hbConsumerEntry) timerHandler() {
	entry.mu.Lock()
	consumer := entry.parent

	var eventType uint8

	// Check timeout
	if entry.hbState == HeartbeatActive {
		// Timeout is expired
		consumer.emcy.ErrorReport(emergency.EmHBConsumerRemoteReset, emergency.ErrHeartbeat, uint32(entry.index))
		entry.nmtState = nmt.StateUnknown
		entry.hbState = HeartbeatTimeout
		eventType = EventTimeout
	}
	entry.mu.Unlock()

	if eventType != 0 && consumer.eventCallback != nil {
		consumer.eventCallback(
			EventTimeout,
			entry.nodeId,
			uint8(entry.index+1),
			nmt.StateUnknown,
		)
	}
	consumer.checkAllMonitored()
}

// Update a heartbeat consumer entry to monitor a new node id & with expected period
func (entry *hbConsumerEntry) update(nodeId uint8, consumerTimeMs uint16) {
	entry.nodeId = nodeId
	entry.timeUs = uint32(consumerTimeMs) * 1000
	entry.nmtState = nmt.StateUnknown
	entry.nmtStatePrev = nmt.StateUnknown

	if entry.nodeId != 0 && entry.timeUs != 0 {
		entry.cobId = uint16(entry.nodeId) + ServiceId
		entry.hbState = HeartbeatUnknown
	} else {
		entry.cobId = 0
		entry.timeUs = 0
		entry.hbState = HeartbeatUnconfigured
	}
}

func (consumer *HBConsumer) checkAllMonitored() {
	consumer.mu.Lock()
	defer consumer.mu.Unlock()

	allMonitoredActiveCurrent := true
	allMonitoredOperationalCurrent := true

	for i := range consumer.entries {
		monitoredNode := consumer.entries[i]
		monitoredNode.mu.Lock()

		if monitoredNode.hbState == HeartbeatUnconfigured {
			monitoredNode.mu.Unlock()
			continue
		}

		if monitoredNode.hbState != HeartbeatActive {
			allMonitoredActiveCurrent = false
		}
		if monitoredNode.nmtState != nmt.StateOperational {
			allMonitoredOperationalCurrent = false
		}
		monitoredNode.mu.Unlock()
	}

	// Clear emergencies when all monitored nodes become active
	if !consumer.allMonitoredActive && allMonitoredActiveCurrent {
		consumer.emcy.ErrorReset(emergency.EmHeartbeatConsumer, 0)
		consumer.emcy.ErrorReset(emergency.EmHBConsumerRemoteReset, 0)
	}
	consumer.allMonitoredActive = allMonitoredActiveCurrent
	consumer.allMonitoredOperational = allMonitoredOperationalCurrent
}

// Add a consumer node, index is 0-based
func (consumer *HBConsumer) updateConsumerEntry(index uint8, nodeId uint8, consumerTimeMs uint16) error {
	if int(index) >= len(consumer.entries) {
		return canopen.ErrIllegalArgument
	}
	// Check duplicate entries : monitor node id more than once
	if consumerTimeMs != 0 && nodeId != 0 {
		for i, entry := range consumer.entries {
			if int(index) != i && entry.timeUs != 0 && entry.nodeId == nodeId {
				return canopen.ErrIllegalArgument
			}
		}
	}
	// Update corresponding entry
	entry := consumer.entries[index]
	entry.mu.Lock()
	entry.update(nodeId, consumerTimeMs)
	entry.mu.Unlock()

	// Configure RX buffer for hearbeat reception, clear previous subscription if exists
	if entry.hbState != HeartbeatUnconfigured {
		if entry.rxCancel != nil {
			entry.rxCancel()
		}
		consumer.logger.Info("will monitor", "monitoredId", entry.nodeId, "timeoutMs", entry.timeUs/1000)
		rxCancel, err := consumer.bm.Subscribe(uint32(entry.cobId), 0x7FF, false, entry)
		entry.rxCancel = rxCancel
		return err
	}
	return nil
}

// Callback on event for heartbeat consumer
// Events can be : boot-up, timeout, nmt change, ...
func (consumer *HBConsumer) OnEvent(callback HBEventCallback) {
	consumer.mu.Lock()
	defer consumer.mu.Unlock()

	consumer.eventCallback = callback
}

// Start relevant timers
func (consumer *HBConsumer) Start() {
	consumer.mu.Lock()
	defer consumer.mu.Unlock()

	for _, entry := range consumer.entries {
		entry.mu.Lock()
		if entry.hbState != HeartbeatUnconfigured && entry.timer == nil {
			entry.timer = time.AfterFunc(time.Duration(entry.timeUs)*time.Microsecond, entry.timerHandler)
		}
		entry.mu.Unlock()
	}
}

// Stop any timers
func (consumer *HBConsumer) Stop() {
	consumer.mu.Lock()
	defer consumer.mu.Unlock()

	for _, entry := range consumer.entries {
		entry.mu.Lock()
		if entry.timer != nil {
			entry.timer.Stop()
			entry.timer = nil
		}
		// Reset states
		entry.nmtState = nmt.StateUnknown
		entry.nmtStatePrev = nmt.StateUnknown
		if entry.hbState != HeartbeatUnconfigured {
			entry.hbState = HeartbeatUnknown
		}
		entry.mu.Unlock()
	}
	consumer.allMonitoredActive = false
	consumer.allMonitoredOperational = false
}

func (consumer *HBConsumer) OnStateChange(state uint8) {
	isOperational := (state == nmt.StateOperational) || (state == nmt.StatePreOperational)

	consumer.mu.Lock()
	prevOperational := consumer.isOperational
	consumer.isOperational = isOperational
	consumer.mu.Unlock()

	if isOperational && !prevOperational {
		consumer.Start()
	} else if !isOperational && prevOperational {
		consumer.Stop()
	}
}

func NewHBConsumer(bm *canopen.BusManager, logger *slog.Logger, emcy *emergency.EMCY, entry1016 *od.Entry) (*HBConsumer, error) {

	if entry1016 == nil || bm == nil || emcy == nil {
		return nil, canopen.ErrIllegalArgument
	}

	if logger == nil {
		logger = slog.Default()
	}

	consumer := &HBConsumer{bm: bm, logger: logger.With("service", "[HB]"), emcy: emcy}

	// Get number of nodes to monitor and create a monitor for each node
	nbEntries := uint8(entry1016.SubCount() - 1)
	consumer.logger.Info("number of entries to monitor nodes", "nb", nbEntries)
	consumer.entries = make([]*hbConsumerEntry, nbEntries)
	for i := range consumer.entries {
		consumer.entries[i] = &hbConsumerEntry{parent: consumer, index: i}
	}

	// For each entry, get expected heartbeat period and node id to monitor
	for i := 0; i < int(nbEntries); i++ {
		hbConsValue, err := entry1016.Uint32(uint8(i) + 1)
		if err != nil {
			consumer.logger.Error("reading failed",
				"name", entry1016.Name,
				"index", fmt.Sprintf("x%x", entry1016.Index),
				"subindex", fmt.Sprintf("x%x", i+1),
				"error", err,
			)
			return nil, canopen.ErrOdParameters
		}
		nodeId := uint8(hbConsValue >> 16)
		time := uint16(hbConsValue & 0xFFFF)
		err = consumer.updateConsumerEntry(uint8(i), nodeId, time)
		if err != nil {
			return nil, err
		}
	}
	entry1016.AddExtension(consumer, od.ReadEntryDefault, writeEntry1016)
	return consumer, nil

}
