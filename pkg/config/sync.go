package config

func (config *NodeConfigurator) ReadCobIdSYNC() (cobId uint32, err error) {
	return config.client.ReadUint32(config.nodeId, 0x1005, 0x0)
}

func (config *NodeConfigurator) ReadCounterOverflow() (uint8, error) {
	return config.client.ReadUint8(config.nodeId, 0x1019, 0x0)
}

func (config *NodeConfigurator) ReadCommunicationPeriod() (uint32, error) {
	return config.client.ReadUint32(config.nodeId, 0x1006, 0)
}

func (config *NodeConfigurator) ReadWindowLengthPdos() (uint32, error) {
	return config.client.ReadUint32(config.nodeId, 0x1007, 0)
}

func (config *NodeConfigurator) ProducerEnableSYNC() error {
	// Changing COB-ID is not allowed if already producer, read first
	cobId, err := config.ReadCobIdSYNC()
	if err != nil {
		return err
	}
	cobId |= (1 << 30)
	return config.client.WriteRaw(config.nodeId, 0x1005, 0x0, cobId, false)
}

func (config *NodeConfigurator) ProducerDisableSYNC() error {
	// Changing COB-ID is not allowed if already producer, read first
	cobId, err := config.ReadCobIdSYNC()
	if err != nil {
		return err
	}
	mask := ^(uint32(1) << 30)
	cobId &= mask
	return config.client.WriteRaw(config.nodeId, 0x1005, 0x0, cobId, false)
}

// Change sync can id, sync should be disabled before changing this
func (config *NodeConfigurator) WriteCanIdSYNC(canId uint16) error {
	return config.client.WriteRaw(config.nodeId, 0x1005, 0x0, uint32(canId), false)
}

// Sync should have communication period of 0 before changing this
func (config *NodeConfigurator) WriteCounterOverflow(counter uint8) error {
	return config.client.WriteRaw(config.nodeId, 0x1019, 0x0, counter, false)
}

func (config *NodeConfigurator) WriteCommunicationPeriod(periodUs uint32) error {
	return config.client.WriteRaw(config.nodeId, 0x1006, 0, periodUs, false)
}

func (config *NodeConfigurator) WriteWindowLengthPdos(windowPeriodUs uint32) error {
	return config.client.WriteRaw(config.nodeId, 0x1007, 0, windowPeriodUs, false)
}
