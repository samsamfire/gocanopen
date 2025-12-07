package main

import (
	"log"
	"time"

	"github.com/samsamfire/gocanopen/pkg/network"
	"github.com/samsamfire/gocanopen/pkg/od"
)

func main() {
	// Create a new network
	net := network.NewNetwork(nil)

	// Connect to the CAN bus
	err := net.Connect("socketcan", "vcan0", 500000)
	if err != nil {
		log.Fatalf("Failed to connect to CAN bus: %v", err)
	}
	defer net.Disconnect()

	// Create a local node (slave) with node ID 0x10 and a default object dictionary
	node, err := net.CreateLocalNode(0x10, od.Default())
	if err != nil {
		log.Fatalf("Failed to create local node: %v", err)
	}

	log.Printf("Node %d created and running. Press Ctrl+C to exit.", node.GetID())

	// Keep the main thread alive
	for {
		time.Sleep(1 * time.Second)
	}
}
