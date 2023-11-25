package canopen

import (
	"testing"

	log "github.com/sirupsen/logrus"
)

var BaseObjectDictionaryParsed ObjectDictionary

func createOD() ObjectDictionary {
	od := NewOD()
	od.AddVariable(0x1016, "entry1016", Variable{Data: []byte{0x10, 0x20}, Attribute: ODA_SDO_R | ODA_SDO_W})
	od.AddVariable(0x1017, "entry1017", Variable{Data: []byte{0x10, 0x20}, Attribute: ODA_SDO_R | ODA_SDO_W})
	od.AddVariable(0x1018, "entry1018", Variable{Data: []byte{0x10, 0x20}, Attribute: ODA_SDO_R | ODA_SDO_W})
	od.AddRecord(0x1030, "entry1030", []Record{{Variable{Data: []byte{0x10, 0x20}, Attribute: ODA_SDO_R | ODA_SDO_W}, 0}})
	return od
}

func TestFind(t *testing.T) {
	od := createOD()
	entry := od.Index(0x1118)
	if entry != nil {
		t.Errorf("Entry should be nil")

	}

	entry = od.Index(0x1016)
	if entry.Index != 0x1016 {
		t.Errorf("Wrong index %x", entry.Index)
	}

}

func TestSub(t *testing.T) {
	od := createOD()
	entry := od.Index(0x1018)
	if entry == nil {
		t.Errorf("Entry %x should exist", 0x1018)
	}
	// Test access to subindex > 1 for variable
	_, err := entry.CreateStreamer(1, true)
	if err != ODR_SUB_NOT_EXIST {
		t.Errorf("%d", err)
	}
	// Test that subindex 0 returns ODR_OK
	_, err = entry.CreateStreamer(0, true)
	if err != nil {
		t.Error(err)
	}
	// Test access to subindex 0 of Record should return ODR_OK
	entry = od.Index(0x1030)
	_, err = entry.CreateStreamer(0, true)
	if err != nil {
		t.Error()
	}
	// Test access to out of range subindex
	_, err = entry.CreateStreamer(10, true)
	if err != ODR_SUB_NOT_EXIST {
		t.Error()
	}

}

// Test reading OD variables
func TestGetUint(t *testing.T) {
	BaseObjectDictionaryParsed, err := ParseEDS("testdata/base.eds", 0x10)
	if err != nil {
		t.Error(err)
	}
	entry := BaseObjectDictionaryParsed.Index(0x2003)
	if entry == nil {
		t.Error()
	}

	var data uint16
	entry.GetUint16(0, &data)
	if data != 0x4444 {
		t.Errorf("Wrong value : %x", data)
	}
	var data2 uint8
	err = entry.GetUint8(0, &data2)
	if err != ODR_TYPE_MISMATCH {
		t.Error()
	}

}

// Test reading SDO client parameter entry
func TestReadSDO1280(t *testing.T) {
	od, err := ParseEDS("testdata/base.eds", 0x10)
	if err != nil {
		t.Fatalf("could not parse eds : %v", err)
	}
	entry := od.Index(0x1280)
	log.Infof("Entry 1280 : %v", entry)
	if entry == nil {
		t.Error()
	}
	_, err = entry.CreateStreamer(0, true)

	if err != nil {
		t.Errorf("Failed to get sub object of 1280 %v", err)
	}

}

// Test reader writer disabled
func TestReadWriteDisabled(t *testing.T) {
	//var streamer ObjectStreamer
	od, err := ParseEDS("testdata/base.eds", 0x10)
	if err != nil {
		t.Fatal(err)
	}
	entry := od.Index(0x2001)
	if entry == nil {
		t.Error("Empty entry")
	}
	extension := Extension{Object: nil, Read: ReadEntryDisabled, Write: WriteEntryDisabled, flagsPDO: [32]uint8{0}}
	entry.Extension = &extension
	streamer, err := entry.CreateStreamer(0, false)
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
