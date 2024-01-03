package canopen

import log "github.com/sirupsen/logrus"

// A Stream to an OD entry
type Stream struct {
	Data       []byte
	DataOffset uint32
	DataLength uint32
	Object     any // Custom objects can be used when using an OD extension
	Attribute  uint8
	Subindex   uint8
}

type StreamReader func(stream *Stream, buffer []byte, countRead *uint16) error
type StreamWriter func(stream *Stream, buffer []byte, countWritten *uint16) error

// Streamer is created before accessing an OD entry
// It creates a buffer from OD Data []byte slice and provides a default reader
// and a default writer
type Streamer struct {
	stream Stream
	read   StreamReader
	write  StreamWriter
}

// extension object, is used for extending functionnality of an OD entry
// This package has some pre-made extensions for CiA defined entries
type extension struct {
	object   any          // Any object to link with extension
	read     StreamReader // A [StreamReader] that will be called when reading entry
	write    StreamWriter // A [StreamWriter] that will be called when writing to entry
	flagsPDO [OD_FLAGS_PDO_SIZE]uint8
}

// Implements io.Reader
func (streamer *Streamer) Read(b []byte) (n int, err error) {
	countRead := uint16(0)
	err = streamer.read(&streamer.stream, b, &countRead)
	return int(countRead), err
}

// Implements io.Writer
func (streamer *Streamer) Write(b []byte) (n int, err error) {
	countWritten := uint16(0)
	err = streamer.write(&streamer.stream, b, &countWritten)
	return int(countWritten), err
}

// Create an object streamer for a given od entry + subindex
func NewStreamer(entry *Entry, subIndex uint8, origin bool) (*Streamer, error) {
	if entry == nil || entry.Object == nil {
		return nil, ODR_IDX_NOT_EXIST
	}
	streamer := &Streamer{}
	object := entry.Object
	// attribute, dataOrig and dataLength, depends on object type
	switch object := object.(type) {
	case *Variable:
		if subIndex > 0 {
			return nil, ODR_SUB_NOT_EXIST
		}
		if object.DataType == DOMAIN && entry.Extension == nil {
			// Domain entries require extensions to be used, by default they are disabled
			streamer.read = ReadEntryDisabled
			streamer.write = WriteEntryDisabled
			streamer.stream.Object = nil
			streamer.stream.DataOffset = 0
			streamer.stream.Subindex = subIndex
			log.Warnf("[OD][x%x] no extension has been specified for this domain object", entry.Index)
			return streamer, nil
		}
		streamer.stream.Attribute = object.Attribute
		streamer.stream.Data = object.data
		streamer.stream.DataLength = object.DataLength()

	case *VariableList:
		variable, err := object.GetSubObject(subIndex)
		if err != nil {
			return nil, err
		}
		streamer.stream.Attribute = variable.Attribute
		streamer.stream.Data = variable.data
		streamer.stream.DataLength = variable.DataLength()

	default:
		log.Errorf("[OD][x%x] error, unknown type : %+v", entry.Index, object)
		return nil, ODR_DEV_INCOMPAT
	}
	// Add normal reader / writer for object
	if entry.Extension == nil || origin {
		streamer.read = ReadEntryDefault
		streamer.write = WriteEntryDefault
		streamer.stream.Object = nil
		streamer.stream.DataOffset = 0
		streamer.stream.Subindex = subIndex
		return streamer, nil
	}
	// Add extension reader / writer for object
	if entry.Extension.read == nil {
		streamer.read = ReadEntryDisabled
	} else {
		streamer.read = entry.Extension.read
	}
	if entry.Extension.write == nil {
		streamer.write = WriteEntryDisabled
	} else {
		streamer.write = entry.Extension.write
	}
	streamer.stream.Object = entry.Extension.object
	streamer.stream.DataOffset = 0
	streamer.stream.Subindex = subIndex
	return streamer, nil
}

// This is the default "StreamReader" type for every OD entry
// It Reads a value from the original OD location i.e. [Stream] object
// And writes it inside data. It also updates the actual read count, countRead
func ReadEntryDefault(stream *Stream, data []byte, countRead *uint16) error {
	if stream == nil || stream.Data == nil || data == nil || countRead == nil {
		return ODR_DEV_INCOMPAT
	}

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
