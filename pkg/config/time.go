package config

import "github.com/samsamfire/gocanopen/pkg/od"

func (config *NodeConfigurator) ReadCobIdTIME() (cobId uint32, err error) {
	return config.client.ReadUint32(config.nodeId, od.EntryCobIdTIME, 0)
}

func (config *NodeConfigurator) ProducerEnableTIME() error {
	cobId, err := config.ReadCobIdTIME()
	if err != nil {
		return err
	}
	cobId |= (1 << 30)
	return config.client.WriteRaw(config.nodeId, od.EntryCobIdTIME, 0x0, cobId, false)
}

func (config *NodeConfigurator) ProducerDisableTIME() error {
	cobId, err := config.ReadCobIdTIME()
	if err != nil {
		return err
	}
	mask := ^(uint32(1) << 30)
	cobId &= mask
	return config.client.WriteRaw(config.nodeId, od.EntryCobIdTIME, 0x0, cobId, false)
}

func (config *NodeConfigurator) ConsumerEnableTIME() error {
	cobId, err := config.ReadCobIdTIME()
	if err != nil {
		return err
	}
	cobId |= (1 << 31)
	return config.client.WriteRaw(config.nodeId, od.EntryCobIdTIME, 0x0, cobId, false)
}

func (config *NodeConfigurator) ConsumerDisableTIME() error {
	cobId, err := config.ReadCobIdTIME()
	if err != nil {
		return err
	}
	mask := ^(uint32(1) << 31)
	cobId &= mask
	return config.client.WriteRaw(config.nodeId, od.EntryCobIdTIME, 0x0, cobId, false)
}
