package virtual

import (
	"sync"
	"testing"
	"time"

	can "github.com/samsamfire/gocanopen/pkg/can"
	"github.com/stretchr/testify/assert"
)

// CAN server should be running for this to work

var VCAN_CHANNEL string = "localhost:18888"

func newVcan(channel string) *VirtualCanBus {
	canBus, _ := NewVirtualCanBus(channel)
	vcan, _ := canBus.(*VirtualCanBus)
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

type FrameReceiver struct {
	mu     sync.Mutex
	frames []can.Frame
}

func (frameReceiver *FrameReceiver) Handle(frame can.Frame) {
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
	frameReceiver := FrameReceiver{frames: make([]can.Frame, 0)}
	frameReceiver.mu.Lock()
	vcan2.Subscribe(&frameReceiver)
	frameReceiver.mu.Unlock()
	// Send 100 frames from vcan 1 && read 100 frames from vcan2
	// Check order and value
	frame := can.Frame{ID: 0x111, Flags: 0, DLC: 8, Data: [8]byte{0, 1, 2, 3, 4, 5, 6, 7}}
	for i := 0; i < 10; i++ {
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
	frameReceiver := FrameReceiver{frames: make([]can.Frame, 0)}
	vcan1.Subscribe(&frameReceiver)
	frame := can.Frame{ID: 0x111, Flags: 0, DLC: 8, Data: [8]byte{0, 1, 2, 3, 4, 5, 6, 7}}
	vcan1.Send(frame)
	// Tiny sleep
	time.Sleep(time.Millisecond * 10)
	assert.Equal(t, len(frameReceiver.frames), 0)

	// Activate receive own
	vcan1.receiveOwn = true
	vcan1.Send(frame)
	assert.NotEqual(t, len(frameReceiver.frames), 0)
}
