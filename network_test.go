package canopen

import (
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

func TestRead(t *testing.T) {
	network := createNetwork()
	_, err := network.Read(NODE_ID_TEST, "UNSIGNED8 value", "")
	if err != nil {
		t.Fatal(err)
	}

}
