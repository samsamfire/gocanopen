package main

// Demo used for automated testing

import (
	"log/slog"
	"os"

	"net/http"
	_ "net/http/pprof"

	"github.com/samsamfire/gocanopen/pkg/network"
	"github.com/samsamfire/gocanopen/pkg/od"
)

var DEFAULT_NODE_ID = 0x10
var DEFAULT_CAN_INTERFACE = "can0"

const (
	INIT     = 0
	RUNNING  = 1
	RESETING = 2
)

func main() {

	go func() {
		http.ListenAndServe("localhost:6060", nil)
	}()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	network := network.NewNetwork(nil)
	network.SetLogger(logger)

	err := network.Connect("virtualcan", "127.0.0.1:18889", 500000)
	if err != nil {
		panic(err)
	}

	// Load node EDS, this will be used to generate all the CANopen objects
	// Basic template can be found in the current directory
	node, err := network.CreateLocalNode(uint8(DEFAULT_NODE_ID), od.Default())
	if err != nil {
		panic(err)
	}
	//Add file extension
	node.GetOD().AddFile(0x200F, "File", "example.bin", os.O_RDONLY|os.O_CREATE, os.O_CREATE|os.O_TRUNC|os.O_WRONLY)
	select {}
}
