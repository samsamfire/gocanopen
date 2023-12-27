package canopen

import (
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func init() {
	// Set the logger to debug
	log.SetLevel(log.DebugLevel)
}

func TestSDOReadExpedited(t *testing.T) {
	network := createNetwork()
	defer network.Disconnect()
	data := make([]byte, 10)
	for i := 0; i < 8; i++ {
		_, err := network.sdoClient.ReadRaw(NODE_ID_TEST, 0x2001+uint16(i), 0, data)
		assert.Nil(t, err)
	}
}

func TestSDOReadWriteLocal(t *testing.T) {
	network := createNetwork()
	defer network.Disconnect()
	node, err := network.CreateNode(0x55, "testdata/base.eds")
	assert.Nil(t, err)
	localNode := node.(*LocalNode)
	_, err = localNode.SDOclients[0].ReadUint32(0x55, 0x2007, 0x0)
	assert.Nil(t, err)
	err = localNode.SDOclients[0].WriteRaw(0x55, 0x2007, 0x0, uint32(5656), false)
	assert.Nil(t, err)
	val, err := localNode.SDOclients[0].ReadUint32(0x55, 0x2007, 0x0)
	assert.Nil(t, err)
	assert.Equal(t, uint32(5656), val)
}

func TestSDOReadBlock(t *testing.T) {
	network := createNetwork()
	defer network.Disconnect()
	_, err := network.sdoClient.ReadAll(NODE_ID_TEST, 0x1021, 0)
	assert.Nil(t, err)

}

func TestSDOWriteBlock(t *testing.T) {
	network := createNetwork()
	defer network.Disconnect()
	data := []byte("some random string some random string some random string some random string some random stringsome random string some random string")
	node := network.Nodes[NODE_ID_TEST]
	node.GetOD().AddFile(0x3333, "File entry", "./here.txt", os.O_RDWR|os.O_CREATE, os.O_RDWR|os.O_CREATE)
	err := network.sdoClient.WriteRaw(NODE_ID_TEST, 0x3333, 0, data, false)
	assert.Nil(t, err)
}
