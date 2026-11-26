package network

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"

	canopen "github.com/samsamfire/gocanopen/v2"
	"github.com/samsamfire/gocanopen/v2/pkg/can/virtual"
	"github.com/samsamfire/gocanopen/v2/pkg/nmt"
	"github.com/samsamfire/gocanopen/v2/pkg/od"
	"github.com/samsamfire/gocanopen/v2/pkg/sdo"
	"github.com/stretchr/testify/assert"
)

// A reader with a buffer smaller than what is left in the client fifo when
// the transfer finishes must still get every byte before EOF.
func TestReaderSmallBuffer(t *testing.T) {
	network := CreateNetworkTest()
	network2 := CreateNetworkEmptyTest()
	defer network2.Disconnect()
	defer network.Disconnect()

	file, err := os.CreateTemp("", "filename")
	assert.Nil(t, err)
	data := make([]byte, 2000)
	for i := range data {
		data[i] = byte(i)
	}
	_, err = file.Write(data)
	assert.Nil(t, err)
	assert.Nil(t, file.Close())
	local := network.controllers[NodeIdTest].GetNode()
	local.GetOD().AddFile(0x3335, "File entry", file.Name(), os.O_RDONLY, os.O_RDONLY)

	node, err := network2.AddRemoteNode(NodeIdTest, nil)
	assert.Nil(t, err)
	client := node.SDOClient

	for _, block := range []bool{false, true} {
		rw, err := client.NewRawReader(NodeIdTest, 0x3335, 0, block, 0)
		assert.Nil(t, err)
		read, err := io.ReadAll(iotest.OneByteReader(rw))
		assert.Nil(t, err)
		assert.Equal(t, data, read, "block %v", block)
	}
}

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
// CAN bus losing frames on a busy network. Only sub block frames are droppable.
type lossyBus struct {
	canopen.Bus
	sdoTxCobId uint32
	dropOneIn  int32
	counter    atomic.Int32
}

// Whether the protocol is able to recover from this frame being lost.
// CiA 301 only defines a retransmission mechanism for SDO sub-block segments
//
// The block upload initiate response and end response (both scs 6, so the three
// upper bits of byte 0 set to 0b110) are not retransmitable
func (b *lossyBus) recoverable(frame canopen.Frame) bool {
	if frame.ID != b.sdoTxCobId {
		return true
	}
	return frame.Data[0] != 0x80 && (frame.Data[0]&0xE0) != 0xC0
}

type lossyListener struct {
	bus      *lossyBus
	upstream canopen.FrameListener
}

func (l *lossyListener) Handle(frame canopen.Frame) {
	if l.bus.recoverable(frame) && l.bus.counter.Add(1)%l.bus.dropOneIn == 0 {
		return
	}
	l.upstream.Handle(frame)
}

func (b *lossyBus) Subscribe(cb canopen.FrameListener) error {
	return b.Bus.Subscribe(&lossyListener{bus: b, upstream: cb})
}

func createLossyNetworkTest(nodeId uint8, dropOneIn int32) *Network {
	canBus, _ := NewBus("virtual", "localhost:18888", 0)
	bus := canBus.(*virtual.Bus)
	bus.SetReceiveOwn(true)
	network := NewNetwork(&lossyBus{
		Bus:        bus,
		sdoTxCobId: uint32(sdo.ServerServiceId) + uint32(nodeId),
		dropOneIn:  dropOneIn,
	})
	if err := network.Connect(); err != nil {
		panic(err)
	}
	return &network
}

// A block upload has to survive sub-block segments being lost on the bus : the
// server rewinds its stream and re-transmits the un-acknowledged ones.
func TestSDOBlockUploadWithFrameLoss(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()

	// Read the reference before the lossy client connects : it shares the
	// same SDO channel and would otherwise still be receiving this burst of
	// frames when starting its own transfer.
	reference, err := network.ReadAll(NodeIdTest, 0x1021, 0)
	assert.Nil(t, err)
	assert.NotEmpty(t, reference)

	client := createLossyNetworkTest(NodeIdTest, 100)
	defer client.Disconnect()

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

// A segmented upload should not stop at the server buffer boundary
func TestSDOUploadBigReader(t *testing.T) {
	network := createNetworkBigEntriesTest(t)
	network2 := CreateNetworkEmptyTest()
	defer network2.Disconnect()
	defer network.Disconnect()

	buffer := make([]byte, 4000)
	n, err := network2.ReadRaw(NodeIdTest, 0x3001, 0, buffer)
	assert.Nil(t, err)
	assert.Equal(t, 3000, n)
	assert.True(t, bytes.Equal([]byte(strings.Repeat("b", 3000)), buffer[:n]), "uploaded data does not match entry")
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

// Size indicated by the server on a block upload is 32 bits, it should
// not be truncated to its lower byte
func TestSDOBlockUploadSizeIndicated(t *testing.T) {
	network := createNetworkBigEntriesTest(t)
	network2 := CreateNetworkEmptyTest()
	defer network2.Disconnect()
	defer network.Disconnect()

	data, err := network2.ReadAll(NodeIdTest, 0x3000, 0)
	assert.Nil(t, err)
	assert.Equal(t, []byte(strings.Repeat("a", 1500)), data)
}

// A segmented download bigger than the client fifo should refill
// the fifo instead of sending empty segments forever
func TestSDOSegmentedDownloadBiggerThanFifo(t *testing.T) {
	network := createNetworkBigEntriesTest(t)
	network2 := CreateNetworkEmptyTest()
	defer network2.Disconnect()
	defer network.Disconnect()

	expected := []byte(strings.Repeat("d", 3000))
	done := make(chan error, 1)
	go func() {
		done <- network2.WriteRaw(NodeIdTest, 0x3003, 0, expected, false)
	}()
	select {
	case err := <-done:
		assert.Nil(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("segmented download did not finish")
	}
	local, err := network.Local(NodeIdTest)
	assert.Nil(t, err)
	streamer, err := local.GetOD().Streamer(0x3003, 0, true)
	assert.Nil(t, err)
	assert.Equal(t, expected, streamer.Data)
}

// A segmented download without size indication should not be checked
// against the size of a previous transfer
func TestSDOSegmentedDownloadNoSizeAfterUpload(t *testing.T) {
	network := createNetworkBigEntriesTest(t)
	network2 := CreateNetworkEmptyTest()
	defer network2.Disconnect()
	defer network.Disconnect()

	_, err := network2.ReadUint32(NodeIdTest, 0x2007, 0)
	assert.Nil(t, err)
	w, err := network2.NewRawWriter(NodeIdTest, 0x3002, 0, false, 0)
	assert.Nil(t, err)
	n, err := w.Write([]byte("12345678"))
	assert.Nil(t, err)
	assert.Equal(t, 8, n)
}

// SDO requests received in STOPPED should be discarded, and not
// executed once the node leaves STOPPED
func TestSDORequestsInStoppedDiscarded(t *testing.T) {
	network := CreateNetworkTest()
	network2 := CreateNetworkEmptyTest()
	defer network2.Disconnect()
	defer network.Disconnect()
	network2.SetTimeout(200)

	local, err := network.Local(NodeIdTest)
	assert.Nil(t, err)
	before, err := local.ReadUint32(uint16(0x2007), 0)
	assert.Nil(t, err)

	assert.Nil(t, network2.Command(NodeIdTest, nmt.CommandEnterStopped))
	assert.Eventually(t, func() bool {
		return local.NMT.GetInternalState() == nmt.StateStopped
	}, 1*time.Second, 20*time.Millisecond)
	err = network2.WriteRaw(NodeIdTest, 0x2007, 0, before+1, false)
	assert.Equal(t, sdo.AbortTimeout, err)

	assert.Nil(t, network2.Command(NodeIdTest, nmt.CommandEnterPreOperational))
	assert.Eventually(t, func() bool {
		return local.NMT.GetInternalState() == nmt.StatePreOperational
	}, 1*time.Second, 20*time.Millisecond)
	time.Sleep(300 * time.Millisecond)
	after, err := local.ReadUint32(uint16(0x2007), 0)
	assert.Nil(t, err)
	assert.Equal(t, before, after)
}

// A custom writer can return any error, it should result in an abort
// and not crash the sdo server
func TestSDOExpeditedDownloadCustomError(t *testing.T) {
	odict := od.Default()
	odict.Index(0x2007).AddExtension(nil, od.ReadEntryDefault, func(stream *od.Stream, data []byte) (uint16, error) {
		return 0, errors.New("value refused by application")
	})
	network := CreateNetworkEmptyTest()
	defer network.Disconnect()
	local, err := network.CreateLocalNode(NodeIdTest, odict)
	assert.Nil(t, err)
	assert.Eventually(t, func() bool {
		return local.NMT.GetInternalState() == nmt.StateOperational
	}, 1*time.Second, 20*time.Millisecond)
	network2 := CreateNetworkEmptyTest()
	defer network2.Disconnect()

	err = network2.WriteRaw(NodeIdTest, 0x2007, 0, uint32(1), false)
	assert.Equal(t, sdo.AbortGeneral, err)
}

// A block download with the size indicated should survive a lost segment,
// re-transmitted segments should not be counted twice
func TestSDOBlockDownloadSizeIndicatedWithFrameLoss(t *testing.T) {
	odict := od.Default()
	_, err := odict.AddVariableType(0x3003, "huge variable", od.OCTET_STRING, od.AttributeSdoRw, strings.Repeat("c", 3000))
	assert.Nil(t, err)
	// Server looses the 5th segment of the first sub-block
	network := createDropOnceNetworkTest(func(frame canopen.Frame) bool {
		return frame.ID == uint32(sdo.ClientServiceId)+uint32(NodeIdTest) && frame.Data[0] == 5
	})
	defer network.Disconnect()
	_, err = network.CreateLocalNode(NodeIdTest, odict)
	assert.Nil(t, err)
	network2 := CreateNetworkEmptyTest()
	defer network2.Disconnect()

	expected := []byte(strings.Repeat("d", 3000))
	w, err := network2.NewRawWriter(NodeIdTest, 0x3003, 0, true, uint32(len(expected)))
	assert.Nil(t, err)
	n, err := w.Write(expected)
	assert.Nil(t, err)
	assert.Equal(t, len(expected), n)
	raw, err := odict.Index(0x3003).SubIndex(0)
	assert.Nil(t, err)
	assert.Equal(t, expected, raw.Bytes())
}

// A block download should survive losing the first or the last segment of a
// sub-block : the server acknowledges the last segment received after a timeout
func TestSDOBlockDownloadSubBlockEdgeSegmentLost(t *testing.T) {
	for _, seqno := range []uint8{1, sdo.BlockMaxSize} {
		odict := od.Default()
		_, err := odict.AddVariableType(0x3003, "huge variable", od.OCTET_STRING, od.AttributeSdoRw, strings.Repeat("c", 3000))
		assert.Nil(t, err)
		network := createDropOnceNetworkTest(func(frame canopen.Frame) bool {
			return frame.ID == uint32(sdo.ClientServiceId)+uint32(NodeIdTest) && frame.Data[0] == seqno
		})
		_, err = network.CreateLocalNode(NodeIdTest, odict)
		assert.Nil(t, err)
		network2 := CreateNetworkEmptyTest()

		expected := []byte(strings.Repeat("d", 3000))
		w, err := network2.NewRawWriter(NodeIdTest, 0x3003, 0, true, 0)
		assert.Nil(t, err)
		n, err := w.Write(expected)
		assert.Nil(t, err, "segment %v lost", seqno)
		assert.Equal(t, len(expected), n, "segment %v lost", seqno)
		raw, err := odict.Index(0x3003).SubIndex(0)
		assert.Nil(t, err)
		assert.Equal(t, expected, raw.Bytes(), "segment %v lost", seqno)

		network2.Disconnect()
		network.Disconnect()
	}
}

// A block upload of an entry whose end is only known once it has been read
// (e.g. a reader) should flag the last segment, even if it ends a sub-block
func TestSDOBlockUploadReaderSubBlockBoundary(t *testing.T) {
	payload := bytes.Repeat([]byte("0123456789"), 700)
	odict := od.Default()
	odict.AddReader(0x3001, "reader", bytes.NewReader(payload))
	network := CreateNetworkEmptyTest()
	defer network.Disconnect()
	_, err := network.CreateLocalNode(NodeIdTest, odict)
	assert.Nil(t, err)
	network2 := CreateNetworkEmptyTest()
	defer network2.Disconnect()

	network2.SetBlockMaxSize(100)
	data, err := network2.ReadAll(NodeIdTest, 0x3001, 0)
	assert.Nil(t, err)
	assert.Equal(t, payload, data)
}

// Reading into a buffer smaller than the entry should fail, abort the
// transfer and not prevent the next transfers
func TestSDOReadRawBufferTooSmall(t *testing.T) {
	network := createNetworkBigEntriesTest(t)
	network2 := CreateNetworkEmptyTest()
	defer network2.Disconnect()
	defer network.Disconnect()

	buffer := make([]byte, 10)
	_, err := network2.ReadRaw(NodeIdTest, 0x3003, 0, buffer)
	assert.Equal(t, sdo.AbortDataLong, err)
	// Expedited transfer, the whole value is already received
	_, err = network2.ReadUint8(NodeIdTest, 0x2007, 0)
	assert.Equal(t, sdo.AbortDataLong, err)
	// Next transfer is not affected
	_, err = network2.ReadUint32(NodeIdTest, 0x2007, 0)
	assert.Nil(t, err)
}

// The transfer is finished after a Write, writing again should
// fail instead of silently dropping the data
func TestSDOWriterSecondWrite(t *testing.T) {
	network := createNetworkBigEntriesTest(t)
	network2 := CreateNetworkEmptyTest()
	defer network2.Disconnect()
	defer network.Disconnect()

	expected := []byte(strings.Repeat("d", 3000))
	for _, block := range []bool{false, true} {
		w, err := network2.NewRawWriter(NodeIdTest, 0x3003, 0, block, 0)
		assert.Nil(t, err)
		n, err := w.Write(expected)
		assert.Nil(t, err)
		assert.Equal(t, len(expected), n)
		n, err = w.Write(expected)
		assert.Equal(t, sdo.ErrTransferDone, err, "block %v", block)
		assert.Equal(t, 0, n)
	}
}

// Records the command byte of SDO client requests
type sdoRequestSpy struct {
	commands chan byte
}

func (s *sdoRequestSpy) Handle(frame canopen.Frame) {
	select {
	case s.commands <- frame.Data[0]:
	default:
	}
}

// WriteRaw with forceSegmented must not use an expedited transfer
func TestSDOWriteRawForceSegmented(t *testing.T) {
	network := CreateNetworkTest()
	network2 := CreateNetworkEmptyTest()
	defer network2.Disconnect()
	defer network.Disconnect()

	spy := &sdoRequestSpy{commands: make(chan byte, 100)}
	cancel, err := network2.Subscribe(0x600+uint32(NodeIdTest), 0x7FF, false, spy)
	assert.Nil(t, err)
	defer cancel()

	firstCommand := func() byte {
		select {
		case command := <-spy.commands:
			return command
		case <-time.After(time.Second):
			t.Fatal("no SDO request seen")
			return 0
		}
	}
	drain := func() {
		for len(spy.commands) > 0 {
			<-spy.commands
		}
	}

	for _, force := range []bool{false, true} {
		drain()
		err = network2.WriteRaw(NodeIdTest, 0x2007, 0, uint32(0x12345678), force)
		assert.Nil(t, err)
		// Download initiate : bit 1 is the expedited flag
		expedited := firstCommand()&0x02 != 0
		assert.Equal(t, !force, expedited, "forceSegmented=%v", force)
		value, err := network2.ReadUint32(NodeIdTest, 0x2007, 0)
		assert.Nil(t, err)
		assert.EqualValues(t, 0x12345678, value)
	}
}
