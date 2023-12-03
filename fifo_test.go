package canopen

import "testing"

func TestFifoWrite(t *testing.T) {
	fifo := NewFifo(100)
	res := fifo.Write([]byte{1, 2, 3, 4, 5}, nil)
	if res != 5 {
		t.Errorf("Written only %v", res)
	}
	if fifo.writePos != 5 {
		t.Errorf("Write position is %v", fifo.writePos)
	}
	if fifo.readPos != 0 {
		t.Error()
	}
	res = fifo.Write(make([]byte, 500), nil)
	if res != 94 {
		t.Errorf("Wrote %v", res)
	}
	res = fifo.Write([]byte{1}, nil)
	if res != 0 {
		t.Error()
	}
	// Free up some space by reading then re writing
	var eof bool = false
	fifo.Read(make([]byte, 10), &eof)
	res = fifo.Write(make([]byte, 10), nil)
	if res != 10 {
		t.Error()
	}

}

func TestFifoRead(t *testing.T) {
	fifo := NewFifo(100)
	receive_buffer := make([]byte, 10)
	var eof bool = false
	res := fifo.Read(receive_buffer, &eof)
	if res != 0 {
		t.Error()
	}
	// Write to fifo
	res = fifo.Write([]byte{1, 2, 3, 4}, nil)
	if res != 4 && fifo.writePos != 4 {
		t.Error()
	}
	res = fifo.Read(receive_buffer, &eof)
	if res != 4 {
		t.Errorf("Res is %v", res)
	}
}

func TestFifoAltRead(t *testing.T) {
	fifo := NewFifo(101)
	if fifo.AltGetOccupied() != 0 {
		t.Fatal("fifo should be empty")
	}
	rxBuffer := make([]byte, 7)
	res := fifo.AltRead(rxBuffer)
	if res != 0 {
		t.Error()
	}
	// Write to fifo
	for i := 0; i < 10; i++ {
		res = fifo.Write([]byte("1234567891"), nil)
		if res != 10 {
			t.Fatalf("should be exactly 10, got %v", res)
		}
	}
	res = fifo.AltRead(rxBuffer)
	if res != 7 || string(rxBuffer) != "1234567" {
		t.Fatal("alt read problem")
	}
	if fifo.AltGetOccupied() != 93 {
		t.Fatalf("should be 93 left, instead %v", fifo.AltGetOccupied())
	}
}
