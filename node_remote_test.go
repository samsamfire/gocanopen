package canopen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateRemoteNode(t *testing.T) {
	network := createNetwork()
	networkRemote := createNetworkEmpty()
	defer network.Disconnect()
	defer networkRemote.Disconnect()
	node, err := networkRemote.AddNode(NODE_ID_TEST, "testdata/base.eds", true)
	assert.Nil(t, err)
	assert.NotNil(t, node)
	err = node.InitPDOs(true)
	assert.Nil(t, err, err)
}

// func TestRemoteNodeRPDO(t *testing.T) {
// 	network := createNetwork()
// 	networkRemote := createNetworkEmpty()
// 	defer network.Disconnect()
// 	defer networkRemote.Disconnect()
// 	node, err := networkRemote.AddNode(NODE_ID_TEST, "testdata/base.eds", true)
// 	assert.Nil(t, err)
// 	assert.NotNil(t, node)
// 	err = node.InitPDOs(true)
// 	assert.Nil(t, err, err)
// 	err = network.WriteRaw(NODE_ID_TEST, 0x2002, 0, []byte{10})
// 	assert.Nil(t, err)
// 	time.Sleep(1 * time.Second)
// 	read := make([]byte, 1)
// 	node.sdoClient.ReadRaw(0, 0x2002, 0x0, read)
// 	assert.Equal(t, NODE_RUNNING, node.GetState())
// 	assert.Equal(t, []byte{10}, read)
// }
