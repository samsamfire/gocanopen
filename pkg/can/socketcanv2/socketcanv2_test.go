package socketcanv2

import (
	"testing"
	"time"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"
)

func createSocketCanBus() *Bus {
	sock, _ := NewBus("vcan0")
	socketcanbus := sock.(*Bus)
	socketcanbus.Connect()
	socketcanbus.SetReceiveOwn(true)
	return socketcanbus
}

func TestConnectDisconnect(t *testing.T) {
	sock, err := NewBus("vcan0")
	assert.Nil(t, err)
	err = sock.Connect()
	assert.Nil(t, err)
	err = sock.Disconnect()
	assert.Nil(t, err)
	for range 500 {
		err = sock.Connect()
		assert.Nil(t, err)
		err = sock.Disconnect()
		assert.Nil(t, err)
	}
}

func TestDisconnect(t *testing.T) {
	sock, err := NewBus("vcan0")
	assert.Nil(t, err)
	err = sock.Disconnect()
	assert.Nil(t, err)
}

type frameListener struct {
	frames []canopen.Frame
}

func (f *frameListener) Handle(frame canopen.Frame) {
	f.frames = append(f.frames, frame)
}

func TestSendReceive(t *testing.T) {
	can0 := createSocketCanBus()
	can1 := createSocketCanBus()
	defer can0.Disconnect()
	defer can1.Disconnect()

	listener := &frameListener{frames: make([]canopen.Frame, 0)}
	can1.Subscribe(listener)
	for range 500 {
		can0.Send(canopen.NewFrame(0x100, 0, 8))
	}
	time.Sleep(100 * time.Millisecond)
	assert.Len(t, listener.frames, 500)

}

func TestSendReceiveWithReceiveOwn(t *testing.T) {
	listener := &frameListener{frames: make([]canopen.Frame, 0)}
	sock, err := NewBus("vcan0")
	err = sock.Connect()
	defer sock.Disconnect()
	assert.Nil(t, err)
	socketcanbus := sock.(*Bus)
	assert.Nil(t, err)
	err = sock.Send(canopen.NewFrame(0x100, 0, 0))
	assert.Nil(t, err)
	err = sock.Subscribe(listener)
	assert.Nil(t, err)

	// Enable own reception
	err = socketcanbus.SetReceiveOwn(true)
	assert.Nil(t, err)
	time.Sleep(100 * time.Millisecond)
	assert.Len(t, listener.frames, 0)

	for range 500 {
		err := sock.Send(canopen.NewFrame(0x122, 0, 8))
		assert.Nil(t, err)
	}
	time.Sleep(100 * time.Millisecond)
	assert.Len(t, listener.frames, 500)

	// Disable own reception
	listener.frames = make([]canopen.Frame, 0)
	err = socketcanbus.SetReceiveOwn(false)
	assert.Nil(t, err)

	for range 500 {
		err := sock.Send(canopen.NewFrame(0x122, 0, 8))
		assert.Nil(t, err)
	}
	time.Sleep(100 * time.Millisecond)
	assert.Len(t, listener.frames, 0)
}

func TestFilterNoReception(t *testing.T) {
	can0 := createSocketCanBus()
	can1 := createSocketCanBus()
	defer can0.Disconnect()
	defer can1.Disconnect()

	listener := &frameListener{frames: make([]canopen.Frame, 0)}
	can1.Subscribe(listener)
	err := can1.SetFilters([]unix.CanFilter{{Id: 0x50, Mask: 0x7FF}})
	assert.Nil(t, err)
	for range 500 {
		can0.Send(canopen.NewFrame(0x100, 0, 8))
	}
	time.Sleep(100 * time.Millisecond)
	assert.Len(t, listener.frames, 0)
}
