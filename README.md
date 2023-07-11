## go-canopen

This package is an implementation of the CANopen protocol (CiA 301) written entirely in golang.

### Features

All the main components of CiA 301 are supported, these include :

- SDO server/client
- NMT master/slave
- TPDO/RPDO
- EMERGENCY producer/consumer
- SYNC producer/consumer

This implementation does not require the use of an external tool to generate some source code from an EDS file.
EDS files can be loaded dynamically from disk to create local nodes.

### Supported CAN transceivers

Currently this package only supports socketcan using : [brutella/can](https://github.com/brutella/can)
Feel free to contribute to add specific drivers, we will find a way to integrate them in this repo.
The socketcan example can be found inside cmd/canopen/driver.go

### Example usage

The following is an example from the main file from the cmd/canopen example package.

```go
	log.SetLevel(log.DebugLevel)
    uint8 NODE_ID = 0x10
	// Create a new socket can bus <-- this should be handled by user if another interface is desired
	bus, e := NewSocketcanBus("can0")
	if e != nil {
		fmt.Printf("could not connect to interface %v : %v\n", "can0", e)
		os.Exit(1)
	}
	busManager := &canopen.BusManager{}
	busManager.Init(&bus)
	bus.Subscribe(busManager)
	bus.Connect()

	// Load node EDS, this will be used to generate all the CANopen objects
	// Basic EDS can be found in this repo
	object_dictionary, err := canopen.ParseEDS("path/to/object_dictionary.eds", NODE_ID)
	if err != nil {
		fmt.Printf("error encountered when loading EDS : %v\n", err)
		os.Exit(1)
	}

	// This is an example of how one could run this with a state machine
	appState := INIT
	nodeState := canopen.RESET_NOT
	var node canopen.Node
	quit := make(chan bool)
	// These are timer values and can be adjusted
	startBackground := time.Now()
	backgroundPeriod := time.Duration(10 * time.Millisecond)
	startMain := time.Now()
	mainPeriod := time.Duration(1 * time.Millisecond)

	for {

		switch appState {
		case INIT:
			// Create and initialize a CANopen node
			node = canopen.Node{Config: nil, BusManager: busManager, NMT: nil}
			err = node.Init(nil, nil, object_dictionary, nil, canopen.NMT_STARTUP_TO_OPERATIONAL, 500, 1000, 1000, true, NODE_iD)
			if err != nil {
				fmt.Printf("failed to initialize the node : %v\n", err)
				os.Exit(1)
			}
			err = node.InitPDO(object_dictionary, uint8(*node_id))
			if err != nil {
				fmt.Printf("failed to initiallize PDOs : %v\n", err)
				os.Exit(1)
			}
			// Start go routing that handles background tasks (PDO, SYNC, ...)
			go func() {
				for {
					select {
					case <-quit:
						log.Info("Quitting go routine")
						return
					default:
						elapsed := time.Since(startBackground)
						startBackground = time.Now()
						timeDifferenceUs := uint32(elapsed.Microseconds())
						syncWas := node.ProcessSYNC(timeDifferenceUs, nil)
						node.ProcessTPDO(syncWas, timeDifferenceUs, nil)
						node.ProcessRPDO(syncWas, timeDifferenceUs, nil)
						time.Sleep(backgroundPeriod)
					}
				}
			}()
			appState = RUNNING

		case RUNNING:
			elapsed := time.Since(startMain)
			startMain = time.Now()
			timeDifferenceUs := uint32(elapsed.Microseconds())
			nodeState = node.Process(false, timeDifferenceUs, nil)
			// <-- Add application code HERE
			time.Sleep(mainPeriod)
			if nodeState == canopen.RESET_APP || nodeState == canopen.RESET_COMM {
				appState = RESETING
			}
		case RESETING:
			quit <- true
			appState = INIT
		}
	}
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
