package canopen

import (
	"testing"
)

var BaseObjectDictionaryParsed ObjectDictionary

func createOD() ObjectDictionary {
	od := NewOD()
	od.entries[0x1016] = NewVariableEntry(0x1016, []byte{0x10, 0x20, 0x10, 0x20}, ODA_SDO_R|ODA_SDO_W)
	od.entries[0x1017] = NewVariableEntry(0x1017, []byte{0x10, 0x20, 0x10, 0x20}, ODA_SDO_R|ODA_SDO_W)
	od.entries[0x1018] = NewVariableEntry(0x1018, []byte{0x10, 0x20, 0x10, 0x20}, ODA_SDO_R|ODA_SDO_W)
	od.entries[0x1019] = NewVariableEntry(0x1019, []byte{0x10, 0x20, 0x10, 0x20}, ODA_SDO_R|ODA_SDO_W)
	od.entries[0x1030] = NewRecordEntry(0x1030, []Record{
		{Variable: Variable{Data: []byte{0x10, 0x20}, Attribute: ODA_SDO_R | ODA_SDO_W}, Subindex: 0},
		{Variable: Variable{Data: []byte{0x10, 0x20}, Attribute: ODA_SDO_R | ODA_SDO_W}, Subindex: 0},
		{Variable: Variable{Data: []byte{0x10, 0x20}, Attribute: ODA_SDO_R | ODA_SDO_W}, Subindex: 0},
	})

	return od
}

func TestFind(t *testing.T) {
	od := createOD()
	entry := od.Find(0x1118)
	if entry != nil {
		t.Errorf("Entry should be nil")

	}

	entry = od.Find(0x1016)
	if entry.Index != 0x1016 {
		t.Errorf("Wrong index %x", entry.Index)
	}

}

func TestSub(t *testing.T) {
	od := createOD()
	entry := od.Find(0x1018)
	if entry == nil {
		t.Errorf("Entry %d should exist", 0x1018)
	}
	streamer := &ObjectStreamer{}
	// Test access to subindex > 1 for variable
	err := entry.Sub(1, true, streamer)
	if err != ODR_SUB_NOT_EXIST {
		t.Errorf("%d", err)
	}
	// Test that subindex 0 returns ODR_OK
	err = entry.Sub(0, true, streamer)
	if err != nil {
		t.Error(err)
	}
	// Test access to subindex 0 of Record should return ODR_OK
	entry = od.Find(0x1030)
	err = entry.Sub(0, true, streamer)
	if err != nil {
		t.Error()
	}
	// Test access to out of range subindex
	err = entry.Sub(10, true, streamer)
	if err != ODR_SUB_NOT_EXIST {
		t.Error()
	}

}

// Test reading OD variables
func TestRead(t *testing.T) {
	BaseObjectDictionaryParsed, err := ParseEDS("base.eds", 0x10)
	if err != nil {
		t.Error(err)
	}
	entry := BaseObjectDictionaryParsed.Find(0x2001)
	if entry == nil {
		t.Error()
	}
	var streamer ObjectStreamer
	err = entry.Sub(0, true, &streamer)

	var data uint16
	entry.ReadUint16(0, &data)
	if data != 0x1555 {
		t.Errorf("Wrong value : %x", data)
	}

}

// Test reader writer disabled
func TestReadWriteDisabled(t *testing.T) {
	//var streamer ObjectStreamer
	BaseObjectDictionaryParsed, _ := ParseEDS("base.eds", 0x10)
	entry := BaseObjectDictionaryParsed.Find(0x2001)
	if entry == nil {
		t.Error("Empty entry")
	}
	extension := Extension{Object: nil, Reader: &DisabledReader{}, Writer: &DisabledWriter{}, flagsPDO: [4]uint8{0, 0, 0, 0}}
	entry.Extension = &extension
	streamer := ObjectStreamer{}
	err := entry.Sub(0, false, &streamer)
	if err != nil {
		t.Error()
	}
	_, err = streamer.Reader.Read([]byte{0})
	if err != ODR_UNSUPP_ACCESS {
		t.Error(err)
	}
	_, err = streamer.Writer.Write([]byte{0})
	if err != ODR_UNSUPP_ACCESS {
		t.Error(err)
	}
}
