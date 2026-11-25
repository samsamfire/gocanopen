package kvaser

import (
	"testing"

	canopen "github.com/samsamfire/gocanopen/v2"
	"github.com/stretchr/testify/assert"
)

func TestCanlibIdFlags(t *testing.T) {
	for _, frameId := range []uint32{
		0x123,
		0x123 | canopen.CanRtrFlag,
		0x18FF0255 | canopen.CanEffFlag,
		0x18FF0255 | canopen.CanEffFlag | canopen.CanRtrFlag,
	} {
		id, flags := toCanlibId(frameId)
		assert.Equal(t, frameId, fromCanlibId(id, flags))
	}
	id, flags := toCanlibId(0x123)
	assert.Equal(t, uint32(0x123), id)
	assert.Equal(t, uint32(canMSG_STD), flags)
	id, flags = toCanlibId(0x18FF0255 | canopen.CanEffFlag)
	assert.Equal(t, uint32(0x18FF0255), id)
	assert.Equal(t, uint32(canMSG_EXT), flags)
	assert.Equal(t, 0x123|canopen.CanErrFlag, fromCanlibId(0x123, canMSG_STD|canMSG_ERROR_FRAME))
}
