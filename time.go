package canopen

import (
	"encoding/binary"
	"time"

	log "github.com/sirupsen/logrus"
)

type TIME struct {
	*busManager
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

func (time *TIME) Handle(frame Frame) {
	if len(frame.Data) != 6 {
		return
	}
	copy(time.rawTimestamp[:], frame.Data[:])
	time.rxNew = true

}

func (time *TIME) process(nmtIsPreOrOperational bool, timeDifferenceUs uint32) bool {
	timestampReceived := false
	if nmtIsPreOrOperational && time.isConsumer {
		if time.rxNew {
			time.ms = binary.LittleEndian.Uint32(time.rawTimestamp[0:4]) & 0x0FFFFFFF
			time.days = binary.LittleEndian.Uint16(time.rawTimestamp[4:6])
			time.residualUs = 0
			timestampReceived = true
			time.rxNew = false
		}
	} else {
		time.rxNew = false
	}
	ms := uint32(0)
	if !timestampReceived && (timeDifferenceUs > 0) {
		us := timeDifferenceUs + uint32(time.residualUs)
		ms = us / 1000
		time.residualUs = uint16(us % 1000)
		time.ms += ms
		if time.ms >= 1000*60*60*24 {
			time.ms -= 1000 * 60 * 60 * 24
			time.days += 1
		}
	}
	if nmtIsPreOrOperational && time.isProducer && time.producerIntervalMs > 0 {
		if time.producerTimerMs >= time.producerIntervalMs {
			time.producerTimerMs -= time.producerIntervalMs
			frame := NewFrame(time.cobId, 0, 6)
			binary.LittleEndian.PutUint32(frame.Data[0:4], time.ms)
			binary.LittleEndian.PutUint16(frame.Data[4:6], time.days)
			time.Send(frame)
		} else {
			time.producerTimerMs += ms
		}
	} else {
		time.producerTimerMs = time.producerIntervalMs
	}

	return timestampReceived
}

// Sets the internal time
func (time_obj *TIME) SetInternalTime() {
	timeBegin := time.Date(1984, time.January, 1, 0, 0, 0, 0, time.Local)
	duration := time.Since(timeBegin)
	// Get the total number of days since 1st of jan 1984
	days := uint16(duration.Hours() / 24)
	// Get number of milliseconds after midnight
	midnight := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, time.Local)
	ms := time.Since(midnight).Milliseconds()
	time_obj.residualUs = 0
	time_obj.ms = uint32(ms)
	time_obj.days = days
	log.Infof("[TIME] setting the date to %v", time.Now())
	log.Infof("[TIME] days since 01/01/1984 : %v | ms since 00:00 : %v", days, ms)
}

func NewTIME(bm *busManager, entry1012 *Entry, producerIntervalMs uint32) (*TIME, error) {
	if entry1012 == nil || bm == nil {
		return nil, ErrIllegalArgument
	}
	time := &TIME{busManager: bm}
	// Read param from OD
	cobIdTimestamp, err := entry1012.Uint32(0)
	if err != nil {
		log.Errorf("[TIME][%x|%x] reading cob id timestamp failed : %v", entry1012.Index, 0x0, err)
		return nil, ErrOdParameters
	}
	entry1012.AddExtension(time, ReadEntryDefault, writeEntry1012)
	cobId := cobIdTimestamp & 0x7FF
	time.isConsumer = (cobIdTimestamp & 0x80000000) != 0
	time.isProducer = (cobIdTimestamp & 0x40000000) != 0
	time.rxNew = false
	time.cobId = cobId
	if time.isConsumer {
		err := bm.Subscribe(cobId, 0x7FF, false, time)
		if err != nil {
			return nil, ErrIllegalArgument
		}
	}
	time.SetInternalTime()
	time.producerIntervalMs = producerIntervalMs
	time.producerTimerMs = producerIntervalMs
	log.Infof("[TIME] initialized time object | producer : %v, consumer : %v", time.isProducer, time.isConsumer)
	if time.isProducer {
		log.Infof("[TIME] publish period is %v ms", producerIntervalMs)
	}
	return time, err
}
