package fifo

import "github.com/samsamfire/gocanopen/internal/crc"

// Circular Fifo object used in some modules like SDO client
type Fifo struct {
	buffer     []byte
	writePos   int
	readPos    int
	altReadPos int
	started    bool
	aux        int
}

func NewFifo(size uint16) *Fifo {
	f := &Fifo{
		buffer:     make([]byte, size),
		writePos:   0,
		readPos:    0,
		altReadPos: 0,
		started:    false,
		aux:        0,
	}
	return f
}

func (f *Fifo) Reset() {
	f.readPos = 0
	f.writePos = 0
	f.started = false
}

func (f *Fifo) GetSpace() int {
	sizeLeft := f.readPos - f.writePos - 1
	if sizeLeft < 0 {
		sizeLeft += len(f.buffer)
	}
	return sizeLeft
}

func (f *Fifo) GetOccupied() int {
	sizeOccupied := f.writePos - f.readPos
	if sizeOccupied < 0 {
		sizeOccupied += len(f.buffer)
	}
	return sizeOccupied
}

// Write data to fifo
func (f *Fifo) Write(buffer []byte, crc *crc.CRC16) int {

	if buffer == nil {
		return 0
	}
	writeCounter := 0

	for _, element := range buffer {
		writePosNext := f.writePos + 1
		if writePosNext == f.readPos || (writePosNext == len(f.buffer) && f.readPos == 0) {
			break
		}
		f.buffer[f.writePos] = element
		writeCounter += 1
		if crc != nil {
			crc.Single(element)
		}
		if writePosNext == len(f.buffer) {
			f.writePos = 0

		} else {
			f.writePos += 1
		}

	}
	return writeCounter

}

// Read data from fifo and return number of bytes read
func (f *Fifo) Read(buffer []byte, eof *bool) int {
	var readCounter int = 0
	if buffer == nil {
		return 0
	}
	if eof != nil {
		*eof = false
	}
	if f.readPos == f.writePos || buffer == nil {
		return 0
	}
	for index := range buffer {
		if f.readPos == f.writePos {
			break
		}
		buffer[index] = f.buffer[f.readPos]

		readCounter++
		f.readPos++

		if f.readPos == len(f.buffer) {
			f.readPos = 0
		}
	}
	return readCounter
}

// Alternate begin
func (f *Fifo) AltBegin(offset int) int {
	var i int
	f.altReadPos = f.readPos
	for i = offset; i > 0; i-- {
		if f.altReadPos == f.writePos {
			break
		}
		f.altReadPos++
		if f.altReadPos == len(f.buffer) {
			f.altReadPos = 0
		}
	}
	return offset - i
}

func (f *Fifo) AltFinish(crc *crc.CRC16) {

	if crc == nil {
		f.readPos = f.altReadPos
		return
	}
	for f.readPos != f.altReadPos {
		crc.Single(f.buffer[f.readPos])
		f.readPos++
		if f.readPos == len(f.buffer) {
			f.readPos = 0
		}
	}
}

func (f *Fifo) AltRead(buffer []byte) int {

	readCounter := int(0)
	for index := range buffer {
		if f.altReadPos == f.writePos {
			break
		}
		buffer[index] = f.buffer[f.altReadPos]
		readCounter++
		f.altReadPos++

		if f.altReadPos == len(f.buffer) {
			f.altReadPos = 0
		}
	}
	return readCounter
}

func (f *Fifo) AltGetOccupied() int {
	sizeOccupied := f.writePos - f.altReadPos
	if sizeOccupied < 0 {
		sizeOccupied += len(f.buffer)
	}
	return sizeOccupied
}
