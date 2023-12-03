package canopen

import "testing"

func TestCcittSingle(t *testing.T) {
	crc := CRC16(0)
	crc.ccittSingle(10)
	if crc != 0xA14A {
		t.Errorf("Was expecting 0xA14A, got %x", crc)
	}

}
