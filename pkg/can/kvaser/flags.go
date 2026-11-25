package kvaser

import canopen "github.com/samsamfire/gocanopen/v2"

// canlib message flags
const (
	canMSG_RTR         = 0x0001
	canMSG_STD         = 0x0002
	canMSG_EXT         = 0x0004
	canMSG_ERROR_FRAME = 0x0020
)

// Convert a [canopen.Frame] ID with SocketCAN like flags to canlib id and flags
func toCanlibId(frameId uint32) (id uint32, flags uint32) {
	if frameId&canopen.CanEffFlag != 0 {
		id, flags = frameId&canEffMask, canMSG_EXT
	} else {
		id, flags = frameId&canopen.CanSffMask, canMSG_STD
	}
	if frameId&canopen.CanRtrFlag != 0 {
		flags |= canMSG_RTR
	}
	return id, flags
}

// Convert canlib id and flags to a [canopen.Frame] ID with SocketCAN like flags
func fromCanlibId(id uint32, flags uint32) uint32 {
	if flags&canMSG_EXT != 0 {
		id |= canopen.CanEffFlag
	}
	if flags&canMSG_RTR != 0 {
		id |= canopen.CanRtrFlag
	}
	if flags&canMSG_ERROR_FRAME != 0 {
		id |= canopen.CanErrFlag
	}
	return id
}

const canEffMask = 0x1FFFFFFF
