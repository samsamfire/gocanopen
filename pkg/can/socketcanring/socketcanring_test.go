//go:build linux

package socketcanring

import (
	"os"
	"sync"
	"testing"
	"time"

	canopen "github.com/samsamfire/gocanopen/v2"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"
)

type frameListener struct {
	mu     sync.Mutex
	frames []canopen.Frame
}

func (f *frameListener) Handle(frame canopen.Frame) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.frames = append(f.frames, frame)
}

func (f *frameListener) Frames() []canopen.Frame {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]canopen.Frame{}, f.frames...)
}

// Needs a vcan0 interface
func newBusOrSkip(t *testing.T) canopen.Bus {
	t.Helper()
	if os.Getenv("CI") != "" {
		t.Skip("test not run in CI environment because of github limitations")
	}
	bus, err := NewBus("vcan0")
	if err != nil {
		t.Skipf("vcan0 interface not available, skipping test : %v", err)
	}
	return bus
}

// Send a frame from a separate raw CAN socket
func sendRaw(t *testing.T, id uint32, data byte) {
	t.Helper()
	fd, err := unix.Socket(unix.AF_CAN, unix.SOCK_RAW, unix.CAN_RAW)
	assert.Nil(t, err)
	defer unix.Close(fd)
	iface, err := NewBus("vcan0")
	assert.Nil(t, err)
	defer iface.Disconnect()
	assert.Nil(t, unix.Bind(fd, &unix.SockaddrCAN{Ifindex: iface.(*Bus).ifindex}))
	frame := []byte{byte(id), byte(id >> 8), byte(id >> 16), byte(id >> 24), 1, 0, 0, 0, data, 0, 0, 0, 0, 0, 0, 0}
	_, err = unix.Write(fd, frame)
	assert.Nil(t, err)
}

// The bus should receive frames from other nodes, also after
// being disconnected and connected again
func TestReceiveAndReconnect(t *testing.T) {
	bus := newBusOrSkip(t)
	listener := &frameListener{}
	assert.Nil(t, bus.Subscribe(listener))
	assert.Nil(t, bus.Connect())
	sendRaw(t, 0x123, 0xAA)
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, []canopen.Frame{{ID: 0x123, DLC: 1, Data: [8]byte{0xAA}}}, listener.Frames())

	assert.Nil(t, bus.Disconnect())
	assert.Nil(t, bus.Connect())
	defer bus.Disconnect()
	sendRaw(t, 0x124, 0xBB)
	time.Sleep(100 * time.Millisecond)
	frames := listener.Frames()
	assert.Len(t, frames, 2)
	assert.Equal(t, uint32(0x124), frames[len(frames)-1].ID)
}

// Frames sent by the bus should only be received if enabled
func TestReceiveOwn(t *testing.T) {
	bus := newBusOrSkip(t)
	listener := &frameListener{}
	assert.Nil(t, bus.Subscribe(listener))
	assert.Nil(t, bus.Connect())
	defer bus.Disconnect()

	for range 10 {
		assert.Nil(t, bus.Send(canopen.Frame{ID: 0x111, DLC: 1, Data: [8]byte{0x11}}))
	}
	sendRaw(t, 0x111, 0x11)
	time.Sleep(100 * time.Millisecond)
	// Only the frame from another socket, even if identical
	assert.Len(t, listener.Frames(), 1)

	assert.Nil(t, bus.(*Bus).SetReceiveOwn(true))
	assert.Nil(t, bus.Send(canopen.Frame{ID: 0x222, DLC: 1}))
	time.Sleep(100 * time.Millisecond)
	frames := listener.Frames()
	assert.Len(t, frames, 2)
	assert.Equal(t, uint32(0x222), frames[len(frames)-1].ID)
}

// Only the frames matching a filter should be received
func TestFilters(t *testing.T) {
	bus := newBusOrSkip(t)
	listener := &frameListener{}
	assert.Nil(t, bus.Subscribe(listener))
	assert.Nil(t, bus.Connect())
	defer bus.Disconnect()

	assert.Nil(t, bus.(*Bus).SetFilters([]unix.CanFilter{{Id: 0x50, Mask: 0x7FF}}))
	sendRaw(t, 0x100, 0)
	sendRaw(t, 0x50, 0)
	time.Sleep(100 * time.Millisecond)
	frames := listener.Frames()
	assert.Len(t, frames, 1)
	assert.Equal(t, uint32(0x50), frames[0].ID)

	// Inverted filter
	assert.Nil(t, bus.(*Bus).SetFilters([]unix.CanFilter{{Id: 0x50 | unix.CAN_INV_FILTER, Mask: 0x7FF}}))
	sendRaw(t, 0x50, 0)
	sendRaw(t, 0x100, 0)
	time.Sleep(100 * time.Millisecond)
	frames = listener.Frames()
	assert.Len(t, frames, 2)
	assert.Equal(t, uint32(0x100), frames[len(frames)-1].ID)
}
