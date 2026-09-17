package network

import (
	"io"
	"sync/atomic"
	"testing"
	"time"

	canopen "github.com/samsamfire/gocanopen/v2"
	"github.com/samsamfire/gocanopen/v2/pkg/can/virtual"
	"github.com/samsamfire/gocanopen/v2/pkg/sdo"
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

func TestTransferAfterTimeout(t *testing.T) {
	network := CreateNetworkTest()
	network2 := CreateNetworkEmptyTest()
	defer network2.Disconnect()
	defer network.Disconnect()
	node, err := network2.AddRemoteNode(NodeIdTest, nil)
	assert.Nil(t, err)
	client := node.SDOClient

	// No server on this node id ==> transfer times out
	buffer := make([]byte, 10)
	_, err = client.ReadRaw(NodeIdTest+10, 0x2001, 0, buffer)
	assert.Equal(t, sdo.AbortTimeout, err)

	// Following transfers on an existing node should still work
	for range 2 {
		n, err := client.ReadRaw(NodeIdTest, 0x2001, 0, buffer)
		assert.Nil(t, err)
		assert.EqualValues(t, 1, n)
	}
	err = client.WriteRaw(NodeIdTest, 0x2001, 0, uint8(0), false)
	assert.Nil(t, err)
}

const blockTransferString = "AStringCannotBeLongerThanTheDefaultValue"

func TestBlockTransferAfterTimeout(t *testing.T) {
	network := CreateNetworkTest()
	network2 := CreateNetworkEmptyTest()
	defer network2.Disconnect()
	defer network.Disconnect()
	node, err := network2.AddRemoteNode(NodeIdTest, nil)
	assert.Nil(t, err)
	client := node.SDOClient
	// Keep the test short, the timers are only checked relative to this
	client.SetTimeout(200)
	client.SetTimeoutBlockTransfer(200)
	timeout := 200 * time.Millisecond

	t.Run("block upload", func(t *testing.T) {
		// No server on this node id ==> transfer times out
		for range 2 {
			start := time.Now()
			_, err := client.ReadAll(NodeIdTest+10, 0x2009, 0)
			assert.Equal(t, sdo.AbortTimeout, err)
			assert.Greater(t, time.Since(start), timeout/2)
		}
		// Block uploads on an existing node should still work
		for range 2 {
			data, err := client.ReadAll(NodeIdTest, 0x2009, 0)
			assert.Nil(t, err)
			assert.EqualValues(t, blockTransferString, string(data))
		}
	})

	t.Run("block download", func(t *testing.T) {
		for range 2 {
			start := time.Now()
			w, err := client.NewRawWriter(NodeIdTest+10, 0x2009, 0, true, uint32(len(blockTransferString)))
			assert.Nil(t, err)
			_, err = w.Write([]byte(blockTransferString))
			assert.Equal(t, sdo.AbortTimeout, err)
			assert.Greater(t, time.Since(start), timeout/2)
		}
		// Block downloads on an existing node should still work
		for range 2 {
			w, err := client.NewRawWriter(NodeIdTest, 0x2009, 0, true, uint32(len(blockTransferString)))
			assert.Nil(t, err)
			n, err := w.Write([]byte(blockTransferString))
			assert.Nil(t, err)
			assert.Equal(t, len(blockTransferString), n)
		}
	})
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

// lossyBus drops one frame every dropOneIn received frames, to emulate a real
// CAN bus losing frames on a busy network.
type lossyBus struct {
	canopen.Bus
	dropOneIn int32
	counter   int32
}

type lossyListener struct {
	bus      *lossyBus
	upstream canopen.FrameListener
}

func (l *lossyListener) Handle(frame canopen.Frame) {
	if atomic.AddInt32(&l.bus.counter, 1)%l.bus.dropOneIn == 0 {
		return
	}
	l.upstream.Handle(frame)
}

func (b *lossyBus) Subscribe(cb canopen.FrameListener) error {
	return b.Bus.Subscribe(&lossyListener{bus: b, upstream: cb})
}

func createLossyNetworkTest(dropOneIn int32) *Network {
	canBus, _ := NewBus("virtual", "localhost:18888", 0)
	bus := canBus.(*virtual.Bus)
	bus.SetReceiveOwn(true)
	network := NewNetwork(&lossyBus{Bus: bus, dropOneIn: dropOneIn})
	if err := network.Connect(); err != nil {
		panic(err)
	}
	return &network
}

// A block upload has to survive frames being lost on the bus : the server
// rewinds its stream and re-transmits the un-acknowledged segments.
func TestSDOBlockUploadWithFrameLoss(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	client := createLossyNetworkTest(100)
	defer client.Disconnect()

	reference, err := network.ReadAll(NodeIdTest, 0x1021, 0)
	assert.Nil(t, err)
	assert.NotEmpty(t, reference)

	for i := range 5 {
		received, err := client.ReadAll(NodeIdTest, 0x1021, 0)
		assert.Nil(t, err, "transfer %v failed", i)
		assert.Equal(t, reference, received, "transfer %v returned corrupted data", i)
	}
}

// Create a network with a local node whose object dictionary contains entries
// that are bigger than the sdo server internal buffers
func createNetworkBigEntriesTest(t *testing.T) *Network {
	t.Helper()
	odict := od.Default()
	// Bigger than the server intermediate buffer
	_, err := odict.AddVariableType(0x3000, "big variable", od.OCTET_STRING, od.AttributeSdoRw, strings.Repeat("a", 1500))
	assert.Nil(t, err)
	odict.AddReader(0x3001, "big reader", bytes.NewReader([]byte(strings.Repeat("b", 3000))))
	// Even size, too big for an expedited transfer
	_, err = odict.AddVariableType(0x3002, "medium variable", od.OCTET_STRING, od.AttributeSdoRw, "abcdefgh")
	assert.Nil(t, err)
	// Bigger than the server internal buffer
	_, err = odict.AddVariableType(0x3003, "huge variable", od.OCTET_STRING, od.AttributeSdoRw, strings.Repeat("c", 3000))
	assert.Nil(t, err)

	network := CreateNetworkEmptyTest()
	_, err = network.CreateLocalNode(NodeIdTest, odict)
	assert.Nil(t, err)
	return network
}
// A download into an entry that is bigger than the server buffer, the server
// has to write it to the object dictionary in several chunks
func TestSDODownloadBigVariable(t *testing.T) {
	network := createNetworkBigEntriesTest(t)
	network2 := CreateNetworkEmptyTest()
	defer network2.Disconnect()
	defer network.Disconnect()

	expected := []byte(strings.Repeat("d", 3000))
	w, err := network2.NewRawWriter(NodeIdTest, 0x3003, 0, true, uint32(len(expected)))
	assert.Nil(t, err)
	n, err := w.Write(expected)
	assert.Nil(t, err)
	assert.Equal(t, len(expected), n)

	local, err := network.Local(NodeIdTest)
	assert.Nil(t, err)
	streamer, err := local.GetOD().Streamer(0x3003, 0, true)
	assert.Nil(t, err)
	assert.True(t, bytes.Equal(expected, streamer.Data), "entry does not contain the downloaded data")
}
// dropOnceBus drops the first received frame matching match, to emulate
// a single frame being lost on the bus
type dropOnceBus struct {
	canopen.Bus
	match   func(frame canopen.Frame) bool
	dropped atomic.Bool
}

type dropOnceListener struct {
	bus      *dropOnceBus
	upstream canopen.FrameListener
}

func (l *dropOnceListener) Handle(frame canopen.Frame) {
	if l.bus.match(frame) && l.bus.dropped.CompareAndSwap(false, true) {
		return
	}
	l.upstream.Handle(frame)
}

func (b *dropOnceBus) Subscribe(cb canopen.FrameListener) error {
	return b.Bus.Subscribe(&dropOnceListener{bus: b, upstream: cb})
}

func createDropOnceNetworkTest(match func(frame canopen.Frame) bool) *Network {
	canBus, _ := NewBus("virtual", "localhost:18888", 0)
	bus := canBus.(*virtual.Bus)
	bus.SetReceiveOwn(true)
	network := NewNetwork(&dropOnceBus{Bus: bus, match: match})
	if err := network.Connect(); err != nil {
		panic(err)
	}
	return &network
}

// A block upload has to survive a lost segment, also when the entry is small
// enough to be read in one go : the server rewinds its stream and re-transmits
// the un-acknowledged segments.
func TestSDOBlockUploadSmallEntryWithFrameLoss(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	// Lose the second segment of the first sub-block
	client := createDropOnceNetworkTest(func(frame canopen.Frame) bool {
		return frame.ID == uint32(sdo.ServerServiceId)+uint32(NodeIdTest) && frame.Data[0] == 2
	})
	defer client.Disconnect()

	// 0x2009 is 40 bytes, it fits inside the server buffer
	received, err := client.ReadAll(NodeIdTest, 0x2009, 0)
	assert.Nil(t, err)
	assert.Equal(t, blockTransferString, string(received))
}

// An expedited download into a string entry : the null terminators that the
// server adds should not be taken from the received frame
func TestSDOExpeditedDownloadString(t *testing.T) {
	network := CreateNetworkTest()
	network2 := CreateNetworkEmptyTest()
	defer network2.Disconnect()
	defer network.Disconnect()

	// 0x2009 is a 40 byte VISIBLE_STRING, 4 bytes or less are sent expedited
	err := network2.WriteRaw(NodeIdTest, 0x2009, 0, []byte("abcd"), false)
	assert.Nil(t, err)
	data, err := network2.ReadAll(NodeIdTest, 0x2009, 0)
	assert.Nil(t, err)
	assert.Equal(t, "abcd", string(data))
}

// An upload of an entry that is bigger than the server intermediate buffer
func TestSDOUploadBigVariable(t *testing.T) {
	network := createNetworkBigEntriesTest(t)
	network2 := CreateNetworkEmptyTest()
	defer network2.Disconnect()
	defer network.Disconnect()

	buffer := make([]byte, 2000)
	n, err := network2.ReadRaw(NodeIdTest, 0x3000, 0, buffer)
	assert.Nil(t, err)
	assert.Equal(t, 1500, n)
	assert.True(t, bytes.Equal([]byte(strings.Repeat("a", 1500)), buffer[:n]), "uploaded data does not match entry")
}
