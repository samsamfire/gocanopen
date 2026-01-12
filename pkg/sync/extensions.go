package sync

import (
	"encoding/binary"
	"time"

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
	// Stop any pending timers if for example if producer / consumer changed
	sync.isProducer = isProducer
	sync.mu.Unlock()
	sync.Stop()
	sync.Start()
	sync.mu.Lock()
	sync.logger.Info("sync type", "isProducer", isProducer)
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
	sync.syncCyclePeriod = time.Duration(cyclePeriodUs) * time.Microsecond

	if sync.syncCyclePeriod != 0 {
		sync.mu.Unlock()
		sync.resetTimers()
		sync.mu.Lock()
	}
	sync.logger.Info("updating communication cycle", "cyclePeriod", sync.syncCyclePeriod)
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
	sync.mu.Lock()
	defer sync.mu.Unlock()

	windowLengthUs := binary.LittleEndian.Uint32(data)
	sync.syncWindowLength = time.Duration(windowLengthUs) * time.Microsecond
	sync.logger.Info("updating synchronous window length", "windowLength", sync.syncWindowLength)

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

	if sync.syncCyclePeriod != 0 {
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
