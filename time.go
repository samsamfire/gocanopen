package canopen

import (
	"encoding/binary"
	"time"

	log "github.com/sirupsen/logrus"
)

type TIME struct {
	RawTimestamp       [6]byte
	Ms                 uint32 // Milliseconds after midnight
	Days               uint16 // Days since 1st january 1984
	ResidualUs         uint16 // Residual Us calculated when processed
	IsConsumer         bool
	IsProducer         bool
	RxNew              bool
	ProducerIntervalMs uint32
	ProducerTimerMs    uint32
	busManager         *BusManager
	TxBuffer           *BufferTxFrame
	ExtensionEntry1012 Extension
}

func (time *TIME) Handle(frame Frame) {
	if len(frame.Data) != 6 {
		return
	}
	copy(time.RawTimestamp[:], frame.Data[:])
	time.RxNew = true

}

func (time *TIME) Init(entry1012 *Entry, busManager *BusManager, producerIntervalMs uint32) error {
	if time == nil || entry1012 == nil || busManager == nil {
		return CO_ERROR_ILLEGAL_ARGUMENT
	}
	// Read param from OD
	cobIdTimestamp := uint32(0)
	ret := entry1012.GetUint32(0, &cobIdTimestamp)
	if ret != nil {
		log.Errorf("[TIME][%x|%x] reading cob id timestamp failed : %v", entry1012.Index, 0x0, ret)
		return CO_ERROR_OD_PARAMETERS
	}
	time.ExtensionEntry1012.Object = time
	time.ExtensionEntry1012.Read = ReadEntryOriginal
	time.ExtensionEntry1012.Write = WriteEntry1012
	entry1012.AddExtension(&time.ExtensionEntry1012)

	cobId := cobIdTimestamp & 0x7FF
	time.IsConsumer = (cobIdTimestamp & 0x80000000) != 0
	time.IsProducer = (cobIdTimestamp & 0x40000000) != 0
	time.RxNew = false
	var err error
	if time.IsConsumer {
		err = busManager.InsertRxBuffer(
			cobId,
			0x7FF,
			false,
			time,
		)
		if err != nil {
			return CO_ERROR_ILLEGAL_ARGUMENT
		}
	}
	time.busManager = busManager
	time.TxBuffer, _, err = busManager.InsertTxBuffer(
		cobId,
		false,
		6,
		false,
	)
	if time.TxBuffer == nil || err != nil {
		return CO_ERROR_ILLEGAL_ARGUMENT
	}
	time.SetInternalTime()
	time.ProducerIntervalMs = producerIntervalMs
	time.ProducerTimerMs = producerIntervalMs
	log.Infof("[TIME] initialized time object | producer : %v, consumer : %v", time.IsProducer, time.IsConsumer)
	if time.IsProducer {
		log.Infof("[TIME] publish period is %v ms", producerIntervalMs)
	}
	return nil
}

func (time *TIME) Process(nmtIsPreOrOperational bool, timeDifferenceUs uint32) bool {
	timestampReceived := false
	if nmtIsPreOrOperational && time.IsConsumer {
		if time.RxNew {
			time.Ms = binary.LittleEndian.Uint32(time.RawTimestamp[0:4]) & 0x0FFFFFFF
			time.Days = binary.LittleEndian.Uint16(time.RawTimestamp[4:6])
			time.ResidualUs = 0
			timestampReceived = true
			time.RxNew = false
		}
	} else {
		time.RxNew = false
	}
	ms := uint32(0)
	if !timestampReceived && (timeDifferenceUs > 0) {
		us := timeDifferenceUs + uint32(time.ResidualUs)
		ms = us / 1000
		time.ResidualUs = uint16(us % 1000)
		time.Ms += ms
		if time.Ms >= 1000*60*60*24 {
			time.Ms -= 1000 * 60 * 60 * 24
			time.Days += 1
		}
	}
	if nmtIsPreOrOperational && time.IsProducer && time.ProducerIntervalMs > 0 {
		if time.ProducerTimerMs >= time.ProducerIntervalMs {
			time.ProducerTimerMs -= time.ProducerIntervalMs
			binary.LittleEndian.PutUint32(time.TxBuffer.Data[0:4], time.Ms)
			binary.LittleEndian.PutUint16(time.TxBuffer.Data[4:6], time.Days)
			time.busManager.Send(*time.TxBuffer)
		} else {
			time.ProducerTimerMs += ms
		}
	} else {
		time.ProducerTimerMs = time.ProducerIntervalMs
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
	time_obj.ResidualUs = 0
	time_obj.Ms = uint32(ms)
	time_obj.Days = days
	log.Infof("[TIME] setting the date to %v", time.Now())
	log.Infof("[TIME] days since 01/01/1984 : %v | ms since 00:00 : %v", days, ms)
}
