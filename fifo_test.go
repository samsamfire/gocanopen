package canopen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFifoWrite(t *testing.T) {
	fifo := NewFifo(100)
	res := fifo.Write([]byte{1, 2, 3, 4, 5}, nil)
	assert.Equal(t, 5, res)
	assert.Equal(t, 5, fifo.writePos)
	assert.Equal(t, 0, fifo.readPos)

	res = fifo.Write(make([]byte, 500), nil)
	assert.Equal(t, 94, res)
	res = fifo.Write([]byte{1}, nil)
	assert.Equal(t, 0, res)

	// Free up some space by reading then re writing
	var eof bool = false
	fifo.Read(make([]byte, 10), &eof)
	res = fifo.Write(make([]byte, 10), nil)
	assert.Equal(t, 10, res)
}

func TestFifoRead(t *testing.T) {
	fifo := NewFifo(100)
	receive_buffer := make([]byte, 10)
	var eof bool = false
	res := fifo.Read(receive_buffer, &eof)
	assert.Equal(t, 0, res)

	// Write to fifo
	res = fifo.Write([]byte{1, 2, 3, 4}, nil)
	assert.Equal(t, 4, res)
	assert.Equal(t, 4, fifo.writePos)
	res = fifo.Read(receive_buffer, &eof)
	assert.Equal(t, 4, res)
}

func TestFifoAltRead(t *testing.T) {
	fifo := NewFifo(101)
	assert.Equal(t, 0, fifo.AltGetOccupied())

	rxBuffer := make([]byte, 7)
	res := fifo.AltRead(rxBuffer)
	assert.Equal(t, 0, res)

	// Write to fifo
	for i := 0; i < 10; i++ {
		res = fifo.Write([]byte("1234567891"), nil)
		assert.Equal(t, 10, res)
	}
	res = fifo.AltRead(rxBuffer)
	assert.Equal(t, 7, res)
	assert.Equal(t, "1234567", string(rxBuffer))
	assert.Equal(t, 93, fifo.AltGetOccupied())
}
