package time

import (
	"encoding/binary"
	"sync"
	"time"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/od"
	log "github.com/sirupsen/logrus"
)

// time origin is 1st of jan 1984
var timestampOrigin = time.Date(1984, time.January, 1, 0, 0, 0, 0, time.Local)

type TIME struct {
	*canopen.BusManager
	mu                 sync.Mutex
	rawTimestamp       [6]byte
	ms                 uint32 // Milliseconds after midnight
	days               uint16 // Days since 1st january 1984
	residualUs         uint16 // Residual Us calculated when processed
	isConsumer         bool
	isProducer         bool
	rxNew              bool
	producerIntervalMs uint32
	producerTimerMs    uint32
	cobId              uint32
}

// Handle [TIME] related RX CAN frames
func (t *TIME) Handle(frame canopen.Frame) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if frame.DLC != 6 {
		return
	}
	copy(t.rawTimestamp[:], frame.Data[:])
	t.rxNew = true
}

// Process [TIME] state machine and TX CAN frames
// This returns whether timestamp has been received and if any error occured
// This should be called periodically
func (t *TIME) Process(nmtIsPreOrOperational bool, timeDifferenceUs uint32) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	timestampReceived := false
	if nmtIsPreOrOperational && t.isConsumer {
		if t.rxNew {
			t.ms = binary.LittleEndian.Uint32(t.rawTimestamp[0:4]) & 0x0FFFFFFF
			t.days = binary.LittleEndian.Uint16(t.rawTimestamp[4:6])
			t.residualUs = 0
			timestampReceived = true
			t.rxNew = false
		}
	} else {
		t.rxNew = false
	}
	ms := uint32(0)
	if !timestampReceived && (timeDifferenceUs > 0) {
		us := timeDifferenceUs + uint32(t.residualUs)
		ms = us / 1000
		t.residualUs = uint16(us % 1000)
		t.ms += ms
		if t.ms >= 1000*60*60*24 {
			t.ms -= 1000 * 60 * 60 * 24
			t.days += 1
		}
	}
	var err error
	if nmtIsPreOrOperational && t.isProducer && t.producerIntervalMs > 0 {
		if t.producerTimerMs < t.producerIntervalMs {
			t.producerTimerMs += ms
			return timestampReceived, nil
		}
		t.producerTimerMs -= t.producerIntervalMs
		frame := canopen.NewFrame(t.cobId, 0, 6)
		binary.LittleEndian.PutUint32(frame.Data[0:4], t.ms)
		binary.LittleEndian.PutUint16(frame.Data[4:6], t.days)
		return timestampReceived, t.Send(frame)

	}
	t.producerTimerMs = t.producerIntervalMs
	return timestampReceived, err
}

// Sets the internal time
func (t *TIME) SetInternalTime(internalTime time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	// Get the total number of days since 1st of jan 1984
	days := uint16(internalTime.Sub(timestampOrigin).Hours() / 24)
	// Get number of milliseconds after midnight
	midnight := time.Date(internalTime.Year(), internalTime.Month(), internalTime.Day(), 0, 0, 0, 0, time.Local)
	ms := internalTime.Sub(midnight).Milliseconds()
	t.residualUs = 0
	t.ms = uint32(ms)
	t.days = days
	log.Infof("[TIME] setting the date to %v", internalTime)
	log.Infof("[TIME] days since 01/01/1984 : %v | ms since 00:00 : %v", days, ms)
}

// Update the producer interval time in milliseconds
func (t *TIME) SetProducerIntervalMs(producerIntervalMs uint32) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.producerIntervalMs = producerIntervalMs
	t.producerTimerMs = producerIntervalMs
}

// Get the internal time
func (t *TIME) InternalTime() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	internalTime := timestampOrigin.AddDate(0, 0, int(t.days))
	return internalTime.Add(time.Duration(t.ms)*time.Millisecond + time.Duration(t.residualUs)*time.Microsecond)
}

// Check if time producer
func (t *TIME) Producer() bool {
	return t.isProducer
}

// Check if time consumer
func (t *TIME) Consumer() bool {
	return t.isConsumer
}

func NewTIME(bm *canopen.BusManager, entry1012 *od.Entry, producerIntervalMs uint32) (*TIME, error) {
	if entry1012 == nil || bm == nil {
		return nil, canopen.ErrIllegalArgument
	}
	t := &TIME{BusManager: bm}
	// Read param from OD
	cobId, err := entry1012.Uint32(0)
	if err != nil {
		log.Errorf("[TIME][%x|%x] reading cob id timestamp failed : %v", entry1012.Index, 0x0, err)
		return nil, canopen.ErrOdParameters
	}
	entry1012.AddExtension(t, od.ReadEntryDefault, writeEntry1012)
	t.isConsumer = (cobId & 0x80000000) != 0
	t.isProducer = (cobId & 0x40000000) != 0
	t.cobId = cobId & 0x7FF
	if t.isConsumer {
		err := bm.Subscribe(t.cobId, 0x7FF, false, t)
		if err != nil {
			return nil, canopen.ErrIllegalArgument
		}
	}
	t.SetInternalTime(time.Now())
	t.producerIntervalMs = producerIntervalMs
	t.producerTimerMs = producerIntervalMs
	log.Infof("[TIME] initialized time object | producer : %v, consumer : %v", t.isProducer, t.isConsumer)
	if t.isProducer {
		log.Infof("[TIME] publish period is %v ms", producerIntervalMs)
	}
	return t, err
}
