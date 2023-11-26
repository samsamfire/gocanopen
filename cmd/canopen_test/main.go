package main

// Demo used for automated testing

import (
	"canopen"
	"flag"
	"os"

	log "github.com/sirupsen/logrus"
)

var DEFAULT_NODE_ID = 0x10
var DEFAULT_CAN_INTERFACE = "can0"

const (
	INIT     = 0
	RUNNING  = 1
	RESETING = 2
)

func main() {
	log.SetLevel(log.DebugLevel)
	// Command line arguments
	eds_path := flag.String("p", "", "eds file path")
	flag.Parse()

	network := canopen.NewNetwork(nil)
	err := network.Connect("virtualcan", "127.0.0.1:18888", 500000)
	if err != nil {
		panic(err)
	}

	// Load node EDS, this will be used to generate all the CANopen objects
	// Basic template can be found in the current directory
	node, err := network.CreateNode(uint8(DEFAULT_NODE_ID), *eds_path)
	if err != nil {
		panic(err)
	}
	// Add file extension
	node.OD.AddFile(0x200F, "File", "example.bin", os.O_RDONLY|os.O_CREATE, os.O_CREATE|os.O_TRUNC|os.O_WRONLY)
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
