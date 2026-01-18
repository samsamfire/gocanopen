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
	bm             *canopen.BusManager
	mu             sync.Mutex
	logger         *slog.Logger
	emcy           *emergency.EMCY
	entries        []*hbConsumerEntry
	allActive      bool
	allOperational bool
	eventCallback  HBEventCallback
	isOperational  bool
}

type HBEventCallback func(event uint8, index uint8, nodeId uint8, nmtState uint8)

func (c *HBConsumer) checkAllMonitored() {
	c.mu.Lock()
	defer c.mu.Unlock()

	allActive, allOperational := true, true

	for _, node := range c.entries {
		node.mu.Lock()
		hState, nState := node.hbState, node.nmtState
		node.mu.Unlock()

		if hState == HeartbeatUnconfigured {
			continue
		}
		if hState != HeartbeatActive {
			allActive = false
		}
		if nState != nmt.StateOperational {
			allOperational = false
		}
	}

	if !c.allActive && allActive {
		c.emcy.ErrorReset(emergency.EmHeartbeatConsumer, 0)
		c.emcy.ErrorReset(emergency.EmHBConsumerRemoteReset, 0)
	}

	c.allActive = allActive
	c.allOperational = allOperational
}

// Add a consumer node, index is 0-based
func (c *HBConsumer) updateConsumerEntry(index uint8, nodeId uint8, period time.Duration) error {
	if int(index) >= len(c.entries) {
		return canopen.ErrIllegalArgument
	}
	// Check duplicate entries : monitor node id more than once
	if period != 0 && nodeId != 0 {
		for i, entry := range c.entries {
			if int(index) != i && entry.timeoutPeriod != 0 && entry.nodeId == nodeId {
				return canopen.ErrIllegalArgument
			}
		}
	}
	// Update corresponding entry
	entry := c.entries[index]
	entry.stop()
	return entry.start(nodeId, period)
}

// Callback on event for heartbeat consumer
// Events can be : boot-up, timeout, nmt change, ...
func (c *HBConsumer) OnEvent(callback HBEventCallback) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.eventCallback = callback
}

// Start relevant timers
func (c *HBConsumer) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, entry := range c.entries {
		entry.start(entry.nodeId, entry.timeoutPeriod)
	}
}

// Stop any timers
func (c *HBConsumer) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, entry := range c.entries {
		entry.stop()
	}
	c.allActive = false
	c.allOperational = false
}

func (c *HBConsumer) OnStateChange(state uint8) {
	isOperational := (state == nmt.StateOperational) || (state == nmt.StatePreOperational)

	c.mu.Lock()
	prevOperational := c.isOperational
	c.isOperational = isOperational
	c.mu.Unlock()

	if isOperational && !prevOperational {
		c.Start()
	} else if !isOperational && prevOperational {
		c.Stop()
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
		consumer.entries[i] = &hbConsumerEntry{
			parent:  consumer,
			logger:  logger,
			bm:      bm,
			odIndex: i,
		}
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
