package config

import (
	"time"

	"github.com/samsamfire/gocanopen/pkg/od"
)

func (config *NodeConfigurator) ReadCobIdSYNC() (cobId uint32, err error) {
	return config.client.ReadUint32(config.nodeId, od.EntryCobIdSYNC, 0x0)
}

func (config *NodeConfigurator) ReadCounterOverflow() (uint8, error) {
	return config.client.ReadUint8(config.nodeId, od.EntrySynchronousCounterOverflow, 0x0)
}

func (config *NodeConfigurator) ReadCommunicationPeriod() (time.Duration, error) {
	period, err := config.client.ReadUint32(config.nodeId, od.EntryCommunicationCyclePeriod, 0)
	if err != nil {
		return 0, err
	}
	return time.Duration(period) * time.Millisecond, nil
}

func (config *NodeConfigurator) ReadWindowLengthPdos() (time.Duration, error) {
	period, err := config.client.ReadUint32(config.nodeId, od.EntrySynchronousWindowLength, 0)
	if err != nil {
		return 0, err
	}
	return time.Duration(period) * time.Millisecond, nil
}

func (config *NodeConfigurator) ProducerEnableSYNC() error {
	// Changing COB-ID is not allowed if already producer, read first
	cobId, err := config.ReadCobIdSYNC()
	if err != nil {
		return err
	}
	cobId |= (1 << 30)
	return config.client.WriteRaw(config.nodeId, od.EntryCobIdSYNC, 0x0, cobId, false)
}

func (config *NodeConfigurator) ProducerDisableSYNC() error {
	// Changing COB-ID is not allowed if already producer, read first
	cobId, err := config.ReadCobIdSYNC()
	if err != nil {
		return err
	}
	mask := ^(uint32(1) << 30)
	cobId &= mask
	return config.client.WriteRaw(config.nodeId, od.EntryCobIdSYNC, 0x0, cobId, false)
}

// Change sync can id, sync should be disabled before changing this
func (config *NodeConfigurator) WriteCanIdSYNC(canId uint16) error {
	return config.client.WriteRaw(config.nodeId, od.EntryCobIdSYNC, 0x0, uint32(canId), false)
}

// Sync should have communication period of 0 before changing this
func (config *NodeConfigurator) WriteCounterOverflow(counter uint8) error {
	return config.client.WriteRaw(config.nodeId, od.EntrySynchronousCounterOverflow, 0x0, counter, false)
}

func (config *NodeConfigurator) WriteCommunicationPeriod(period time.Duration) error {
	return config.client.WriteRaw(config.nodeId, od.EntryCommunicationCyclePeriod, 0, uint32(period.Milliseconds()), false)
}

func (config *NodeConfigurator) WriteWindowLengthPdos(period time.Duration) error {
	return config.client.WriteRaw(config.nodeId, od.EntrySynchronousWindowLength, 0, uint32(period.Milliseconds()), false)
}
