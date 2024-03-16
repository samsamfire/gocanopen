package config

import "github.com/samsamfire/gocanopen/pkg/sdo"

// NodeConfigurator provides helper methods for
// updating CANopen reserved configuration objects
// i.e. objects between 0x1000 and 0x2000.
// No EDS files need to be loaded for configuring these parameters
type NodeConfigurator struct {
	RPDO PDOConfig
	TPDO PDOConfig
	SYNC SYNCConfig
	HB   HBConfig
	NMT  NMTConfig
	TIME TIMEConfig
}

// Create a new [NodeConfigurator] for given ID and SDOClient
func NewNodeConfigurator(nodeId uint8, client *sdo.SDOClient) NodeConfigurator {
	configurator := NodeConfigurator{}
	configurator.RPDO = NewRPDOConfigurator(nodeId, client)
	configurator.TPDO = NewTPDOConfigurator(nodeId, client)
	configurator.SYNC = NewSYNCConfigurator(nodeId, client)
	configurator.HB = NewHBConfigurator(nodeId, client)
	configurator.NMT = NewNMTConfigurator(nodeId, client)
	configurator.TIME = NewTIMEConfigurator(nodeId, client)
	return configurator
}
