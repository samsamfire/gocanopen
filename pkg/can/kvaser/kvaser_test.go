package kvaser

import (
	"testing"
	"time"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/stretchr/testify/assert"
)

func TestConnect(t *testing.T) {
	kvaser, err := NewKvaserBus("name")
	assert.Nil(t, err)
	assert.NotNil(t, kvaser)
	err = kvaser.Connect(0, OpenAcceptVirtual|OpenOverrideExclusive)
	assert.Nil(t, err)
	err = kvaser.Disconnect()
	assert.Nil(t, err)
}

type listener struct {
	frames []canopen.Frame
}

func (l *listener) Handle(frame canopen.Frame) {
	l.frames = append(l.frames, frame)
}

func TestSendRead(t *testing.T) {
	sender, _ := NewKvaserBus("")
	other, err := NewKvaserBus("")
	other.Connect(0, OpenAcceptVirtual)
	assert.Nil(t, err)
	err = sender.Connect(1, OpenAcceptVirtual|OpenRequireInitAccess)
	assert.Nil(t, err)
	reader, err := NewKvaserBus("")
	assert.Nil(t, err)
	err = reader.Connect(1, OpenAcceptVirtual|OpenNoInitAccess)
	assert.Nil(t, err)
	callback := &listener{frames: make([]canopen.Frame, 0)}
	err = reader.Subscribe(callback)
	assert.Nil(t, err)

	for i := range uint32(100) {
		frame := canopen.NewFrame(i, 0, 8)
		frame.Data[0] = 10 + uint8(i)
		frame.Data[7] = 20 + uint8(i)
		err = sender.Send(frame)
		assert.Nil(t, err)
	}
	time.Sleep(100 * time.Millisecond)
	// Read back all frames
	assert.Len(t, callback.frames, 100)
	for i := range 100 {
		assert.Equal(t, uint8(8), callback.frames[i].DLC)
		assert.Equal(t, uint32(i), callback.frames[i].ID)
	}
}

func TestKvaserError(t *testing.T) {
	// Test correct error
	err := NewKvaserError(-3)
	assert.Equal(t, "Specified device not found (-3)", err.Error())
	// Test out of bounds error
	err = NewKvaserError(-5003)
	assert.Contains(t, err.Error(), "unable to get description")
	// 0 is equivalent to no error
	err = NewKvaserError(0)
	assert.Nil(t, err)
}

func TestUtils(t *testing.T) {
	version := GerVersion()
	assert.NotEqual(t, "0.0", version)
	assert.NotEqual(t, ".", version)
	channels := GetNbChannels()
	assert.NotEqual(t, 0, channels)
}
