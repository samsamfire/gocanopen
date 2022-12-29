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
	ODR_OK = 0
	/** SDO abort 0x05040005 - Out of memory */
	ODR_OUT_OF_MEM = 1
	/** SDO abort 0x06010000 - Unsupported access to an object */
	ODR_UNSUPP_ACCESS = 2
	/** SDO abort 0x06010001 - Attempt to read a write only object */
	ODR_WRITEONLY = 3
	/** SDO abort 0x06010002 - Attempt to write a read only object */
	ODR_READONLY = 4
	/** SDO abort 0x06020000 - Object does not exist in the object dict. */
	ODR_IDX_NOT_EXIST = 5
	/** SDO abort 0x06040041 - Object cannot be mapped to the PDO */
	ODR_NO_MAP = 6
	/** SDO abort 0x06040042 - PDO length exceeded */
	ODR_MAP_LEN = 7
	/** SDO abort 0x06040043 - General parameter incompatibility reasons */
	ODR_PAR_INCOMPAT = 8
	/** SDO abort 0x06040047 - General internal incompatibility in device */
	ODR_DEV_INCOMPAT = 9
	/** SDO abort 0x06060000 - Access failed due to hardware error */
	ODR_HW = 10
	/** SDO abort 0x06070010 - Data type does not match */
	ODR_TYPE_MISMATCH = 11
	/** SDO abort 0x06070012 - Data type does not match, length too high */
	ODR_DATA_LONG = 12
	/** SDO abort 0x06070013 - Data type does not match, length too short */
	ODR_DATA_SHORT = 13
	/** SDO abort 0x06090011 - Sub index does not exist */
	ODR_SUB_NOT_EXIST = 14
	/** SDO abort 0x06090030 - Invalid value for parameter (download only) */
	ODR_INVALID_VALUE = 15
	/** SDO abort 0x06090031 - Value range of parameter written too high */
	ODR_VALUE_HIGH = 16
	/** SDO abort 0x06090032 - Value range of parameter written too low */
	ODR_VALUE_LOW = 17
	/** SDO abort 0x06090036 - Maximum value is less than minimum value */
	ODR_MAX_LESS_MIN = 18
	/** SDO abort 0x060A0023 - Resource not available: SDO connection */
	ODR_NO_RESOURCE = 19
	/** SDO abort 0x08000000 - General error */
	ODR_GENERAL = 20
	/** SDO abort 0x08000020 - Data cannot be transferred or stored to app */
	ODR_DATA_TRANSF = 21
	/** SDO abort 0x08000021 - Data can't be transferred (local control) */
	ODR_DATA_LOC_CTRL = 22
	/** SDO abort 0x08000022 - Data can't be transf. (present device state) */
	ODR_DATA_DEV_STATE = 23
	/** SDO abort 0x08000023 - Object dictionary not present */
	ODR_OD_MISSING = 24
	/** SDO abort 0x08000024 - No data available */
	ODR_NO_DATA = 25
	/** Last element, number of responses */
	ODR_COUNT = 26
)

type OD_stream_t struct {
	/** Pointer to original data object, defined by Object Dictionary. Default
	 * read/write functions operate on it. If memory for data object is not
	 * specified by Object Dictionary, then dataOrig is NULL.
	 */
	dataOrig []uint8
	/** Pointer to object, passed by @ref OD_extension_init(). Can be used
	 * inside read / write functions from IO extension.
	 */
	object *uint8
	/** In case of large data, dataOffset indicates position of already
	 * transferred data */
	dataOffset uint32
	/** Attribute bit-field of the OD sub-object, see @ref OD_attributes_t */
	attribute uint8
	/** Sub index of the OD sub-object, informative */
	subIndex uint8
}

/**
 * Extension of OD object, which can optionally be specified by application in
 * initialization phase with @ref OD_extension_init() function.
 */
type OD_Extension_t struct {
	/** Object on which read and write will operate, part of @ref OD_stream_t */
	object *interface{}
	/** Application specified read function pointer. If NULL, then read will be
	 * disabled. @ref OD_readOriginal can be used here to keep the original read
	 * function. For function description see @ref OD_IO_t. */
	read func(stream *OD_stream_t, buf []uint8, countRead *uint32) uint8
	/** Application specified write function pointer. If NULL, then write will
	 * be disabled. @ref OD_writeOriginal can be used here to keep the original
	 * write function. For function description see @ref OD_IO_t. */
	write func(stream *OD_stream_t, buf []uint8, countWritten *uint32) uint8
	/**PDO flags bit-field provides one bit for each OD variable, which exist
	 * inside OD object at specific sub index. If application clears that bit,
	 * and OD variable is mapped to an event driven TPDO, then TPDO will be
	 * sent.
	 *
	 * @ref OD_FLAGS_PDO_SIZE can have a value from 0 to 32 bytes, which
	 * corresponds to 0 to 256 available bits. If, for example,
	 * @ref OD_FLAGS_PDO_SIZE has value 4, then OD variables with sub index up
	 * to 31 will have the TPDO requesting functionality.
	 * See also @ref OD_requestTPDO and @ref OD_TPDOtransmitted. */
	flagsPDO [4]uint8
}

type OD_IO_t struct {
}

type OD_entry_t struct {
	/** Object Dictionary index */
	index uint16
	/** Number of all sub-entries, including sub-entry at sub-index 0 */
	subEntriesCount uint8
	/** Type of the odObject, indicated by @ref OD_objectTypes_t enumerator. */
	odObjectType uint8
	/** OD object of type indicated by odObjectType, from which @ref OD_getSub()
	* fetches the information */
	odObject *interface{}
	/** Extension to OD, specified by application */
	extension *OD_Extension_t
}

type OD_t struct {
	/** Number of elements in the list, without last element, which is blank */
	size uint16
	/** List OD entries (table of contents), ordered by index */
	list []OD_entry_t
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

func (stream OD_stream_t) ReadOriginal(count uint32) (result ODR, data []uint8) {
	dataLenToCopy := uint32(len(stream.dataOrig)) /* length of OD variable */
	dataOrig := stream.dataOrig
	if dataOrig == nil {
		return ODR_SUB_NOT_EXIST, nil
	}

	var returnCode ODR
	returnCode = ODR_OK

	/* If previous read was partial or OD variable length is larger than
	 * current buffer size, then data was (will be) read in several segments */

	if stream.dataOffset > 0 || dataLenToCopy > count {
		if stream.dataOffset >= dataLenToCopy {
			return ODR_DEV_INCOMPAT, nil
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
	return returnCode, stream.dataOrig[stream.dataOffset : stream.dataOffset+dataLenToCopy]
}

func (stream OD_stream_t) WriteOriginal(data []uint8) (result ODR) {
	if data == nil {
		return ODR_DEV_INCOMPAT
	}
	dataLenToCopy := uint32(len(stream.dataOrig)) /* length of OD variable */
	dataOrig := stream.dataOrig
	count := uint32(len(data))

	if dataOrig == nil {
		return ODR_SUB_NOT_EXIST
	}

	var returnCode ODR
	returnCode = ODR_OK

	/* If previous write was partial or OD variable length is larger than
	 * current buffer size, then data was (will be) written in several
	 * segments */

	if stream.dataOffset > 0 || dataLenToCopy > count {
		if stream.dataOffset >= dataLenToCopy {
			return ODR_DEV_INCOMPAT
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
		return ODR_DATA_LONG
	}
	copy(stream.dataOrig[stream.dataOffset:stream.dataOffset+dataLenToCopy], data)
	return returnCode
}
