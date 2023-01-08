package canopen

import "testing"

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
	// Test access to subindex > 1 for variable
	result, _ := entry.Sub(1, true)
	if result != ODR_SUB_NOT_EXIST {
		t.Errorf("%d", result)
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
