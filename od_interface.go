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

var OD_TO_SDO_ABORT_MAP = map[ODR]SDOAbortCode{
	ODR_OK:             SDO_ABORT_NONE,
	ODR_OUT_OF_MEM:     SDO_ABORT_OUT_OF_MEM,
	ODR_UNSUPP_ACCESS:  SDO_ABORT_UNSUPPORTED_ACCESS,
	ODR_WRITEONLY:      SDO_ABORT_WRITEONLY,
	ODR_READONLY:       SDO_ABORT_READONLY,
	ODR_IDX_NOT_EXIST:  SDO_ABORT_NOT_EXIST,
	ODR_NO_MAP:         SDO_ABORT_NO_MAP,
	ODR_MAP_LEN:        SDO_ABORT_MAP_LEN,
	ODR_PAR_INCOMPAT:   SDO_ABORT_PRAM_INCOMPAT,
	ODR_DEV_INCOMPAT:   SDO_ABORT_DEVICE_INCOMPAT,
	ODR_HW:             SDO_ABORT_HW,
	ODR_TYPE_MISMATCH:  SDO_ABORT_TYPE_MISMATCH,
	ODR_DATA_LONG:      SDO_ABORT_DATA_LONG,
	ODR_DATA_SHORT:     SDO_ABORT_DATA_SHORT,
	ODR_SUB_NOT_EXIST:  SDO_ABORT_SUB_UNKNOWN,
	ODR_INVALID_VALUE:  SDO_ABORT_INVALID_VALUE,
	ODR_VALUE_HIGH:     SDO_ABORT_VALUE_HIGH,
	ODR_VALUE_LOW:      SDO_ABORT_VALUE_LOW,
	ODR_MAX_LESS_MIN:   SDO_ABORT_MAX_LESS_MIN,
	ODR_NO_RESOURCE:    SDO_ABORT_NO_RESOURCE,
	ODR_GENERAL:        SDO_ABORT_GENERAL,
	ODR_DATA_TRANSF:    SDO_ABORT_DATA_TRANSF,
	ODR_DATA_LOC_CTRL:  SDO_ABORT_DATA_LOC_CTRL,
	ODR_DATA_DEV_STATE: SDO_ABORT_DATA_DEV_STATE,
	ODR_OD_MISSING:     SDO_ABORT_DATA_OD,
	ODR_NO_DATA:        SDO_ABORT_NO_DATA,
}

func (odr ODR) Error() string {
	abort := odr.GetSDOAbordCode()
	return abort.Error()
}

// Get the associated abort code, if the code is not present in map, return ODR_DEV_INCOMPAT
func (result ODR) GetSDOAbordCode() SDOAbortCode {
	abort_code, ok := OD_TO_SDO_ABORT_MAP[result]
	if ok {
		return SDOAbortCode(abort_code)
	} else {
		return OD_TO_SDO_ABORT_MAP[ODR_DEV_INCOMPAT]
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
	entriesByIndexValue map[uint16]*Entry
	entriesByIndexName  map[string]*Entry
}

// An entry can be any object type : variable, array, record or domain
type Entry struct {
	Index             uint16
	Name              string
	Object            any
	Extension         *Extension
	subEntriesNameMap map[string]uint8
}

// Add a record to OD
func (od *ObjectDictionary) AddRecord(index uint16, name string, record []Record) {
	od.addEntry(&Entry{Index: index, Name: name, Object: record, Extension: nil, subEntriesNameMap: map[string]uint8{}})
}

// Add an array to OD
func (od *ObjectDictionary) AddArray(index uint16, name string, array Array) {
	od.addEntry(&Entry{Index: index, Name: name, Object: array, Extension: nil, subEntriesNameMap: map[string]uint8{}})
}

// Add a variable to OD
func (od *ObjectDictionary) AddVariable(index uint16, name string, variable Variable) {
	od.addEntry(&Entry{Index: index, Name: name, Object: variable, Extension: nil, subEntriesNameMap: map[string]uint8{}})
}

// Add an entry to OD, existing entry will be replaced
func (od *ObjectDictionary) addEntry(entry *Entry) {
	_, entryIndexValueExists := od.entriesByIndexValue[entry.Index]
	if entryIndexValueExists {
		log.Warnf("[OD] overwritting entry index x%x", entry.Index)
	}
	od.entriesByIndexValue[entry.Index] = entry
	od.entriesByIndexName[entry.Name] = entry
}

// Add file like object entry to OD
func (od *ObjectDictionary) AddFile(index uint16, indexName string, filePath string, mode int) error {
	log.Infof("[OD] adding file object entry : %v at x%x", filePath, index)
	fileObject := &FileObject{FilePath: filePath, ReadWriteMode: mode}
	variable := Variable{
		Data:           []byte{},
		DataLength:     0,
		Name:           indexName,
		DataType:       DOMAIN,
		Attribute:      ODA_SDO_RW,
		ParameterValue: "",
		DefaultValue:   []byte{},
		Index:          index,
		SubIndex:       0,
	}
	od.AddVariable(index, indexName, variable)
	entry := od.Index(index)
	entry.AddExtension(fileObject, ReadEntryFileObject, WriteEntryFileObject)
	return nil
}

// Get an entry corresponding to a given index
// Index can either be a string, int or uint16
// This method does not return an error for chaining
func (od *ObjectDictionary) Index(index any) *Entry {
	var entry *Entry
	switch ind := index.(type) {
	case string:
		entry = od.entriesByIndexName[ind]
	case int:
		entry = od.entriesByIndexValue[uint16(ind)]
	case uint:
		entry = od.entriesByIndexValue[uint16(ind)]
	case uint16:
		entry = od.entriesByIndexValue[ind]
	default:
		return nil
	}
	return entry
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
	Read     ExtensionReader
	Write    ExtensionWriter
	flagsPDO [OD_FLAGS_PDO_SIZE]uint8
}

type ExtensionReader func(stream *Stream, buffer []byte, countRead *uint16) error
type ExtensionWriter func(stream *Stream, buffer []byte, countWritten *uint16) error

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

// OD variable object used for holding any sub object
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
	Index           uint16
	SubIndex        uint8
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

// Add a member to Entry, this is only possible for Array or Record objects
func (entry *Entry) AddMember(section *ini.Section, name string, nodeId uint8, subIndex uint8) error {

	switch object := entry.Object.(type) {
	case Variable:
		return fmt.Errorf("cannot add a member to variable type")
	case Array:
		variable, err := buildVariable(section, name, nodeId, entry.Index, subIndex)
		if err != nil {
			return err
		}
		object.Variables[subIndex] = *variable
		entry.Object = object
		entry.subEntriesNameMap[name] = subIndex
		return nil

	case []Record:
		variable, err := buildVariable(section, name, nodeId, entry.Index, subIndex)
		if err != nil {
			return err
		}
		entry.Object = append(object, Record{Subindex: subIndex, Variable: *variable})
		entry.subEntriesNameMap[name] = subIndex
		return nil

	default:
		return fmt.Errorf("add member not supported for %T", object)
	}
}

// Add an extension to entry and return created extension
func (entry *Entry) AddExtension(object any, read ExtensionReader, write ExtensionWriter) *Extension {
	extension := &Extension{Object: object, Read: read, Write: write}
	entry.Extension = extension
	return extension
}

// Get variable from given sub index
// Subindex can either be a string or an int, or uint8
func (entry *Entry) SubIndex(subIndex any) (v *Variable, e error) {
	if entry == nil {
		return nil, ODR_IDX_NOT_EXIST
	}
	switch object := entry.Object.(type) {
	case Variable:
		if subIndex != 0 && subIndex != "" {
			return nil, ODR_SUB_NOT_EXIST
		}
		return &object, nil
	case Array:
		subEntriesCount := len(object.Variables)
		switch sub := subIndex.(type) {
		case string:
			subIndexInt, ok := entry.subEntriesNameMap[sub]
			if ok {
				return &object.Variables[subIndexInt], nil
			}
			return nil, ODR_SUB_NOT_EXIST
		case int:
			if uint8(sub) >= uint8(subEntriesCount) {
				return nil, ODR_SUB_NOT_EXIST
			}
			return &object.Variables[uint8(sub)], nil
		case uint8:
			if sub >= uint8(subEntriesCount) {
				return nil, ODR_SUB_NOT_EXIST
			}
			return &object.Variables[sub], nil
		default:
			return nil, ODR_DEV_INCOMPAT
		}
	case []Record:
		records := object
		var record *Record
		switch sub := subIndex.(type) {
		case string:
			subIndexInt, ok := entry.subEntriesNameMap[sub]
			if ok {
				for i := range records {
					if records[i].Subindex == subIndexInt {
						record = &records[i]
						return &record.Variable, nil
					}
				}
			}
			return nil, ODR_SUB_NOT_EXIST
		case int:
			for i := range records {
				if records[i].Subindex == uint8(sub) {
					record = &records[i]
					return &record.Variable, nil
				}
			}
			return nil, ODR_SUB_NOT_EXIST
		case uint8:
			for i := range records {
				if records[i].Subindex == sub {
					record = &records[i]
					return &record.Variable, nil
				}
			}
			return nil, ODR_SUB_NOT_EXIST
		default:
			return nil, ODR_DEV_INCOMPAT

		}
	default:
		// This is not normal
		return nil, ODR_DEV_INCOMPAT
	}

}

// Get sub object type
func (entry *Entry) GetSubEntryDataType(subIndex uint8) (dataType byte, e error) {
	switch object := entry.Object.(type) {
	case Variable:
		if subIndex != 0 {
			return 0, ODR_SUB_NOT_EXIST
		}
		return object.DataType, e
	case Array:
		subEntriesCount := len(object.Variables)
		if subIndex >= uint8(subEntriesCount) {
			return 0, ODR_SUB_NOT_EXIST
		}
		return object.Variables[subIndex].DataType, e

	case []Record:
		records := object
		var record *Record
		for i := range records {
			if records[i].Subindex == subIndex {
				record = &records[i]
				break
			}
		}
		if record == nil {
			return 0, ODR_SUB_NOT_EXIST
		} else {
			return record.Variable.DataType, e
		}
	default:
		// This is not normal
		return 0, fmt.Errorf("the entry accessed %v has an invalid type", entry)
	}
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

// Create an object streamer for a given (index,subIndex)
func (entry *Entry) CreateStreamer(subIndex uint8, origin bool) (*ObjectStreamer, error) {

	if entry == nil || entry.Object == nil {
		return nil, ODR_IDX_NOT_EXIST
	}
	streamer := &ObjectStreamer{}
	object := entry.Object
	// attribute, dataOrig and dataLength, depends on object type
	switch object := object.(type) {
	case Variable:
		if subIndex > 0 {
			return nil, ODR_SUB_NOT_EXIST
		}
		streamer.stream.Attribute = object.Attribute
		streamer.stream.Data = object.Data
		streamer.stream.DataLength = uint32(len(object.Data))

	case Array:
		subEntriesCount := len(object.Variables)
		if subIndex >= uint8(subEntriesCount) {
			return nil, ODR_SUB_NOT_EXIST
		}
		streamer.stream.Attribute = object.Variables[subIndex].Attribute
		streamer.stream.Data = object.Variables[subIndex].Data
		streamer.stream.DataLength = uint32(len(object.Variables[subIndex].Data))

	case []Record:
		records := object
		var record *Record
		for i := range records {
			if records[i].Subindex == subIndex {
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
		streamer.stream.Subindex = subIndex
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
	streamer.stream.Subindex = subIndex
	return streamer, nil
}

// Read exactly len(b) bytes from OD at (index,subIndex)
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

// Write exactly len(b) bytes to OD at (index,subIndex)
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
	return ObjectDictionary{entriesByIndexValue: make(map[uint16]*Entry), entriesByIndexName: make(map[string]*Entry)}
}
