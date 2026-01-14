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
	mu            sync.Mutex
	nodeId        uint8
	cobId         uint16
	nmtState      uint8
	nmtStatePrev  uint8
	hbState       uint8
	timeoutPeriod time.Duration
	timer         *time.Timer
	rxCancel      func()
	parent        *HBConsumer
	odIndex       int
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
	event := EventNone

	if entry.nmtState == nmt.StateInitializing {
		// Boot up message is an error if previously received (means reboot)
		if entry.hbState == HeartbeatActive {
			consumer.emcy.ErrorReport(emergency.EmHBConsumerRemoteReset, emergency.ErrHeartbeat, uint32(entry.odIndex))
		}
		// Signal reboot
		event = EventBoot
		entry.hbState = HeartbeatUnknown
	} else {
		// Signal Boot-up
		if entry.hbState != HeartbeatActive {
			event = EventStarted
		}
		// Heartbeat message
		entry.hbState = HeartbeatActive
	}

	// Reset timer
	entry.mu.Unlock()
	entry.restartTimeoutTimer()
	entry.mu.Lock()

	// Execute callbacks
	if event != EventNone && consumer.eventCallback != nil {
		consumer.eventCallback(
			event,
			entry.nodeId,
			uint8(entry.odIndex+1),
			nmt.StateInitializing,
		)
	}

	nmtChanged := entry.nmtState != entry.nmtStatePrev

	if nmtChanged && consumer.eventCallback != nil {
		consumer.eventCallback(
			EventChanged,
			entry.nodeId,
			uint8(entry.odIndex+1),
			entry.nmtState,
		)
	}
	entry.nmtStatePrev = entry.nmtState
	entry.mu.Unlock()

	consumer.checkAllMonitored()
}

func (entry *hbConsumerEntry) restartTimeoutTimer() {
	entry.mu.Lock()
	defer entry.mu.Unlock()

	if entry.timeoutPeriod == 0 {
		return
	}
	if entry.timer == nil {
		entry.timer = time.AfterFunc(entry.timeoutPeriod, entry.timeoutHandler)
	} else {
		entry.timer.Reset(entry.timeoutPeriod)
	}
}

func (entry *hbConsumerEntry) timeoutHandler() {
	entry.mu.Lock()
	parent := entry.parent

	var eventType uint8

	// Check timeout
	if entry.hbState == HeartbeatActive {
		// Timeout is expired
		parent.emcy.ErrorReport(emergency.EmHBConsumerRemoteReset, emergency.ErrHeartbeat, uint32(entry.odIndex))
		entry.nmtState = nmt.StateUnknown
		entry.hbState = HeartbeatTimeout
		eventType = EventTimeout
	}
	entry.mu.Unlock()

	if eventType != 0 && parent.eventCallback != nil {
		parent.eventCallback(
			EventTimeout,
			entry.nodeId,
			uint8(entry.odIndex+1),
			nmt.StateUnknown,
		)
	}
	parent.checkAllMonitored()
}

// Update a heartbeat consumer entry to monitor a new node id & with expected period
func (entry *hbConsumerEntry) update(nodeId uint8, period time.Duration) {
	entry.nodeId = nodeId
	entry.timeoutPeriod = period
	entry.nmtState = nmt.StateUnknown
	entry.nmtStatePrev = nmt.StateUnknown

	if entry.nodeId != 0 && entry.timeoutPeriod != 0 {
		entry.cobId = uint16(entry.nodeId) + ServiceId
		entry.hbState = HeartbeatUnknown
	} else {
		entry.cobId = 0
		entry.timeoutPeriod = 0
		entry.hbState = HeartbeatUnconfigured
	}
}
