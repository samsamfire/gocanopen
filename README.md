## go-canopen

This package is an implementation of the CANopen protocol (CiA 301) written entirely in golang.
Its goal is to be simple, yet efficient.
It can act as a master on the bus but it also supports implementation of regular canopen nodes.

### Features

All the main components of CiA 301 are supported, these include :

- SDO server/client
- NMT master/slave
- HB producer/consumer
- TPDO/RPDO
- EMERGENCY producer/consumer
- SYNC producer/consumer
- TIME

CiA 309 (Network)

- HTTP gateway


This implementation does not require the use of an external tool to generate some source code from an EDS file.
EDS files can be loaded dynamically from disk to create local nodes with default supported objects.

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

### Example usage

Examples can be found in `/cmd`

Basic setup example :

```go
	network := canopen.NewNetwork(nil)
	e := network.Connect("socketcan", "can0", 500000)
	if e != nil {
		panic(e)
	}
	defer network.Disconnect()

	// Background processing
	go func() { network.Process() }()

	// Load a remote node OD to be able to read values from strings
	e = network.AddNode(0x10, "../../testdata/base.eds")
	if e != nil {
		panic(e)
	}

	network.Read(0x10, "INTEGER16 value", "")
```

### Work ongoing

- More testing
- Adding support for kvaser (canlib)
- Reduce boilerplate as much as possible
- Improve API around "master" behaviour

### Testing

For testing, this library relies on the canopen python package, and tries to maximize coverage of the canopen stack.
More tests are always welcome if you believe that some part of the spec is not properly tested.

Tests can be run with :
`pytest -v`

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
Testing is done using the python implementation by Christian Sandberg ([https://github.com/christiansandberg/canopen])
