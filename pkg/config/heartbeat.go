package config

import "github.com/samsamfire/gocanopen/pkg/od"

// Read current monitored nodes
// Returns a list of all the entries composed as the id of the monitored node
// And the expected period in ms
func (config *NodeConfigurator) ReadMonitoredNodes() ([][]uint16, error) {
	nbMonitored, err := config.ReadMaxMonitorable()
	if err != nil {
		return nil, err
	}
	monitored := make([][]uint16, 0)
	for i := uint8(1); i <= nbMonitored; i++ {
		periodAndId, err := config.client.ReadUint32(config.nodeId, od.EntryConsumerHeartbeatTime, i)
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
func (config *NodeConfigurator) ReadMaxMonitorable() (uint8, error) {
	nbMonitored, err := config.client.ReadUint8(config.nodeId, od.EntryConsumerHeartbeatTime, 0x0)
	if err != nil {
		return 0, err
	}
	return nbMonitored, nil
}

// Add or update a node to monitor with the expected heartbeat period
// Index needs to be between 1 & the max nodes that can be monitored
func (config *NodeConfigurator) WriteMonitoredNode(index uint8, nodeId uint8, periodMs uint16) error {
	periodAndId := uint32(nodeId)<<16 + uint32(periodMs&0xFFFF)
	return config.client.WriteRaw(config.nodeId, od.EntryConsumerHeartbeatTime, index, periodAndId, false)
}

// Read a nodes heartbeat period and returns it in milliseconds
func (config *NodeConfigurator) ReadHeartbeatPeriod() (uint16, error) {
	return config.client.ReadUint16(config.nodeId, od.EntryProducerHeartbeatTime, 0)
}

// Update a nodes heartbeat period in milliseconds
func (config *NodeConfigurator) WriteHeartbeatPeriod(periodMs uint16) error {
	return config.client.WriteRaw(config.nodeId, od.EntryProducerHeartbeatTime, 0, periodMs, false)
}
