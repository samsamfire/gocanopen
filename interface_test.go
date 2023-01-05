package canopen

import "testing"

func createOD() ObjectDictionary {
	od := ObjectDictionary{
		[]Entry{
			NewVariableEntry(0x1016, []byte{0x10, 0x20, 0x10, 0x20}, ODA_SDO_R|ODA_SDO_W),
			NewVariableEntry(0x1017, []byte{0x10, 0x20, 0x10, 0x20}, ODA_SDO_R|ODA_SDO_W),
			NewVariableEntry(0x1018, []byte{0x10, 0x20, 0x10, 0x20}, ODA_SDO_R|ODA_SDO_W),
			NewVariableEntry(0x1019, []byte{0x10, 0x20, 0x10, 0x20}, ODA_SDO_R|ODA_SDO_W),
			NewVariableEntry(0x1020, []byte{0x10, 0x20, 0x10, 0x20}, ODA_SDO_R|ODA_SDO_W),
			NewRecordEntry(0x1030, []Record{
				Record{Data: []byte{0x10, 0x20}, Subindex: 0, Attribute: ODA_SDO_R | ODA_SDO_W},
				Record{Data: []byte{0x10, 0x20}, Subindex: 1, Attribute: ODA_SDO_R | ODA_SDO_W},
				Record{Data: []byte{0x10, 0x20}, Subindex: 2, Attribute: ODA_SDO_R | ODA_SDO_W},
			}),
		},
	}

	return od
}

func TestFind(t *testing.T) {
	entry1 := NewVariableEntry(0x1016, []byte{0x10, 0x20}, ODA_SDO_R|ODA_SDO_W)
	entry2 := NewVariableEntry(0x1017, []byte{0x10, 0x20}, ODA_SDO_R|ODA_SDO_W)
	entries := make([]Entry, 2)
	entries[0] = entry1
	entries[1] = entry2
	od := ObjectDictionary{entries}
	entry := od.Find(0x1018)
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
	entry := od.Find(0x1020)
	// Test access to subindex > 1 for variable
	result, _ := entry.Sub(1, true)
	if result != ODR_SUB_NOT_EXIST {
		t.Error()
	}
	// Test that subindex 0 returns ODR_OK
	result, _ = entry.Sub(0, true)
	if result != ODR_OK {
		t.Error()
	}
	// Test access to subindex 0 of Record should return ODR_OK
	entry = od.Find(0x1030)
	result, _ = entry.Sub(0, true)
	if result != ODR_OK {
		t.Error()
	}
	// Test access to out of range subindex
	result, _ = entry.Sub(10, true)
	if result != ODR_SUB_NOT_EXIST {
		t.Error()
	}

	// if result != 0 {
	// 	t.Errorf("ODR is not 0 %d", result)
	// }
}
