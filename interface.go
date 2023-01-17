package canopen

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

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
	Data   bytes.Buffer // Buffer created from []byte slice
	Object *any         // Object can be used in case an extension is used
	/** In case of large data, dataOffset indicates position of already
	 * transferred data */
	// DataOffset uint32 Should not be necessary because this information is already contained
	/** Attribute bit-field of the OD sub-object, see @ref OD_attributes_t */
	Attribute ODA
	/** Sub index of the OD sub-object, informative */
	Subindex uint8
}

// func NewODStream() ODStream {
// 	return ODStream{Data: bytes.Buffer{}, Object: nil, DataOffset: 0, Attribute: 0, Subindex: 0}
// }

// Extension object, is used for extending some functionnality to some OD entries
// Reader must be a custom reader for object
// Writer must be a custom reader for object
type Extension struct {
	Object   *any
	Reader   io.Reader
	Writer   io.Writer
	flagsPDO [4]uint8
}

// i.e this is some sort of reader writer
type ObjectStreamer struct {
	Stream *Stream
	Reader io.Reader
	Writer io.Writer
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

/**
* Read value from original OD location into data
data can be of any type
*/

// func (stream *ODStream) ReadOriginal(data []byte) error {
// 	dataLenToCopy := uint32(len(stream.dataOrig)) /* length of OD variable */
// 	count := uint32(len(data))
// 	dataOrig := stream.dataOrig
// 	if dataOrig == nil {
// 		return ODR_SUB_NOT_EXIST, 0
// 	}

// 	var returnCode ODR
// 	returnCode = ODR_OK

// 	/* If previous read was partial or OD variable length is larger than
// 	 * current buffer size, then data was (will be) read in several segments */

// 	if stream.dataOffset > 0 || dataLenToCopy > count {
// 		if stream.dataOffset >= dataLenToCopy {
// 			return ODR_DEV_INCOMPAT, 0
// 		}
// 		/* Reduce for already copied data */
// 		dataLenToCopy -= stream.dataOffset

// 		if dataLenToCopy > count {
// 			/* Not enough space in destination buffer */
// 			dataLenToCopy = count
// 			stream.dataOffset += dataLenToCopy
// 			returnCode = ODR_PARTIAL
// 		} else {
// 			stream.dataOffset = 0 /* copy finished, reset offset */
// 		}
// 	}
// 	copy(data, stream.dataOrig[stream.dataOffset:stream.dataOffset+dataLenToCopy])
// 	return returnCode, dataLenToCopy
// }

// func (stream ODStream) WriteOriginal(data []byte) (result ODR, n uint32) {
// 	if data == nil {
// 		return ODR_DEV_INCOMPAT, 0
// 	}
// 	dataLenToCopy := uint32(len(stream.dataOrig)) /* length of OD variable */
// 	dataOrig := stream.dataOrig
// 	count := uint32(len(data))

// 	if dataOrig == nil {
// 		return ODR_SUB_NOT_EXIST, 0
// 	}

// 	var returnCode ODR
// 	returnCode = ODR_OK

// 	/* If previous write was partial or OD variable length is larger than
// 	 * current buffer size, then data was (will be) written in several
// 	 * segments */

// 	if stream.dataOffset > 0 || dataLenToCopy > count {
// 		if stream.dataOffset >= dataLenToCopy {
// 			return ODR_DEV_INCOMPAT, 0
// 		}
// 		/* reduce for already copied data */
// 		dataLenToCopy -= stream.dataOffset

// 		if dataLenToCopy > count {
// 			/* Remaining data space in OD variable is larger than current count
// 			 * of data, so only current count of data will be copied */
// 			dataLenToCopy = count
// 			stream.dataOffset += dataLenToCopy
// 			returnCode = ODR_PARTIAL
// 		} else {
// 			stream.dataOffset = 0 /* copy finished, reset offset */
// 		}
// 	}

// 	if dataLenToCopy < count {
// 		/* OD variable is smaller than current amount of data */
// 		return ODR_DATA_LONG, 0
// 	}
// 	copy(stream.dataOrig[stream.dataOffset:stream.dataOffset+dataLenToCopy], data)
// 	return returnCode, dataLenToCopy
// }

type DisabledReader struct{}
type DisabledWriter struct{}

func (reader *DisabledReader) Read(b []byte) (n int, err error) {
	return 0, ODR_UNSUPP_ACCESS
}

func (writer *DisabledWriter) Write(b []byte) (n int, err error) {
	return 0, ODR_UNSUPP_ACCESS
}

/* Write value to variable from Object Dictionary disabled, see OD_IO_t */
// func (stream ODStream) WriteDisabled(data []byte) (result ODR, n uint32) {
// 	return ODR_UNSUPP_ACCESS, 0
// }

// /* Read value from variable from Object Dictionary disabled, see OD_IO_t*/
// func (stream ODStream) ReadDisabled(data []byte) (result ODR, n uint32) {
// 	return ODR_UNSUPP_ACCESS, 0
// }

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
		stream.Data = *bytes.NewBuffer(object.Data)

	case Array:
		subEntriesCount := len(object.Variables)
		if subindex >= uint8(subEntriesCount) {
			return ODR_SUB_NOT_EXIST
		}
		stream.Attribute = object.Variables[subindex].Attribute
		stream.Data = *bytes.NewBuffer(object.Variables[subindex].Data)

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
		stream.Data = *bytes.NewBuffer(record.Variable.Data)

	default:
		log.Errorf("Error, unknown type : %+v", object)
		return ODR_DEV_INCOMPAT
	}
	log.Infof("Created stream object at %x|%x, %v", entry.Index, subindex, streamer.Stream)
	// Populate the used readers or writers if an extension is used
	if entry.Extension == nil || origin {
		streamer.Reader = bufio.NewReader(&stream.Data)
		streamer.Writer = bufio.NewWriter(&stream.Data)
		stream.Object = nil
	} else {
		log.Infof("Special object extension at %x|%x, %v", entry.Index, subindex, streamer.Stream)
		if entry.Extension.Reader == nil {
			streamer.Reader = &DisabledReader{}
		} else {
			streamer.Reader = entry.Extension.Reader
		}
		if entry.Extension.Writer == nil {
			streamer.Writer = &DisabledWriter{}
		} else {
			streamer.Writer = entry.Extension.Writer
		}
		stream.Object = entry.Extension.Object
	}

	/* Reset stream data offset */
	// stream.dataOffset = 0
	stream.Subindex = subindex
	log.Infof("Created stream object at %x|%x, %v", entry.Index, subindex, streamer.Stream.Data)
	return nil
}

// TODO check also that the length is correct

// Read Uint8 inside object dictionary
func (entry *Entry) ReadUint8(subindex uint8, data *uint8) error {
	streamer := &ObjectStreamer{}
	// Create and populate the streamer object
	err := entry.Sub(subindex, true, streamer)
	if err != nil {
		return err
	}
	return binary.Read(streamer.Reader, binary.LittleEndian, data)
}

// Read Uint16 inside object dictionary
func (entry *Entry) ReadUint16(subindex uint8, data *uint16) error {
	streamer := &ObjectStreamer{}
	// Create and populate the streamer object
	err := entry.Sub(subindex, true, streamer)
	if err != nil {
		return err
	}
	return binary.Read(streamer.Reader, binary.LittleEndian, data)
}

// Read Uint32 inside object dictionary
func (entry *Entry) ReadUint32(subindex uint8, data *uint32) error {
	streamer := &ObjectStreamer{}
	// Create and populate the streamer object
	err := entry.Sub(subindex, true, streamer)
	if err != nil {
		return err
	}
	return binary.Read(streamer.Reader, binary.LittleEndian, data)
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
