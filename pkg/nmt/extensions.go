package nmt

import (
	"encoding/binary"

	"github.com/samsamfire/gocanopen/pkg/od"
	log "github.com/sirupsen/logrus"
)

// [NMT] update heartbeat period
func writeEntry1017(stream *od.Stream, data []byte, countWritten *uint16) error {
	if stream.Subindex != 0 || data == nil || len(data) != 2 || countWritten == nil || stream == nil {
		return od.ErrDevIncompat
	}
	nmt, ok := stream.Object.(*NMT)
	if !ok {
		return od.ErrDevIncompat
	}
	nmt.mu.Lock()
	defer nmt.mu.Unlock()

	nmt.hearbeatProducerTimeUs = uint32(binary.LittleEndian.Uint16(data)) * 1000
	nmt.hearbeatProducerTimer = 0
	log.Debugf("[OD][EXTENSION][NMT] updated heartbeat period to %v ms", nmt.hearbeatProducerTimeUs/1000)
	return od.WriteEntryDefault(stream, data, countWritten)
}
