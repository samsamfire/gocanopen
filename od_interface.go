package canopen

import (
	"encoding/binary"
	"fmt"

	log "github.com/sirupsen/logrus"
	"gopkg.in/ini.v1"
)

type ODR int8

const (
	ODR_PARTIAL        ODR = -1
	ODR_OK             ODR = 0
	ODR_OUT_OF_MEM     ODR = 1
	ODR_UNSUPP_ACCESS  ODR = 2
	ODR_WRITEONLY      ODR = 3
	ODR_READONLY       ODR = 4
	ODR_IDX_NOT_EXIST  ODR = 5
	ODR_NO_MAP         ODR = 6
	ODR_MAP_LEN        ODR = 7
	ODR_PAR_INCOMPAT   ODR = 8
	ODR_DEV_INCOMPAT   ODR = 9
	ODR_HW             ODR = 10
	ODR_TYPE_MISMATCH  ODR = 11
	ODR_DATA_LONG      ODR = 12
	ODR_DATA_SHORT     ODR = 13
	ODR_SUB_NOT_EXIST  ODR = 14
	ODR_INVALID_VALUE  ODR = 15
	ODR_VALUE_HIGH     ODR = 16
	ODR_VALUE_LOW      ODR = 17
	ODR_MAX_LESS_MIN   ODR = 18
	ODR_NO_RESOURCE    ODR = 19
	ODR_GENERAL        ODR = 20
	ODR_DATA_TRANSF    ODR = 21
	ODR_DATA_LOC_CTRL  ODR = 22
	ODR_DATA_DEV_STATE ODR = 23
	ODR_OD_MISSING     ODR = 24
	ODR_NO_DATA        ODR = 25
	ODR_COUNT          ODR = 26
)

func (odr ODR) Error() string {
	abort := odr.GetSDOAbordCode()
	return abort.Error()
}

// Get the associated abort code, if the code is not present in map, return ODR_DEV_INCOMPAT
func (result ODR) GetSDOAbordCode() SDOAbortCode {
	abort_code, ok := SDO_ABORT_MAP[result]
	if ok {
		return SDOAbortCode(abort_code)
	} else {
		return SDO_ABORT_MAP[ODR_DEV_INCOMPAT]
	}
}

/**
 * Attributes (bit masks) for OD sub-object.
 */
type ODA uint8

const (
	ODA_SDO_R  ODA = 0x01 /**< SDO server may read from the variable */
	ODA_SDO_W  ODA = 0x02 /**< SDO server may write to the variable */
	ODA_SDO_RW ODA = 0x03 /**< SDO server may read from or write to the variable */
	ODA_TPDO   ODA = 0x04 /**< Variable is mappable into TPDO (can be read) */
	ODA_RPDO   ODA = 0x08 /**< Variable is mappable into RPDO (can be written) */
	ODA_TRPDO  ODA = 0x0C /**< Variable is mappable into TPDO or RPDO */
	ODA_TSRDO  ODA = 0x10 /**< Variable is mappable into transmitting SRDO */
	ODA_RSRDO  ODA = 0x20 /**< Variable is mappable into receiving SRDO */
	ODA_TRSRDO ODA = 0x30 /**< Variable is mappable into tx or rx SRDO */
	ODA_MB     ODA = 0x40 /**< Variable is multi-byte ((u)int16_t to (u)int64_t) */
	ODA_STR    ODA = 0x80 /**< Shorter value, than specified variable size, may be
	  written to the variable. SDO write will fill remaining memory with zeroes.
	  Attribute is used for VISIBLE_STRING and UNICODE_STRING. */
)

// Object dictionary contains all node data
type ObjectDictionary struct {
	entries map[uint16]*Entry
}

// An entry can be any object type : variable, array, record or domain
type Entry struct {
	Index     uint16
	Name      string
	Object    any
	Extension *Extension
}

// Add a new entry to OD
func (od *ObjectDictionary) AddEntry(entry *Entry) {
	_, ok := od.entries[entry.Index]
	if ok {
		log.Warnf("[OD] overwritting entry %x", entry.Index)
	}
	od.entries[entry.Index] = entry
}

// Get the entry corresponding to the given index
func (od *ObjectDictionary) Index(index uint16) *Entry {
	entry, ok := od.entries[index]
	if ok {
		return entry
	} else {
		return nil
	}
}

type Stream struct {
	Data       []byte
	DataOffset uint32
	DataLength uint32
	Object     any // Custom objects can be used when using an OD extension
	Attribute  ODA
	Subindex   uint8
}

func (stream *Stream) Mappable() bool {
	return stream.Attribute&(ODA_TRPDO|ODA_TRSRDO) != 0
}

// Extension object, is used for extending some functionnality to some OD entries
// Reader must be a custom reader for object
// Writer must be a custom reader for object
type Extension struct {
	Object   any
	Read     func(stream *Stream, buffer []byte, countRead *uint16) error
	Write    func(stream *Stream, buffer []byte, countWritten *uint16) error
	flagsPDO [OD_FLAGS_PDO_SIZE]uint8
}

/*
ObjectStreamer is created before accessing an OD entry
It creates a buffer from OD Data []byte slice and provides a default reader
and a default writer
*/
type ObjectStreamer struct {
	stream Stream
	read   func(stream *Stream, buffer []byte, countRead *uint16) error
	write  func(stream *Stream, buffer []byte, countWritten *uint16) error
}

// Implements io.Reader
func (streamer *ObjectStreamer) Read(b []byte) (n int, err error) {
	countRead := uint16(0)
	err = streamer.read(&streamer.stream, b, &countRead)
	return int(countRead), err
}

// Implements io.Writer
func (streamer *ObjectStreamer) Write(b []byte) (n int, err error) {
	countWritten := uint16(0)
	err = streamer.write(&streamer.stream, b, &countWritten)
	return int(countWritten), err
}

// Add a member to Entry, this is only possible for Array or Record objects
func (entry *Entry) AddMember(section *ini.Section, name string, nodeId uint8, subindex uint8) error {

	switch object := entry.Object.(type) {
	case Variable:
		return fmt.Errorf("cannot add a member to variable type")
	case Array:
		variable, err := buildVariable(section, name, nodeId, entry.Index, subindex)
		if err != nil {
			return err
		}
		object.Variables[subindex] = *variable
		entry.Object = object
		return nil

	case []Record:
		variable, err := buildVariable(section, name, nodeId, entry.Index, subindex)
		if err != nil {
			return err
		}
		entry.Object = append(object, Record{Subindex: subindex, Variable: *variable})
		return nil

	default:
		return fmt.Errorf("add member not supported for %T", object)
	}
}

// Add or replace an extension to the Entry
func (entry *Entry) AddExtension(extension *Extension) error {
	entry.Extension = extension
	return nil
}

// Get number of sub entries. Depends on type
func (entry *Entry) SubEntriesCount() int {

	switch object := entry.Object.(type) {
	case Variable:
		return 1
	case Array:
		return len(object.Variables)

	case []Record:
		return len(object)

	default:
		// This is not normal
		log.Errorf("The entry %v has an invalid type", entry)
		return 1
	}
}

type FileInfo struct {
	FileName         string
	FileVersion      string
	FileRevision     string
	LastEDS          string
	EDSVersion       string
	Description      string
	CreationTime     string
	CreationDate     string
	CreatedBy        string
	ModificationTime string
	ModificationDate string
	ModifiedBy       string
}

/*Object for basic OD variable */
type Variable struct {
	Data            []byte
	DataLength      uint32 //Can be different than len(Data) for strings
	Name            string
	DataType        byte
	Attribute       ODA // Attribute contains the access type and pdo mapping info
	ParameterValue  string
	DefaultValue    []byte
	StorageLocation string
	LowLimit        int
	HighLimit       int
}

type Array struct {
	Variables []Variable
}
type Record struct {
	Variable Variable
	Subindex uint8
}

// Read value from original OD location and transfer it into a new byte slice
func ReadEntryOriginal(stream *Stream, data []byte, countRead *uint16) error {
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

// Write value from byte slice to original OD location
func WriteEntryOriginal(stream *Stream, data []byte, countWritten *uint16) error {

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
	if dataLenToCopy < count {
		return ODR_DATA_LONG
	}

	copy(stream.Data[stream.DataOffset:stream.DataOffset+uint32(dataLenToCopy)], data)
	*countWritten = uint16(dataLenToCopy)
	return err
}

// Read value from variable from Object Dictionary disabled
func ReadEntryDisabled(stream *Stream, data []byte, countRead *uint16) error {
	return ODR_UNSUPP_ACCESS
}

// Write value to variable from Object Dictionary disabled
func WriteEntryDisabled(stream *Stream, data []byte, countWritten *uint16) error {
	return ODR_UNSUPP_ACCESS
}

// Create an object streamer for a given (index,subindex)
func (entry *Entry) CreateStreamer(subindex uint8, origin bool) (*ObjectStreamer, error) {

	if entry == nil || entry.Object == nil {
		return nil, ODR_IDX_NOT_EXIST
	}
	streamer := &ObjectStreamer{}
	object := entry.Object
	// attribute, dataOrig and dataLength, depends on object type
	switch object := object.(type) {
	case Variable:
		if subindex > 0 {
			return nil, ODR_SUB_NOT_EXIST
		}
		streamer.stream.Attribute = object.Attribute
		streamer.stream.Data = object.Data
		streamer.stream.DataLength = uint32(len(object.Data))

	case Array:
		subEntriesCount := len(object.Variables)
		if subindex >= uint8(subEntriesCount) {
			return nil, ODR_SUB_NOT_EXIST
		}
		streamer.stream.Attribute = object.Variables[subindex].Attribute
		streamer.stream.Data = object.Variables[subindex].Data
		streamer.stream.DataLength = uint32(len(object.Variables[subindex].Data))

	case []Record:
		records := object
		var record *Record
		for i := range records {
			if records[i].Subindex == subindex {
				record = &records[i]
				break
			}
		}
		if record == nil {
			return nil, ODR_SUB_NOT_EXIST
		}
		streamer.stream.Attribute = record.Variable.Attribute
		streamer.stream.Data = record.Variable.Data
		streamer.stream.DataLength = uint32(len(record.Variable.Data))

	default:
		log.Errorf("[OD] error, unknown type : %+v", object)
		return nil, ODR_DEV_INCOMPAT
	}
	// Add normal reader / writer for object
	if entry.Extension == nil || origin {
		streamer.read = ReadEntryOriginal
		streamer.write = WriteEntryOriginal
		streamer.stream.Object = nil
		streamer.stream.DataOffset = 0
		streamer.stream.Subindex = subindex
		return streamer, nil
	}
	// Add extension reader / writer for object
	if entry.Extension.Read == nil {
		streamer.read = ReadEntryDisabled
	} else {
		streamer.read = entry.Extension.Read
	}
	if entry.Extension.Write == nil {
		streamer.write = WriteEntryDisabled
	} else {
		streamer.write = entry.Extension.Write
	}
	streamer.stream.Object = entry.Extension.Object
	streamer.stream.DataOffset = 0
	streamer.stream.Subindex = subindex
	return streamer, nil
}

// Read exactly len(b) bytes from OD at (index,subindex)
// Origin parameter controls extension usage if exists
func (entry *Entry) readSubExactly(subIndex uint8, b []byte, origin bool) error {
	streamer, err := entry.CreateStreamer(subIndex, origin)
	if err != nil {
		return err
	}
	if int(streamer.stream.DataLength) != len(b) {
		return ODR_TYPE_MISMATCH
	}
	_, err = streamer.Read(b)
	return err
}

// Write exactly len(b) bytes to OD at (index,subindex)
// Origin parameter controls extension usage if exists
func (entry *Entry) writeSubExactly(subIndex uint8, b []byte, origin bool) error {
	streamer, err := entry.CreateStreamer(subIndex, origin)
	if err != nil {
		return err
	}
	if int(streamer.stream.DataLength) != len(b) {
		return ODR_TYPE_MISMATCH
	}
	_, err = streamer.Write(b)
	return err

}

// Getptr inside OD, similar to read
func (entry *Entry) GetPtr(subIndex uint8, length uint16) (*[]byte, error) {
	streamer, err := entry.CreateStreamer(subIndex, true)
	if err != nil {
		return nil, err
	}
	if int(streamer.stream.DataLength) != int(length) && length != 0 {
		return nil, ODR_TYPE_MISMATCH
	}
	return &streamer.stream.Data, nil
}

// Read Uint8 inside object dictionary
func (entry *Entry) GetUint8(subIndex uint8, data *uint8) error {
	b := make([]byte, 1)
	err := entry.readSubExactly(subIndex, b, true)
	if err != nil {
		return err
	}
	*data = b[0]
	return nil
}

// Read Uint16 inside object dictionary
func (entry *Entry) GetUint16(subIndex uint8, data *uint16) error {
	b := make([]byte, 2)
	err := entry.readSubExactly(subIndex, b, true)
	if err != nil {
		return err
	}
	*data = binary.LittleEndian.Uint16(b)
	return nil
}

// Read Uint32 inside object dictionary
func (entry *Entry) GetUint32(subIndex uint8, data *uint32) error {
	b := make([]byte, 4)
	err := entry.readSubExactly(subIndex, b, true)
	if err != nil {
		return err
	}
	*data = binary.LittleEndian.Uint32(b)
	return nil
}

// Read Uint64 inside object dictionary
func (entry *Entry) GetUint64(subIndex uint8, data *uint64) error {
	b := make([]byte, 8)
	err := entry.readSubExactly(subIndex, b, true)
	if err != nil {
		return err
	}
	*data = binary.LittleEndian.Uint64(b)
	return nil
}

// Set Uint8, 16 , 32 , 64
func (entry *Entry) SetUint8(subIndex uint8, data uint8, origin bool) error {
	b := []byte{data}
	err := entry.writeSubExactly(subIndex, b, origin)
	if err != nil {
		return err
	}
	return nil
}

func (entry *Entry) SetUint16(subIndex uint8, data uint16, origin bool) error {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, data)
	err := entry.writeSubExactly(subIndex, b, origin)
	if err != nil {
		return err
	}
	return nil
}

func (entry *Entry) SetUint32(subIndex uint8, data uint32, origin bool) error {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, data)
	err := entry.writeSubExactly(subIndex, b, origin)
	if err != nil {
		return err
	}
	return nil
}

func (entry *Entry) SetUint64(subIndex uint8, data uint64, origin bool) error {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, data)
	err := entry.writeSubExactly(subIndex, b, origin)
	if err != nil {
		return err
	}
	return nil
}

func isIDRestricted(canId uint16) bool {
	return canId <= 0x7f ||
		(canId >= 0x101 && canId <= 0x180) ||
		(canId >= 0x581 && canId <= 0x5FF) ||
		(canId >= 0x601 && canId <= 0x67F) ||
		(canId >= 0x6E0 && canId <= 0x6FF) ||
		canId >= 0x701
}

func NewOD() ObjectDictionary {
	return ObjectDictionary{entries: make(map[uint16]*Entry)}
}

/* Create a new Object dictionary Entry of Variable type */
func NewVariableEntry(index uint16, data []byte, attribute ODA) *Entry {
	Object := Variable{Data: data, Attribute: attribute}
	return &Entry{Index: index, Object: Object, Extension: nil}
}

/* Create a new Object dictionary Entry of Record type, Object is an empty slice of Record elements */
func NewRecordEntry(index uint16, records []Record) *Entry {
	return &Entry{Index: index, Object: records, Extension: nil}
}
