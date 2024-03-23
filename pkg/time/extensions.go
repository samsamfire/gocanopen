package time

import (
	"encoding/binary"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/od"
)

// [TIME] update cob id & if should be producer
func writeEntry1012(stream *od.Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || stream.Subindex != 0 || countWritten == nil || len(data) != 4 {
		return od.ODR_DEV_INCOMPAT
	}
	t, ok := stream.Object.(*TIME)
	if !ok {
		return od.ODR_DEV_INCOMPAT
	}
	cobIdTimestamp := binary.LittleEndian.Uint32(data)
	var canId = uint16(cobIdTimestamp & 0x7FF)
	if (cobIdTimestamp&0x3FFFF800) != 0 || canopen.IsIDRestricted(canId) {
		return od.ODR_INVALID_VALUE
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.isProducer = (cobIdTimestamp & 0x40000000) != 0
	t.isConsumer = (cobIdTimestamp & 0x80000000) != 0
	if t.isConsumer {
		err := t.Subscribe(t.cobId, 0x7FF, false, t)
		if err != nil {
			return od.ODR_DEV_INCOMPAT
		}
	}
	return od.WriteEntryDefault(stream, data, countWritten)
}
