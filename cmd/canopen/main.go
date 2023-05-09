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

const (
	INIT     = 0
	RUNNING  = 1
	RESETING = 2
)

func main() {
	log.SetLevel(log.DebugLevel)
	// Command line arguments
	can_interface := flag.String("i", DEFAULT_CAN_INTERFACE, "socketcan interface e.g. can0,vcan0")
	node_id := flag.Int("n", DEFAULT_NODE_ID, "node id")
	eds_path := flag.String("p", "", "eds file path")
	flag.Parse()

	// Create a new socket can bus <-- this can be adapted to use another interface
	bus, e := NewSocketcanBus(*can_interface)
	if e != nil {
		fmt.Printf("could not connect to interface %v : %v\n", *can_interface, e)
		os.Exit(1)
	}
	busManager := &canopen.BusManager{}
	busManager.Init(&bus)
	bus.Subscribe(busManager)
	bus.Connect()

	// Load node EDS, this will be used to generate all the CANopen objects
	// Basic template can be found in the current directory
	object_dictionary, err := canopen.ParseEDS(*eds_path, uint8(*node_id))
	if err != nil {
		fmt.Printf("error encountered when loading EDS : %v\n", err)
		os.Exit(1)
	}

	// This is an example of a custom Extension to add a DOMAIN variable
	// In particular this example is able to read and write to a local file
	// via SDO block transfer
	domain_entry := object_dictionary.Index(0x200F)
	extension := canopen.Extension{Object: nil, Read: ReadEntry200F, Write: WriteEntry200F}
	domain_entry.AddExtension(&extension)

	// This is an example of how one could run this with a state machine

	appState := INIT
	nodeState := canopen.RESET_NOT
	var node canopen.Node
	//NODE_STATE := canopen.RESET_NOT
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
			err = node.Init(nil, nil, object_dictionary, nil, canopen.NMT_STARTUP_TO_OPERATIONAL, 500, 1000, 1000, true, uint8(*node_id))
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
}
