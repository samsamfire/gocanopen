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

func TestReadEDS(t *testing.T) {
	network := CreateNetworkTest()
	network2 := CreateNetworkEmptyTest()
	defer network2.Disconnect()
	defer network.Disconnect()
	t.Run("local node", func(t *testing.T) {
		od, err := network.ReadEDS(NODE_ID_TEST, nil)
		assert.Nil(t, err)
		assert.NotNil(t, od.Index(0x1021))
	})
	t.Run("remote node", func(t *testing.T) {
		od, err := network2.ReadEDS(NODE_ID_TEST, nil)
		assert.Nil(t, err)
		assert.NotNil(t, od.Index(0x1021))
	})
	t.Run("with invalid format handler", func(t *testing.T) {
		local, _ := network.Local(NODE_ID_TEST)
		// Replace EDS format with another value
		_, err := local.GetOD().AddVariableType(0x1022, "Storage Format", od.UNSIGNED8, od.ATTRIBUTE_SDO_RW, "0x10")
		assert.Nil(t, err)
		_, err = network2.ReadEDS(NODE_ID_TEST, nil)
		assert.Equal(t, ErrEdsFormat, err)
	})
}

func TestRemoveNode(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	network.RemoveNode(NODE_ID_TEST)
}
