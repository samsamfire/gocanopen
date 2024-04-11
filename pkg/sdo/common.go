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

// Common defines to both SDO server and SDO client
type SDOAbortCode uint32
type SDOState uint8

const (
	SDO_CLIENT_TIMEOUT = 1000
	SDO_SERVER_TIMEOUT = 1000
	CLIENT_ID          = 0x600
	SERVER_ID          = 0x580
)

const (
	SDO_STATE_IDLE                      SDOState = 0x00
	SDO_STATE_ABORT                     SDOState = 0x01
	SDO_STATE_DOWNLOAD_LOCAL_TRANSFER   SDOState = 0x10
	SDO_STATE_DOWNLOAD_INITIATE_REQ     SDOState = 0x11
	SDO_STATE_DOWNLOAD_INITIATE_RSP     SDOState = 0x12
	SDO_STATE_DOWNLOAD_SEGMENT_REQ      SDOState = 0x13
	SDO_STATE_DOWNLOAD_SEGMENT_RSP      SDOState = 0x14
	SDO_STATE_UPLOAD_LOCAL_TRANSFER     SDOState = 0x20
	SDO_STATE_UPLOAD_INITIATE_REQ       SDOState = 0x21
	SDO_STATE_UPLOAD_INITIATE_RSP       SDOState = 0x22
	SDO_STATE_UPLOAD_SEGMENT_REQ        SDOState = 0x23
	SDO_STATE_UPLOAD_SEGMENT_RSP        SDOState = 0x24
	SDO_STATE_DOWNLOAD_BLK_INITIATE_REQ SDOState = 0x51
	SDO_STATE_DOWNLOAD_BLK_INITIATE_RSP SDOState = 0x52
	SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ SDOState = 0x53
	SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_RSP SDOState = 0x54
	SDO_STATE_DOWNLOAD_BLK_END_REQ      SDOState = 0x55
	SDO_STATE_DOWNLOAD_BLK_END_RSP      SDOState = 0x56
	SDO_STATE_UPLOAD_BLK_INITIATE_REQ   SDOState = 0x61
	SDO_STATE_UPLOAD_BLK_INITIATE_RSP   SDOState = 0x62
	SDO_STATE_UPLOAD_BLK_INITIATE_REQ2  SDOState = 0x63
	SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ  SDOState = 0x64
	SDO_STATE_UPLOAD_BLK_SUBBLOCK_CRSP  SDOState = 0x65
	SDO_STATE_UPLOAD_BLK_END_SREQ       SDOState = 0x66
	SDO_STATE_UPLOAD_BLK_END_CRSP       SDOState = 0x67
)

const (
	SDO_ABORT_TOGGLE_BIT         SDOAbortCode = 0x05030000
	SDO_ABORT_TIMEOUT            SDOAbortCode = 0x05040000
	SDO_ABORT_CMD                SDOAbortCode = 0x05040001
	SDO_ABORT_BLOCK_SIZE         SDOAbortCode = 0x05040002
	SDO_ABORT_SEQ_NUM            SDOAbortCode = 0x05040003
	SDO_ABORT_CRC                SDOAbortCode = 0x05040004
	SDO_ABORT_OUT_OF_MEM         SDOAbortCode = 0x05040005
	SDO_ABORT_UNSUPPORTED_ACCESS SDOAbortCode = 0x06010000
	SDO_ABORT_WRITEONLY          SDOAbortCode = 0x06010001
	SDO_ABORT_READONLY           SDOAbortCode = 0x06010002
	SDO_ABORT_NOT_EXIST          SDOAbortCode = 0x06020000
	SDO_ABORT_NO_MAP             SDOAbortCode = 0x06040041
	SDO_ABORT_MAP_LEN            SDOAbortCode = 0x06040042
	SDO_ABORT_PRAM_INCOMPAT      SDOAbortCode = 0x06040043
	SDO_ABORT_DEVICE_INCOMPAT    SDOAbortCode = 0x06040047
	SDO_ABORT_HW                 SDOAbortCode = 0x06060000
	SDO_ABORT_TYPE_MISMATCH      SDOAbortCode = 0x06070010
	SDO_ABORT_DATA_LONG          SDOAbortCode = 0x06070012
	SDO_ABORT_DATA_SHORT         SDOAbortCode = 0x06070013
	SDO_ABORT_SUB_UNKNOWN        SDOAbortCode = 0x06090011
	SDO_ABORT_INVALID_VALUE      SDOAbortCode = 0x06090030
	SDO_ABORT_VALUE_HIGH         SDOAbortCode = 0x06090031
	SDO_ABORT_VALUE_LOW          SDOAbortCode = 0x06090032
	SDO_ABORT_MAX_LESS_MIN       SDOAbortCode = 0x06090036
	SDO_ABORT_NO_RESOURCE        SDOAbortCode = 0x060A0023
	SDO_ABORT_GENERAL            SDOAbortCode = 0x08000000
	SDO_ABORT_DATA_TRANSF        SDOAbortCode = 0x08000020
	SDO_ABORT_DATA_LOC_CTRL      SDOAbortCode = 0x08000021
	SDO_ABORT_DATA_DEV_STATE     SDOAbortCode = 0x08000022
	SDO_ABORT_DATA_OD            SDOAbortCode = 0x08000023
	SDO_ABORT_NO_DATA            SDOAbortCode = 0x08000024
)

var SDO_ABORT_EXPLANATION_MAP = map[SDOAbortCode]string{
	SDO_ABORT_TOGGLE_BIT:         "Toggle bit not altered",
	SDO_ABORT_TIMEOUT:            "SDO protocol timed out",
	SDO_ABORT_CMD:                "Command specifier not valid or unknown",
	SDO_ABORT_BLOCK_SIZE:         "Invalid block size in block mode",
	SDO_ABORT_SEQ_NUM:            "Invalid sequence number in block mode",
	SDO_ABORT_CRC:                "CRC error (block mode only)",
	SDO_ABORT_OUT_OF_MEM:         "Out of memory",
	SDO_ABORT_UNSUPPORTED_ACCESS: "Unsupported access to an object",
	SDO_ABORT_WRITEONLY:          "Attempt to read a write only object",
	SDO_ABORT_READONLY:           "Attempt to write a read only object",
	SDO_ABORT_NOT_EXIST:          "Object does not exist in the object dictionary",
	SDO_ABORT_NO_MAP:             "Object cannot be mapped to the PDO",
	SDO_ABORT_MAP_LEN:            "Num and len of object to be mapped exceeds PDO len",
	SDO_ABORT_PRAM_INCOMPAT:      "General parameter incompatibility reasons",
	SDO_ABORT_DEVICE_INCOMPAT:    "General internal incompatibility in device",
	SDO_ABORT_HW:                 "Access failed due to hardware error",
	SDO_ABORT_TYPE_MISMATCH:      "Data type does not match, length does not match",
	SDO_ABORT_DATA_LONG:          "Data type does not match, length too high",
	SDO_ABORT_DATA_SHORT:         "Data type does not match, length too short",
	SDO_ABORT_SUB_UNKNOWN:        "Sub index does not exist",
	SDO_ABORT_INVALID_VALUE:      "Invalid value for parameter (download only)",
	SDO_ABORT_VALUE_HIGH:         "Value range of parameter written too high",
	SDO_ABORT_VALUE_LOW:          "Value range of parameter written too low",
	SDO_ABORT_MAX_LESS_MIN:       "Maximum value is less than minimum value.",
	SDO_ABORT_NO_RESOURCE:        "Resource not available: SDO connection",
	SDO_ABORT_GENERAL:            "General error",
	SDO_ABORT_DATA_TRANSF:        "Data cannot be transferred or stored to application",
	SDO_ABORT_DATA_LOC_CTRL:      "Data cannot be transferred because of local control",
	SDO_ABORT_DATA_DEV_STATE:     "Data cannot be tran. because of present device state",
	SDO_ABORT_DATA_OD:            "Object dict. not present or dynamic generation fails",
	SDO_ABORT_NO_DATA:            "No data available",
}

var OD_TO_SDO_ABORT_MAP = map[od.ODR]SDOAbortCode{
	od.ODR_OUT_OF_MEM:     SDO_ABORT_OUT_OF_MEM,
	od.ODR_UNSUPP_ACCESS:  SDO_ABORT_UNSUPPORTED_ACCESS,
	od.ODR_WRITEONLY:      SDO_ABORT_WRITEONLY,
	od.ODR_READONLY:       SDO_ABORT_READONLY,
	od.ODR_IDX_NOT_EXIST:  SDO_ABORT_NOT_EXIST,
	od.ODR_NO_MAP:         SDO_ABORT_NO_MAP,
	od.ODR_MAP_LEN:        SDO_ABORT_MAP_LEN,
	od.ODR_PAR_INCOMPAT:   SDO_ABORT_PRAM_INCOMPAT,
	od.ODR_DEV_INCOMPAT:   SDO_ABORT_DEVICE_INCOMPAT,
	od.ODR_HW:             SDO_ABORT_HW,
	od.ODR_TYPE_MISMATCH:  SDO_ABORT_TYPE_MISMATCH,
	od.ODR_DATA_LONG:      SDO_ABORT_DATA_LONG,
	od.ODR_DATA_SHORT:     SDO_ABORT_DATA_SHORT,
	od.ODR_SUB_NOT_EXIST:  SDO_ABORT_SUB_UNKNOWN,
	od.ODR_INVALID_VALUE:  SDO_ABORT_INVALID_VALUE,
	od.ODR_VALUE_HIGH:     SDO_ABORT_VALUE_HIGH,
	od.ODR_VALUE_LOW:      SDO_ABORT_VALUE_LOW,
	od.ODR_MAX_LESS_MIN:   SDO_ABORT_MAX_LESS_MIN,
	od.ODR_NO_RESOURCE:    SDO_ABORT_NO_RESOURCE,
	od.ODR_GENERAL:        SDO_ABORT_GENERAL,
	od.ODR_DATA_TRANSF:    SDO_ABORT_DATA_TRANSF,
	od.ODR_DATA_LOC_CTRL:  SDO_ABORT_DATA_LOC_CTRL,
	od.ODR_DATA_DEV_STATE: SDO_ABORT_DATA_DEV_STATE,
	od.ODR_OD_MISSING:     SDO_ABORT_DATA_OD,
	od.ODR_NO_DATA:        SDO_ABORT_NO_DATA,
}

// Get the associated abort code, if the code is not present in map, return ODR_DEV_INCOMPAT
func ConvertOdToSdoAbort(oderr od.ODR) SDOAbortCode {
	abort_code, ok := OD_TO_SDO_ABORT_MAP[oderr]
	if ok {
		return SDOAbortCode(abort_code)
	} else {
		return OD_TO_SDO_ABORT_MAP[od.ODR_DEV_INCOMPAT]
	}
}

func (abort SDOAbortCode) Error() string {
	return fmt.Sprintf("x%x : %s", uint32(abort), abort.Description())
}

func (abort SDOAbortCode) Description() string {
	description, ok := SDO_ABORT_EXPLANATION_MAP[abort]
	if ok {
		return description
	}
	return SDO_ABORT_EXPLANATION_MAP[SDO_ABORT_GENERAL]
}

type SDOResponse struct {
	raw [8]byte
}

func (response *SDOResponse) isResponseValid(state SDOState) bool {
	switch state {

	// Download
	case SDO_STATE_DOWNLOAD_INITIATE_RSP:
		if response.raw[0] == 0x60 {
			return true
		}
		return false
	case SDO_STATE_DOWNLOAD_SEGMENT_RSP:
		if (response.raw[0] & 0xEF) == 0x20 {
			return true
		}
	case SDO_STATE_DOWNLOAD_BLK_INITIATE_RSP:
		if (response.raw[0] & 0xFB) == 0xA0 {
			return true
		}

	case SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ, SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_RSP:
		if response.raw[0] == 0xA2 {
			return true
		}
	case SDO_STATE_DOWNLOAD_BLK_END_RSP:
		if response.raw[0] == 0xA1 {
			return true
		}
	case SDO_STATE_UPLOAD_INITIATE_RSP:
		if (response.raw[0] & 0xF0) == 0x40 {
			return true
		}
	case SDO_STATE_UPLOAD_SEGMENT_RSP:
		if (response.raw[0] & 0xE0) == 0x00 {
			return true
		}
	case SDO_STATE_UPLOAD_BLK_INITIATE_RSP:
		if (response.raw[0]&0xF9) == 0xC0 || (response.raw[0]&0xF0) == 0x40 {
			return true
		}
	case SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ:
		// TODO but not checked in normal upload function
		return true

	case SDO_STATE_UPLOAD_BLK_END_SREQ:
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

func (response *SDOResponse) GetAbortCode() SDOAbortCode {
	return SDOAbortCode(binary.LittleEndian.Uint32(response.raw[4:]))
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
