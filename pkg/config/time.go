package config

func (config *NodeConfigurator) ReadCobIdTIME() (cobId uint32, err error) {
	return config.client.ReadUint32(config.nodeId, 0x1012, 0)
}

func (config *NodeConfigurator) ProducerEnableTIME() error {
	cobId, err := config.ReadCobIdTIME()
	if err != nil {
		return err
	}
	cobId |= (1 << 30)
	return config.client.WriteRaw(config.nodeId, 0x1012, 0x0, cobId, false)
}

func (config *NodeConfigurator) ProducerDisableTIME() error {
	cobId, err := config.ReadCobIdTIME()
	if err != nil {
		return err
	}
	mask := ^(uint32(1) << 30)
	cobId &= mask
	return config.client.WriteRaw(config.nodeId, 0x1012, 0x0, cobId, false)
}

func (config *NodeConfigurator) ConsumerEnableTIME() error {
	cobId, err := config.ReadCobIdTIME()
	if err != nil {
		return err
	}
	cobId |= (1 << 31)
	return config.client.WriteRaw(config.nodeId, 0x1012, 0x0, cobId, false)
}

func (config *NodeConfigurator) ConsumerDisable() error {
	cobId, err := config.ReadCobIdTIME()
	if err != nil {
		return err
	}
	mask := ^(uint32(1) << 31)
	cobId &= mask
	return config.client.WriteRaw(config.nodeId, 0x1012, 0x0, cobId, false)
}
