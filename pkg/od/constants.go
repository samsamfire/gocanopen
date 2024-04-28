package od

import (
	"fmt"
	"strconv"
)

type ODR int8

const (
	ErrPartial      ODR = -1
	ErrNo           ODR = 0
	ErrOutOfMem     ODR = 1
	ErrUnsuppAccess ODR = 2
	ErrWriteOnly    ODR = 3
	ErrReadonly     ODR = 4
	ErrIdxNotExist  ODR = 5
	ErrNoMap        ODR = 6
	ErrMapLen       ODR = 7
	ErrParIncompat  ODR = 8
	ErrDevIncompat  ODR = 9
	ErrHw           ODR = 10
	ErrTypeMismatch ODR = 11
	ErrDataLong     ODR = 12
	ErrDataShort    ODR = 13
	ErrSubNotExist  ODR = 14
	ErrInvalidValue ODR = 15
	ErrValueHigh    ODR = 16
	ErrValueLow     ODR = 17
	ErrMaxLessMin   ODR = 18
	ErrNoRessource  ODR = 19
	ErrGeneral      ODR = 20
	ErrDataTransf   ODR = 21
	ErrDataLocCtrl  ODR = 22
	ErrDataDevState ODR = 23
	ErrOdMissing    ODR = 24
	ErrNoData       ODR = 25
	ErrCount        ODR = 26
)

func (odr ODR) Error() string {
	return fmt.Sprintf("OD error %v", strconv.Itoa(int(odr)))
}

const (
	IndexRpdoCommunicationBase = uint16(0x1400)
	IndexRpdoMappingBase       = uint16(0x1600)
	IndexTpdoCommunicationBase = uint16(0x1800)
	IndexTpdoMappingBase       = uint16(0x1A00)
	MaxMappedEntriesPdo        = uint8(8)
	FlagsPdoSize               = uint8(32)
)

// Object dictionary object attribute
const (
	AttributeSdoR   uint8 = 0x01 // SDO server may read from the variable
	AttributeSdoW   uint8 = 0x02 // SDO server may write to the variable
	AttributeSdoRw  uint8 = 0x03 // SDO server may read from or write to the variable
	AttributeTpdo   uint8 = 0x04 // Variable is mappable into TPDO (can be read)
	AttributeRpdo   uint8 = 0x08 // Variable is mappable into RPDO (can be written)
	AttributeTrpdo  uint8 = 0x0C // Variable is mappable into TPDO or RPDO
	AttributeTsrdo  uint8 = 0x10 // Variable is mappable into transmitting SRDO
	AttributeRsrdo  uint8 = 0x20 // Variable is mappable into receiving SRDO
	AttributeTrsrdo uint8 = 0x30 // Variable is mappable into tx or rx SRDO
	AttributeMb     uint8 = 0x40 // Variable is multi-byte ((u)int16_t to (u)int64_t)
	// Shorter value, than specified variable size, may be
	// written to the variable. SDO write will fill remaining memory with zeroes.
	// Attribute is used for VISIBLE_STRING and UNICODE_STRING.
	AttributeStr uint8 = 0x80
)
