package canopen

import (
	"encoding/binary"
	"fmt"

	log "github.com/sirupsen/logrus"

	"gopkg.in/ini.v1"
)

/**
 * Return codes from OD access functions.
 *
 * @ref OD_getSDOabCode() can be used to retrieve corresponding SDO abort code.
 */

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
	err_string, ok := SDO_ABORT_EXPLANATION_MAP[odr]
	if ok {
		return err_string
	} else {
		return SDO_ABORT_EXPLANATION_MAP[ODR_DEV_INCOMPAT]
	}
}

var SDO_ABORT_EXPLANATION_MAP = map[ODR]string{
	0:  "No abort",
	1:  "Out of memory",
	2:  "Unsupported access to an object",
	3:  "Attempt to read a write only object",
	4:  "Attempt to write a read only object",
	5:  "Object does not exist in the object dictionary",
	6:  "Object cannot be mapped to the PDO",
	7:  "Num and len of object to be mapped exceeds PDO len",
	8:  "General parameter incompatibility reasons",
	9:  "General internal incompatibility in device",
	10: "Access failed due to hardware error",
	11: "Data type does not match, length does not match",
	12: "Data type does not match, length too high",
	13: "Data type does not match, length too short",
	14: "Sub index does not exist",
	15: "Invalid value for parameter (download only)",
	16: "Value range of parameter written too high",
	17: "Value range of parameter written too low",
	18: "Maximum value is less than minimum value.",
	19: "Resource not available: SDO connection",
	20: "General error",
	21: "Data cannot be transferred or stored to application",
	22: "Data cannot be transferred because of local control",
	23: "Data cannot be tran. because of present device state",
	24: "Object dict. not present or dynamic generation fails",
	25: "No data available",
}

var SDO_ABORT_MAP = map[ODR]uint32{
	0:  0x00000000, /* No abort */
	1:  0x05040005, /* Out of memory */
	2:  0x06010000, /* Unsupported access to an object */
	3:  0x06010001, /* Attempt to read a write only object */
	4:  0x06010002, /* Attempt to write a read only object */
	5:  0x06020000, /* Object does not exist in the object dictionary */
	6:  0x06040041, /* Object cannot be mapped to the PDO */
	7:  0x06040042, /* Num and len of object to be mapped exceeds PDO len */
	8:  0x06040043, /* General parameter incompatibility reasons */
	9:  0x06040047, /* General internal incompatibility in device */
	10: 0x06060000, /* Access failed due to hardware error */
	11: 0x06070010, /* Data type does not match, length does not match */
	12: 0x06070012, /* Data type does not match, length too high */
	13: 0x06070013, /* Data type does not match, length too short */
	14: 0x06090011, /* Sub index does not exist */
	15: 0x06090030, /* Invalid value for parameter (download only). */
	16: 0x06090031, /* Value range of parameter written too high */
	17: 0x06090032, /* Value range of parameter written too low */
	18: 0x06090036, /* Maximum value is less than minimum value. */
	19: 0x060A0023, /* Resource not available: SDO connection */
	20: 0x08000000, /* General error */
	21: 0x08000020, /* Data cannot be transferred or stored to application */
	22: 0x08000021, /* Data cannot be transferred because of local control */
	23: 0x08000022, /* Data cannot be tran. because of present device state */
	24: 0x08000023, /* Object dict. not present or dynamic generation fails */
	25: 0x08000024, /* No data available */
}

// Get the associated abort code, if the code is not present in map, return ODR_DEV_INCOMPAT
func (result ODR) GetSDOAbordCode() uint32 {
	abort_code, ok := SDO_ABORT_MAP[result]
	if ok {
		return abort_code
	} else {
		return SDO_ABORT_MAP[ODR_DEV_INCOMPAT]
	}
}

const (
	/** This type corresponds to CANopen Object Dictionary object with object
	 * code equal to VAR. OD object is type of @ref OD_obj_var_t and represents
	 * single variable of any type (any length), located on sub-index 0. Other
	 * sub-indexes are not used. */
	ODT_VAR = 0x01
	/** This type corresponds to CANopen Object Dictionary object with object
	 * code equal to ARRAY. OD object is type of @ref OD_obj_array_t and
	 * represents array of variables with the same type, located on sub-indexes
	 * above 0. Sub-index 0 is of type uint8_t and usually represents length of
	 * the array. */
	ODT_ARR = 0x02
	/** This type corresponds to CANopen Object Dictionary object with object
	 * code equal to RECORD. This type of OD object represents structure of
	 * the variables. Each variable from the structure can have own type and
	 * own attribute. OD object is an array of elements of type
	 * @ref OD_obj_var_t. Variable at sub-index 0 is of type uint8_t and usually
	 * represents number of sub-elements in the structure. */
	ODT_REC = 0x03
	/** Mask for basic type */
	ODT_TYPE_MASK = 0x0F
)

const (
	OBJ_DOMAIN byte = 2
	OBJ_VAR    byte = 7
	OBJ_ARR    byte = 8
	OBJ_RECORD byte = 9
)

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

type ObjectDictionary struct {
	entries map[uint16]Entry
}

// Add a new entry to OD
func (od *ObjectDictionary) AddEntry(entry Entry) {
	od.entries[entry.Index] = entry
}

// String representation of object dictionary
func (od *ObjectDictionary) Print() {
	for k, v := range od.entries {
		fmt.Printf("key[%d] value[%+v]\n", k, v)
	}
}

// Find entry inside object dictionary, returns nil if not found
func (od *ObjectDictionary) Find(index uint16) *Entry {

	entry, ok := od.entries[index]
	if ok {
		return &entry
	} else {
		return nil
	}
}

/*
ObjectStreamer is created before accessing an OD entry
It creates a buffer from OD Data []byte slice and provides a default reader
and a default writer using bufio
*/
type Stream struct {
	Data       []byte
	Object     any // Object can be used in case an extension is used
	DataOffset uint32
	Attribute  ODA
	Subindex   uint8
}

// Extension object, is used for extending some functionnality to some OD entries
// Reader must be a custom reader for object
// Writer must be a custom reader for object
type Extension struct {
	Object   any
	Read     func(stream *Stream, buffer []byte, countRead *uint16) error
	Write    func(stream *Stream, buffer []byte, countWritten *uint16) error
	flagsPDO [4]uint8
}

// i.e this is some sort of reader writer
type ObjectStreamer struct {
	Stream *Stream
	Read   func(stream *Stream, buffer []byte, countRead *uint16) error
	Write  func(stream *Stream, buffer []byte, countWritten *uint16) error
}

type Entry struct {
	Index     uint16
	Name      string
	Object    interface{}
	Extension *Extension
}

// Add a member to an Entry object, this is only possible for Array or Record objects
func (entry *Entry) AddMember(section *ini.Section, name string, nodeId uint8, subindex uint8) error {
	switch object := entry.Object.(type) {
	case Variable:
		return fmt.Errorf("Cannot add member to variable type")
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
		record := Record{Subindex: subindex, Variable: *variable}
		entry.Object = append(object, record)
		return nil
	default:
		return fmt.Errorf("Add member not supported for %T", object)
	}
}

// Add or replace an extension to the Entry
func (entry *Entry) AddExtension(extension *Extension) error {
	entry.Extension = extension
	return nil
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
	Name            string
	DataType        byte
	Attribute       ODA // Attribute contains the access type and pdo mapping info
	ParameterValue  string
	DefaultValue    []byte
	StorageLocation string
	LowLimit        int
	HighLimit       int
}

/**
 * Object for OD array of variables, used for "ARRAY" type OD objects
 */
type Array struct {
	Variables []Variable
}

/*
*
  - Object for OD sub-elements, used in "RECORD" type OD objects
    Basically a Variable object but also has a subindex
*/
type Record struct {
	Variable Variable
	Subindex uint8
}

// Read value from original OD location and transfer it into a new byte slice
func ReadEntryOriginal(stream *Stream, data []byte, countRead *uint16) error {

	if stream == nil || data == nil || countRead == nil {
		return ODR_DEV_INCOMPAT
	}

	if stream.Data == nil {
		return ODR_SUB_NOT_EXIST
	}

	dataLenToCopy := len(stream.Data)
	count := len(data)
	var err error

	// If reading already started or the not enough space in buffer, read
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
	// Copy from offset position to dataLenToCopy inside read buffer

	copy(data, stream.Data[stream.DataOffset:stream.DataOffset+uint32(dataLenToCopy)])
	*countRead = uint16(dataLenToCopy)
	return err

}

// Write value from byte slice to original OD location
func WriteEntryOriginal(stream *Stream, data []byte, countWritten *uint16) error {

	if stream == nil || data == nil || countWritten == nil {
		return ODR_DEV_INCOMPAT
	}
	if stream.Data == nil {
		return ODR_DEV_INCOMPAT
	}

	dataLenToCopy := len(stream.Data)
	count := len(data)
	var err error

	/* If previous write was partial or OD variable length is larger than
	 * current buffer size, then data was (will be) written in several
	 * segments */

	if stream.DataOffset > 0 || dataLenToCopy > count {
		if stream.DataOffset >= uint32(dataLenToCopy) {
			return ODR_DEV_INCOMPAT
		}
		/* reduce for already copied data */
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

/*Get SubObject and create an object streamer */
func (entry *Entry) Sub(subindex uint8, origin bool, streamer *ObjectStreamer) error {

	if entry == nil || entry.Object == nil {
		return ODR_IDX_NOT_EXIST
	}

	stream := Stream{}
	streamer.Stream = &stream
	object := entry.Object
	/* attribute, dataOrig and dataLength, depends on object type */
	switch object := object.(type) {
	case Variable:
		if subindex > 0 {
			return ODR_SUB_NOT_EXIST
		}
		stream.Attribute = object.Attribute
		stream.Data = object.Data

	case Array:
		subEntriesCount := len(object.Variables)
		if subindex >= uint8(subEntriesCount) {
			return ODR_SUB_NOT_EXIST
		}
		stream.Attribute = object.Variables[subindex].Attribute
		stream.Data = object.Variables[subindex].Data

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
			return ODR_SUB_NOT_EXIST
		}
		stream.Attribute = record.Variable.Attribute
		stream.Data = record.Variable.Data

	default:
		log.Errorf("Error, unknown type : %+v", object)
		return ODR_DEV_INCOMPAT
	}

	// Populate the used readers or writers if an extension is used
	if entry.Extension == nil || origin {
		log.Infof("Created stream object with default read/write for %x|%x, %v", entry.Index, subindex, streamer.Stream)
		streamer.Read = ReadEntryOriginal
		streamer.Write = WriteEntryOriginal
		stream.Object = nil
	} else {
		log.Infof("Created stream object with extension for %x|%x, %v", entry.Index, subindex, streamer.Stream)
		if entry.Extension.Read == nil {
			streamer.Read = ReadEntryDisabled
		} else {
			streamer.Read = entry.Extension.Read
		}
		if entry.Extension.Write == nil {
			streamer.Write = WriteEntryDisabled
		} else {
			streamer.Write = entry.Extension.Write
		}
		stream.Object = entry.Extension.Object
	}

	// Reset the stream DataOffset as if it were not read/written before
	stream.Subindex = subindex
	return nil
}

// Get value inside OD and read it into data
func (entry *Entry) Get(subIndex uint8, buffer []byte, length uint16, origin bool) error {
	streamer := &ObjectStreamer{}
	var countRead uint16 = 0
	err := entry.Sub(subIndex, origin, streamer)
	if err != nil {
		return err
	}
	if len(streamer.Stream.Data) != int(length) {
		return ODR_TYPE_MISMATCH
	}
	return streamer.Read(streamer.Stream, buffer, &countRead)
}

// Set value inside OD and write it into data
func (entry *Entry) Set(subIndex uint8, buffer []byte, length uint16, origin bool) error {
	streamer := &ObjectStreamer{}
	var countWritten uint16 = 0
	err := entry.Sub(subIndex, origin, streamer)
	if err != nil {
		return err
	}
	if len(streamer.Stream.Data) != int(length) {
		return ODR_TYPE_MISMATCH
	}
	return streamer.Write(streamer.Stream, buffer, &countWritten)

}

// Read Uint8 inside object dictionary
func (entry *Entry) GetUint8(subIndex uint8, data *uint8) error {
	buffer := make([]byte, 1)
	err := entry.Get(subIndex, buffer, 1, true)
	if err != nil {
		return err
	}
	*data = buffer[0]
	return nil
}

// Read Uint16 inside object dictionary
func (entry *Entry) GetUint16(subIndex uint8, data *uint16) error {
	buffer := make([]byte, 2)
	err := entry.Get(subIndex, buffer, 2, true)
	if err != nil {
		log.Errorf("Error %v", err)
		return err
	}
	*data = binary.LittleEndian.Uint16(buffer)
	fmt.Printf("Data value is %v", *data)
	return nil
}

// Read Uint32 inside object dictionary
func (entry *Entry) GetUint32(subIndex uint8, data *uint32) error {
	buffer := make([]byte, 4)
	err := entry.Get(subIndex, buffer, 4, true)
	if err != nil {
		return err
	}
	*data = binary.LittleEndian.Uint32(buffer)
	return nil
}

// Read Uint64 inside object dictionary
func (entry *Entry) GetUint64(subIndex uint8, data *uint64) error {
	buffer := make([]byte, 8)
	err := entry.Get(subIndex, buffer, 8, true)
	if err != nil {
		return err
	}
	*data = binary.LittleEndian.Uint64(buffer)
	return nil
}

// Set Uint8, 16 , 32 , 64
func (entry *Entry) SetUint8(subIndex uint8, data uint8) error {
	buffer := []byte{data}
	err := entry.Set(subIndex, buffer, 1, true)
	if err != nil {
		return err
	}
	return nil
}

func (entry *Entry) SetUint16(subIndex uint8, data uint16) error {
	buffer := make([]byte, 2)
	binary.LittleEndian.PutUint16(buffer, data)
	err := entry.Set(subIndex, buffer, 2, true)
	if err != nil {
		return err
	}
	return nil
}

func (entry *Entry) SetUint32(subIndex uint8, data uint32) error {
	buffer := make([]byte, 4)
	binary.LittleEndian.PutUint32(buffer, data)
	err := entry.Set(subIndex, buffer, 4, true)
	if err != nil {
		return err
	}
	return nil
}

func (entry *Entry) SetUint64(subIndex uint8, data uint64) error {
	buffer := make([]byte, 8)
	binary.LittleEndian.PutUint64(buffer, data)
	err := entry.Set(subIndex, buffer, 8, true)
	if err != nil {
		return err
	}
	return nil
}

func NewOD() ObjectDictionary {
	return ObjectDictionary{entries: make(map[uint16]Entry)}
}

/* Create a new Object dictionary Entry of Variable type */
func NewVariableEntry(index uint16, data []byte, attribute ODA) Entry {
	Object := Variable{Data: data, Attribute: attribute}
	return Entry{Index: index, Object: Object, Extension: nil}
}

/* Create a new Object dictionary Entry of Record type, Object is an empty slice of Record elements */
func NewRecordEntry(index uint16, records []Record) Entry {
	return Entry{Index: index, Object: records, Extension: nil}
}
