package config

import "github.com/samsamfire/gocanopen/pkg/sdo"

type NMTConfig struct {
	nodeId    uint8
	sdoClient *sdo.SDOClient
}

func NewNMTConfigurator(nodeId uint8, sdoClient *sdo.SDOClient) NMTConfig {
	return NMTConfig{nodeId: nodeId, sdoClient: sdoClient}
}

// Read a nodes heartbeat period and returns it in milliseconds
func (config *NMTConfig) ReadHeartbeatPeriod() (uint16, error) {
	return config.sdoClient.ReadUint16(config.nodeId, 0x1017, 0)
}

// Update a nodes heartbeat period in milliseconds
func (config *NMTConfig) WriteHeartbeatPeriod(periodMs uint16) error {
	return config.sdoClient.WriteRaw(config.nodeId, 0x1017, 0, periodMs, false)
}
