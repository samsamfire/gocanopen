package main

import (
	"flag"
	"fmt"

	"github.com/samsamfire/gocanopen/pkg/gateway/http"
	"github.com/samsamfire/gocanopen/pkg/network"
	log "github.com/sirupsen/logrus"
)

var DEFAULT_NODE_ID = 0x20
var DEFAULT_CAN_INTERFACE = "vcan0"
var DEFAULT_HTTP_PORT = 8090

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

	network := network.NewNetwork(nil)
	e := network.Connect("", *channel, 500000)
	if e != nil {
		panic(e)
	}
	gateway := http.NewGatewayServer(&network, 1, 1, 1000)
	gateway.ListenAndServe(fmt.Sprintf(":%d", DEFAULT_HTTP_PORT))

}
