package time

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"sync"
	"time"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/od"
)

// time origin is 1st of jan 1984
var TimestampOrigin = time.Date(1984, time.January, 1, 0, 0, 0, 0, time.Local)

type TIME struct {
	bm            *canopen.BusManager
	logger        *slog.Logger
	mu            sync.Mutex
	isConsumer    bool
	isProducer    bool
	timeInternal  time.Time
	timeProducer  time.Duration
	timerProducer *time.Timer
	cobId         uint32
	isOperational bool
	rxCancel      func()
}

// Handle [TIME] related RX CAN frames
func (t *TIME) Handle(frame canopen.Frame) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if frame.DLC != 6 {
		return
	}

	if t.isConsumer {
		t.timeInternal = convertByteToTime(frame.Data)
		t.logger.Debug("setting internal time to", "internal", t.timeInternal.String())
	}
}

func (t *TIME) SetOperational(operational bool) {
	t.mu.Lock()
	t.isOperational = operational
	t.mu.Unlock()
	if operational {
		t.Start()
	} else {
		t.Stop()
	}
}

func (t *TIME) Start() {
	t.resetTimerProducer()
}

func (t *TIME) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.timerProducer != nil {
		t.timerProducer.Stop()
	}
}

func (t *TIME) resetTimerProducer() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.isProducer {
		return
	}

	if t.timerProducer == nil {
		t.timerProducer = time.AfterFunc(t.timeProducer, t.timerProducerHandler)
	} else {
		t.timerProducer.Reset(t.timeProducer)
	}
}

func (t *TIME) timerProducerHandler() {
	t.mu.Lock()
	frame := canopen.NewFrame(t.cobId, 0, 6)
	buff := convertTimeToByte(t.timeInternal)
	frame.Data = buff
	t.mu.Unlock()
	_ = t.bm.Send(frame)
	t.Start()
}

// Sets the internal time
func (t *TIME) SetInternalTime(internalTime time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.timeInternal = internalTime
	t.logger.Info("setting date", "internal time", t.timeInternal)
}

// Update the producer interval time
func (t *TIME) SetProducerInterval(interval time.Duration) {
	t.mu.Lock()
	t.timeProducer = interval
	t.mu.Unlock()
	t.Stop()
	t.Start()
}

// Get the internal time
func (t *TIME) InternalTime() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.timeInternal
}

// Check if time producer
func (t *TIME) Producer() bool {
	return t.isProducer
}

// Check if time consumer
func (t *TIME) Consumer() bool {
	return t.isConsumer
}

func NewTIME(
	bm *canopen.BusManager,
	logger *slog.Logger,
	entry1012 *od.Entry,
	producerInterval time.Duration,
) (*TIME, error) {
	if entry1012 == nil || bm == nil {
		return nil, canopen.ErrIllegalArgument
	}

	if logger == nil {
		logger = slog.Default()
	}

	t := &TIME{bm: bm, logger: logger.With("service", "[TIME]")}
	// Read param from OD
	cobId, err := entry1012.Uint32(0)
	if err != nil {
		t.logger.Error("reading cob id timestamp failed",
			"index", fmt.Sprintf("x%x", entry1012.Index),
			"subindex", "0x0",
			"error", err,
		)
		return nil, canopen.ErrOdParameters
	}
	entry1012.AddExtension(t, od.ReadEntryDefault, writeEntry1012)
	t.isConsumer = (cobId & 0x80000000) != 0
	t.isProducer = (cobId & 0x40000000) != 0
	t.cobId = cobId & 0x7FF
	if t.isConsumer {
		rxCancel, err := bm.Subscribe(t.cobId, 0x7FF, false, t)
		t.rxCancel = rxCancel
		if err != nil {
			return nil, canopen.ErrIllegalArgument
		}
	}
	t.timeProducer = producerInterval
	t.SetInternalTime(time.Now())
	t.logger.Info("initialized time object", "producer", t.isProducer, "consumer", t.isConsumer)
	if t.isProducer {
		t.Start()
		t.logger.Info("publish period", "period", producerInterval)
	}
	return t, err
}

// Convert from raw []byte to [time.Time]
func convertByteToTime(data [8]byte) time.Time {
	if len(data) < 6 {
		return time.Time{}
	}
	ms := int(binary.LittleEndian.Uint32(data[0:4]) & 0x0FFFFFFF)
	days := int(binary.LittleEndian.Uint16(data[4:6]))
	internalTime := TimestampOrigin.AddDate(0, 0, days)
	return internalTime.Add(time.Duration(ms) * time.Millisecond)
}

// Convert from [time.Time] to raw []byte
func convertTimeToByte(t time.Time) [8]byte {
	var data [8]byte
	// Get the total number of days since 1st of jan 1984
	days := uint16(t.Sub(TimestampOrigin).Hours() / 24)
	// Get number of milliseconds after midnight
	midnight := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	ms := t.Sub(midnight).Milliseconds()

	binary.LittleEndian.PutUint32(data[0:4], uint32(ms))
	binary.LittleEndian.PutUint16(data[4:6], uint16(days))
	return data
}
