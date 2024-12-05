package config

import (
	"log/slog"

	"github.com/samsamfire/gocanopen/pkg/sdo"
)

// NodeConfigurator provides helper methods for
// reading / updating CANopen reserved configuration objects
// i.e. objects between 0x1000 and 0x2000.
// No EDS files need to be loaded for configuring these parameters
// This uses an SDO client to access the different objects
type NodeConfigurator struct {
	logger *slog.Logger
	client *sdo.SDOClient
	nodeId uint8
}

// Create a new [NodeConfigurator] for given ID and SDOClient
func NewNodeConfigurator(nodeId uint8, logger *slog.Logger, client *sdo.SDOClient) *NodeConfigurator {
	if logger == nil {
		logger = slog.Default()
	}
	configurator := NodeConfigurator{logger: logger.With("service", "[CONFIG]"), client: client, nodeId: nodeId}
	return &configurator
}
