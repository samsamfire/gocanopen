package config

import "github.com/samsamfire/gocanopen/pkg/sdo"

type TIMEConfig struct {
	*sdo.SDOClient
	nodeId uint8
}

func NewTIMEConfigurator(nodeId uint8, sdoClient *sdo.SDOClient) TIMEConfig {
	return TIMEConfig{nodeId: nodeId, SDOClient: sdoClient}
}

func (config *TIMEConfig) ReadCobId() (cobId uint32, err error) {
	return config.ReadUint32(config.nodeId, 0x1012, 0)
}

func (config *TIMEConfig) ProducerEnable() error {
	cobId, err := config.ReadCobId()
	if err != nil {
		return err
	}
	cobId |= (1 << 30)
	return config.WriteRaw(config.nodeId, 0x1012, 0x0, cobId, false)
}

func (config *TIMEConfig) ProducerDisable() error {
	cobId, err := config.ReadCobId()
	if err != nil {
		return err
	}
	mask := ^(uint32(1) << 30)
	cobId &= mask
	return config.WriteRaw(config.nodeId, 0x1012, 0x0, cobId, false)
}

func (config *TIMEConfig) ConsumerEnable() error {
	cobId, err := config.ReadCobId()
	if err != nil {
		return err
	}
	cobId |= (1 << 31)
	return config.WriteRaw(config.nodeId, 0x1012, 0x0, cobId, false)
}

func (config *TIMEConfig) ConsumerDisable() error {
	cobId, err := config.ReadCobId()
	if err != nil {
		return err
	}
	mask := ^(uint32(1) << 31)
	cobId &= mask
	return config.WriteRaw(config.nodeId, 0x1012, 0x0, cobId, false)
}
