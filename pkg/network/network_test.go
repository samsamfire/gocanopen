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
	bus := canBus.(*virtual.Bus)
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
	_, err := network.CreateLocalNode(NODE_ID_TEST+1, "../../testdata/test_zipped_format.eds")
	assert.Nil(t, err)

	t.Run("local node ascii format", func(t *testing.T) {
		od, err := network.ReadEDS(NODE_ID_TEST, od.DefaultEDSFormatHandler)
		assert.Nil(t, err)
		assert.NotNil(t, od.Index(0x1021))
	})
	t.Run("local node zipped format local", func(t *testing.T) {
		assert.Nil(t, err)
		od, err := network.ReadEDS(NODE_ID_TEST+1, od.DefaultEDSFormatHandler)
		assert.Nil(t, err)
		assert.NotNil(t, od.Index(0x1021))
	})
	t.Run("local node zipped format remote", func(t *testing.T) {
		od, err := network2.ReadEDS(NODE_ID_TEST+1, od.DefaultEDSFormatHandler)
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
		_, err := local.GetOD().AddVariableType(0x1022, "Storage Format", od.UNSIGNED8, od.AttributeSdoRw, "0x10")
		assert.Nil(t, err)
		_, err = network2.ReadEDS(NODE_ID_TEST, nil)
		assert.Equal(t, od.ErrEdsFormat, err)
	})
}

func TestAddRemoveNodes(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	t.Run("remove node", func(t *testing.T) {
		err := network.RemoveNode(0x12)
		assert.Equal(t, ErrNotFound, err)
		err = network.RemoveNode(NODE_ID_TEST)
		assert.Nil(t, err)
		_, err = network.CreateLocalNode(NODE_ID_TEST, od.Default())
		assert.Len(t, network.nodes, 1)
		assert.Nil(t, err)
		err = network.RemoveNode(NODE_ID_TEST)
		assert.Nil(t, err)
		assert.Len(t, network.nodes, 0)
	})
	t.Run("add node", func(t *testing.T) {
		// Test creating multiple nodes with same id
		assert.Len(t, network.nodes, 0)
		_, err := network.CreateLocalNode(NODE_ID_TEST, od.Default())
		assert.Nil(t, err)
		_, err = network.CreateLocalNode(NODE_ID_TEST, od.Default())
		assert.Equal(t, ErrIdConflict, err)
		// Test adding multiple nodes with same id
		_, err = network.AddRemoteNode(NODE_ID_TEST, od.Default())
		assert.NotEmpty(t, ErrIdConflict, err)
	})

}
