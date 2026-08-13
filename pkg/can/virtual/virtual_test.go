package virtual

import (
	"net"
	"sync"
	"testing"
	"time"

	canopen "github.com/samsamfire/gocanopen/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CAN server should be running for this to work

var VCAN_CHANNEL string = "localhost:18888"

func newVcan(channel string) *Bus {
	canBus, _ := NewVirtualCanBus(channel)
	vcan, _ := canBus.(*Bus)
	return vcan
}

func TestSendAndRecv(t *testing.T) {
	vcan1 := newVcan(VCAN_CHANNEL)
	vcan2 := newVcan(VCAN_CHANNEL)
	err1 := vcan1.Connect()
	err2 := vcan2.Connect()
	if err1 != nil || err2 != nil {
		t.Fatal("failed to connect", err1, err2)
	}
	defer vcan1.Disconnect()
	defer vcan2.Disconnect()
	// Send 100 frames from vcan 1 && read 100 frames from vcan2
	// Check order and value
	// This test can fail with bad network conditions
	// frame := can.Frame{ID: 0x111, Flags: 0, DLC: 8, Data: [8]byte{0, 1, 2, 3, 4, 5, 6, 7}}
	// for i := 0; i < 100; i++ {
	// 	frame.Data[0] = uint8(i)
	// 	vcan1.Send(frame)
	// }
	// for i := 0; i < 100; i++ {
	// 	frame, err := vcan2.Recv()
	// 	assert.Nil(t, err)
	// 	assert.Equal(t, uint8(i), frame.Data[0])
	// }
}

// Create a bus on top of an in memory connection, the other end is returned so
// that the test can control how the frames are segmented
func newPipedVcan(t *testing.T) (*Bus, net.Conn) {
	t.Helper()
	local, remote := net.Pipe()
	t.Cleanup(func() {
		local.Close()
		remote.Close()
	})
	vcan := newVcan("")
	vcan.conn = local
	return vcan, remote
}

// TCP is a stream, a frame can be delivered in several segments. A short read
// must not be treated as an error, doing so leaves the stream out of sync and
// permanently kills the reception routine.
func TestRecvSegmentedFrame(t *testing.T) {
	vcan, remote := newPipedVcan(t)

	frame := canopen.Frame{ID: 0x111, Flags: 0, DLC: 8, Data: [8]byte{0, 1, 2, 3, 4, 5, 6, 7}}
	frameBytes, err := serializeFrame(frame)
	require.NoError(t, err)

	// Worst case segmentation : one byte at a time, so every read is short
	go func() {
		for _, b := range frameBytes {
			_, _ = remote.Write([]byte{b})
		}
	}()

	received, err := vcan.Recv()
	require.NoError(t, err)
	assert.Equal(t, frame, *received)
}

// Several frames can also be delivered inside a single segment, none of them
// should be lost
func TestRecvCoalescedFrames(t *testing.T) {
	vcan, remote := newPipedVcan(t)

	first := canopen.Frame{ID: 0x111, Flags: 0, DLC: 8, Data: [8]byte{0, 1, 2, 3, 4, 5, 6, 7}}
	second := canopen.Frame{ID: 0x222, Flags: 0, DLC: 2, Data: [8]byte{0xAA, 0xBB}}
	firstBytes, err := serializeFrame(first)
	require.NoError(t, err)
	secondBytes, err := serializeFrame(second)
	require.NoError(t, err)

	go func() {
		_, _ = remote.Write(append(firstBytes, secondBytes...))
	}()

	received, err := vcan.Recv()
	require.NoError(t, err)
	assert.Equal(t, first, *received)
	received, err = vcan.Recv()
	require.NoError(t, err)
	assert.Equal(t, second, *received)
}

// An idle connection is not an error, it must stay usable
func TestRecvNoMessage(t *testing.T) {
	vcan, remote := newPipedVcan(t)

	_, err := vcan.Recv()
	assert.ErrorIs(t, err, ErrNoMsg)

	// The connection is still usable afterwards
	frame := canopen.Frame{ID: 0x111, Flags: 0, DLC: 1, Data: [8]byte{0x42}}
	frameBytes, err := serializeFrame(frame)
	require.NoError(t, err)
	go func() {
		_, _ = remote.Write(frameBytes)
	}()

	received, err := vcan.Recv()
	require.NoError(t, err)
	assert.Equal(t, frame, *received)
}

type FrameReceiver struct {
	mu     sync.Mutex
	frames []canopen.Frame
}

func (frameReceiver *FrameReceiver) Handle(frame canopen.Frame) {
	frameReceiver.mu.Lock()
	defer frameReceiver.mu.Unlock()
	frameReceiver.frames = append(frameReceiver.frames, frame)
}

func TestSendAndSubscribe(t *testing.T) {
	vcan1 := newVcan(VCAN_CHANNEL)
	vcan2 := newVcan(VCAN_CHANNEL)
	defer vcan1.Disconnect()
	defer vcan2.Disconnect()
	err1 := vcan1.Connect()
	err2 := vcan2.Connect()
	if err1 != nil || err2 != nil {
		t.Fatal("failed to connect", err1, err2)
	}
	frameReceiver := FrameReceiver{frames: make([]canopen.Frame, 0)}
	frameReceiver.mu.Lock()
	vcan2.Subscribe(&frameReceiver)
	frameReceiver.mu.Unlock()
	// Send 100 frames from vcan 1 && read 100 frames from vcan2
	// Check order and value
	frame := canopen.Frame{ID: 0x111, Flags: 0, DLC: 8, Data: [8]byte{0, 1, 2, 3, 4, 5, 6, 7}}
	for i := range 10 {
		frame.Data[0] = uint8(i)
		vcan1.Send(frame)
	}
	// Tiny sleep
	time.Sleep(time.Millisecond * 500)
	frameReceiver.mu.Lock()
	defer frameReceiver.mu.Unlock()
	// assert.GreaterOrEqual(t, len(frameReceiver.frames), 10)
	// for i, frame := range frameReceiver.frames {
	// 	assert.EqualValues(t, 0x111, frame.ID)
	// 	assert.EqualValues(t, uint8(i), frame.Data[0])
	// }
}

func TestReceiveOwn(t *testing.T) {
	vcan1 := newVcan(VCAN_CHANNEL)
	defer vcan1.Disconnect()
	frameReceiver := FrameReceiver{frames: make([]canopen.Frame, 0)}
	vcan1.Subscribe(&frameReceiver)
	frame := canopen.Frame{ID: 0x111, Flags: 0, DLC: 8, Data: [8]byte{0, 1, 2, 3, 4, 5, 6, 7}}
	vcan1.Send(frame)
	// Tiny sleep
	time.Sleep(time.Millisecond * 10)
	assert.Equal(t, len(frameReceiver.frames), 0)

	// Activate receive own
	vcan1.receiveOwn = true
	vcan1.Send(frame)
	assert.NotEqual(t, len(frameReceiver.frames), 0)
}
