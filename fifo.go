package canopen

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
	fifo := &Fifo{
		buffer:     make([]byte, size),
		writePos:   0,
		readPos:    0,
		altReadPos: 0,
		started:    false,
		aux:        0,
	}
	return fifo
}

func (fifo *Fifo) Reset() {
	fifo.readPos = 0
	fifo.writePos = 0
	fifo.started = false
}

func (fifo *Fifo) GetSpace() int {
	sizeLeft := fifo.readPos - fifo.writePos - 1
	if sizeLeft < 0 {
		sizeLeft += len(fifo.buffer)
	}
	return sizeLeft
}

func (fifo *Fifo) GetOccupied() int {
	sizeOccupied := fifo.writePos - fifo.readPos
	if sizeOccupied < 0 {
		sizeOccupied += len(fifo.buffer)
	}
	return sizeOccupied
}

// Write data to fifo
func (fifo *Fifo) Write(buffer []byte, crc *crc16) int {

	if buffer == nil {
		return 0
	}
	writeCounter := 0

	for _, element := range buffer {
		writePosNext := fifo.writePos + 1
		if writePosNext == fifo.readPos || (writePosNext == len(fifo.buffer) && fifo.readPos == 0) {
			break
		}
		fifo.buffer[fifo.writePos] = element
		writeCounter += 1
		if crc != nil {
			crc.ccittSingle(element)
		}
		if writePosNext == len(fifo.buffer) {
			fifo.writePos = 0

		} else {
			fifo.writePos += 1
		}

	}
	return writeCounter

}

// Read data from fifo and return number of bytes read
func (fifo *Fifo) Read(buffer []byte, eof *bool) int {
	var readCounter int = 0
	if buffer == nil {
		return 0
	}
	if eof != nil {
		*eof = false
	}
	if fifo.readPos == fifo.writePos || buffer == nil {
		return 0
	}
	for index := range buffer {
		if fifo.readPos == fifo.writePos {
			break
		}
		buffer[index] = fifo.buffer[fifo.readPos]

		readCounter++
		fifo.readPos++

		if fifo.readPos == len(fifo.buffer) {
			fifo.readPos = 0
		}
	}
	return readCounter
}

// Alternate begin
func (fifo *Fifo) AltBegin(offset int) int {
	var i int
	fifo.altReadPos = fifo.readPos
	for i = offset; i > 0; i-- {
		if fifo.altReadPos == fifo.writePos {
			break
		}
		fifo.altReadPos++
		if fifo.altReadPos == len(fifo.buffer) {
			fifo.altReadPos = 0
		}
	}
	return offset - i
}

func (fifo *Fifo) AltFinish(crc *crc16) {

	if crc == nil {
		fifo.readPos = fifo.altReadPos
		return
	}
	for fifo.readPos != fifo.altReadPos {
		crc.ccittSingle(fifo.buffer[fifo.readPos])
		fifo.readPos++
		if fifo.readPos == len(fifo.buffer) {
			fifo.readPos = 0
		}
	}
}

func (fifo *Fifo) AltRead(buffer []byte) int {

	readCounter := int(0)
	for index := range buffer {
		if fifo.altReadPos == fifo.writePos {
			break
		}
		buffer[index] = fifo.buffer[fifo.altReadPos]
		readCounter++
		fifo.altReadPos++

		if fifo.altReadPos == len(fifo.buffer) {
			fifo.altReadPos = 0
		}
	}
	return readCounter
}

func (fifo *Fifo) AltGetOccupied() int {
	sizeOccupied := fifo.writePos - fifo.altReadPos
	if sizeOccupied < 0 {
		sizeOccupied += len(fifo.buffer)
	}
	return sizeOccupied
}
