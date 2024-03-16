package network_test

import (
	"github.com/samsamfire/gocanopen/pkg/can"
	"github.com/samsamfire/gocanopen/pkg/can/virtual"
	"github.com/samsamfire/gocanopen/pkg/network"
)

const NODE_ID_TEST uint8 = 0x30

func CreateNetworkEmptyTest() *network.Network {
	canBus, _ := can.NewBus("virtual", "localhost:18888", 0)
	bus := canBus.(*virtual.VirtualCanBus)
	bus.SetReceiveOwn(true)
	network := network.NewNetwork(bus)
	e := network.Connect()
	if e != nil {
		panic(e)
	}
	return &network
}

func CreateNetworkTest() *network.Network {
	network := CreateNetworkEmptyTest()
	_, err := network.CreateLocalNode(NODE_ID_TEST, "testdata/base.eds")
	if err != nil {
		panic(err)
	}
	return network
}
