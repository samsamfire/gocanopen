package od

import (
	"errors"
	"fmt"
	"strconv"
)

var ErrEdsFormat = errors.New("invalid EDS format")

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

var ErrorDescriptionMap = map[ODR]string{
	ErrPartial:      "Incomplete transfer",
	ErrNo:           "No error",
	ErrOutOfMem:     "Out of memory",
	ErrUnsuppAccess: "Unsupported access to an object",
	ErrWriteOnly:    "Attempt to read a write only object",
	ErrReadonly:     "Attempt to write a read only object",
	ErrIdxNotExist:  "Object does not exist in the object dictionary",
	ErrNoMap:        "Object cannot be mapped to the PDO",
	ErrMapLen:       "Num and len of object to be mapped exceeds PDO len",
	ErrParIncompat:  "General parameter incompatibility reasons",
	ErrDevIncompat:  "General internal incompatibility in device",
	ErrHw:           "Access failed due to hardware error",
	ErrTypeMismatch: "Data type does not match, length does not match",
	ErrDataLong:     "Data type does not match, length too high",
	ErrDataShort:    "Data type does not match, length too short",
	ErrSubNotExist:  "Sub index does not exist",
	ErrInvalidValue: "Invalid value for parameter (download only)",
	ErrValueHigh:    "Value range of parameter written too high",
	ErrValueLow:     "Value range of parameter written too low",
	ErrMaxLessMin:   "Maximum value is less than minimum value.",
	ErrNoRessource:  "Resource not available: SDO connection",
	ErrGeneral:      "General error",
	ErrDataTransf:   "Data cannot be transferred or stored to application",
	ErrDataLocCtrl:  "Data cannot be transferred because of local control",
	ErrDataDevState: "Data cannot be tran. because of present device state",
	ErrOdMissing:    "Object dict. not present or dynamic generation fails",
	ErrNoData:       "No data available",
}

func (odr ODR) Error() string {
	description, ok := ErrorDescriptionMap[odr]
	if !ok {
		return fmt.Sprintf("OD error %v (%v)", strconv.Itoa(int(odr)), "unknown")
	}
	return fmt.Sprintf("OD error %v (%v)", strconv.Itoa(int(odr)), description)
}

const (
	MaxMappedEntriesPdo = uint8(8)
	FlagsPdoSize        = uint8(32)
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

// Standard CANopen object entries index
const (
	EntryDeviceType                  uint16 = 0x1000
	EntryErrorRegister               uint16 = 0x1001
	EntryManufacturerStatusRegister  uint16 = 0x1003
	EntryCobIdSYNC                   uint16 = 0x1005
	EntryCommunicationCyclePeriod    uint16 = 0x1006
	EntrySynchronousWindowLength     uint16 = 0x1007
	EntryManufacturerDeviceName      uint16 = 0x1008
	EntryManufacturerHardwareVersion uint16 = 0x1009
	EntryManufacturerSoftwareVersion uint16 = 0x100A
	EntryStoreParameters             uint16 = 0x1010
	EntryRestoreDefaultParameters    uint16 = 0x1011
	EntryCobIdTIME                   uint16 = 0x1012
	EntryHighResTimestamp            uint16 = 0x1013
	EntryCobIdEMCY                   uint16 = 0x1014
	EntryInhibitTimeEMCY             uint16 = 0x1015
	EntryConsumerHeartbeatTime       uint16 = 0x1016
	EntryProducerHeartbeatTime       uint16 = 0x1017
	EntryIdentityObject              uint16 = 0x1018
	EntrySynchronousCounterOverflow  uint16 = 0x1019
	EntryStoreEDS                    uint16 = 0x1021
	EntryStorageFormat               uint16 = 0x1022
	EntryRPDOCommunicationStart      uint16 = 0x1400
	EntryRPDOCommunicationEnd        uint16 = 0x15FF
	EntryRPDOMappingStart            uint16 = 0x1600
	EntryRPDOMappingEnd              uint16 = 0x17FF
	EntryTPDOCommunicationStart      uint16 = 0x1800
	EntryTPDOCommunicationEnd        uint16 = 0x19FF
	EntryTPDOMappingStart            uint16 = 0x1A00
	EntryTPDOMappingEnd              uint16 = 0x1BFF
)

// Standard CANopen object areas
const (
	AreaCommunicationProfileStart        uint16 = 0x1000
	AreaCommunicationProfileEnd          uint16 = 0x1FFF
	AreaManufacturerSpecificProfileStart uint16 = 0x2000
	AreaManufacturerSpecificProfileEnd   uint16 = 0x5FFF
	AreaDeviceProfileStart               uint16 = 0x6000
	AreaDeviceProfileEnd                 uint16 = 0x9FFF
	AreaInterfaceProfileStart            uint16 = 0xA000
	AreaInterfaceProfileEnd              uint16 = 0xBFFF
	AreaFutureUseStart                   uint16 = 0xC000
	AreaFutureUseEnd                     uint16 = 0xFFFF
)

// EDS formats
const (
	FormatEDSAscii  = 0
	FormatEDSZipped = 0x90
)
