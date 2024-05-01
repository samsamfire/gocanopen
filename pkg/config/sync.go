package config

import "github.com/samsamfire/gocanopen/pkg/sdo"

type SYNCConfig struct {
	*sdo.SDOClient
	nodeId uint8
}

func NewSYNCConfigurator(nodeId uint8, sdoClient *sdo.SDOClient) *SYNCConfig {
	return &SYNCConfig{nodeId: nodeId, SDOClient: sdoClient}
}

func (config *SYNCConfig) ReadCobId() (cobId uint32, err error) {
	return config.ReadUint32(config.nodeId, 0x1005, 0x0)
}

func (config *SYNCConfig) ReadCounterOverflow() (uint8, error) {
	return config.ReadUint8(config.nodeId, 0x1019, 0x0)
}

func (config *SYNCConfig) ReadCommunicationPeriod() (uint32, error) {
	return config.ReadUint32(config.nodeId, 0x1006, 0)
}

func (config *SYNCConfig) ReadWindowLengthPdos() (uint32, error) {
	return config.ReadUint32(config.nodeId, 0x1007, 0)
}

func (config *SYNCConfig) ProducerEnable() error {
	// Changing COB-ID is not allowed if already producer, read first
	cobId, err := config.ReadCobId()
	if err != nil {
		return err
	}
	cobId |= (1 << 30)
	return config.WriteRaw(config.nodeId, 0x1005, 0x0, cobId, false)
}

func (config *SYNCConfig) ProducerDisable() error {
	// Changing COB-ID is not allowed if already producer, read first
	cobId, err := config.ReadCobId()
	if err != nil {
		return err
	}
	mask := ^(uint32(1) << 30)
	cobId &= mask
	return config.WriteRaw(config.nodeId, 0x1005, 0x0, cobId, false)
}

// Change sync can id, sync should be disabled before changing this
func (config *SYNCConfig) WriteCanId(canId uint16) error {
	return config.WriteRaw(config.nodeId, 0x1005, 0x0, uint32(canId), false)
}

// Sync should have communication period of 0 before changing this
func (config *SYNCConfig) WriteCounterOverflow(counter uint8) error {
	return config.WriteRaw(config.nodeId, 0x1019, 0x0, counter, false)
}

func (config *SYNCConfig) WriteCommunicationPeriod(periodUs uint32) error {
	return config.WriteRaw(config.nodeId, 0x1006, 0, periodUs, false)
}

func (config *SYNCConfig) WriteWindowLengthPdos(windowPeriodUs uint32) error {
	return config.WriteRaw(config.nodeId, 0x1007, 0, windowPeriodUs, false)
}
