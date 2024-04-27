package sdo

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/samsamfire/gocanopen/internal/crc"
	"github.com/samsamfire/gocanopen/pkg/od"
	log "github.com/sirupsen/logrus"
)

var ErrWrongClientReturnValue = errors.New("wrong client return value")
var ErrInvalidArgs = errors.New("error in arguments")

type internalState uint8

const (
	DefaultClientTimeout = 1000
	DefaultServerTimeout = 1000
	ClientBaseId         = 0x600
	ServerBaseId         = 0x580
)

const (
	stateIdle                   internalState = 0x00
	stateAbort                  internalState = 0x01
	stateDownloadLocalTransfer  internalState = 0x10
	stateDownloadInitiateReq    internalState = 0x11
	stateDownloadInitiateRsp    internalState = 0x12
	stateDownloadSegmentReq     internalState = 0x13
	stateDownloadSegmentRsp     internalState = 0x14
	stateUploadLocalTransfer    internalState = 0x20
	stateUploadInitiateReq      internalState = 0x21
	stateUploadInitiateRsp      internalState = 0x22
	stateUploadSegmentReq       internalState = 0x23
	stateUploadSegmentRsp       internalState = 0x24
	stateDownloadBlkInitiateReq internalState = 0x51
	stateDownloadBlkInitiateRsp internalState = 0x52
	stateDownloadBlkSubblockReq internalState = 0x53
	stateDownloadBlkSubblockRsp internalState = 0x54
	stateDownloadBlkEndReq      internalState = 0x55
	stateDownloadBlkEndRsp      internalState = 0x56
	stateUploadBlkInitiateReq   internalState = 0x61
	stateUploadBlkInitiateRsp   internalState = 0x62
	stateUploadBlkInitiateReq2  internalState = 0x63
	stateUploadBlkSubblockSreq  internalState = 0x64
	stateUploadBlkSubblockCrsp  internalState = 0x65
	stateUploadBlkEndSreq       internalState = 0x66
	stateUploadBlkEndCrsp       internalState = 0x67
)

const (
	AbortToggleBit         Abort = 0x05030000
	AbortTimeout           Abort = 0x05040000
	AbortCmd               Abort = 0x05040001
	AbortBlockSize         Abort = 0x05040002
	AbortSeqNum            Abort = 0x05040003
	AbortCRC               Abort = 0x05040004
	AbortOutOfMem          Abort = 0x05040005
	AbortUnsupportedAccess Abort = 0x06010000
	AbortWriteOnly         Abort = 0x06010001
	AbortReadOnly          Abort = 0x06010002
	AbortNotExist          Abort = 0x06020000
	AbortNoMap             Abort = 0x06040041
	AbortMapLen            Abort = 0x06040042
	AbortParamIncompat     Abort = 0x06040043
	AbortDeviceIncompat    Abort = 0x06040047
	AbortHardware          Abort = 0x06060000
	AbortTypeMismatch      Abort = 0x06070010
	AbortDataLong          Abort = 0x06070012
	AbortDataShort         Abort = 0x06070013
	AbortSubUnknown        Abort = 0x06090011
	AbortInvalidValue      Abort = 0x06090030
	AbortValueHigh         Abort = 0x06090031
	AbortValueLow          Abort = 0x06090032
	AbortMaxLessMin        Abort = 0x06090036
	AbortNoRessource       Abort = 0x060A0023
	AbortGeneral           Abort = 0x08000000
	AbortDataTransfer      Abort = 0x08000020
	AbortDataLocalControl  Abort = 0x08000021
	AbortDataDeviceState   Abort = 0x08000022
	AbortDataOD            Abort = 0x08000023
	AbortNoData            Abort = 0x08000024
)

var AbortCodeDescriptionMap = map[Abort]string{
	AbortToggleBit:         "Toggle bit not altered",
	AbortTimeout:           "SDO protocol timed out",
	AbortCmd:               "Command specifier not valid or unknown",
	AbortBlockSize:         "Invalid block size in block mode",
	AbortSeqNum:            "Invalid sequence number in block mode",
	AbortCRC:               "CRC error (block mode only)",
	AbortOutOfMem:          "Out of memory",
	AbortUnsupportedAccess: "Unsupported access to an object",
	AbortWriteOnly:         "Attempt to read a write only object",
	AbortReadOnly:          "Attempt to write a read only object",
	AbortNotExist:          "Object does not exist in the object dictionary",
	AbortNoMap:             "Object cannot be mapped to the PDO",
	AbortMapLen:            "Num and len of object to be mapped exceeds PDO len",
	AbortParamIncompat:     "General parameter incompatibility reasons",
	AbortDeviceIncompat:    "General internal incompatibility in device",
	AbortHardware:          "Access failed due to hardware error",
	AbortTypeMismatch:      "Data type does not match, length does not match",
	AbortDataLong:          "Data type does not match, length too high",
	AbortDataShort:         "Data type does not match, length too short",
	AbortSubUnknown:        "Sub index does not exist",
	AbortInvalidValue:      "Invalid value for parameter (download only)",
	AbortValueHigh:         "Value range of parameter written too high",
	AbortValueLow:          "Value range of parameter written too low",
	AbortMaxLessMin:        "Maximum value is less than minimum value.",
	AbortNoRessource:       "Resource not available: SDO connection",
	AbortGeneral:           "General error",
	AbortDataTransfer:      "Data cannot be transferred or stored to application",
	AbortDataLocalControl:  "Data cannot be transferred because of local control",
	AbortDataDeviceState:   "Data cannot be tran. because of present device state",
	AbortDataOD:            "Object dict. not present or dynamic generation fails",
	AbortNoData:            "No data available",
}

var OdToAbortMap = map[od.ODR]Abort{
	od.ODR_OUT_OF_MEM:     AbortOutOfMem,
	od.ODR_UNSUPP_ACCESS:  AbortUnsupportedAccess,
	od.ODR_WRITEONLY:      AbortWriteOnly,
	od.ODR_READONLY:       AbortReadOnly,
	od.ODR_IDX_NOT_EXIST:  AbortNotExist,
	od.ODR_NO_MAP:         AbortNoMap,
	od.ODR_MAP_LEN:        AbortMapLen,
	od.ODR_PAR_INCOMPAT:   AbortParamIncompat,
	od.ODR_DEV_INCOMPAT:   AbortDeviceIncompat,
	od.ODR_HW:             AbortHardware,
	od.ODR_TYPE_MISMATCH:  AbortTypeMismatch,
	od.ODR_DATA_LONG:      AbortDataLong,
	od.ODR_DATA_SHORT:     AbortDataShort,
	od.ODR_SUB_NOT_EXIST:  AbortSubUnknown,
	od.ODR_INVALID_VALUE:  AbortInvalidValue,
	od.ODR_VALUE_HIGH:     AbortValueHigh,
	od.ODR_VALUE_LOW:      AbortValueLow,
	od.ODR_MAX_LESS_MIN:   AbortMaxLessMin,
	od.ODR_NO_RESOURCE:    AbortNoRessource,
	od.ODR_GENERAL:        AbortGeneral,
	od.ODR_DATA_TRANSF:    AbortDataTransfer,
	od.ODR_DATA_LOC_CTRL:  AbortDataLocalControl,
	od.ODR_DATA_DEV_STATE: AbortDataDeviceState,
	od.ODR_OD_MISSING:     AbortDataOD,
	od.ODR_NO_DATA:        AbortNoData,
}

type Abort uint32

// Get the associated abort code, if the code is not present in map, return ODR_DEV_INCOMPAT
func ConvertOdToSdoAbort(oderr od.ODR) Abort {
	abort_code, ok := OdToAbortMap[oderr]
	if ok {
		return Abort(abort_code)
	} else {
		return OdToAbortMap[od.ODR_DEV_INCOMPAT]
	}
}

func (abort Abort) Error() string {
	return fmt.Sprintf("x%x : %s", uint32(abort), abort.Description())
}

func (abort Abort) Description() string {
	description, ok := AbortCodeDescriptionMap[abort]
	if ok {
		return description
	}
	return AbortCodeDescriptionMap[AbortGeneral]
}

type SDOResponse struct {
	raw [8]byte
}

// Checks whether response command is an expected value in the present
// state
func (response *SDOResponse) isResponseCommandValid(state internalState) bool {

	switch state {
	case stateDownloadInitiateRsp:
		if response.raw[0] == 0x60 {
			return true
		}
		return false
	case stateDownloadSegmentRsp:
		if (response.raw[0] & 0xEF) == 0x20 {
			return true
		}
	case stateDownloadBlkInitiateRsp:
		if (response.raw[0] & 0xFB) == 0xA0 {
			return true
		}
	case stateDownloadBlkSubblockReq, stateDownloadBlkSubblockRsp:
		if response.raw[0] == 0xA2 {
			return true
		}
	case stateDownloadBlkEndRsp:
		if response.raw[0] == 0xA1 {
			return true
		}
	case stateUploadInitiateRsp:
		if (response.raw[0] & 0xF0) == 0x40 {
			return true
		}
	case stateUploadSegmentRsp:
		if (response.raw[0] & 0xE0) == 0x00 {
			return true
		}
	case stateUploadBlkInitiateRsp:
		if (response.raw[0]&0xF9) == 0xC0 || (response.raw[0]&0xF0) == 0x40 {
			return true
		}
	case stateUploadBlkSubblockSreq:
		// TODO but not checked in normal upload function
		return true
	case stateUploadBlkEndSreq:
		if (response.raw[0] & 0xE3) == 0xC1 {
			return true
		}

	}
	log.Errorf("Invalid response received, with code : %x", response.raw[0])
	return false

}

func (response *SDOResponse) IsAbort() bool {
	return response.raw[0] == 0x80
}

func (response *SDOResponse) GetAbortCode() Abort {
	return Abort(binary.LittleEndian.Uint32(response.raw[4:]))
}

func (response *SDOResponse) GetIndex() uint16 {
	return binary.LittleEndian.Uint16(response.raw[1:3])
}

func (response *SDOResponse) GetSubindex() uint8 {
	return response.raw[3]
}

func (response *SDOResponse) GetToggle() uint8 {
	return response.raw[0] & 0x10
}

func (response *SDOResponse) GetBlockSize() uint8 {
	return response.raw[4]
}

func (response *SDOResponse) GetNumberOfSegments() uint8 {
	return response.raw[1]
}

func (response *SDOResponse) IsCRCEnabled() bool {
	return (response.raw[0] & 0x04) != 0
}

func (response *SDOResponse) GetCRCClient() crc.CRC16 {
	return crc.CRC16((binary.LittleEndian.Uint16(response.raw[1:3])))
}
