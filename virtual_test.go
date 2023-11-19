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
	defer vcan1.Close()
	defer vcan2.Close()
	// Send 100 frames from vcan 1 && read 100 frames from vcan2
	// Check order and value
	bufferFrame := BufferTxFrame{0x111, 8, [8]byte{0, 1, 2, 3, 4, 5, 6, 7}, false, false, 0}
	for i := 0; i < 100; i++ {
		bufferFrame.Data[0] = uint8(i)
		vcan1.Send(bufferFrame)
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
	defer vcan2.Close()
	// Send 100 frames from vcan 1 && read 100 frames from vcan2
	// Check order and value
	bufferFrame := BufferTxFrame{0x111, 8, [8]byte{0, 1, 2, 3, 4, 5, 6, 7}, false, false, 0}
	for i := 0; i < 100; i++ {
		bufferFrame.Data[0] = uint8(i)
		vcan1.Send(bufferFrame)
	}
	// Tiny sleep
	time.Sleep(time.Millisecond * 100)
	if len(frameReceiver.frames) != 100 {
		t.Fatal("should have received 100 frames got ", len(frameReceiver.frames))
	}
	for i, frame := range frameReceiver.frames {
		if frame.Data[0] != uint8(i) {
			t.Fatal("unexpected value :", frame.Data[0], "expected", uint8(i))
		}
	}
	time.Sleep(time.Second * 4)
}
