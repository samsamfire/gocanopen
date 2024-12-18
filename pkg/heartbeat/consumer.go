package heartbeat

import (
	"fmt"
	"log/slog"
	"sync"

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
	timeoutTimer uint32
	timeUs       uint32
	rxNew        bool
}

// Hearbeat consumer object for monitoring node hearbeats
type HBConsumer struct {
	*canopen.BusManager
	logger                    *slog.Logger
	mu                        sync.Mutex
	emcy                      *emergency.EMCY
	entries                   []*hbConsumerEntry
	allMonitoredActive        bool
	allMonitoredOperational   bool
	nmtIsPreOrOperationalPrev bool
	eventCallback             HBEventCallback
}

type HBEventCallback func(event uint8, index uint8, nodeId uint8, nmtState uint8)

// Handle [HBConsumer] related RX CAN frames
func (entry *hbConsumerEntry) Handle(frame canopen.Frame) {
	entry.mu.Lock()
	defer entry.mu.Unlock()

	if frame.DLC != 1 {
		return
	}
	entry.nmtState = frame.Data[0]
	entry.rxNew = true
}

// Update a heartbeat consumer entry to monitor a new node id & with expected period
func (entry *hbConsumerEntry) update(nodeId uint8, consumerTimeMs uint16) {
	entry.nodeId = nodeId
	entry.timeUs = uint32(consumerTimeMs) * 1000
	entry.nmtState = nmt.StateUnknown
	entry.nmtStatePrev = nmt.StateUnknown
	entry.rxNew = false

	if entry.nodeId != 0 && entry.timeUs != 0 {
		entry.cobId = uint16(entry.nodeId) + ServiceId
		entry.hbState = HeartbeatUnknown
	} else {
		entry.cobId = 0
		entry.timeUs = 0
		entry.hbState = HeartbeatUnconfigured
	}
}

// Process [HBConsumer] state machine and TX CAN frames
// This should be called periodically
func (consumer *HBConsumer) Process(nmtIsPreOrOperational bool, timeDifferenceUs uint32, timerNextUs *uint32) {
	consumer.mu.Lock()
	defer consumer.mu.Unlock()

	allMonitoredActiveCurrent := true
	allMonitoredOperationalCurrent := true
	if nmtIsPreOrOperational && consumer.nmtIsPreOrOperationalPrev {
		for i := range consumer.entries {
			monitoredNode := consumer.entries[i]
			monitoredNode.mu.Lock()

			timeDifferenceUsCopy := timeDifferenceUs
			// If unconfigured skip to next iteration
			if monitoredNode.hbState == HeartbeatUnconfigured {
				monitoredNode.mu.Unlock()
				continue
			}
			if monitoredNode.rxNew {
				if monitoredNode.nmtState == nmt.StateInitializing {
					// Boot up message is an error if previously received (means reboot)
					if monitoredNode.hbState == HeartbeatActive {
						consumer.emcy.ErrorReport(emergency.EmHBConsumerRemoteReset, emergency.ErrHeartbeat, uint32(i))
					}
					// Signal reboot
					consumer.mu.Unlock()
					if consumer.eventCallback != nil {
						consumer.eventCallback(
							EventBoot,
							monitoredNode.nodeId,
							uint8(i+1),
							nmt.StateInitializing,
						)
					}
					consumer.mu.Lock()
					monitoredNode.hbState = HeartbeatUnknown
				} else {
					// Signal Boot-up
					consumer.mu.Unlock()
					if monitoredNode.hbState != HeartbeatActive && consumer.eventCallback != nil {
						consumer.eventCallback(
							EventStarted,
							monitoredNode.nodeId,
							uint8(i+1),
							nmt.StateInitializing,
						)
					}
					consumer.mu.Lock()
					// Heartbeat message
					monitoredNode.hbState = HeartbeatActive
					monitoredNode.timeoutTimer = 0
					timeDifferenceUsCopy = 0
				}
				monitoredNode.rxNew = false
			}
			// Check timeout
			if monitoredNode.hbState == HeartbeatActive {
				monitoredNode.timeoutTimer += timeDifferenceUsCopy
				if monitoredNode.timeoutTimer >= monitoredNode.timeUs {
					// Timeout is expired
					consumer.emcy.ErrorReport(emergency.EmHBConsumerRemoteReset, emergency.ErrHeartbeat, uint32(i))
					monitoredNode.nmtState = nmt.StateUnknown
					monitoredNode.hbState = HeartbeatTimeout
					// Signal timeout
					consumer.mu.Unlock()
					if consumer.eventCallback != nil {
						consumer.eventCallback(
							EventTimeout,
							monitoredNode.nodeId,
							uint8(i+1),
							nmt.StateUnknown,
						)
					}
					consumer.mu.Lock()
				} else if timerNextUs != nil {
					// Calculate when to recheck
					diff := monitoredNode.timeUs - monitoredNode.timeoutTimer
					if *timerNextUs > diff {
						*timerNextUs = diff
					}
				}
			}

			if monitoredNode.hbState != HeartbeatActive {
				allMonitoredActiveCurrent = false
			}
			if monitoredNode.nmtState != nmt.StateOperational {
				allMonitoredOperationalCurrent = false
			}

			if monitoredNode.nmtState != monitoredNode.nmtStatePrev {
				// Signal NMT change
				consumer.mu.Unlock()
				if consumer.eventCallback != nil {
					consumer.eventCallback(
						EventChanged,
						monitoredNode.nodeId,
						uint8(i+1),
						monitoredNode.nmtState,
					)
				}
				consumer.mu.Lock()
				monitoredNode.nmtStatePrev = monitoredNode.nmtState
			}
			monitoredNode.mu.Unlock()
		}
	} else if nmtIsPreOrOperational || consumer.nmtIsPreOrOperationalPrev {
		// pre or operational state changed, clear vars
		for i := range consumer.entries {
			monitoredNode := consumer.entries[i]
			monitoredNode.mu.Lock()

			monitoredNode.nmtState = nmt.StateUnknown
			monitoredNode.nmtStatePrev = nmt.StateUnknown
			monitoredNode.rxNew = false
			if monitoredNode.hbState != HeartbeatUnconfigured {
				monitoredNode.hbState = HeartbeatUnknown
			}
			monitoredNode.mu.Unlock()
		}
		allMonitoredActiveCurrent = false
		allMonitoredOperationalCurrent = false
	}

	// Clear emergencies when all monitored nodes become active
	if !consumer.allMonitoredActive && allMonitoredActiveCurrent {
		consumer.emcy.ErrorReset(emergency.EmHeartbeatConsumer, 0)
		consumer.emcy.ErrorReset(emergency.EmHBConsumerRemoteReset, 0)
	}
	consumer.allMonitoredActive = allMonitoredActiveCurrent
	consumer.allMonitoredOperational = allMonitoredOperationalCurrent
	consumer.nmtIsPreOrOperationalPrev = nmtIsPreOrOperational
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
	entry.update(nodeId, consumerTimeMs)

	// Configure RX buffer for hearbeat reception
	if entry.hbState != HeartbeatUnconfigured {
		consumer.logger.Info("will monitor", "monitoredId", entry.nodeId, "timeoutMs", entry.timeUs/1000)
		return consumer.Subscribe(uint32(entry.cobId), 0x7FF, false, entry)
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

func NewHBConsumer(bm *canopen.BusManager, logger *slog.Logger, emcy *emergency.EMCY, entry1016 *od.Entry) (*HBConsumer, error) {

	if entry1016 == nil || bm == nil || emcy == nil {
		return nil, canopen.ErrIllegalArgument
	}

	if logger == nil {
		logger = slog.Default()
	}

	consumer := &HBConsumer{BusManager: bm, logger: logger.With("service", "[HB]"), emcy: emcy}

	// Get number of nodes to monitor and create a monitor for each node
	nbEntries := uint8(entry1016.SubCount() - 1)
	consumer.logger.Info("number of entries to monitor nodes", "nb", nbEntries)
	consumer.entries = make([]*hbConsumerEntry, nbEntries)
	for i := range consumer.entries {
		consumer.entries[i] = &hbConsumerEntry{}
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
