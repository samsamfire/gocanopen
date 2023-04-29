package canopen

import (
	"encoding/binary"

	log "github.com/sirupsen/logrus"
)

// Common defines to both SDO server and SDO client
type SDOAbortCode uint32

const (
	CO_SDO_AB_NONE               SDOAbortCode = 0x00000000
	CO_SDO_AB_TOGGLE_BIT         SDOAbortCode = 0x05030000
	CO_SDO_AB_TIMEOUT            SDOAbortCode = 0x05040000
	CO_SDO_AB_CMD                SDOAbortCode = 0x05040001
	CO_SDO_AB_BLOCK_SIZE         SDOAbortCode = 0x05040002
	CO_SDO_AB_SEQ_NUM            SDOAbortCode = 0x05040003
	CO_SDO_AB_CRC                SDOAbortCode = 0x05040004
	CO_SDO_AB_OUT_OF_MEM         SDOAbortCode = 0x05040005
	CO_SDO_AB_UNSUPPORTED_ACCESS SDOAbortCode = 0x06010000
	CO_SDO_AB_WRITEONLY          SDOAbortCode = 0x06010001
	CO_SDO_AB_READONLY           SDOAbortCode = 0x06010002
	CO_SDO_AB_NOT_EXIST          SDOAbortCode = 0x06020000
	CO_SDO_AB_NO_MAP             SDOAbortCode = 0x06040041
	CO_SDO_AB_MAP_LEN            SDOAbortCode = 0x06040042
	CO_SDO_AB_PRAM_INCOMPAT      SDOAbortCode = 0x06040043
	CO_SDO_AB_DEVICE_INCOMPAT    SDOAbortCode = 0x06040047
	CO_SDO_AB_HW                 SDOAbortCode = 0x06060000
	CO_SDO_AB_TYPE_MISMATCH      SDOAbortCode = 0x06070010
	CO_SDO_AB_DATA_LONG          SDOAbortCode = 0x06070012
	CO_SDO_AB_DATA_SHORT         SDOAbortCode = 0x06070013
	CO_SDO_AB_SUB_UNKNOWN        SDOAbortCode = 0x06090011
	CO_SDO_AB_INVALID_VALUE      SDOAbortCode = 0x06090030
	CO_SDO_AB_VALUE_HIGH         SDOAbortCode = 0x06090031
	CO_SDO_AB_VALUE_LOW          SDOAbortCode = 0x06090032
	CO_SDO_AB_MAX_LESS_MIN       SDOAbortCode = 0x06090036
	CO_SDO_AB_NO_RESOURCE        SDOAbortCode = 0x060A0023
	CO_SDO_AB_GENERAL            SDOAbortCode = 0x08000000
	CO_SDO_AB_DATA_TRANSF        SDOAbortCode = 0x08000020
	CO_SDO_AB_DATA_LOC_CTRL      SDOAbortCode = 0x08000021
	CO_SDO_AB_DATA_DEV_STATE     SDOAbortCode = 0x08000022
	CO_SDO_AB_DATA_OD            SDOAbortCode = 0x08000023
	CO_SDO_AB_NO_DATA            SDOAbortCode = 0x08000024
)

var SDO_ABORT_EXPLANATION_MAP = map[SDOAbortCode]string{
	CO_SDO_AB_NONE:               "No abort",
	CO_SDO_AB_TOGGLE_BIT:         "Toggle bit not altered",
	CO_SDO_AB_TIMEOUT:            "SDO protocol timed out",
	CO_SDO_AB_CMD:                "Command specifier not valid or unknown",
	CO_SDO_AB_BLOCK_SIZE:         "Invalid block size in block mode",
	CO_SDO_AB_SEQ_NUM:            "Invalid sequence number in block mode",
	CO_SDO_AB_CRC:                "CRC error (block mode only)",
	CO_SDO_AB_OUT_OF_MEM:         "Out of memory",
	CO_SDO_AB_UNSUPPORTED_ACCESS: "Unsupported access to an object",
	CO_SDO_AB_WRITEONLY:          "Attempt to read a write only object",
	CO_SDO_AB_READONLY:           "Attempt to write a read only object",
	CO_SDO_AB_NOT_EXIST:          "Object does not exist in the object dictionary",
	CO_SDO_AB_NO_MAP:             "Object cannot be mapped to the PDO",
	CO_SDO_AB_MAP_LEN:            "Num and len of object to be mapped exceeds PDO len",
	CO_SDO_AB_PRAM_INCOMPAT:      "General parameter incompatibility reasons",
	CO_SDO_AB_DEVICE_INCOMPAT:    "General internal incompatibility in device",
	CO_SDO_AB_HW:                 "Access failed due to hardware error",
	CO_SDO_AB_TYPE_MISMATCH:      "Data type does not match, length does not match",
	CO_SDO_AB_DATA_LONG:          "Data type does not match, length too high",
	CO_SDO_AB_DATA_SHORT:         "Data type does not match, length too short",
	CO_SDO_AB_SUB_UNKNOWN:        "Sub index does not exist",
	CO_SDO_AB_INVALID_VALUE:      "Invalid value for parameter (download only)",
	CO_SDO_AB_VALUE_HIGH:         "Value range of parameter written too high",
	CO_SDO_AB_VALUE_LOW:          "Value range of parameter written too low",
	CO_SDO_AB_MAX_LESS_MIN:       "Maximum value is less than minimum value.",
	CO_SDO_AB_NO_RESOURCE:        "Resource not available: SDO connection",
	CO_SDO_AB_GENERAL:            "General error",
	CO_SDO_AB_DATA_TRANSF:        "Data cannot be transferred or stored to application",
	CO_SDO_AB_DATA_LOC_CTRL:      "Data cannot be transferred because of local control",
	CO_SDO_AB_DATA_DEV_STATE:     "Data cannot be tran. because of present device state",
	CO_SDO_AB_DATA_OD:            "Object dict. not present or dynamic generation fails",
	CO_SDO_AB_NO_DATA:            "No data available",
}

var SDO_ABORT_MAP = map[ODR]SDOAbortCode{
	0:  CO_SDO_AB_NONE,               /* No abort */
	1:  CO_SDO_AB_OUT_OF_MEM,         /* Out of memory */
	2:  CO_SDO_AB_UNSUPPORTED_ACCESS, /* Unsupported access to an object */
	3:  CO_SDO_AB_WRITEONLY,          /* Attempt to read a write only object */
	4:  CO_SDO_AB_READONLY,           /* Attempt to write a read only object */
	5:  CO_SDO_AB_NOT_EXIST,          /* Object does not exist in the object dictionary */
	6:  CO_SDO_AB_NO_MAP,             /* Object cannot be mapped to the PDO */
	7:  CO_SDO_AB_MAP_LEN,            /* Num and len of object to be mapped exceeds PDO len */
	8:  CO_SDO_AB_PRAM_INCOMPAT,      /* General parameter incompatibility reasons */
	9:  CO_SDO_AB_DEVICE_INCOMPAT,    /* General internal incompatibility in device */
	10: CO_SDO_AB_HW,                 /* Access failed due to hardware error */
	11: CO_SDO_AB_TYPE_MISMATCH,      /* Data type does not match, length does not match */
	12: CO_SDO_AB_DATA_LONG,          /* Data type does not match, length too high */
	13: CO_SDO_AB_DATA_SHORT,         /* Data type does not match, length too short */
	14: CO_SDO_AB_SUB_UNKNOWN,        /* Sub index does not exist */
	15: CO_SDO_AB_INVALID_VALUE,      /* Invalid value for parameter (download only). */
	16: CO_SDO_AB_VALUE_HIGH,         /* Value range of parameter written too high */
	17: CO_SDO_AB_VALUE_LOW,          /* Value range of parameter written too low */
	18: CO_SDO_AB_MAX_LESS_MIN,       /* Maximum value is less than minimum value. */
	19: CO_SDO_AB_NO_RESOURCE,        /* Resource not available: SDO connection */
	20: CO_SDO_AB_GENERAL,            /* General error */
	21: CO_SDO_AB_DATA_TRANSF,        /* Data cannot be transferred or stored to application */
	22: CO_SDO_AB_DATA_LOC_CTRL,      /* Data cannot be transferred because of local control */
	23: CO_SDO_AB_DATA_DEV_STATE,     /* Data cannot be tran. because of present device state */
	24: CO_SDO_AB_DATA_OD,            /* Object dict. not present or dynamic generation fails */
	25: CO_SDO_AB_NO_DATA,            /* No data available */
}

func (abort SDOAbortCode) Error() string {

	err_string, ok := SDO_ABORT_EXPLANATION_MAP[abort]
	if ok {
		return err_string
	} else {
		log.Errorf("Abort is %x", uint32(abort))
		return SDO_ABORT_EXPLANATION_MAP[CO_SDO_AB_GENERAL]
	}
}

type SDOResponse struct {
	raw [8]byte
}

func (response *SDOResponse) isResponseValid(state uint8) bool {
	switch state {

	// Download
	case CO_SDO_ST_DOWNLOAD_INITIATE_RSP:
		if response.raw[0] == 0x60 {
			return true
		}
		return false
	case CO_SDO_ST_DOWNLOAD_SEGMENT_RSP:
		if (response.raw[0] & 0xEF) == 0x20 {
			return true
		}
	case CO_SDO_ST_DOWNLOAD_BLK_INITIATE_RSP:
		if (response.raw[0] & 0xFB) == 0xA0 {
			return true
		}

	case CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_REQ, CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_RSP:
		if response.raw[0] == 0xA2 {
			return true
		}
	case CO_SDO_ST_DOWNLOAD_BLK_END_RSP:
		if response.raw[0] == 0xA1 {
			return true
		}
	case CO_SDO_ST_UPLOAD_INITIATE_RSP:
		if (response.raw[0] & 0xF0) == 0x40 {
			return true
		}
	case CO_SDO_ST_UPLOAD_SEGMENT_RSP:
		if (response.raw[0] & 0xE0) == 0x00 {
			return true
		}
	case CO_SDO_ST_UPLOAD_BLK_INITIATE_RSP:
		if (response.raw[0]&0xF9) == 0xC0 || (response.raw[0]&0xF0) == 0x40 {
			return true
		}
	case CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_SREQ:
		//TODO but not checked in normal upload function
		return true

	case CO_SDO_ST_UPLOAD_BLK_END_SREQ:
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

func (response *SDOResponse) GetCRCClient() uint16 {
	return binary.LittleEndian.Uint16(response.raw[1:3])
}
