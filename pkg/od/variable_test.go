package od

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var supportedTypes []uint8 = []uint8{
	BOOLEAN,
	UNSIGNED8,
	UNSIGNED16,
	UNSIGNED32,
	UNSIGNED64,
	INTEGER8,
	INTEGER16,
	INTEGER32,
	INTEGER64,
	REAL32,
	REAL64,
}

func TestEncode(t *testing.T) {

	data, err := EncodeFromString("0x10", UNSIGNED8, 0)
	assert.Nil(t, err)
	assert.EqualValues(t, []byte{0x10}, data)

	data, _ = EncodeFromString("0x10", UNSIGNED16, 0)
	assert.EqualValues(t, []byte{0x10, 0x00}, data)

	data, _ = EncodeFromString("0x10", UNSIGNED32, 0)
	assert.EqualValues(t, []byte{0x10, 0x00, 0x00, 0x00}, data)

	data, _ = EncodeFromString("0x20", INTEGER8, 0)
	assert.EqualValues(t, []byte{0x20}, data)

	data, _ = EncodeFromString("0x20", INTEGER16, 0)
	assert.EqualValues(t, []byte{0x20, 0x00}, data)

	data, _ = EncodeFromString("0x20", INTEGER32, 0)
	assert.EqualValues(t, []byte{0x20, 0x00, 0x00, 0x00}, data)

	data, _ = EncodeFromString("0x1", BOOLEAN, 0)
	assert.EqualValues(t, []byte{0x1}, data)

	_, err = EncodeFromString("90000", UNSIGNED8, 0)
	assert.NotNil(t, err)

	_, err = EncodeFromString("0.01", REAL32, 0)
	assert.Nil(t, err)

	_, err = EncodeFromString("0.01", REAL64, 0)
	assert.Nil(t, err)

	data, err = EncodeFromString("-20", INTEGER8, 0)
	assert.Nil(t, err)
	assert.EqualValues(t, []byte{0xec}, data)

	data, err = EncodeFromString("-20", INTEGER16, 0)
	assert.Nil(t, err)
	assert.EqualValues(t, []byte{0xec, 0xff}, data)

	data, err = EncodeFromString("-20", INTEGER32, 0)
	assert.Nil(t, err)
	assert.EqualValues(t, []byte{0xec, 0xff, 0xff, 0xff}, data)

	data, err = EncodeFromString("-20", INTEGER64, 0)
	assert.Nil(t, err)
	assert.EqualValues(t, []byte{0xec, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, data)

	data, err = EncodeFromGeneric(uint8(0))
	assert.Equal(t, []byte{uint8(0)}, data)
	assert.Nil(t, err)

}

func TestEncodeEmpty(t *testing.T) {
	for _, datatype := range supportedTypes {
		data, err := EncodeFromString("", datatype, 0)
		assert.Nil(t, err)
		// Check corresponding byte array is all 0
		for _, value := range data {
			assert.Equal(t, byte(0), value)
		}
	}
}
