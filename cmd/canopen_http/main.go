package main

import (
	"canopen"
	"flag"

	"net/http"

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

func gateway309_5(w http.ResponseWriter, req *http.Request) {
	fmt.Print("hello endpoint called")
	fmt.Fprintf(w, "<i>hello</i>\n")
}

func main() {
	log.SetLevel(log.DebugLevel)
	// Command line arguments
	channel := flag.String("i", DEFAULT_CAN_INTERFACE, "socketcan channel e.g. can0,vcan0")
	flag.Parse()

	network := canopen.NewNetwork(nil)
	e := network.ConnectAndProcess("", *channel, 500000)
	if e != nil {
		panic(e)
	}
	gateway := canopen.NewGateway(1, 1, 100, &network)
	gateway.ListenAndServe(":8090")

}
