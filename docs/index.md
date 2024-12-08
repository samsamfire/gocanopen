# Introduction

This package is aimed to be a modern, efficient and compliant CANopen stack
written entirely in go. It features both **slave** & **master** control and
supports **dynamic OD creation** directly via EDS file.

This project has been inspired by two other existing projects:

* [CANopenNode](https://github.com/CANopenNode/CANopenNode) a C implementation slave side.
* [canopen](https://github.com/christiansandberg/canopen) a python implementation mostly for master control.

This project implements both **slave** & **master** side using an efficient API.
Currently the following is implemented :

| Service name | Implemented |
| ------------ | ----------- |
| SDO server   | yes |
| SDO client   | yes |
| NMT master   | yes |
| NMT slave    | yes |
| HB producer  | yes |
| HB consumer  | yes |
| TPDO         | yes |
| RPDO         | yes |
| EMERGENCY  producer   | yes |
| EMERGENCY  consumer   | yes |
| SYNC producer | yes |
| SYNC consumer | yes |
| TIME producer | yes |
| TIME consumer | yes |
| LSS producer | **no**|
| LSS consumer | **no**|

## Basic Example

This is a short example that connects to socketcan, reads remote node device name \
and performs a network scan.

``` go
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
