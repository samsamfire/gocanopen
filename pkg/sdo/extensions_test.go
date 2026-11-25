package sdo

import (
	"encoding/binary"
	"testing"

	canopen "github.com/samsamfire/gocanopen/v2"
	"github.com/samsamfire/gocanopen/v2/pkg/od"
	"github.com/stretchr/testify/assert"
)

// Writing the client COB-IDs in 0x1280 should configure the client accordingly
func TestWriteEntry1280CobIds(t *testing.T) {
	odict := od.Default()
	client, err := NewSDOClient(canopen.NewBusManager(nil), nil, odict, 0x20, DefaultClientTimeout, odict.Index(0x1280))
	assert.Nil(t, err)

	write := func(subindex uint8, cobId uint32) {
		streamer, err := odict.Streamer(0x1280, subindex, false)
		assert.Nil(t, err)
		data := make([]byte, 4)
		binary.LittleEndian.PutUint32(data, cobId)
		_, err = streamer.Write(data)
		assert.Nil(t, err)
	}
	write(1, 0x480)
	write(2, 0x500)
	assert.Equal(t, uint32(0x480), client.cobIdClientToServer)
	assert.Equal(t, uint32(0x500), client.cobIdServerToClient)
	assert.True(t, client.valid)
}
