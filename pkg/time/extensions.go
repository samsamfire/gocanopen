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
	time, ok := stream.Object.(*TIME)
	if !ok {
		return od.ODR_DEV_INCOMPAT
	}
	cobIdTimestamp := binary.LittleEndian.Uint32(data)
	var canId = uint16(cobIdTimestamp & 0x7FF)
	if (cobIdTimestamp&0x3FFFF800) != 0 || canopen.IsIDRestricted(canId) {
		return od.ODR_INVALID_VALUE
	}
	time.isConsumer = (cobIdTimestamp & 0x80000000) != 0
	time.isProducer = (cobIdTimestamp & 0x40000000) != 0

	return od.WriteEntryDefault(stream, data, countWritten)
}
