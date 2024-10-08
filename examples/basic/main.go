package main

import (
	"fmt"

	"github.com/samsamfire/gocanopen/pkg/network"
	"github.com/samsamfire/gocanopen/pkg/od"
)

func main() {
	network := network.NewNetwork(nil)
	err := network.Connect("socketcan", "can0", 500_000)
	if err != nil {
		panic(err)
	}
	defer network.Disconnect()

	// Add a remote node to the network, either by providing an EDS file
	// Or downloading from the node. We use here a default OD available with the library
	node, err := network.AddRemoteNode(0x10, od.Default())
	if err != nil {
		panic(err)
	}

	// Read standard entry containing device name (0x1008)
	value, err := node.Configurator().ReadManufacturerDeviceName()
	if err != nil {
		fmt.Printf("error reading node %v device name : %v\n", node.GetID(), err)
	} else {
		fmt.Printf("node %v device name is %v\n", node.GetID(), value)
	}

	// Perform a network scan to detect other nodes...
	res, err := network.Scan(1000)
	if err != nil {
		panic(err)
	}
	fmt.Println("scanned the following nodes : ", res)
}
