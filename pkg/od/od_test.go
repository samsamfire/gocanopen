package od

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func createOD() *ObjectDictionary {
	od := NewOD()
	od.AddVariableType(0x3016, "entry3016", UNSIGNED8, AttributeSdoRw, "0x10")
	od.AddVariableType(0x3017, "entry3017", UNSIGNED16, AttributeSdoRw, "0x20")
	od.AddVariableType(0x3018, "entry3018", UNSIGNED32, AttributeSdoRw, "0x30")
	record := NewRecord()
	record.AddSubObject(0, "sub0", UNSIGNED8, AttributeSdoRw, "0x11")
	od.AddVariableList(0x3030, "entry3030", record)
	return od
}

func TestFind(t *testing.T) {
	od := createOD()
	entry := od.Index(0x1118)
	assert.Nil(t, entry)
	entry = od.Index(0x3016)
	assert.NotNil(t, entry)
	variable, err := od.Index(0x3016).SubIndex(0)
	assert.Nil(t, err)
	assert.NotNil(t, variable)
}

// Test reading OD variables
func TestEntryUint(t *testing.T) {
	odParsed := Default()

	entry := odParsed.Index(0x2003)
	assert.NotNil(t, entry)

	data, _ := entry.Uint16(0)
	assert.EqualValues(t, 0x4444, data)

	_, err := entry.Uint8(0)
	assert.Equal(t, ErrTypeMismatch, err)
}

// Test reading SDO client parameter entry
func TestReadSDO1280(t *testing.T) {
	od := Default()
	entry := od.Index(0x1280)
	assert.NotNil(t, entry)
	_, err := NewStreamer(entry, 0, true)
	assert.Nil(t, err)
}

// Test reader writer disabled
func TestReadWriteDisabled(t *testing.T) {
	od := Default()
	entry := od.Index(0x2001)
	assert.NotNil(t, entry)
	extension := extension{object: nil, read: ReadEntryDisabled, write: WriteEntryDisabled, flagsPDO: [32]uint8{0}}
	entry.extension = &extension
	streamer, err := NewStreamer(entry, 0, false)
	assert.Nil(t, err)

	_, err = streamer.Read([]byte{0})
	assert.Equal(t, ErrUnsuppAccess, err)

	var countWrite uint16
	err = streamer.reader(&streamer.Stream, []byte{0}, &countWrite)
	assert.Equal(t, ErrUnsuppAccess, err)
}

func TestAddRPDO(t *testing.T) {
	od := NewOD()
	err := od.AddRPDO(1)
	assert.Nil(t, err)
}

func TestAddReader(t *testing.T) {
	od := NewOD()
	buffer := bytes.NewReader(make([]byte, 10))
	od.AddReader(0x1, "hello", buffer)
}
