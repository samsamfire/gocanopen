package canopen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncode(t *testing.T) {

	data, err := encode("0x10", UNSIGNED8, 0)
	assert.Nil(t, err)
	assert.EqualValues(t, []byte{0x10}, data)

	data, _ = encode("0x10", UNSIGNED16, 0)
	assert.EqualValues(t, []byte{0x10, 0x00}, data)

	data, _ = encode("0x10", UNSIGNED32, 0)
	assert.EqualValues(t, []byte{0x10, 0x00, 0x00, 0x00}, data)

	data, _ = encode("0x20", INTEGER8, 0)
	assert.EqualValues(t, []byte{0x20}, data)

	data, _ = encode("0x20", INTEGER16, 0)
	assert.EqualValues(t, []byte{0x20, 0x00}, data)

	data, _ = encode("0x20", INTEGER32, 0)
	assert.EqualValues(t, []byte{0x20, 0x00, 0x00, 0x00}, data)

	data, _ = encode("0x1", BOOLEAN, 0)
	assert.EqualValues(t, []byte{0x1}, data)

	_, err = encode("90000", UNSIGNED8, 0)
	assert.NotNil(t, err)

}
