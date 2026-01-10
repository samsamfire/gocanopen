package sync

import (
	"fmt"
	"log/slog"
	s "sync"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/emergency"
	"github.com/samsamfire/gocanopen/pkg/od"
)

const (
	EventNone         uint8 = 0 // No SYNC event in last cycle
	EventRxOrTx       uint8 = 1 // SYNC message was received or transmitted in last cycle
	EventPassedWindow uint8 = 2 // Time has just passed SYNC window in last cycle (0x1007)
)

type SYNC struct {
	bm                  *canopen.BusManager
	logger              *slog.Logger
	mu                  s.Mutex
	subMu               s.Mutex
	subscribers         []chan uint8
	emcy                *emergency.EMCY
	rxNew               bool
	receiveError        uint8
	rxToggle            bool
	timeoutError        uint8
	counterOverflow     uint8
	counter             uint8
	syncIsOutsideWindow bool
	timer               uint32
	commCyclePeriod     *od.Entry
	syncWindowLength    *od.Entry
	isProducer          bool
	cobId               uint32
	txBuffer            canopen.Frame
	rxCancel            func()
}

// Subscribe returns a channel that receives the sync counter
// on every valid SYNC message
func (sync *SYNC) Subscribe() chan uint8 {
	sync.subMu.Lock()
	defer sync.subMu.Unlock()
	ch := make(chan uint8, 1)
	sync.subscribers = append(sync.subscribers, ch)
	return ch
}

// UnsubscribeSync removes the subscriber channel and closes it
func (sync *SYNC) Unsubscribe(ch chan uint8) {
	sync.subMu.Lock()
	defer sync.subMu.Unlock()
	for i, sub := range sync.subscribers {
		if sub == ch {
			sync.subscribers = append(sync.subscribers[:i], sync.subscribers[i+1:]...)
			close(ch)
			return
		}
	}
}

func (sync *SYNC) notifySubscribers() {
	sync.subMu.Lock()
	defer sync.subMu.Unlock()
	for _, ch := range sync.subscribers {
		select {
		case ch <- sync.counter:
		default:
			// Channel full, drop event
		}
	}
}

// Handle [SYNC] related RX CAN frames
func (sync *SYNC) Handle(frame canopen.Frame) {
	sync.mu.Lock()
	defer sync.mu.Unlock()

	syncReceived := false
	if sync.counterOverflow == 0 {
		if frame.DLC == 0 {
			syncReceived = true
		} else {
			sync.receiveError = frame.DLC | 0x40
		}
	} else {
		if frame.DLC == 1 {
			sync.counter = frame.Data[0]
			syncReceived = true
		} else {
			sync.receiveError = frame.DLC | 0x80
		}
	}
	if syncReceived {
		sync.rxToggle = !sync.rxToggle
		sync.rxNew = true
		// Notify subscribers
		sync.notifySubscribers()
	}
}

// Process [SYNC] state machine and TX CAN frames
// It returns the according sync event
// This should be called periodically
func (sync *SYNC) Process(nmtIsPreOrOperational bool, timeDifferenceUs uint32) uint8 {
	sync.mu.Lock()
	defer sync.mu.Unlock()

	status := EventNone
	if !nmtIsPreOrOperational {
		sync.rxNew = false
		sync.receiveError = 0
		sync.counter = 0
		sync.timer = 0
		return EventNone
	}

	timerNew := sync.timer + timeDifferenceUs
	if timerNew > sync.timer {
		sync.timer = timerNew
	}
	if sync.rxNew {
		sync.timer = 0
		sync.rxNew = false
		status = EventRxOrTx
	}
	commCyclePeriod, err := sync.commCyclePeriod.Uint32(0)
	if err != nil {
		sync.logger.Warn("failed to read comm cycle period", "error", err)
	}
	if commCyclePeriod > 0 {
		if sync.isProducer {
			if sync.timer >= commCyclePeriod {
				status = EventRxOrTx
				sync.mu.Unlock()
				sync.send()
				sync.mu.Lock()
			}
		} else if sync.timeoutError == 1 {
			periodTimeout := commCyclePeriod + commCyclePeriod>>1
			if periodTimeout < commCyclePeriod {
				periodTimeout = 0xFFFFFFFF
			}
			if sync.timer > periodTimeout {
				sync.emcy.Error(true, emergency.EmSyncTimeOut, emergency.ErrCommunication, sync.timer)
				sync.logger.Warn("timeout error", "timer", sync.timer, "period", periodTimeout)
				sync.timeoutError = 2
			}
		}
	}
	synchronousWindowLength, err := sync.syncWindowLength.Uint32(0)
	if err != nil {
		sync.logger.Warn("failed to read sync window length", "error", err)
	}
	if synchronousWindowLength > 0 && sync.timer > synchronousWindowLength {
		if !sync.syncIsOutsideWindow {
			status = EventPassedWindow
		}
		sync.syncIsOutsideWindow = true
	} else {
		sync.syncIsOutsideWindow = false
	}

	// Check reception errors in handler
	if sync.receiveError != 0 {
		sync.emcy.Error(true, emergency.EmSyncLength, emergency.ErrSyncDataLength, sync.timer)
		sync.logger.Warn("reception error", "error", sync.receiveError, "timer", sync.timer)
		sync.receiveError = 0
	}
	if status == EventRxOrTx {
		if sync.timeoutError == 2 {
			sync.emcy.Error(false, emergency.EmSyncTimeOut, 0, 0)
			sync.logger.Warn("reset error")
		}
		sync.timeoutError = 1
	}
	return status
}

func (sync *SYNC) send() {
	sync.mu.Lock()

	sync.counter += 1
	if sync.counter > sync.counterOverflow {
		sync.counter = 1
	}
	sync.timer = 0
	sync.rxToggle = !sync.rxToggle
	sync.txBuffer.Data[0] = sync.counter
	sync.mu.Unlock()
	// When listening to own messages, this will trigger Handle to be called
	// So make sure sync is unlocked before sending
	_ = sync.bm.Send(sync.txBuffer)
}

func (sync *SYNC) Counter() uint8 {
	sync.mu.Lock()
	defer sync.mu.Unlock()

	return sync.counter
}

func (sync *SYNC) RxToggle() bool {
	sync.mu.Lock()
	defer sync.mu.Unlock()

	return sync.rxToggle
}

func (sync *SYNC) CounterOverflow() uint8 {
	sync.mu.Lock()
	defer sync.mu.Unlock()

	return sync.counterOverflow
}

func NewSYNC(
	bm *canopen.BusManager,
	logger *slog.Logger,
	emergency *emergency.EMCY,
	entry1005 *od.Entry,
	entry1006 *od.Entry,
	entry1007 *od.Entry,
	entry1019 *od.Entry,
) (*SYNC, error) {

	if logger == nil {
		logger = slog.Default()
	}

	sync := &SYNC{bm: bm, logger: logger.With("service", "[SYNC]")}
	if entry1005 == nil {
		return nil, canopen.ErrIllegalArgument
	}

	cobIdSync, err := entry1005.Uint32(0)
	if err != nil {
		sync.logger.Error("error reading COB-ID",
			"index", fmt.Sprintf("x%x", entry1005.Index),
			"name", entry1005.Name,
		)
		return nil, canopen.ErrOdParameters
	}
	entry1005.AddExtension(sync, od.ReadEntryDefault, writeEntry1005)

	if entry1006 == nil {
		sync.logger.Error("not found", "index", "x1006", "name", "COMM CYCLE PERIOD")
		return nil, canopen.ErrOdParameters
	} else if entry1007 == nil {
		sync.logger.Error("not found", "index", "x1007", "name", "SYNCHRONOUS WINDOW LENGTH not found")
		return nil, canopen.ErrOdParameters
	}

	entry1006.AddExtension(sync, od.ReadEntryDefault, writeEntry1006)
	commCyclePeriod, err := entry1006.Uint32(0)
	if err != nil {
		sync.logger.Error("read error", "index", "x1006", "name", entry1006.Name, "error", err)
		return nil, canopen.ErrOdParameters
	}
	sync.commCyclePeriod = entry1006
	sync.logger.Info("communication cycle period", "index", "x1006", "period", commCyclePeriod)

	entry1007.AddExtension(sync, od.ReadEntryDefault, writeEntry1007)
	syncWindowLength, err := entry1007.Uint32(0)
	if err != nil {
		sync.logger.Error("read error", "index", "x1007", "name", entry1007.Name, "error", err)
		return nil, canopen.ErrOdParameters
	}
	sync.syncWindowLength = entry1007
	sync.logger.Info("sync window length",
		"index", "x1007",
		"name", entry1007.Name,
		"window length", syncWindowLength,
	)

	// This one is not mandatory
	var syncCounterOverflow uint8 = 0
	if entry1019 != nil {
		syncCounterOverflow, err = entry1019.Uint8(0)
		if err != nil {
			sync.logger.Error("read error", "index", "x1019", "name", entry1019.Name)
			return nil, canopen.ErrOdParameters
		}
		if syncCounterOverflow == 1 {
			syncCounterOverflow = 2
		} else if syncCounterOverflow > 240 {
			syncCounterOverflow = 240
		}
		entry1019.AddExtension(sync, od.ReadEntryDefault, writeEntry1019)
		sync.logger.Info("sync counter overflow",
			"index", "x1019",
			"name", entry1019.Name,
			"counter overflow", syncCounterOverflow,
		)
	}
	sync.counterOverflow = syncCounterOverflow
	sync.emcy = emergency
	sync.isProducer = (cobIdSync & 0x40000000) != 0
	sync.cobId = cobIdSync & 0x7FF

	rxCancel, err := sync.bm.Subscribe(sync.cobId, 0x7FF, false, sync)
	sync.rxCancel = rxCancel
	if err != nil {
		return nil, err
	}
	var frameSize uint8 = 0
	if syncCounterOverflow != 0 {
		frameSize = 1
	}
	sync.txBuffer = canopen.NewFrame(sync.cobId, 0, frameSize)
	sync.logger.Info("initialization finished")
	return sync, nil
}
