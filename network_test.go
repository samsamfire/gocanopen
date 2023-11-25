package canopen

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

func createNetwork() *Network {
	bus := NewVirtualCanBus("localhost:18888")
	bus.receiveOwn = true
	network := NewNetwork(bus)
	e := network.Connect()
	if e != nil {
		panic(e)
	}
	_, e = network.CreateNode(NODE_ID_TEST, "testdata/base.eds")
	if e != nil {
		panic(e)
	}
	go func() { network.Process() }()
	return &network
}

func TestMain(m *testing.M) {
	vcan_server := "/usr/local/bin/virtualcan"
	if os.Getenv("VIRTUALCAN_SERVER_BIN") != "" {
		vcan_server = os.Getenv("VIRTUALCAN_SERVER_BIN")
	}
	fmt.Printf("starting vcan server\n")
	cmd := exec.Command(vcan_server, "--port", "18888")
	if err := cmd.Start(); err != nil {
		fmt.Printf("Failed to start virtual can server: %v", err)
		os.Exit(1)
	}
	exit := m.Run()
	os.Exit(exit)
}

func TestRead(t *testing.T) {
	network := createNetwork()
	_, err := network.Read(NODE_ID_TEST, "UNSIGNED8 value", "")
	if err != nil {
		t.Fatal(err)
	}

}
