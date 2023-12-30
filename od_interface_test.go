package canopen

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func createOD() *ObjectDictionary {
	od := NewOD()
	od.AddVariableType(0x3016, "entry3016", UNSIGNED8, ATTRIBUTE_SDO_RW, "0x10")
	od.AddVariableType(0x3017, "entry3017", UNSIGNED16, ATTRIBUTE_SDO_RW, "0x20")
	od.AddVariableType(0x3018, "entry3018", UNSIGNED32, ATTRIBUTE_SDO_RW, "0x30")
	record := NewRecord()
	record.AddSubObject(0, "sub0", UNSIGNED8, ATTRIBUTE_SDO_RW, "0x11")
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
func TestGetUint(t *testing.T) {
	BaseObjectDictionaryParsed, err := ParseEDSFromFile("testdata/base.eds", 0x10)
	if err != nil {
		t.Error(err)
	}
	entry := BaseObjectDictionaryParsed.Index(0x2003)
	if entry == nil {
		t.Error()
	}

	data, _ := entry.Uint16(0)
	if data != 0x4444 {
		t.Errorf("Wrong value : %x", data)
	}
	_, err = entry.Uint8(0)
	if err != ODR_TYPE_MISMATCH {
		t.Error()
	}

}

// Test reading SDO client parameter entry
func TestReadSDO1280(t *testing.T) {
	od, err := ParseEDSFromFile("testdata/base.eds", 0x10)
	if err != nil {
		t.Fatalf("could not parse eds : %v", err)
	}
	entry := od.Index(0x1280)
	log.Infof("Entry 1280 : %v", entry)
	if entry == nil {
		t.Error()
	}
	_, err = NewStreamer(entry, 0, true)

	if err != nil {
		t.Errorf("Failed to get sub object of 1280 %v", err)
	}

}

// Test reader writer disabled
func TestReadWriteDisabled(t *testing.T) {
	//var streamer ObjectStreamer
	od, err := ParseEDSFromFile("testdata/base.eds", 0x10)
	if err != nil {
		t.Fatal(err)
	}
	entry := od.Index(0x2001)
	if entry == nil {
		t.Fatal("Empty entry")
	}
	extension := Extension{Object: nil, Read: ReadEntryDisabled, Write: WriteEntryDisabled, flagsPDO: [32]uint8{0}}
	entry.Extension = &extension
	streamer, err := NewStreamer(entry, 0, false)
	if err != nil {
		t.Error()
	}
	_, err = streamer.Read([]byte{0})
	if err != ODR_UNSUPP_ACCESS {
		t.Error(err)
	}
	var countWrite uint16
	err = streamer.read(&streamer.stream, []byte{0}, &countWrite)
	if err != ODR_UNSUPP_ACCESS {
		t.Error(err)
	}
}

func TestAddRPDO(t *testing.T) {
	od := NewOD()
	err := od.AddRPDO(1)
	assert.Nil(t, err)
}
