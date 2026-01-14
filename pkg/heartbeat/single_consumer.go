package heartbeat

import (
	"sync"
	"time"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/emergency"
	"github.com/samsamfire/gocanopen/pkg/nmt"
)

// Node specific hearbeat consumer part
type hbConsumerEntry struct {
	mu           sync.Mutex
	nodeId       uint8
	cobId        uint16
	nmtState     uint8
	nmtStatePrev uint8
	hbState      uint8
	period       time.Duration
	timer        *time.Timer
	rxCancel     func()
	parent       *HBConsumer
	index        int
}

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
		entry.timer.Reset(entry.period)
	} else {
		entry.timer = time.AfterFunc(entry.period, entry.timerHandler)
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
func (entry *hbConsumerEntry) update(nodeId uint8, period time.Duration) {
	entry.nodeId = nodeId
	entry.period = period
	entry.nmtState = nmt.StateUnknown
	entry.nmtStatePrev = nmt.StateUnknown

	if entry.nodeId != 0 && entry.period != 0 {
		entry.cobId = uint16(entry.nodeId) + ServiceId
		entry.hbState = HeartbeatUnknown
	} else {
		entry.cobId = 0
		entry.period = 0
		entry.hbState = HeartbeatUnconfigured
	}
}
