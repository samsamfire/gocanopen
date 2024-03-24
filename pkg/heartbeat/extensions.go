package heartbeat

import (
	"encoding/binary"

	"github.com/samsamfire/gocanopen/pkg/od"
	log "github.com/sirupsen/logrus"
)

// [HBConsumer] update heartbeat consumer
func writeEntry1016(stream *od.Stream, data []byte, countWritten *uint16) error {
	consumer, ok := stream.Object.(*HBConsumer)
	if !ok {
		return od.ODR_DEV_INCOMPAT
	}
	consumer.mu.Lock()
	defer consumer.mu.Unlock()

	if stream == nil || stream.Subindex < 1 ||
		int(stream.Subindex) > len(consumer.monitoredNodes) ||
		len(data) != 4 {
		return od.ODR_DEV_INCOMPAT
	}

	hbConsValue := binary.LittleEndian.Uint32(data)
	nodeId := uint8(hbConsValue >> 16)
	time := hbConsValue & 0xFFFF
	log.Debugf("[OD][EXTENSION][HB CONSUMER] will monitor x%x with period %v ms", nodeId, time)
	err := consumer.addHearbeatConsumerNode(stream.Subindex-1, nodeId, uint16(time))
	if err != nil {
		return od.ODR_PAR_INCOMPAT
	}
	return od.WriteEntryDefault(stream, data, countWritten)
}
