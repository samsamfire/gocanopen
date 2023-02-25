package main

import (
	"canopen"
	"flag"
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
)

var DEFAULT_NODE_ID = 0x20
var DEFAULT_CAN_INTERFACE = "can0"

func main() {
	log.SetLevel(log.DebugLevel)
	// Command line arguments
	can_interface := flag.String("i", DEFAULT_CAN_INTERFACE, "socketcan interface e.g. can0,vcan0")
	node_id := flag.Int("n", DEFAULT_NODE_ID, "node id")
	eds_path := flag.String("p", "", "eds file path")
	flag.Parse()

	// Create a new socket can bus
	bus, e := canopen.NewSocketcanBus(*can_interface)
	if e != nil {
		fmt.Printf("could not connect to interface %v : %v\n", *can_interface, e)
		os.Exit(1)
	}

	busManager := &canopen.BusManager{}
	busManager.Init(&bus)
	// Subscribe to incoming messages
	bus.Subscribe(busManager)
	bus.Connect()

	// Load node EDS
	// Basic template can be found in the current directory
	object_dictionary, err := canopen.ParseEDS(*eds_path, uint8(*node_id))
	if err != nil {
		fmt.Printf("error encountered when loading EDS : %v\n", err)
		os.Exit(1)

	}
	// Create and initialize CANopen node
	node := canopen.Node{Config: nil, BusManager: busManager, NMT: nil}
	err = node.Init(nil, nil, object_dictionary, nil, canopen.CO_NMT_STARTUP_TO_OPERATIONAL, 500, 1000, 1000, true, uint8(*node_id))
	if err != nil {
		fmt.Printf("failed Initializing Node : %v\n", err)
		os.Exit(1)
	}
	err = node.InitPDO(object_dictionary, uint8(*node_id))
	if err != nil {
		fmt.Printf("failed to initiallize PDOs : %v\n", err)
		os.Exit(1)
	}

	startBackground := time.Now()
	backgroundPeriod := time.Duration(10 * time.Millisecond)
	startMain := time.Now()
	mainPeriod := time.Duration(10 * time.Millisecond)

	// Go routine responsible for processing background tasks such as PDO and SYNC
	go func() {
		for {
			elapsed := time.Since(startBackground)
			startBackground = time.Now()
			timeDifferenceUs := uint32(elapsed.Microseconds())
			syncWas := node.ProcessSYNC(timeDifferenceUs, nil)
			node.ProcessTPDO(syncWas, timeDifferenceUs, nil)
			time.Sleep(backgroundPeriod)
		}
	}()

	// Main loop
	for {
		elapsed := time.Since(startMain)
		startMain = time.Now()
		timeDifferenceUs := uint32(elapsed.Microseconds())
		node.Process(false, timeDifferenceUs, nil)
		// Add application code HERE
		time.Sleep(mainPeriod)

	}
}
