package network

import (
	"os"
	"sync"
	"testing"
	"time"

	canopen "github.com/samsamfire/gocanopen/v2"
	"github.com/samsamfire/gocanopen/v2/pkg/od"
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
		assert.Nil(b, err)
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
		w, err := network2.NewRawWriter(NodeIdTest, 0x3333, 0, true, 0)
		assert.Nil(t, err)
		n, err := w.Write(data)
		assert.Nil(t, err)
		assert.EqualValues(t, len(data), n)
	})

	t.Run("big block", func(t *testing.T) {
		data := make([]byte, 10_000)
		network2.SetProcessingPeriod(100 * time.Microsecond)
		w, err := network2.NewRawWriter(NodeIdTest, 0x3333, 0, true, 10_000)

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
		w, err := network2.NewRawWriter(NodeIdTest, 0x3333, 0, true, 0)
		assert.Nil(t, err)
		n, err := w.Write(data)
		assert.Nil(t, err)
		assert.EqualValues(t, len(data), n)
		data2, err := network.ReadAll(NodeIdTest, 0x3333, 0)
		assert.Nil(t, err)
		assert.Equal(t, data, data2)
	})
}

type blockSizeSpy struct {
	mu    sync.Mutex
	sizes []uint8
}

// Handle records the block size requested by the client in block upload
// initiate (0xA4) and sub-block confirmation (0xA2) frames.
func (s *blockSizeSpy) Handle(frame canopen.Frame) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch frame.Data[0] {
	case 0xA4:
		s.sizes = append(s.sizes, frame.Data[4])
	case 0xA2:
		s.sizes = append(s.sizes, frame.Data[2])
	}
}

// Some devices only support small blocks : the block size set with
// SetBlockMaxSize has to bound every block requested during an upload.
func TestClientBlockUploadMaxSize(t *testing.T) {
	network := CreateNetworkTest()
	network2 := CreateNetworkEmptyTest()
	defer network.Disconnect()
	defer network2.Disconnect()

	file, err := os.CreateTemp("", "filename")
	assert.Nil(t, err)
	data := make([]byte, 2000)
	for i := range data {
		data[i] = byte(i)
	}
	_, err = file.Write(data)
	assert.Nil(t, err)
	assert.Nil(t, file.Close())
	node := network.controllers[NodeIdTest].GetNode()
	node.GetOD().AddFile(0x3334, "File entry", file.Name(), os.O_RDONLY, os.O_RDONLY)

	spy := &blockSizeSpy{}
	cancel, err := network.Subscribe(0x600+uint32(NodeIdTest), 0x7FF, false, spy)
	assert.Nil(t, err)
	defer cancel()

	network2.SetBlockMaxSize(30)
	read, err := network2.ReadAll(NodeIdTest, 0x3334, 0)
	assert.Nil(t, err)
	assert.Equal(t, data, read)

	spy.mu.Lock()
	defer spy.mu.Unlock()
	// 2000 bytes need several sub-blocks of 30 segments
	assert.Greater(t, len(spy.sizes), 2)
	for _, size := range spy.sizes {
		assert.LessOrEqual(t, size, uint8(30))
	}
}
