package main

import (
	"canopen"
	"flag"
	"fmt"

	log "github.com/sirupsen/logrus"
)

var DEFAULT_NODE_ID = 0x20
var DEFAULT_CAN_INTERFACE = "vcan0"

const (
	INIT     = 0
	RUNNING  = 1
	RESETING = 2
)

func main() {
	log.SetLevel(log.DebugLevel)
	// Command line arguments
	channel := flag.String("i", DEFAULT_CAN_INTERFACE, "socketcan channel e.g. can0,vcan0")
	flag.Parse()

	network := canopen.NewNetwork(nil)
	err := network.Connect("", *channel, 500000)
	if err != nil {
		panic(err)
	}

	// Load corresponding OD to be able to read values from strings
	err = network.AddNode(0x10, "../../testdata/base.eds")
	if err != nil {
		panic(err)
	}
	network.Read(0x10, "INTEGER16 value", "")
	network.Read(0x10, "INTEGER8 value", "")
	network.Read(0x10, "INTEGER32 value", "")
	network.Read(0x10, "INTEGER64 value", "")
	network.Read(0x10, "UNSIGNED8 value", "")
	network.Read(0x10, "UNSIGNED16 value", "")
	network.Read(0x10, "UNSIGNED32 value", "")
	network.Read(0x10, "UNSIGNED64 value", "")
	network.Write(0x10, "INTEGER16 value", 0, int16(-10))
	fmt.Println(network.Read(0x10, "INTEGER16 value", ""))
	network.Write(0x10, "INTEGER16 value", 0, int16(50))
	fmt.Println(network.Read(0x10, "INTEGER16 value", ""))
	fmt.Print(network.Command(0x10, canopen.NMT_ENTER_PRE_OPERATIONAL))

}
