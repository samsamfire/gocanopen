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

func TestAddNodeLoadODFromSDO(t *testing.T) {
	network := createNetwork()
	defer network.Disconnect()
	err := network.AddNodeFromSDO(NODE_ID_TEST, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestReadManufacturerInfo(t *testing.T) {
	network := createNetwork()
	defer network.Disconnect()
	info, err := network.ReadManufacturerInformation(NODE_ID_TEST)
	if err != nil {
		t.Fatal(err)
	}
	if info.DeviceName != "DUT" || info.HardwareVersion != "v400" || info.SoftwareVersion != "v1.1.2r" {
		t.Fatal(info)
	}
}
