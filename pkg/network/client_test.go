package network

import (
	"os"
	"testing"

	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/stretchr/testify/assert"
)

func TestSDOReadExpedited(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	data := make([]byte, 10)
	for i := range 8 {
		_, err := network.ReadRaw(NodeIdTest, 0x2001+uint16(i), 0, data)
		assert.Nil(t, err)
	}
}

func TestSDOReadWriteLocal(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	localNode, err := network.CreateLocalNode(0x55, od.Default())
	assert.Nil(t, err)
	client := localNode.SDOclients[0]
	_, err = client.ReadUint32(0x55, 0x2007, 0x0)
	assert.Nil(t, err)
	err = client.WriteRaw(0x55, 0x2007, 0x0, uint32(5656), false)
	assert.Nil(t, err)
	val, err := client.ReadUint32(0x55, 0x2007, 0x0)
	assert.Nil(t, err)
	assert.Equal(t, uint32(5656), val)
	_, err = client.ReadUint64(0x55, 0x201B, 0x0)
	assert.Nil(t, err)
	err = client.WriteRaw(0x55, 0x201B, 0x0, uint64(8989), false)
	assert.Nil(t, err)
	val2, err := client.ReadUint64(0x55, 0x201B, 0x0)
	assert.Nil(t, err)
	assert.EqualValues(t, 8989, val2)
}

var result uint64

func BenchmarkSDOReadLocal(b *testing.B) {
	b.StopTimer()
	network := CreateNetworkTest()
	defer network.Disconnect()
	localNode, err := network.CreateLocalNode(0x55, od.Default())
	assert.Nil(b, err)
	client := localNode
	b.StartTimer()
	var value uint64
	for i := 0; i < b.N; i++ {
		value, err = client.ReadUint(0x201B, 0x0)
	}
	result = value
}

func TestSDOReadBlock(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	_, err := network.ReadAll(NodeIdTest, 0x1021, 0)
	assert.Nil(t, err)

}

func TestClientBlock(t *testing.T) {
	network := CreateNetworkTest()
	network2 := CreateNetworkEmptyTest()
	defer network.Disconnect()
	defer network2.Disconnect()
	node := network.controllers[NodeIdTest].GetNode()
	file, err := os.CreateTemp("", "filename")
	assert.Nil(t, err)
	node.GetOD().AddFile(0x3333, "File entry", file.Name(), os.O_RDWR|os.O_CREATE, os.O_RDWR|os.O_CREATE)
	assert.Nil(t, err)

	t.Run("small block", func(t *testing.T) {
		data := []byte(`some random string some random string some random
		string some random string some random 
		string some random string some random string`)
		w, err := network2.SDOClient.NewRawWriter(NodeIdTest, 0x3333, 0, true, 0)
		assert.Nil(t, err)
		n, err := w.Write(data)
		assert.Nil(t, err)
		assert.EqualValues(t, len(data), n)
	})

	t.Run("big block", func(t *testing.T) {
		data := make([]byte, 10_000)
		network2.SDOClient.SetProcessingPeriod(100)
		w, err := network2.SDOClient.NewRawWriter(NodeIdTest, 0x3333, 0, true, 10_000)

		assert.Nil(t, err)
		n, err := w.Write(data)
		assert.Nil(t, err)
		assert.EqualValues(t, 10_000, n)
	})
	t.Run("write file and read back", func(t *testing.T) {
		file, err := os.CreateTemp("", "filename")
		assert.Nil(t, err)
		node.GetOD().AddFile(0x3333, "File entry", file.Name(), os.O_RDWR|os.O_CREATE, os.O_RDWR|os.O_CREATE)
		assert.Nil(t, err)
		data := []byte(`some random string some random string some random
		string some random string some random 
		string some random string some random string`)
		w, err := network2.SDOClient.NewRawWriter(NodeIdTest, 0x3333, 0, true, 0)
		assert.Nil(t, err)
		n, err := w.Write(data)
		assert.Nil(t, err)
		assert.EqualValues(t, len(data), n)
		data2, err := network.ReadAll(NodeIdTest, 0x3333, 0)
		assert.Nil(t, err)
		assert.Equal(t, data, data2)
	})
}
