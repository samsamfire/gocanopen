package sdo

import (
	"bytes"
	"sync"
	"testing"

	canopen "github.com/samsamfire/gocanopen/v2"
	"github.com/samsamfire/gocanopen/v2/pkg/od"
	"github.com/stretchr/testify/assert"
)

// Bus recording sent frames
type recordBus struct {
	mu     sync.Mutex
	frames []canopen.Frame
}

func (b *recordBus) Connect(...any) error                  { return nil }
func (b *recordBus) Disconnect() error                     { return nil }
func (b *recordBus) Subscribe(canopen.FrameListener) error { return nil }
func (b *recordBus) Send(frame canopen.Frame) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.frames = append(b.frames, frame)
	return nil
}

// A read error while sending a block upload sub-block must be returned, so
// that the server aborts the transfer. The server holds exactly one sub-block,
// so it reads again while sending the last segment, and fails there.
func TestUploadSubBlockReadErrorReturned(t *testing.T) {
	odict := od.Default()
	bus := &recordBus{}
	server, err := NewSDOServer(canopen.NewBusManager(bus), nil, odict, 0x10, 1000, odict.Index(0x1200))
	assert.Nil(t, err)

	entry := odict.Index(0x2007)
	entry.AddExtension(nil, od.ReadEntryDisabled, od.WriteEntryDisabled)
	streamer, err := od.NewStreamer(entry, 0, false)
	assert.Nil(t, err)

	server.streamer = &streamer
	server.buf.Reset()
	server.buf.Write(bytes.Repeat([]byte{0xAA}, int(BlockMaxSize)*BlockSeqSize))
	server.finished = false
	server.sizeIndicated = 0
	server.blockSize = BlockMaxSize
	server.blockSequenceNb = 0
	server.state = stateUploadBlkSubblockSreq

	err = server.processOutgoing()
	assert.Equal(t, ConvertOdToSdoAbort(od.ErrUnsuppAccess), err)
	// Last segment of the sub-block could not be sent
	assert.Len(t, bus.frames, int(BlockMaxSize)-1)
}
