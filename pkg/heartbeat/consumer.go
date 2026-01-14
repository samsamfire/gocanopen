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
	EventNone = uint8(iota)
	EventStarted
	EventTimeout
	EventChanged
	EventBoot
)

// Hearbeat consumer object for monitoring node hearbeats
// Composed of multiple sub [hbConsumerEntry] entries
type HBConsumer struct {
	bm                      *canopen.BusManager
	mu                      sync.Mutex
	logger                  *slog.Logger
	emcy                    *emergency.EMCY
	entries                 []*hbConsumerEntry
	allMonitoredActive      bool
	allMonitoredOperational bool
	eventCallback           HBEventCallback
	isOperational           bool
}

type HBEventCallback func(event uint8, index uint8, nodeId uint8, nmtState uint8)

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
func (consumer *HBConsumer) updateConsumerEntry(index uint8, nodeId uint8, period time.Duration) error {
	if int(index) >= len(consumer.entries) {
		return canopen.ErrIllegalArgument
	}
	// Check duplicate entries : monitor node id more than once
	if period != 0 && nodeId != 0 {
		for i, entry := range consumer.entries {
			if int(index) != i && entry.timeoutPeriod != 0 && entry.nodeId == nodeId {
				return canopen.ErrIllegalArgument
			}
		}
	}
	// Update corresponding entry
	entry := consumer.entries[index]
	entry.mu.Lock()
	entry.update(nodeId, period)
	entry.mu.Unlock()

	// Configure RX buffer for hearbeat reception, clear previous subscription if exists
	if entry.hbState != HeartbeatUnconfigured {
		if entry.rxCancel != nil {
			entry.rxCancel()
		}
		consumer.logger.Info("will monitor", "monitoredId", entry.nodeId, "timeout", period)
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
		if entry.hbState != HeartbeatUnconfigured {
			entry.restartTimeoutTimer()
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
		consumer.entries[i] = &hbConsumerEntry{parent: consumer, odIndex: i}
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
		period := uint16(hbConsValue & 0xFFFF)
		err = consumer.updateConsumerEntry(uint8(i), nodeId, time.Duration(period)*time.Millisecond)
		if err != nil {
			return nil, err
		}
	}
	entry1016.AddExtension(consumer, od.ReadEntryDefault, writeEntry1016)
	return consumer, nil

}
