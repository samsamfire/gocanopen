package time

import (
	"encoding/binary"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/od"
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
	t.mu.Lock()
	defer t.mu.Unlock()
	t.isProducer = (cobIdTimestamp & 0x40000000) != 0
	t.isConsumer = (cobIdTimestamp & 0x80000000) != 0
	if t.isConsumer {
		if t.rxCancel != nil {
			t.rxCancel()
		}
		rxCancel, err := t.bm.Subscribe(t.cobId, 0x7FF, false, t)
		t.rxCancel = rxCancel
		if err != nil {
			return 0, od.ErrDevIncompat
		}
	}
	return od.WriteEntryDefault(stream, data)
}
