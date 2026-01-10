package sync

import (
	"encoding/binary"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/od"
)

// [SYNC] update cob id & if should be producer
func writeEntry1005(stream *od.Stream, data []byte) (uint16, error) {
	if stream == nil || data == nil || stream.Subindex != 0 || len(data) != 4 {
		return 0, od.ErrDevIncompat
	}
	sync, ok := stream.Object.(*SYNC)
	if !ok {
		return 0, od.ErrDevIncompat
	}
	sync.mu.Lock()
	defer sync.mu.Unlock()

	cobIdSync := binary.LittleEndian.Uint32(data)
	sync.logger.Info("udpating COB-ID", "cobId", cobIdSync)
	canId := uint16(cobIdSync & 0x7FF)
	isProducer := (cobIdSync & 0x40000000) != 0
	if (cobIdSync&0xBFFFF800) != 0 || canopen.IsIDRestricted(canId) || (sync.isProducer && isProducer && canId != uint16(sync.cobId)) {
		return 0, od.ErrInvalidValue
	}
	// Reconfigure the receive and transmit buffers only if changed
	if canId != uint16(sync.cobId) {
		if sync.rxCancel != nil {
			sync.rxCancel()
		}
		rxCancel, err := sync.bm.Subscribe(uint32(canId), 0x7FF, false, sync)
		sync.rxCancel = rxCancel
		if err != nil {
			return 0, od.ErrDevIncompat
		}
		var frameSize uint8 = 0
		if sync.counterOverflow != 0 {
			frameSize = 1
		}
		sync.logger.Info("udpating COB-ID", "prev", sync.cobId, "new", canId)
		sync.txBuffer = canopen.NewFrame(uint32(canId), 0, frameSize)
		sync.cobId = uint32(canId)
	}
	// Reset in case sync is producer
	sync.isProducer = isProducer
	if isProducer {
		sync.logger.Info("is producer")
		sync.counter = 0
		sync.timer = 0
	} else {
		sync.logger.Info("not producer")
	}
	return od.WriteEntryDefault(stream, data)
}

// [SYNC] update communication cycle period
func writeEntry1006(stream *od.Stream, data []byte) (uint16, error) {
	if stream == nil || data == nil || stream.Subindex != 0 || len(data) != 4 {
		return 0, od.ErrDevIncompat
	}
	sync, ok := stream.Object.(*SYNC)
	if !ok {
		return 0, od.ErrDevIncompat
	}
	sync.mu.Lock()
	defer sync.mu.Unlock()

	cyclePeriodUs := binary.LittleEndian.Uint32(data)
	sync.logger.Info("updating communication cycle", "periodMs", cyclePeriodUs/1000)
	return od.WriteEntryDefault(stream, data)
}

// [SYNC] update pdo synchronous window length
func writeEntry1007(stream *od.Stream, data []byte) (uint16, error) {
	if stream == nil || data == nil || stream.Subindex != 0 || len(data) != 4 {
		return 0, od.ErrDevIncompat
	}
	sync, ok := stream.Object.(*SYNC)
	if !ok {
		return 0, od.ErrDevIncompat
	}
	windowLengthUs := binary.LittleEndian.Uint32(data)
	sync.logger.Info("updating synchronous window length", "lengthMs", windowLengthUs/1000)
	sync.mu.Lock()
	defer sync.mu.Unlock()

	return od.WriteEntryDefault(stream, data)
}

// [SYNC] update synchronous counter overflow
func writeEntry1019(stream *od.Stream, data []byte) (uint16, error) {
	if stream == nil || data == nil || len(data) != 1 {
		return 0, od.ErrDevIncompat
	}
	sync, ok := stream.Object.(*SYNC)
	if !ok {
		return 0, od.ErrDevIncompat
	}
	sync.mu.Lock()
	defer sync.mu.Unlock()

	syncCounterOverflow := data[0]
	if syncCounterOverflow == 1 || syncCounterOverflow > 240 {
		return 0, od.ErrInvalidValue
	}
	commCyclePeriod, err := sync.commCyclePeriod.Uint32(0)
	if commCyclePeriod != 0 || err != nil {
		return 0, od.ErrDataDevState
	}
	var nbBytes = uint8(0)
	if syncCounterOverflow != 0 {
		nbBytes = 1
	}
	sync.txBuffer = canopen.NewFrame(sync.cobId, 0, nbBytes)
	sync.counterOverflow = syncCounterOverflow
	sync.logger.Info("updating synchronous counter overflow", "overflow", syncCounterOverflow)
	return od.WriteEntryDefault(stream, data)
}
