package nmt

import (
	"encoding/binary"
	"time"

	"github.com/samsamfire/gocanopen/pkg/od"
)

// [NMT] update heartbeat period
func writeEntry1017(stream *od.Stream, data []byte) (uint16, error) {
	if stream.Subindex != 0 || data == nil || len(data) != 2 || stream == nil {
		return 0, od.ErrDevIncompat
	}
	nmt, ok := stream.Object.(*NMT)
	if !ok {
		return 0, od.ErrDevIncompat
	}
	nmt.mu.Lock()
	nmt.periodProducer = time.Duration(binary.LittleEndian.Uint16(data)) * time.Millisecond
	nmt.mu.Unlock()
	if nmt.rxCancel != nil {
		nmt.restartTimerProducer(nmt.periodProducer)
	}
	nmt.logger.Debug("updated heartbeat period", "period", nmt.periodProducer)
	return od.WriteEntryDefault(stream, data)
}
