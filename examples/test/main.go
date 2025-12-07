package main

// Demo used for automated testing

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"net/http"
	_ "net/http/pprof"

	"github.com/samsamfire/gocanopen/pkg/can/socketcanv2"
	"github.com/samsamfire/gocanopen/pkg/network"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/samsamfire/gocanopen/pkg/pdo"
)

var DEFAULT_NODE_ID = 0x10
var DEFAULT_CAN_INTERFACE = "can0"

const (
	INIT     = 0
	RUNNING  = 1
	RESETING = 2
)

func main() {

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	bus, err := socketcanv2.NewBus("can0")
	if err != nil {
		panic(err)
	}
	sBus, _ := bus.(*socketcanv2.Bus)
	err = sBus.SetReceiveOwn(true)
	if err != nil {
		panic(err)
	}
	network := network.NewNetwork(sBus)
	network.SetLogger(logger)

	err = network.Connect("socketcanv2", "can0", 500000)
	if err != nil {
		panic(err)
	}

	//Load node EDS, this will be used to generate all the CANopen objects
	// Basic template can be found in the current directory
	node, err := network.CreateLocalNode(uint8(DEFAULT_NODE_ID), od.Default())
	if err != nil {
		panic(err)
	}

	config := node.Configurator()
	config.WriteCommunicationPeriod(100_000)
	config.ProducerEnableSYNC()
	for i := range pdo.MaxPdoNumber {
		config.EnablePDO(i + 1)
		config.WriteTransmissionType(i+1, 1)
	}

	//Add file extension
	node.GetOD().AddFile(0x200F, "File", "example.bin", os.O_RDONLY|os.O_CREATE, os.O_CREATE|os.O_TRUNC|os.O_WRONLY)

	go func() {
		http.ListenAndServe("localhost:6060", nil)
	}()
	for {
		v, err := node.ReadUint(0x2200, 0)
		if err != nil {
			panic(err)
		}
		time.Sleep(500 * time.Millisecond)
		fmt.Println("rpdo uint8 value", v)
	}
}
