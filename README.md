# gocanopen

[![Go Reference](https://pkg.go.dev/badge/github.com/samsamfire/gocanopen.svg)](https://pkg.go.dev/github.com/samsamfire/gocanopen)
[![Go Report Card](https://goreportcard.com/badge/github.com/samsamfire/gocanopen)](https://goreportcard.com/report/github.com/samsamfire/gocanopen)

**gocanopen** is an implementation of the CANopen protocol (CiA 301) written in pure golang.
It aims to be simple and efficient.
This package can be used for master usage but also supports creating regular CANopen nodes.

### Features

All the main components of CiA 301 are supported, these include :

- SDO server/client
- NMT master/slave
- HB producer/consumer
- TPDO/RPDO
- EMERGENCY producer/consumer
- SYNC producer/consumer
- TIME producer/consumer

Partial support also exists for CiA 309, specifically :

- HTTP gateway

**gocanopen** does not require the use of an external tool to generate some source code from an EDS file.
EDS files can be loaded dynamically from disk to create local nodes with default supported objects.
The library comes with a default embedded EDS file, which can be used when creating nodes.

### Supported CAN transceivers

Currently this package supports the following transceivers:
- socketcan
- kvaser
- virtualcan [here](https://github.com/windelbouwman/virtualcan)

In order to use kvaser, canlib should be downloaded & installed. Specific compile flags are needed :
- CFLAGS: -g -Wall -I/path_to_kvaser/canlib/include
- LDFLAGS: -L/path_to_kvaser/canlib

More transceivers can be added by creating your own driver and implementing the following
interface :

```go
type Bus interface {
	Connect(...any) error                   // Connect to the actual bus
	Disconnect() error                      // Disconnect from bus
	Send(frame Frame) error                 // Send a frame on the bus
	Subscribe(callback FrameListener) error // Subscribe to all can frames
}
```
Feel free to contribute to add specific drivers, we will find a way to integrate them in this repo.
This repo contains two implementation examples : `socketcan.go`, and `virtual.go` used for testing.

### Documentation

1. [Introduction](docs/index.md)
2. [Network](docs/network.md)
2. [Object Dictionary](docs/od.md)
4. [Local nodes](docs/local.md)

### Usage

Examples can be found in `/examples`

Basic setup example :

```go
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

### Work ongoing

- Improve documentation & examples
- More testing
- API improvements

### Testing

Testing is done :
- With unit testing
- Testing against an other implementation in python

Tests are done with `virtualcan` server, which can be used easily with github actions.
More tests are always welcome if you believe that some part of the spec is not properly tested.

For running tests, install the required packages `pip install -r ./tests/requirements.txt`
Run the tests with : `pytest -v`

### Logs

The application uses slog for logging.
Example logger setup with **Debug** level :

```go
logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
network := network.NewNetwork(nil)
network.SetLogger(logger)
```