package config

import "github.com/samsamfire/gocanopen/pkg/sdo"

// NodeConfigurator provides helper methods for
// reading / updating CANopen reserved configuration objects
// i.e. objects between 0x1000 and 0x2000.
// No EDS files need to be loaded for configuring these parameters
// This uses an SDO client to access the different objects
type NodeConfigurator struct {
	client *sdo.SDOClient
	nodeId uint8
}

// Create a new [NodeConfigurator] for given ID and SDOClient
func NewNodeConfigurator(nodeId uint8, client *sdo.SDOClient) *NodeConfigurator {
	configurator := NodeConfigurator{client: client, nodeId: nodeId}
	return &configurator
}
