package time

import (
	"encoding/binary"
	"time"

	canopen "github.com/samsamfire/gocanopen"
	can "github.com/samsamfire/gocanopen/pkg/can"
	"github.com/samsamfire/gocanopen/pkg/od"
	log "github.com/sirupsen/logrus"
)

type TIME struct {
	*canopen.BusManager
	rawTimestamp       [6]byte
	ms                 uint32 // Milliseconds after midnight
	days               uint16 // Days since 1st january 1984
	residualUs         uint16 // Residual Us calculated when processed
	IsConsumer         bool
	IsProducer         bool
	rxNew              bool
	producerIntervalMs uint32
	producerTimerMs    uint32
	cobId              uint32
}

func (t *TIME) Handle(frame can.Frame) {
	if len(frame.Data) != 6 {
		return
	}
	copy(t.rawTimestamp[:], frame.Data[:])
	t.rxNew = true

}

func (t *TIME) Process(nmtIsPreOrOperational bool, timeDifferenceUs uint32) (bool, error) {
	timestampReceived := false
	if nmtIsPreOrOperational && t.IsConsumer {
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
	if nmtIsPreOrOperational && t.IsProducer && t.producerIntervalMs > 0 {
		if t.producerTimerMs >= t.producerIntervalMs {
			t.producerTimerMs -= t.producerIntervalMs
			frame := can.NewFrame(t.cobId, 0, 6)
			binary.LittleEndian.PutUint32(frame.Data[0:4], t.ms)
			binary.LittleEndian.PutUint16(frame.Data[4:6], t.days)
			err = t.Send(frame)
		} else {
			t.producerTimerMs += ms
		}
	} else {
		t.producerTimerMs = t.producerIntervalMs
	}
	return timestampReceived, err
}

// Sets the internal time
func (t *TIME) SetInternalTime() {
	timeBegin := time.Date(1984, time.January, 1, 0, 0, 0, 0, time.Local)
	duration := time.Since(timeBegin)
	// Get the total number of days since 1st of jan 1984
	days := uint16(duration.Hours() / 24)
	// Get number of milliseconds after midnight
	midnight := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, time.Local)
	ms := time.Since(midnight).Milliseconds()
	t.residualUs = 0
	t.ms = uint32(ms)
	t.days = days
	log.Infof("[TIME] setting the date to %v", time.Now())
	log.Infof("[TIME] days since 01/01/1984 : %v | ms since 00:00 : %v", days, ms)
}

func NewTIME(bm *canopen.BusManager, entry1012 *od.Entry, producerIntervalMs uint32) (*TIME, error) {
	if entry1012 == nil || bm == nil {
		return nil, canopen.ErrIllegalArgument
	}
	t := &TIME{BusManager: bm}
	// Read param from OD
	cobIdTimestamp, err := entry1012.Uint32(0)
	if err != nil {
		log.Errorf("[TIME][%x|%x] reading cob id timestamp failed : %v", entry1012.Index, 0x0, err)
		return nil, canopen.ErrOdParameters
	}
	entry1012.AddExtension(t, od.ReadEntryDefault, writeEntry1012)
	cobId := cobIdTimestamp & 0x7FF
	t.IsConsumer = (cobIdTimestamp & 0x80000000) != 0
	t.IsProducer = (cobIdTimestamp & 0x40000000) != 0
	t.rxNew = false
	t.cobId = cobId
	if t.IsConsumer {
		err := bm.Subscribe(cobId, 0x7FF, false, t)
		if err != nil {
			return nil, canopen.ErrIllegalArgument
		}
	}
	t.SetInternalTime()
	t.producerIntervalMs = producerIntervalMs
	t.producerTimerMs = producerIntervalMs
	log.Infof("[TIME] initialized time object | producer : %v, consumer : %v", t.IsProducer, t.IsConsumer)
	if t.IsProducer {
		log.Infof("[TIME] publish period is %v ms", producerIntervalMs)
	}
	return t, err
}
