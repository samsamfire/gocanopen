# gocanopen

[![Go Reference](https://pkg.go.dev/badge/github.com/samsamfire/gocanopen.svg)](https://pkg.go.dev/github.com/samsamfire/gocanopen)
[![Go Report Card](https://goreportcard.com/badge/github.com/samsamfire/gocanopen)](https://goreportcard.com/report/github.com/samsamfire/gocanopen)
[![Unit testing golang](https://github.com/samsamfire/gocanopen/actions/workflows/go_tests.yml/badge.svg)](https://github.com/samsamfire/gocanopen/actions/workflows/go_tests.yml)
[![License](https://img.shields.io/badge/License-MIT%202.0-blue.svg)](https://opensource.org/license/mit)

**gocanopen** is a modern, efficient, and compliant implementation of the CANopen protocol (CiA 301) written entirely in pure Go. It provides robust support for both CANopen master and slave functionalities, allowing dynamic Object Dictionary (OD) creation directly from EDS files without requiring any code generation.

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

### Getting Started

To get started with `gocanopen`, check out our detailed documentation and examples:

*   **Documentation:** [here](https://samsamfire.github.io/gocanopen/)
*   **Examples:** Explore the [`examples/`](./examples) directory for various use cases, including basic setup, master control, and HTTP gateway integration.

### Documentation

Our comprehensive documentation covers various aspects of `gocanopen`:

1.  [Introduction](docs/index.md)
2.  [Network](docs/network.md)
3.  [Remote Node](docs/remote-node.md)
4.  [Local Node](docs/local-node.md)
5.  [Object Dictionary](docs/od.md)
6.  [Configurator](docs/configurator.md)
7.  [CAN driver](docs/can.md)
8.  [SDO](docs/sdo.md)


### Contributing

Contributions are welcome ! Please open a PR.

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
