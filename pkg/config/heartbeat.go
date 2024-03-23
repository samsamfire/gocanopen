package config

import "github.com/samsamfire/gocanopen/pkg/sdo"

type HBConfig struct {
	*sdo.SDOClient
	nodeId uint8
}

func NewHBConfigurator(nodeId uint8, sdoClient *sdo.SDOClient) HBConfig {
	return HBConfig{nodeId: nodeId, SDOClient: sdoClient}
}

// Read current monitored nodes
// Returns a list of all the entries composed as the id of the monitored node
// And the expected period in ms
func (config *HBConfig) ReadMonitoredNodes() ([][]uint16, error) {
	nbMonitored, err := config.ReadMaxMonitorable()
	if err != nil {
		return nil, err
	}
	monitored := make([][]uint16, 0)
	for i := uint8(1); i <= nbMonitored; i++ {
		periodAndId, err := config.ReadUint32(config.nodeId, 0x1016, i)
		if err != nil {
			return monitored, err
		}
		nodeId := uint16((periodAndId >> 16) & 0xFF)
		period := uint16(periodAndId)
		monitored = append(monitored, []uint16{nodeId, period})
	}
	return monitored, nil
}

// Read max available entries for monitoring
func (config *HBConfig) ReadMaxMonitorable() (uint8, error) {
	nbMonitored, err := config.ReadUint8(config.nodeId, 0x1016, 0x0)
	if err != nil {
		return 0, err
	}
	return nbMonitored, nil
}

// Add or update a node to monitor with the expected heartbeat period
// Index needs to be between 1 & the max nodes that can be monitored
func (config *HBConfig) WriteMonitoredNode(index uint8, nodeId uint8, periodMs uint16) error {
	periodAndId := uint32(nodeId)<<16 + uint32(periodMs&0xFFFF)
	return config.WriteRaw(config.nodeId, 0x1016, index, periodAndId, false)
}

// Read a nodes heartbeat period and returns it in milliseconds
func (config *HBConfig) ReadHeartbeatPeriod() (uint16, error) {
	return config.ReadUint16(config.nodeId, 0x1017, 0)
}

// Update a nodes heartbeat period in milliseconds
func (config *HBConfig) WriteHeartbeatPeriod(periodMs uint16) error {
	return config.WriteRaw(config.nodeId, 0x1017, 0, periodMs, false)
}