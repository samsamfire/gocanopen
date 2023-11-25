package canopen

import (
	"testing"
	"time"
)

// CAN server should be running for this to work

var VCAN_CHANNEL string = "localhost:18888"

func TestSendAndRecv(t *testing.T) {
	vcan1 := NewVirtualCanBus(VCAN_CHANNEL)
	vcan2 := NewVirtualCanBus(VCAN_CHANNEL)
	err1 := vcan1.Connect()
	err2 := vcan2.Connect()
	if err1 != nil || err2 != nil {
		t.Fatal("failed to connect", err1, err2)
	}
	defer vcan1.Disconnect()
	defer vcan2.Disconnect()
	// Send 100 frames from vcan 1 && read 100 frames from vcan2
	// Check order and value
	frame := Frame{0x111, 0, 8, [8]byte{0, 1, 2, 3, 4, 5, 6, 7}}
	for i := 0; i < 100; i++ {
		frame.Data[0] = uint8(i)
		vcan1.Send(frame)
	}
	for i := 0; i < 100; i++ {
		frame, err := vcan2.Recv()
		if err != nil {
			t.Fatal("error reading", err)
		}
		if frame.Data[0] != uint8(i) {
			t.Fatal("unexpected value :", frame.Data[0], "expected", uint8(i))
		}
	}
}

type FrameReceiver struct {
	frames []Frame
}

func (frameReceiver *FrameReceiver) Handle(frame Frame) {
	frameReceiver.frames = append(frameReceiver.frames, frame)
}

func TestSendAndSubscribe(t *testing.T) {
	vcan1 := NewVirtualCanBus(VCAN_CHANNEL)
	vcan2 := NewVirtualCanBus(VCAN_CHANNEL)
	err1 := vcan1.Connect()
	err2 := vcan2.Connect()
	if err1 != nil || err2 != nil {
		t.Fatal("failed to connect", err1, err2)
	}
	frameReceiver := FrameReceiver{frames: make([]Frame, 0)}
	vcan2.Subscribe(&frameReceiver)
	//defer vcan1.Close()
	//defer vcan2.Close()
	// Send 100 frames from vcan 1 && read 100 frames from vcan2
	// Check order and value
	frame := Frame{0x111, 0, 8, [8]byte{0, 1, 2, 3, 4, 5, 6, 7}}
	for i := 0; i < 100; i++ {
		frame.Data[0] = uint8(i)
		vcan1.Send(frame)
	}
	// Tiny sleep
	time.Sleep(time.Millisecond * 100)
	if len(frameReceiver.frames) < 100 {
		t.Fatal("should have received at least 100 frames")
	}
	for i, frame := range frameReceiver.frames {
		if frame.ID == 0x111 && frame.Data[0] != uint8(i) {
			t.Fatal("unexpected value :", frame.Data[0], "expected", uint8(i))
		}
	}
}

func TestReceiveOwn(t *testing.T) {
	vcan1 := NewVirtualCanBus(VCAN_CHANNEL)
	frameReceiver := FrameReceiver{frames: make([]Frame, 0)}
	vcan1.Subscribe(&frameReceiver)
	frame := Frame{0x111, 0, 8, [8]byte{0, 1, 2, 3, 4, 5, 6, 7}}
	vcan1.Send(frame)
	// Tiny sleep
	time.Sleep(time.Millisecond * 10)
	if len(frameReceiver.frames) != 0 {
		t.Fatal("should have received zero frames")
	}
	// Activate receive own
	vcan1.receiveOwn = true
	vcan1.Send(frame)
	if len(frameReceiver.frames) != 1 {
		t.Fatal("should have received own frame")
	}
}
