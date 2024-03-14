package canopen

import (
	"testing"

	"github.com/samsamfire/gocanopen/pkg/can"
	"github.com/samsamfire/gocanopen/pkg/can/virtual"
	"github.com/stretchr/testify/assert"
)

const NODE_ID_TEST uint8 = 0x30

func createNetworkEmpty() *Network {
	canBus, _ := can.NewBus("virtual", "localhost:18888", 0)
	bus := canBus.(*virtual.VirtualCanBus)
	bus.SetReceiveOwn(true)
	network := NewNetwork(bus)
	e := network.Connect()
	if e != nil {
		panic(e)
	}
	return &network
}

func createNetwork() *Network {
	network := createNetworkEmpty()
	_, err := network.CreateLocalNode(NODE_ID_TEST, "testdata/base.eds")
	if err != nil {
		panic(err)
	}
	return network
}

func TestAddNodeLoadODFromSDO(t *testing.T) {
	network := createNetwork()
	defer network.Disconnect()
	od, err := network.ReadEDS(NODE_ID_TEST, nil)
	assert.Nil(t, err)
	_, err = network.AddRemoteNode(0x55, od, true)
	assert.Nil(t, err)
}

func TestRemoveNode(t *testing.T) {
	network := createNetwork()
	defer network.Disconnect()
	network.RemoveNode(NODE_ID_TEST)
}
