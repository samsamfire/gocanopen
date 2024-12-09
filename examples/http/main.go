package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/samsamfire/gocanopen/pkg/gateway/http"
	"github.com/samsamfire/gocanopen/pkg/network"
)

const (
	NodeId    = 0x20
	Interface = "vcan0"
	Port      = 8090
)

func main() {

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	// Command line arguments
	channel := flag.String("i", Interface, "socketcan channel e.g. can0,vcan0")
	flag.Parse()

	network := network.NewNetwork(nil)
	network.SetLogger(logger)
	e := network.Connect("socketcan", *channel, 500000)
	if e != nil {
		panic(e)
	}
	gateway := http.NewGatewayServer(&network, logger, 1, 1, 1000)
	gateway.ListenAndServe(fmt.Sprintf(":%d", Port))

}
