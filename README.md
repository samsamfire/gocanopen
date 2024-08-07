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

Currently this package only supports socketcan using : [brutella/can](https://github.com/brutella/can)
However it can easily extended by creating your own driver. The following interface needs to be implemented

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

1. [Introduction](docs/intro.md)
2. [Object Dictionary](docs/od.md)
3. [Remote nodes](docs/remote.md)
4. [Local nodes](docs/local.md)

### Usage

Examples can be found in `/cmd`

Basic setup example :

```go

	import (
		canopen "github.com/samsamfire/gocanopen"
		"github.com/samsamfire/gocanopen/pkg/network"
		"github.com/samsamfire/gocanopen/pkg/od"
	)

	network := canopen.NewNetwork(nil)
	err := network.Connect("socketcan", "can0", 500_000)
	if err != nil {
		panic(err)
	}
	defer network.Disconnect()

	// Load a remote node OD to be able to read values from strings
	err = network.AddNode(0x10, "../../testdata/base.eds")
	if err != nil {
		panic(err)
	}

	network.Read(0x10, "INTEGER16 value", "")

	// Or create a local node using a default OD
	local,err := network.CreateLocalNode(0x20,od.Default())
	if err != nil {
		panic(err)
	}

	// Perform a network scan to detect other nodes...
	res,err := network.Scan(1000)


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

The application uses the log package [logrus](https://github.com/sirupsen/logrus)
Logs can be adjusted with different log levels :

```go
import log "github.com/sirupsen/logrus"

log.SetLevel(log.DebugLevel)
// log.SetLevel(log.WarnLevel)
```

### Credits

This work is heavily based on the C implementation by Janez ([https://github.com/CANopenNode/CANopenNode])
and also inspired by the python implementation by Christian Sandberg ([https://github.com/christiansandberg/canopen])
