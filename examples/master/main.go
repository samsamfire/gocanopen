// Example of master usage
package main

import (
	"github.com/samsamfire/gocanopen/pkg/network"
	log "github.com/sirupsen/logrus"
)

var DEFAULT_NODE_ID = uint8(0x20)
var DEFAULT_CAN_INTERFACE = "can0"
var DEFAULT_CAN_BITRATE = 500_000
var EDS_PATH = "../../testdata/base.eds"

func main() {
	log.SetLevel(log.DebugLevel)

	network := network.NewNetwork(nil)
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
		log.Infof("read : %v", val)
	}
	// Or write values via SDO
	err = node.Write("UNSIGNED64 value", "", uint64(10))
	if err != nil {
		log.Info("failed to write", err)
	}
}
