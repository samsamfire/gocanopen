package canopen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type frameCounter struct {
	count int
}

func (f *frameCounter) Handle(frame Frame) {
	f.count++
}

func TestBusManagerSubscribeMask(t *testing.T) {
	bm := NewBusManager(nil)
	counter := &frameCounter{}
	cancel, err := bm.Subscribe(0x80, 0x780, false, counter)
	assert.Nil(t, err)
	bm.Handle(Frame{ID: 0x81})
	bm.Handle(Frame{ID: 0xFF})
	bm.Handle(Frame{ID: 0x100})
	assert.Equal(t, 2, counter.count)
	cancel()
	bm.Handle(Frame{ID: 0x81})
	assert.Equal(t, 2, counter.count)
}

func TestBusManagerFrameFlags(t *testing.T) {
	bm := NewBusManager(nil)
	data := &frameCounter{}
	rtr := &frameCounter{}
	_, _ = bm.Subscribe(0x255, 0x7FF, false, data)
	_, _ = bm.Subscribe(0x255, 0x7FF, true, rtr)

	bm.Handle(Frame{ID: 0x255})
	assert.Equal(t, 1, data.count)
	// Remote frame should only go to RTR subscribers
	bm.Handle(Frame{ID: CanRtrFlag | 0x255})
	assert.Equal(t, 1, data.count)
	assert.Equal(t, 1, rtr.count)
	// Extended and error frames with the same lower bits should be ignored
	bm.Handle(Frame{ID: CanEffFlag | 0x18FF0255})
	bm.Handle(Frame{ID: CanErrFlag | 0x255})
	assert.Equal(t, 1, data.count)
	assert.Equal(t, 1, rtr.count)
}
