package canopen

/**
 * Return codes from OD access functions.
 *
 * @ref OD_getSDOabCode() can be used to retrieve corresponding SDO abort code.
 */

type ODR int8

const (
	/* !!!! WARNING !!!!
	 * If changing these values, change also OD_getSDOabCode() function!
	 */
	/** Read/write is only partial, make more calls */
	ODR_PARTIAL ODR = -1
	/** SDO abort 0x00000000 - Read/write successfully finished */
	ODR_OK ODR = 0
	/** SDO abort 0x05040005 - Out of memory */
	ODR_OUT_OF_MEM ODR = 1
	/** SDO abort 0x06010000 - Unsupported access to an object */
	ODR_UNSUPP_ACCESS ODR = 2
	/** SDO abort 0x06010001 - Attempt to read a write only object */
	ODR_WRITEONLY ODR = 3
	/** SDO abort 0x06010002 - Attempt to write a read only object */
	ODR_READONLY ODR = 4
	/** SDO abort 0x06020000 - Object does not exist in the object dict. */
	ODR_IDX_NOT_EXIST = 5
	/** SDO abort 0x06040041 - Object cannot be mapped to the PDO */
	ODR_NO_MAP ODR = 6
	/** SDO abort 0x06040042 - PDO length exceeded */
	ODR_MAP_LEN ODR = 7
	/** SDO abort 0x06040043 - General parameter incompatibility reasons */
	ODR_PAR_INCOMPAT ODR = 8
	/** SDO abort 0x06040047 - General internal incompatibility in device */
	ODR_DEV_INCOMPAT ODR = 9
	/** SDO abort 0x06060000 - Access failed due to hardware error */
	ODR_HW ODR = 10
	/** SDO abort 0x06070010 - Data type does not match */
	ODR_TYPE_MISMATCH ODR = 11
	/** SDO abort 0x06070012 - Data type does not match, length too high */
	ODR_DATA_LONG ODR = 12
	/** SDO abort 0x06070013 - Data type does not match, length too short */
	ODR_DATA_SHORT ODR = 13
	/** SDO abort 0x06090011 - Sub index does not exist */
	ODR_SUB_NOT_EXIST ODR = 14
	/** SDO abort 0x06090030 - Invalid value for parameter (download only) */
	ODR_INVALID_VALUE ODR = 15
	/** SDO abort 0x06090031 - Value range of parameter written too high */
	ODR_VALUE_HIGH ODR = 16
	/** SDO abort 0x06090032 - Value range of parameter written too low */
	ODR_VALUE_LOW ODR = 17
	/** SDO abort 0x06090036 - Maximum value is less than minimum value */
	ODR_MAX_LESS_MIN ODR = 18
	/** SDO abort 0x060A0023 - Resource not available: SDO connection */
	ODR_NO_RESOURCE ODR = 19
	/** SDO abort 0x08000000 - General error */
	ODR_GENERAL ODR = 20
	/** SDO abort 0x08000020 - Data cannot be transferred or stored to app */
	ODR_DATA_TRANSF ODR = 21
	/** SDO abort 0x08000021 - Data can't be transferred (local control) */
	ODR_DATA_LOC_CTRL ODR = 22
	/** SDO abort 0x08000022 - Data can't be transf. (present device state) */
	ODR_DATA_DEV_STATE ODR = 23
	/** SDO abort 0x08000023 - Object dictionary not present */
	ODR_OD_MISSING ODR = 24
	/** SDO abort 0x08000024 - No data available */
	ODR_NO_DATA ODR = 25
	/** Last element, number of responses */
	ODR_COUNT ODR = 26
)

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

//Get the associated abort code, if the code is not present in map, return ODR_DEV_INCOMPAT
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

type ODStream struct {
	/** Pointer to original data object, defined by Object Dictionary. Default
	 * read/write functions operate on it. If memory for data object is not
	 * specified by Object Dictionary, then dataOrig is NULL.
	 */
	dataOrig []byte
	/** Pointer to object, passed by @ref OD_extension_init(). Can be used
	 * inside read / write functions from IO extension.
	 */
	object interface{}
	/** In case of large data, dataOffset indicates position of already
	 * transferred data */
	dataOffset uint32
	/** Attribute bit-field of the OD sub-object, see @ref OD_attributes_t */
	attribute ODA
	/** Sub index of the OD sub-object, informative */
	subIndex uint8
}

func NewODStream() ODStream {
	return ODStream{dataOrig: nil, object: nil, dataOffset: 0, attribute: 0, subIndex: 0}
}

/**
 * Extension of OD object, which can optionally be specified by application in
 * initialization phase with @ref OD_extension_init() function.
 */
type Extension struct {
	/** Object on which read and write will operate, part of @ref ODStream */
	object   *interface{}
	Read     func(buffer []byte) (result ODR, n uint32)
	Write    func(buffer []byte) (result ODR, n uint32)
	flagsPDO [4]uint8
}

type OD_io struct {
	/** Object Dictionary stream object, passed to read or write */
	stream ODStream
	Read   func(buffer []byte) (result ODR, n uint32)
	Write  func(buffer []byte) (result ODR, n uint32)
}

type Entry struct {
	index     uint16
	odObject  interface{}
	extension *Extension
}

type ObjectDictionary struct {
	list []Entry
}

/*basic OD variable */
type Variable struct {
	Data      []byte
	Attribute ODA
}

/**
 * Object for OD array of variables, used for "ARRAY" type OD objects
 */
type Array struct {
	Data0             []byte /**< Pointer to data for sub-index 0 */
	Data              []byte /**< Pointer to array of data */
	Attribute0        ODA
	Attribute         ODA /**< Attribute bitfield for array elements */
	DataElementLength uint32
}

/**
 * Object for OD sub-elements, used in "RECORD" type OD objects
 */
type Record struct {
	Data      []byte
	Subindex  uint8
	Attribute ODA
}

/**
 * Read value from original OD location
 *
 * This function can be used inside read / write functions, specified by
 * @ref OD_extension_init(). It reads data directly from memory location
 * specified by Object dictionary. If no IO extension is used on OD entry, then
 * io->read returned by @ref OD_getSub() equals to this function. See
 * also @ref OD_IO_t.
 */

func (stream *ODStream) ReadOriginal(data []byte) (result ODR, n uint32) {
	dataLenToCopy := uint32(len(stream.dataOrig)) /* length of OD variable */
	count := uint32(len(data))
	dataOrig := stream.dataOrig
	if dataOrig == nil {
		return ODR_SUB_NOT_EXIST, 0
	}

	var returnCode ODR
	returnCode = ODR_OK

	/* If previous read was partial or OD variable length is larger than
	 * current buffer size, then data was (will be) read in several segments */

	if stream.dataOffset > 0 || dataLenToCopy > count {
		if stream.dataOffset >= dataLenToCopy {
			return ODR_DEV_INCOMPAT, 0
		}
		/* Reduce for already copied data */
		dataLenToCopy -= stream.dataOffset

		if dataLenToCopy > count {
			/* Not enough space in destination buffer */
			dataLenToCopy = count
			stream.dataOffset += dataLenToCopy
			returnCode = ODR_PARTIAL
		} else {
			stream.dataOffset = 0 /* copy finished, reset offset */
		}
	}
	copy(data, stream.dataOrig[stream.dataOffset:stream.dataOffset+dataLenToCopy])
	return returnCode, dataLenToCopy
}

func (stream ODStream) WriteOriginal(data []byte) (result ODR, n uint32) {
	if data == nil {
		return ODR_DEV_INCOMPAT, 0
	}
	dataLenToCopy := uint32(len(stream.dataOrig)) /* length of OD variable */
	dataOrig := stream.dataOrig
	count := uint32(len(data))

	if dataOrig == nil {
		return ODR_SUB_NOT_EXIST, 0
	}

	var returnCode ODR
	returnCode = ODR_OK

	/* If previous write was partial or OD variable length is larger than
	 * current buffer size, then data was (will be) written in several
	 * segments */

	if stream.dataOffset > 0 || dataLenToCopy > count {
		if stream.dataOffset >= dataLenToCopy {
			return ODR_DEV_INCOMPAT, 0
		}
		/* reduce for already copied data */
		dataLenToCopy -= stream.dataOffset

		if dataLenToCopy > count {
			/* Remaining data space in OD variable is larger than current count
			 * of data, so only current count of data will be copied */
			dataLenToCopy = count
			stream.dataOffset += dataLenToCopy
			returnCode = ODR_PARTIAL
		} else {
			stream.dataOffset = 0 /* copy finished, reset offset */
		}
	}

	if dataLenToCopy < count {
		/* OD variable is smaller than current amount of data */
		return ODR_DATA_LONG, 0
	}
	copy(stream.dataOrig[stream.dataOffset:stream.dataOffset+dataLenToCopy], data)
	return returnCode, dataLenToCopy
}

/* Write value to variable from Object Dictionary disabled, see OD_IO_t */
func (stream ODStream) WriteDisabled(data []byte) (result ODR, n uint32) {
	return ODR_UNSUPP_ACCESS, 0
}

/* Read value from variable from Object Dictionary disabled, see OD_IO_t*/
func (stream ODStream) ReadDisabled(data []byte) (result ODR, n uint32) {
	return ODR_UNSUPP_ACCESS, 0
}

/**
 * Find OD entry in Object Dictionary
 *
 * @param od Object Dictionary
 * @param index CANopen Object Dictionary index of object in Object Dictionary
 *
 * @return Pointer to OD entry or NULL if not found
 */

func (od *ObjectDictionary) Find(index uint16) (entry *Entry) {
	if od == nil || len(od.list) == 0 {
		return nil
	}
	min := 0
	max := len(od.list) - 1

	/* Fast search (binary search) in ordered Object Dictionary. If indexes are mixed,
	 * this won't work. If Object Dictionary has up to N entries, then the
	 * max number of loop passes is log2(N) */
	for min < max {
		/* get entry between min and max */
		cur := (min + max) >> 1
		entry = &od.list[cur]

		if index == entry.index {
			return entry
		}

		if index < entry.index {
			if cur > 0 {
				max = cur - 1
			} else {
				max = cur
			}
		} else {
			min = cur + 1
		}
	}

	if min == max {
		entry = &od.list[min]
		if index == entry.index {
			return entry
		}
	}

	return nil /* entry does not exist in OD */
}

/**
 * Find sub-object with specified sub-index on OD entry returned by OD_find.
 * Function populates io structure with sub-object data.
 *
 * @warning
 * Read and write functions may be called from different threads, so critical
 * sections in custom functions must be observed, see @ref CO_critical_sections.
 *
 * @param entry OD entry returned by @ref OD_find().
 * @param subIndex Sub-index of the variable from the OD object.
 * @param [out] io Structure will be populated on success.
 * @param odOrig If true, then potential IO extension on entry will be
 * ignored and access to data entry in the original OD location will be returned
 *
 * @return Value from @ref ODR_t, "ODR_OK" in case of success.
 */

func (entry *Entry) Sub(subindex uint8, origin bool) (result ODR, io *OD_io) {

	if entry == nil || entry.odObject == nil {
		return ODR_IDX_NOT_EXIST, nil
	}

	io = &OD_io{stream: NewODStream(), Read: nil, Write: nil}
	stream := io.stream
	object := entry.odObject
	/* attribute, dataOrig and dataLength, depends on object type */
	switch object := object.(type) {
	case Variable:
		if subindex > 0 {
			return ODR_SUB_NOT_EXIST, nil
		}
		stream.attribute = object.Attribute
		stream.dataOrig = object.Data

	case Array:
		subEntriesCount := 1 + len(object.Data)/int(object.DataElementLength)
		if subindex >= uint8(subEntriesCount) {
			return ODR_SUB_NOT_EXIST, nil
		}
		if subindex == 0 {
			stream.attribute = object.Attribute0
			stream.dataOrig = object.Data0
			// datalength is 0
		} else {
			stream.attribute = object.Attribute
			// Get the according bytes
			stream.dataOrig = object.Data[object.DataElementLength*uint32(subindex-1) : object.DataElementLength*uint32(subindex)]
		}

	case []Record:
		records := object
		var record *Record
		for i := range records {
			if records[i].Subindex == subindex {
				record = &records[i]
			}
		}
		if record == nil {
			return ODR_SUB_NOT_EXIST, nil
		}
		stream.attribute = record.Attribute
		stream.dataOrig = record.Data

	default:
		return ODR_DEV_INCOMPAT, nil
	}

	/*Populate read/write function pointers either with default or extension provided*/
	if entry.extension == nil || origin {
		io.Read = stream.ReadOriginal
		io.Write = stream.WriteOriginal
		stream.object = nil
	} else {
		if entry.extension.Read == nil {
			io.Read = stream.ReadDisabled
		} else {
			io.Read = entry.extension.Read
		}
		if entry.extension.Write == nil {
			io.Write = stream.WriteDisabled
		} else {
			io.Write = entry.extension.Write
		}
		stream.object = entry.extension.object
	}

	/* Reset stream data offset */
	stream.dataOffset = 0
	stream.subIndex = subindex

	return ODR_OK, io
}

/* Create a new Object dictionary Entry of Variable type */
func NewVariableEntry(index uint16, data []byte, attribute ODA) Entry {
	odObject := Variable{Data: data, Attribute: attribute}
	return Entry{index: index, odObject: odObject, extension: nil}
}

/* Create a new Object dictionary Entry of Record type, odObject is an empty slice of Record elements */
func NewRecordEntry(index uint16, records []Record) Entry {
	return Entry{index: index, odObject: records, extension: nil}
}

/* Create a new Object dictionary Entry of Array type*/
func NewArrayEntry(index uint16, data0 []byte, data []byte, attribute0 ODA, attribute ODA, element_length_bytes uint32) Entry {
	odObject := Array{Data0: data0, Data: data, Attribute0: attribute0, Attribute: attribute, DataElementLength: element_length_bytes}
	return Entry{index: index, odObject: odObject, extension: nil}
}
