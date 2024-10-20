package sync

import (
	"encoding/binary"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/od"
	log "github.com/sirupsen/logrus"
)

// [SYNC] update cob id & if should be producer
func writeEntry1005(stream *od.Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || stream.Subindex != 0 || countWritten == nil || len(data) != 4 {
		return od.ErrDevIncompat
	}
	sync, ok := stream.Object.(*SYNC)
	if !ok {
		return od.ErrDevIncompat
	}
	sync.mu.Lock()
	defer sync.mu.Unlock()

	cobIdSync := binary.LittleEndian.Uint32(data)
	log.Debugf("[OD][EXTENSION][SYNC] updating COB-ID SYNC : %x", cobIdSync)
	canId := uint16(cobIdSync & 0x7FF)
	isProducer := (cobIdSync & 0x40000000) != 0
	if (cobIdSync&0xBFFFF800) != 0 || canopen.IsIDRestricted(canId) || (sync.isProducer && isProducer && canId != uint16(sync.cobId)) {
		return od.ErrInvalidValue
	}
	// Reconfigure the receive and transmit buffers only if changed
	if canId != uint16(sync.cobId) {
		err := sync.Subscribe(uint32(canId), 0x7FF, false, sync)
		if err != nil {
			return od.ErrDevIncompat
		}
		var frameSize uint8 = 0
		if sync.counterOverflow != 0 {
			frameSize = 1
		}
		log.Debugf("[OD][EXTENSION][SYNC] updated COB-ID SYNC to x%x (prev x%x)", canId, sync.cobId)
		sync.txBuffer = canopen.NewFrame(uint32(canId), 0, frameSize)
		sync.cobId = uint32(canId)
	}
	// Reset in case sync is producer
	sync.isProducer = isProducer
	if isProducer {
		log.Debug("[OD][EXTENSION][SYNC] SYNC is producer")
		sync.counter = 0
		sync.timer = 0
	} else {
		log.Debug("[OD][EXTENSION][SYNC] SYNC is not producer")
	}
	return od.WriteEntryDefault(stream, data, countWritten)
}

// [SYNC] update communication cycle period
func writeEntry1006(stream *od.Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || stream.Subindex != 0 || countWritten == nil || len(data) != 4 {
		return od.ErrDevIncompat
	}
	sync, ok := stream.Object.(*SYNC)
	if !ok {
		return od.ErrDevIncompat
	}
	sync.mu.Lock()
	defer sync.mu.Unlock()

	cyclePeriodUs := binary.LittleEndian.Uint32(data)
	log.Debugf("[OD][EXTENSION][SYNC] updating communication cycle period to %v us (%v ms)", cyclePeriodUs, cyclePeriodUs/1000)
	return od.WriteEntryDefault(stream, data, countWritten)
}

// [SYNC] update pdo synchronous window length
func writeEntry1007(stream *od.Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || stream.Subindex != 0 || countWritten == nil || len(data) != 4 {
		return od.ErrDevIncompat
	}
	sync, ok := stream.Object.(*SYNC)
	if !ok {
		return od.ErrDevIncompat
	}
	windowLengthUs := binary.LittleEndian.Uint32(data)
	log.Debugf("[OD][EXTENSION][SYNC] updating synchronous window length to %v us (%v ms)", windowLengthUs, windowLengthUs/1000)
	sync.mu.Lock()
	defer sync.mu.Unlock()

	return od.WriteEntryDefault(stream, data, countWritten)
}

// [SYNC] update synchronous counter overflow
func writeEntry1019(stream *od.Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || countWritten == nil || len(data) != 1 {
		return od.ErrDevIncompat
	}
	sync, ok := stream.Object.(*SYNC)
	if !ok {
		return od.ErrDevIncompat
	}
	sync.mu.Lock()
	defer sync.mu.Unlock()

	syncCounterOverflow := data[0]
	if syncCounterOverflow == 1 || syncCounterOverflow > 240 {
		return od.ErrInvalidValue
	}
	commCyclePeriod, err := sync.commCyclePeriod.Uint32(0)
	if commCyclePeriod != 0 || err != nil {
		return od.ErrDataDevState
	}
	var nbBytes = uint8(0)
	if syncCounterOverflow != 0 {
		nbBytes = 1
	}
	sync.txBuffer = canopen.NewFrame(sync.cobId, 0, nbBytes)
	sync.counterOverflow = syncCounterOverflow
	log.Debugf("[OD][EXTENSION][SYNC] updated synchronous counter overflow to %v", syncCounterOverflow)
	return od.WriteEntryDefault(stream, data, countWritten)
}
