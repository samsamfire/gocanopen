package main

import (
	"canopen"
	"flag"
	"os"

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

	network := canopen.NewNetwork(nil)
	err := network.Connect("", *can_interface, 500000)
	if err != nil {
		panic(err)
	}

	// Load node EDS, this will be used to generate all the CANopen objects
	// Basic template can be found in the current directory
	node, err := network.CreateNode(uint8(*node_id), *eds_path)
	if err != nil {
		panic(err)
	}
	node.OD.AddFile(0x3003, "File", "example2.bin", os.O_APPEND|os.O_CREATE|os.O_RDWR)
	if err != nil {
		panic(err)
	}
	if err != nil {
		panic(err)
	}
	e := network.Process()
	if e != nil {
		panic(e)
	}
}
