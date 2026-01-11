package sync

import (
	"fmt"
	"log/slog"
	s "sync"
	"time"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/emergency"
	"github.com/samsamfire/gocanopen/pkg/od"
)

type SYNC struct {
	bm               *canopen.BusManager
	mu               s.Mutex
	logger           *slog.Logger
	subMu            s.Mutex
	subscribers      []chan uint8
	emcy             *emergency.EMCY
	rxToggle         bool
	counterOverflow  uint8
	counter          uint8
	isProducer       bool
	cobId            uint32
	syncCyclePeriod  time.Duration
	syncWindowLength time.Duration // Unused
	timerProducer    *time.Timer
	timerConsumer    *time.Timer
	timeLastRxTx     time.Time
	inTimeout        bool
	isOperational    bool
	txBuffer         canopen.Frame
	rxCancel         func()
}

// Handle [SYNC] related RX CAN frames
func (sync *SYNC) Handle(frame canopen.Frame) {
	sync.mu.Lock()
	defer sync.mu.Unlock()

	if sync.counterOverflow == 0 {
		if frame.DLC != 0 {
			sync.processError(frame.DLC | 0x40)
			return
		}
	} else {
		if frame.DLC != 1 {
			sync.processError(frame.DLC | 0x80)
			return
		}
		sync.counter = frame.Data[0]
	}

	sync.timeLastRxTx = time.Now()
	sync.rxToggle = !sync.rxToggle
	sync.notifySubscribers()

	// Check if we have any "timeouts"
	if sync.inTimeout {
		sync.inTimeout = false
		sync.emcy.Error(false, emergency.EmSyncTimeOut, 0, 0)
		sync.logger.Warn("reset sync timeout error")
	}
	sync.mu.Unlock()
	sync.resetTimers()
	sync.mu.Lock()

}

func (sync *SYNC) SetOperational(operational bool) {
	sync.mu.Lock()
	sync.isOperational = operational
	if !operational {
		sync.inTimeout = false
		sync.counter = 0
	}
	sync.mu.Unlock()

	if operational {
		sync.Start()
	} else {
		sync.Stop()
	}
}

func (sync *SYNC) processError(error uint8) {
	if error != 0 {
		elapsed := time.Since(sync.timeLastRxTx)
		sync.emcy.Error(true, emergency.EmSyncLength, emergency.ErrSyncDataLength, uint32(elapsed))
		sync.logger.Warn("reception error", "error", error, "timer", elapsed)
	}
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

func (sync *SYNC) Stop() {
	sync.mu.Lock()
	defer sync.mu.Unlock()
	if sync.timerConsumer != nil {
		sync.timerConsumer.Stop()
	}
	if sync.timerProducer != nil {
		sync.timerProducer.Stop()
	}
}

func (sync *SYNC) Start() {
	sync.mu.Lock()
	defer sync.mu.Unlock()
	sync.resetTimers()
}

// Should be called only if mu is locked
func (sync *SYNC) resetTimers() {

	if sync.syncCyclePeriod == 0 {
		return
	}

	timerPeriod := sync.syncCyclePeriod

	if sync.isProducer {
		if sync.timerProducer == nil {
			sync.timerProducer = time.AfterFunc(timerPeriod, sync.timerProducerHandler)
		} else {
			sync.timerProducer.Reset(timerPeriod)
		}
	} else {
		// Allow a bit of slac for consumer
		timerPeriod = time.Duration(float64(timerPeriod) * 1.5)
		if sync.timerConsumer == nil {
			sync.timerConsumer = time.AfterFunc(timerPeriod, sync.timerConsumerHandler)
		} else {
			sync.timerConsumer.Reset(timerPeriod)
		}
	}
}

func (sync *SYNC) timerProducerHandler() {
	// Producer timer elapsed send sync
	sync.send()
	sync.mu.Lock()
	defer sync.mu.Unlock()
	sync.resetTimers()
}

func (sync *SYNC) timerConsumerHandler() {
	// This means that a timemout has occured
	sync.inTimeout = true
	sync.emcy.Error(true, emergency.EmSyncTimeOut, emergency.ErrCommunication, uint32(sync.syncCyclePeriod.Microseconds()))
	sync.logger.Warn("timeout error", "timeout", sync.syncCyclePeriod)
}

func (sync *SYNC) send() {
	sync.mu.Lock()

	sync.counter += 1
	if sync.counter > sync.counterOverflow {
		sync.counter = 1
	}
	sync.timeLastRxTx = time.Now()
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
	sync.syncCyclePeriod = time.Duration(commCyclePeriod) * time.Microsecond
	sync.logger.Info("communication cycle period", "index", "x1006", "period", commCyclePeriod)

	entry1007.AddExtension(sync, od.ReadEntryDefault, writeEntry1007)
	syncWindowLength, err := entry1007.Uint32(0)
	if err != nil {
		sync.logger.Error("read error", "index", "x1007", "name", entry1007.Name, "error", err)
		return nil, canopen.ErrOdParameters
	}
	sync.syncWindowLength = time.Duration(syncWindowLength) * time.Microsecond
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

	sync.resetTimers()
	sync.txBuffer = canopen.NewFrame(sync.cobId, 0, frameSize)
	sync.logger.Info("initialization finished")
	return sync, nil
}
