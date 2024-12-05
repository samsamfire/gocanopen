package heartbeat

import (
	"encoding/binary"

	"github.com/samsamfire/gocanopen/pkg/od"
)

// [HBConsumer] update heartbeat consumer
func writeEntry1016(stream *od.Stream, data []byte, countWritten *uint16) error {
	consumer, ok := stream.Object.(*HBConsumer)
	if !ok {
		return od.ErrDevIncompat
	}
	consumer.mu.Lock()
	defer consumer.mu.Unlock()

	if stream == nil || stream.Subindex < 1 ||
		int(stream.Subindex) > len(consumer.entries) ||
		len(data) != 4 {
		return od.ErrDevIncompat
	}

	hbConsValue := binary.LittleEndian.Uint32(data)
	nodeId := uint8(hbConsValue >> 16)
	periodMs := uint16(hbConsValue & 0xFFFF)
	err := consumer.updateConsumerEntry(stream.Subindex-1, nodeId, periodMs)
	if err != nil {
		return od.ErrParIncompat
	}
	return od.WriteEntryDefault(stream, data, countWritten)
}
