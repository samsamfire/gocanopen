package network

import (
	"testing"
	"time"

	"github.com/samsamfire/gocanopen/pkg/node"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/stretchr/testify/assert"
)

func TestCreateRemoteNode(t *testing.T) {
	network := CreateNetworkTest()
	networkRemote := CreateNetworkEmptyTest()
	defer network.Disconnect()
	defer networkRemote.Disconnect()
	node, err := networkRemote.AddRemoteNode(NODE_ID_TEST, od.Default(), true)
	assert.Nil(t, err)
	assert.NotNil(t, node)
	err = node.InitPDOs(true)
	assert.Nil(t, err, err)
}

func TestRemoteNodeRPDO(t *testing.T) {
	network := CreateNetworkTest()
	networkRemote := CreateNetworkEmptyTest()
	defer network.Disconnect()
	defer networkRemote.Disconnect()
	remoteNode, err := networkRemote.AddRemoteNode(NODE_ID_TEST, od.Default(), true)
	configurator := network.Configurator(NODE_ID_TEST)
	configurator.TPDO.Enable(1)
	assert.Nil(t, err)
	assert.NotNil(t, remoteNode)
	err = network.WriteRaw(NODE_ID_TEST, 0x2002, 0, []byte{10}, false)
	assert.Nil(t, err)
	time.Sleep(500 * time.Millisecond)
	read := make([]byte, 1)
	remoteNode.SDOClient.ReadRaw(0, 0x2002, 0x0, read)
	assert.Equal(t, node.NODE_RUNNING, remoteNode.GetState())
	assert.Equal(t, []byte{10}, read)
}

func TestRemoteNodeRPDOUsingRemote(t *testing.T) {
	network := CreateNetworkTest()
	networkRemote := CreateNetworkEmptyTest()
	defer network.Disconnect()
	defer networkRemote.Disconnect()
	remoteNode, err := networkRemote.AddRemoteNode(NODE_ID_TEST, od.Default(), false)
	configurator := network.Configurator(NODE_ID_TEST)
	configurator.TPDO.Enable(1)
	assert.Nil(t, err)
	assert.NotNil(t, remoteNode)
	err = network.WriteRaw(NODE_ID_TEST, 0x2002, 0, []byte{10}, false)
	assert.Nil(t, err)
	time.Sleep(500 * time.Millisecond)
	read := make([]byte, 1)
	remoteNode.SDOClient.ReadRaw(0, 0x2002, 0x0, read)
	assert.Equal(t, node.NODE_RUNNING, remoteNode.GetState())
	assert.Equal(t, []byte{10}, read)
}
