// Example of master usage
package main

import (
	"log/slog"
	"os"

	"github.com/samsamfire/gocanopen/pkg/network"
)

var DEFAULT_NODE_ID = uint8(0x20)
var DEFAULT_CAN_INTERFACE = "can0"
var DEFAULT_CAN_BITRATE = 500_000
var EDS_PATH = "../../testdata/base.eds"

func main() {

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	network := network.NewNetwork(nil)
	network.SetLogger(logger)

	err := network.Connect("socketcan", DEFAULT_CAN_INTERFACE, DEFAULT_CAN_BITRATE)
	if err != nil {
		panic(err)
	}

	// Add a remote node for master control
	node, err := network.AddRemoteNode(DEFAULT_NODE_ID, "../../testdata/base.eds")
	if err != nil {
		panic(err)
	}
	// Start PDOs, without reading remote configuration (useLocal = true)
	node.StartPDOs(true)
	// Read values via SDO
	val, err := node.ReadUint("UNSIGNED32 value", "")
	if err == nil {
		logger.Info("read", "value", val)
	}
	// Or write values via SDO
	err = node.WriteAnyExact("UNSIGNED64 value", "", uint64(10))
	if err != nil {
		logger.Info("failed to write", "err", err)
	}
}
