package canopen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func createNetworkEmpty() *Network {
	bus := NewVirtualCanBus("localhost:18888")
	bus.receiveOwn = true
	network := NewNetwork(bus)
	e := network.Connect()
	if e != nil {
		panic(e)
	}
	return &network
}

func createNetwork() *Network {
	network := createNetworkEmpty()
	_, err := network.CreateNode(NODE_ID_TEST, "testdata/base.eds")
	if err != nil {
		panic(err)
	}
	return network
}

func TestAddNodeLoadODFromSDO(t *testing.T) {
	network := createNetwork()
	defer network.Disconnect()
	err := network.AddNodeFromSDO(NODE_ID_TEST, nil)
	assert.Nil(t, err)
}
