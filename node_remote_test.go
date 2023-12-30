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
