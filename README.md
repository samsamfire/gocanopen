# 🚀 gocanopen

> A robust, efficient, and pure Golang implementation of the CANopen protocol (CiA 301).

[![Go Reference](https://pkg.go.dev/badge/github.com/samsamfire/gocanopen.svg)](https://pkg.go.dev/github.com/samsamfire/gocanopen)
[![Go Report Card](https://goreportcard.com/badge/github.com/samsamfire/gocanopen)](https://goreportcard.com/report/github.com/samsamfire/gocanopen)
[![Unit testing golang](https://github.com/samsamfire/gocanopen/actions/workflows/go_tests.yml/badge.svg)](https://github.com/samsamfire/gocanopen/actions/workflows/go_tests.yml)
[![License](https://img.shields.io/github/license/samsamfire/gocanopen)](LICENSE)

---

**gocanopen** is designed to be a simple to use library for building CANopen Masters and Slaves in Go.

## ✨ Features

### CiA 301

- **NMT (Network Management)**: Full Master/Slave state machine support.
- **SDO (Service Data Object)**:
  - Server & Client
  - Block Transfer & Segmented Transfer
- **PDO (Process Data Object)**:
  - Dynamic Mapping & Communication parameters
  - Synchronous & Asynchronous transmission (TPDO/RPDO)
- **Heartbeat**: Producer & consumer monitoring
- **SYNC**: Producer & Consumer
- **TIME**: Time stamp object support
- **EMCY**: Emergency object producer & consumer
- **LSS**: Ongoing work

### CiA 309

- **HTTP Gateway**: Experimental support for CiA 309 web interfacing.

### Advanced Capabilities

- **Dynamic EDS Parsing**: Load `.eds` files at runtime to configure nodes instantly.
- **No CGO Required**: Pure Go implementation for maximum portability (except when using specific C-based drivers like Kvaser). 
- **Extensible Bus Interface**: Plug-and-play support for different CAN interfaces.

## 📦 Supported Hardware / Transceivers

| Driver | Description |
| :--- | :--- |
| **SocketCAN** | Standard Linux CAN interface. Native Go support. |
| **VirtualCAN** | TCP/IP based virtual bus, great for testing/CI. |
| **Kvaser** | Requires `canlib` installed (CGO). |

*Want to add your own? Implement the simple `Bus` interface!*

## 📚 Documentation

Visit [docs](https://samsamfire.github.io/gocanopen/)

## 🚀 Quick Start

Here is a complete example of setting up a Network Master and scanning for nodes on a SocketCAN bus.

```go
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/samsamfire/gocanopen/pkg/network"
	"github.com/samsamfire/gocanopen/pkg/od"
)

func main() {
    // 1. Setup Logger (Optional but recommended)
    logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// 2. Initialize the Network
	net := network.NewNetwork(nil)
    net.SetLogger(logger)

    // 3. Connect to the CAN interface (SocketCAN example)
	err := net.Connect("socketcan", "can0", 500_000)
	if err != nil {
		panic(err)
	}
	defer net.Disconnect()

	// 4. Add a Remote Node (e.g., a motor drive at ID 0x10)
    // We use the default embedded OD for basic definitions
	node, err := net.AddRemoteNode(0x10, od.Default())
	if err != nil {
		panic(err)
	}

	// 5. Read a value (Manufacturer Device Name - 0x1008)
    // The Configurator provides typed helpers for standard objects
	devName, err := node.Configurator().ReadManufacturerDeviceName()
	if err != nil {
		fmt.Printf("Error reading device name: %v\n", err)
	} else {
		fmt.Printf("Found Node 0x10: %s\n", devName)
	}

	// 6. Scan the entire network for other nodes
	fmt.Println("Scanning network...")
	foundNodes, err := net.Scan(1000) // 1000ms timeout
	if err != nil {
		panic(err)
	}
	
    for id, info := range foundNodes {
        fmt.Printf(" > Discovered Node ID: 0x%X (Info: %v)\n", id, info)
    }
}
```

## 🧪 Testing

The project uses a combination of:

1. Unit / Integration tests written in **Go**.
2. Integration tests written in **Python** (`tests/`) to test with another reference implementation.

To be able to run tests in Github Actions, we use a virtualcan interface.

To run **Go** tests:

1. Launch virtualcan server :

```bash
virtualcan --port 18888
```

2. Run tests:

```bash
cd pkg && go test ./... -v -p 1
```

To run **Python** tests:

1. Install python requirements

```bash
pip install -r ./tests/requirements.txt
```

2. Launch virtualcan server :

```bash
virtualcan --port 18889
```

3. Run tests:

```bash
cd tests && python -m pytest -v
```

## 🤝 Contributing

Contributions are welcome! Whether it's adding a new CAN driver, fixing a bug, or improving documentation.

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
