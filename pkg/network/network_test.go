package network

import (
	"testing"

	"github.com/samsamfire/gocanopen/pkg/can/virtual"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/stretchr/testify/assert"
)

const NODE_ID_TEST uint8 = 0x30

func CreateNetworkEmptyTest() *Network {
	canBus, _ := NewBus("virtual", "localhost:18888", 0)
	bus := canBus.(*virtual.VirtualCanBus)
	bus.SetReceiveOwn(true)
	network := NewNetwork(bus)
	e := network.Connect()
	if e != nil {
		panic(e)
	}
	return &network
}

func CreateNetworkTest() *Network {
	network := CreateNetworkEmptyTest()
	_, err := network.CreateLocalNode(NODE_ID_TEST, od.Default())
	if err != nil {
		panic(err)
	}
	return network
}

func TestAddNodeLoadODFromSDO(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	od, err := network.ReadEDS(NODE_ID_TEST, nil)
	assert.Nil(t, err)
	_, err = network.AddRemoteNode(0x55, od)
	assert.Nil(t, err)
}

func TestRemoveNode(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	network.RemoveNode(NODE_ID_TEST)
}
