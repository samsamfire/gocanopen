# Introduction

This package was written because I wanted an easy to use and efficient CANopen stack CiA 301
capable of running on embedded devices using golang.

This project has been inspired by two other existing projects:
- [CANopenNode](https://github.com/CANopenNode/CANopenNode) a C implementation slave side.
- [canopen](https://github.com/christiansandberg/canopen) a python implementation mostly for master control.

This project implements both slave & master side using an efficient API.

This documentation does not aim to be a tutorial on how CANopen works, a lot of information is freely available online.

### Example

This is a short example that connects to socketcan and performs various things on a remote node.

``` golang
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

```

Check [Network](network.md) for more info !
