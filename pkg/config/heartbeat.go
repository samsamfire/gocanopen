package config

import (
	"time"

	"github.com/samsamfire/gocanopen/pkg/od"
)

type MonitoredNode struct {
	NodeId         uint8
	HearbeatPeriod time.Duration
}

// Read current monitored nodes
// Returns a list of all the entries composed as the id of the monitored node
// And the expected heartbeat period in ms
func (config *NodeConfigurator) ReadMonitoredNodes() ([]MonitoredNode, error) {
	nbMonitored, err := config.ReadMaxMonitorableNodes()
	if err != nil {
		return nil, err
	}
	monitored := make([]MonitoredNode, 0)
	for i := range nbMonitored {
		periodAndId, err := config.client.ReadUint32(config.nodeId, od.EntryConsumerHeartbeatTime, i+1)
		if err != nil {
			return nil, err
		}
		monitoredNode := MonitoredNode{
			NodeId:         uint8((periodAndId >> 16) & 0xFF),
			HearbeatPeriod: time.Duration(uint16(periodAndId)) * time.Millisecond,
		}
		monitored = append(monitored, monitoredNode)
	}
	return monitored, nil
}

// Read max available entries for monitoring
func (config *NodeConfigurator) ReadMaxMonitorableNodes() (uint8, error) {
	nbMonitored, err := config.client.ReadUint8(config.nodeId, od.EntryConsumerHeartbeatTime, 0x0)
	if err != nil {
		return 0, err
	}
	return nbMonitored, nil
}

// Add or update a node to monitor with the expected heartbeat period
// Index needs to be between 1 & the max nodes that can be monitored
func (config *NodeConfigurator) WriteMonitoredNode(index uint8, nodeId uint8, period time.Duration) error {

	periodAndId := uint32(nodeId)<<16 + uint32(period.Milliseconds()&0xFFFF)
	return config.client.WriteRaw(config.nodeId, od.EntryConsumerHeartbeatTime, index, periodAndId, false)
}

// Read a nodes heartbeat period
func (config *NodeConfigurator) ReadHeartbeatPeriod() (time.Duration, error) {
	period, err := config.client.ReadUint16(config.nodeId, od.EntryProducerHeartbeatTime, 0)
	if err != nil {
		return 0, err
	}
	return time.Duration(period) * time.Millisecond, nil
}

// Update a nodes heartbeat period
func (config *NodeConfigurator) WriteHeartbeatPeriod(period time.Duration) error {
	return config.client.WriteRaw(config.nodeId, od.EntryProducerHeartbeatTime, 0, uint16(period.Milliseconds()), false)
}
