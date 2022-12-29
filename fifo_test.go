package canopen

import "testing"

func TestCcitt_single(t *testing.T) {
	crc := CRC16{0}

	res := crc.Ccitt_single(10)
	if res.crc != 0xA14A {
		t.Errorf("Was expecting 0xA14A, got %x", res)
	}

}

// func TestCcitt_block(t *testing.T) {
// 	crc := CRC16{0}
// 	nums := []uint8{0x12, 0x34}

// 	res := crc.Ccitt_block(nums)
// 	if res != 0x1DF8 {
// 		t.Errorf("Was expecting 0x1DF8, got %x", res)
// 	}

// }
