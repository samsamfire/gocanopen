package od

import (
	"sync"

	log "github.com/sirupsen/logrus"
)

// A Stream object is used for streaming data from / to an OD entry.
// It is meant to be used inside of a [StreamReader] or [StreamWriter] function
// and provides low level access for defining custom behaviour when reading
// or writing to an OD entry.
type Stream struct {
	// Mutex used for synchronizing OD access
	mu *sync.RWMutex
	// The actual corresponding data stored inside of OD
	Data []byte
	// This is used to keep track of how much has been written or read.
	// It is typically used for long running transfers i.e. block transfers.
	DataOffset uint32
	// The actual length of the data inside of the OD. This can be different
	// from len(Data) when manipulating data with varying sizes like strings
	// or buffers.
	DataLength uint32
	// A custom object that can be used when using a custom extension
	// see [AddExtension]
	Object any
	// The OD attribute of the entry inside OD. e.g. ATTRIBUTE_SDO_R
	Attribute uint8
	// The subindex of this OD entry. For a VAR type this is always 0.
	Subindex uint8
}

// A StreamReader is a function that reads from a [Stream] object and
// updates the countRead and the read slice with the read bytes
type StreamReader func(stream *Stream, read []byte, countRead *uint16) error

// A StreamWriter is a function that writes to a [Stream] object
// using the to_write slice and updates countWritten
type StreamWriter func(stream *Stream, to_write []byte, countWritten *uint16) error

// extension object, is used for extending functionnality of an OD entry
// This package has some pre-made extensions for CiA defined entries
type extension struct {
	object   any          // Any object to link with extension
	read     StreamReader // A [StreamReader] that will be called when reading entry
	write    StreamWriter // A [StreamWriter] that will be called when writing to entry
	flagsPDO [OD_FLAGS_PDO_SIZE]uint8
}

// Streamer is created before accessing an OD entry
// It creates a buffer from OD Data []byte slice and provides a default reader
// and a default writer
type Streamer struct {
	Stream
	reader StreamReader
	writer StreamWriter
}

// Implements io.Reader
func (s *Streamer) Read(b []byte) (n int, err error) {
	countRead := uint16(0)
	err = s.reader(&s.Stream, b, &countRead)
	return int(countRead), err
}

// Implements io.Writer
func (s *Streamer) Write(b []byte) (n int, err error) {
	countWritten := uint16(0)
	err = s.writer(&s.Stream, b, &countWritten)
	return int(countWritten), err
}

// Return streamer writer
func (s *Streamer) Writer() StreamWriter {
	return s.writer
}

// Return streamer reader
func (s *Streamer) Reader() StreamReader {
	return s.reader
}

// Sets a new streamer writer
func (s *Streamer) SetWriter(writer StreamWriter) {
	s.writer = writer
}

// Sets a new streamer reader
func (s *Streamer) SetReader(reader StreamReader) {
	s.reader = reader
}

// Returns True if has the specific OD attribute
func (s *Streamer) HasAttribute(attribute uint8) bool {
	return (s.Attribute & attribute) != 0
}

func (s *Streamer) ResetData(size uint32, offset uint32) {
	s.Data = make([]byte, size)
	s.DataOffset = offset
}

func (s *Streamer) SetStream(stream Stream) {
	s.Stream = stream
}

// Create an object streamer for a given od entry + subindex
func NewStreamer(entry *Entry, subIndex uint8, origin bool) (*Streamer, error) {
	if entry == nil || entry.object == nil {
		return nil, ODR_IDX_NOT_EXIST
	}
	streamer := &Streamer{}
	object := entry.object
	// attribute, dataOrig and dataLength, depends on object type
	switch object := object.(type) {
	case *Variable:
		if subIndex > 0 {
			return nil, ODR_SUB_NOT_EXIST
		}
		if object.DataType == DOMAIN && entry.extension == nil {
			// Domain entries require extensions to be used, by default they are disabled
			streamer.reader = ReadEntryDisabled
			streamer.writer = WriteEntryDisabled
			streamer.Object = nil
			streamer.DataOffset = 0
			streamer.Subindex = subIndex
			streamer.mu = &object.mu
			log.Warnf("[OD][x%x] no extension has been specified for this domain object", entry.Index)
			return streamer, nil
		}
		streamer.Attribute = object.Attribute
		streamer.Data = object.value
		streamer.DataLength = object.DataLength()
		streamer.mu = &object.mu

	case *VariableList:
		variable, err := object.GetSubObject(subIndex)
		if err != nil {
			return nil, err
		}
		streamer.Attribute = variable.Attribute
		streamer.Data = variable.value
		streamer.DataLength = variable.DataLength()
		streamer.mu = &variable.mu

	default:
		log.Errorf("[OD][x%x] error, unknown type : %+v", entry.Index, object)
		return nil, ODR_DEV_INCOMPAT
	}
	// Add normal reader / writer for object
	if entry.extension == nil || origin {
		streamer.reader = ReadEntryDefault
		streamer.writer = WriteEntryDefault
		streamer.Object = nil
		streamer.DataOffset = 0
		streamer.Subindex = subIndex
		return streamer, nil
	}
	// Add extension reader / writer for object
	if entry.extension.read == nil {
		streamer.reader = ReadEntryDisabled
	} else {
		streamer.reader = entry.extension.read
	}
	if entry.extension.write == nil {
		streamer.writer = WriteEntryDisabled
	} else {
		streamer.writer = entry.extension.write
	}
	streamer.Object = entry.extension.object
	streamer.DataOffset = 0
	streamer.Subindex = subIndex
	return streamer, nil
}

// This is the default "StreamReader" type for every OD entry
// It Reads a value from the original OD location i.e. [Stream] object
// And writes it inside data. It also updates the actual read count, countRead
func ReadEntryDefault(stream *Stream, data []byte, countRead *uint16) error {
	if stream == nil || stream.Data == nil || data == nil || countRead == nil {
		return ODR_DEV_INCOMPAT
	}
	// Check if variable is accessible (i.e.) no write is currently being performed
	if stream.mu == nil {
		return ODR_DEV_INCOMPAT
	}
	// Reading will hang if entry is already being written to. This is problematic
	// For SDO block transfers.
	stream.mu.RLock()
	defer stream.mu.RUnlock()

	dataLenToCopy := int(stream.DataLength)
	count := len(data)
	var err error

	// If reading already started or not enough space in buffer, read
	// in several calls
	if stream.DataOffset > 0 || dataLenToCopy > count {
		if stream.DataOffset >= uint32(dataLenToCopy) {
			return ODR_DEV_INCOMPAT
		}
		dataLenToCopy -= int(stream.DataOffset)
		if dataLenToCopy > count {
			// Partial read
			dataLenToCopy = count
			stream.DataOffset += uint32(dataLenToCopy)
			err = ODR_PARTIAL
		} else {
			stream.DataOffset = 0
		}
	}
	copy(data, stream.Data[stream.DataOffset:stream.DataOffset+uint32(dataLenToCopy)])
	*countRead = uint16(dataLenToCopy)
	return err

}

// This is the default "StreamWriter" type for every OD entry
// It writes data to the [Stream] object
// It also updates the number write count, countWritten
func WriteEntryDefault(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || stream.Data == nil || data == nil || countWritten == nil {
		return ODR_DEV_INCOMPAT
	}
	// Writing will hang if entry is already being read. This is problematic
	// For SDO block transfers.
	stream.mu.Lock()
	defer stream.mu.Unlock()

	dataLenToCopy := int(stream.DataLength)
	count := len(data)
	var err error

	// If writing already started or not enough space in buffer, read
	// in several calls
	if stream.DataOffset > 0 || dataLenToCopy > count {
		if stream.DataOffset >= uint32(dataLenToCopy) {
			return ODR_DEV_INCOMPAT
		}
		dataLenToCopy -= int(stream.DataOffset)

		if dataLenToCopy > count {
			// Partial write
			dataLenToCopy = count
			stream.DataOffset += uint32(dataLenToCopy)
			err = ODR_PARTIAL
		} else {
			stream.DataOffset = 0
		}
	}

	// OD variable is smaller than the provided buffer
	if dataLenToCopy < count ||
		stream.DataOffset+uint32(dataLenToCopy) > uint32(len(stream.Data)) {
		return ODR_DATA_LONG
	}

	copy(stream.Data[stream.DataOffset:stream.DataOffset+uint32(dataLenToCopy)], data)
	*countWritten = uint16(dataLenToCopy)
	return err
}

// "StreamReader" when the actual OD entry to be read is disabled
func ReadEntryDisabled(stream *Stream, data []byte, countRead *uint16) error {
	return ODR_UNSUPP_ACCESS
}

// "StreamWriter" when the actual OD entry to be written is disabled
func WriteEntryDisabled(stream *Stream, data []byte, countWritten *uint16) error {
	return ODR_UNSUPP_ACCESS
}
