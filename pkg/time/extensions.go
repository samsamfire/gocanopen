package time

import (
	"encoding/binary"

	canopen "github.com/samsamfire/gocanopen/v2"
	"github.com/samsamfire/gocanopen/v2/pkg/od"
)

// [TIME] update cob id & if should be producer
func writeEntry1012(stream *od.Stream, data []byte) (uint16, error) {
	if stream == nil || data == nil || stream.Subindex != 0 || len(data) != 4 {
		return 0, od.ErrDevIncompat
	}
	t, ok := stream.Object.(*TIME)
	if !ok {
		return 0, od.ErrDevIncompat
	}
	cobIdTimestamp := binary.LittleEndian.Uint32(data)
	var canId = uint16(cobIdTimestamp & 0x7FF)
	if (cobIdTimestamp&0x3FFFF800) != 0 || canopen.IsIDRestricted(canId) {
		return 0, od.ErrInvalidValue
	}
	isProducer := (cobIdTimestamp & 0x40000000) != 0
	isConsumer := (cobIdTimestamp & 0x80000000) != 0
	t.mu.Lock()
	defer t.mu.Unlock()
	// CAN-ID can't change while TIME is in use
	if canId != uint16(t.cobId) && (t.isProducer || t.isConsumer) && (isProducer || isConsumer) {
		return 0, od.ErrInvalidValue
	}
	t.isProducer = isProducer
	t.isConsumer = isConsumer
	t.cobId = uint32(canId)
	t.mu.Unlock()
	t.Stop()
	err := t.Start()
	t.mu.Lock()
	if err != nil {
		return 0, od.ErrDevIncompat
	}
	return od.WriteEntryDefault(stream, data)
}
