package network

import (
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestReaderWriter(t *testing.T) {
	network := CreateNetworkTest()
	network2 := CreateNetworkEmptyTest()
	defer network2.Disconnect()
	defer network.Disconnect()
	node, err := network2.AddRemoteNode(NodeIdTest, nil)
	assert.Nil(t, err)
	client := node.SDOClient
	rw, err := client.NewRawReader(NodeIdTest, 0x2001, 0, false, 0)
	assert.Nil(t, err)
	buffer := make([]byte, 10)
	n, err := rw.Read(buffer)
	assert.Equal(t, io.EOF, err)
	assert.EqualValues(t, 1, n)
	// Attempt to re-read should result in EOF
	n, err = rw.Read(buffer)
	assert.EqualValues(t, 0, n)
	assert.Equal(t, io.EOF, err)
	buffer = make([]byte, 4)
	rw, err = client.NewRawReader(NodeIdTest, 0x2003, 0, false, 0)
	assert.Nil(t, err)
	// Attempt to read 4 bytes, but only 2 in reality
	n, err = io.ReadFull(rw, buffer)
	assert.EqualValues(t, io.ErrUnexpectedEOF, err)
	assert.Equal(t, 2, n)
	// Attempt to write corrrect length (1 byte)
	time.Sleep(1 * time.Second)
	w, err := client.NewRawWriter(NodeIdTest, 0x2001, 0, false, 1)
	assert.Nil(t, err)
	n, err = w.Write([]byte{0})
	assert.Nil(t, err)
	assert.Equal(t, 1, n)
	// Attempt to write in two times
	w, err = client.NewRawWriter(NodeIdTest, 0x2003, 0, true, 2)
	assert.Nil(t, err)
	n, err = w.Write([]byte{0, 1})
	assert.Nil(t, err)
	assert.Equal(t, 2, n)
}

func BenchmarkNodeStreamerWriter(b *testing.B) {
	b.StopTimer()
	network := CreateNetworkTest()
	local, err := network.Local(NodeIdTest)
	assert.Nil(b, err)
	assert.NotNil(b, local)
	b.StartTimer()
	for n := 0; n < b.N; n++ {
		value, err := local.ReadUint(0x2007, 0)
		assert.Nil(b, err)
		assert.NotEqual(b, 0, value)
	}
}
