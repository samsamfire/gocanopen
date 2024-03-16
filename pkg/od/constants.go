package od

import (
	"fmt"
	"strconv"
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

const OD_FLAGS_PDO_SIZE uint8 = 32
const (
	BASE_RPDO_COMMUNICATION_INDEX       = uint16(0x1400)
	BASE_RPDO_MAPPING_INDEX             = uint16(0x1600)
	BASE_TPDO_COMMUNICATION_INDEX       = uint16(0x1800)
	BASE_TPDO_MAPPING_INDEX             = uint16(0x1A00)
	PDO_MAX_MAPPED_ENTRIES        uint8 = 8
)

func (odr ODR) Error() string {
	return fmt.Sprintf("OD error %v", strconv.Itoa(int(odr)))
}

// Object dictionary object attribute
const (
	ATTRIBUTE_SDO_R  uint8 = 0x01 // SDO server may read from the variable
	ATTRIBUTE_SDO_W  uint8 = 0x02 // SDO server may write to the variable
	ATTRIBUTE_SDO_RW uint8 = 0x03 // SDO server may read from or write to the variable
	ATTRIBUTE_TPDO   uint8 = 0x04 // Variable is mappable into TPDO (can be read)
	ATTRIBUTE_RPDO   uint8 = 0x08 // Variable is mappable into RPDO (can be written)
	ATTRIBUTE_TRPDO  uint8 = 0x0C // Variable is mappable into TPDO or RPDO
	ATTRIBUTE_TSRDO  uint8 = 0x10 // Variable is mappable into transmitting SRDO
	ATTRIBUTE_RSRDO  uint8 = 0x20 // Variable is mappable into receiving SRDO
	ATTRIBUTE_TRSRDO uint8 = 0x30 // Variable is mappable into tx or rx SRDO
	ATTRIBUTE_MB     uint8 = 0x40 // Variable is multi-byte ((u)int16_t to (u)int64_t)
	// Shorter value, than specified variable size, may be
	// written to the variable. SDO write will fill remaining memory with zeroes.
	// Attribute is used for VISIBLE_STRING and UNICODE_STRING.
	ATTRIBUTE_STR uint8 = 0x80
)
