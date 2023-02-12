package canopen

import (
	"encoding/binary"
)

// This file regroups special functions that are executed when reading or writing to object dictionary

// Write dummy : write method that doesn't write anything when an object is not present inside OD
func WriteDummy(stream *Stream, data []byte, countWritten *uint16) error {
	if countWritten != nil {
		*countWritten = uint16(len(data))
	}
	return nil
}

// Read dummy : read method that doesn't read anything when an object is not present inside OD
func ReadDummy(stream *Stream, data []byte, countRead *uint16) error {
	if countRead == nil || data == nil || stream == nil {
		return ODR_DEV_INCOMPAT
	}
	if len(data) > len(stream.Data) {
		*countRead = uint16(len(stream.Data))
	} else {
		*countRead = uint16(len(data))
	}
	return nil
}

// Read RPDO or TPDO communication parameter
func ReadEntry14xxOr18xx(stream *Stream, data []byte, countRead *uint16) error {
	err := ReadEntryOriginal(stream, data, countRead)
	// Add node id when reading subindex 1
	if err == nil && stream.Subindex == 1 && *countRead == 4 {
		// Get the corresponding object, either TPDO or RPDO
		var pdo *PDOBase
		switch v := stream.Object.(type) {
		case *RPDO:
			pdo = &v.Base
		case *TPDO:
			pdo = &v.Base
		default:
			return ODR_DEV_INCOMPAT
		}
		cobId := binary.LittleEndian.Uint32(data)
		canId := uint16(cobId & 0x7FF)
		// Add ID if not contained
		if canId != 0 && canId == (pdo.PreDefinedIdent&0xFF80) {
			cobId = (cobId & 0xFFFF0000) | uint32(pdo.PreDefinedIdent)
		}
		// If PDO not valid, set bit 32
		if !pdo.Valid {
			cobId |= 0x80000000
		}
		binary.LittleEndian.PutUint32(data, cobId)
	}
	return err
}

// Write RPDO communication parameter
func WriteEntry14xx(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || countWritten == nil || len(data) > 4 {
		return ODR_DEV_INCOMPAT
	}
	rpdo, ok := stream.Object.(*RPDO)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	pdo := &rpdo.Base
	bufCopy := make([]byte, len(data))
	copy(bufCopy, data)
	switch stream.Subindex {
	case 1:
		// COB id used by PDO
		cobId := binary.LittleEndian.Uint32(data)
		canId := cobId & 0x7FF
		valid := (cobId & 0x80000000) == 0
		/* bits 11...29 must be zero, PDO must be disabled on change,
		 * CAN_ID == 0 is not allowed, mapping must be configured before
		 * enabling the PDO */

		if (cobId&0x3FFFF800) != 0 ||
			valid && pdo.Valid && canId != uint32(pdo.ConfiguredIdent) ||
			valid && isIDRestricted(uint16(canId)) ||
			valid && pdo.MappedObjectsCount == 0 {
			return ODR_INVALID_VALUE
		}

		// Parameter changed ?
		if valid != pdo.Valid || canId != uint32(pdo.ConfiguredIdent) {
			// If default id is written store to OD without node id
			if canId == uint32(pdo.PreDefinedIdent) {
				binary.LittleEndian.PutUint32(bufCopy, cobId&0xFFFFFF80)
			}
			if !valid {
				canId = 0
			}
			err := pdo.Canmodule.UpdateRxBuffer(
				pdo.BufferIdx,
				canId,
				0x7FF,
				false,
				rpdo,
			)
			if valid && err == nil {
				pdo.Valid = true
				pdo.ConfiguredIdent = uint16(canId)
			} else {
				pdo.Valid = false
				rpdo.RxNew[0] = false
				rpdo.RxNew[1] = false
				if err != nil {
					return ODR_DEV_INCOMPAT
				}
			}
		}

	case 2:
		// Transmission type
		transmissionType := data[0]
		if transmissionType > CO_PDO_TRANSM_TYPE_SYNC_240 && transmissionType < CO_PDO_TRANSM_TYPE_SYNC_EVENT_LO {
			return ODR_INVALID_VALUE
		}
		synchronous := transmissionType <= CO_PDO_TRANSM_TYPE_SYNC_240
		// Remove old message from second buffer
		if rpdo.Synchronous != synchronous {
			rpdo.RxNew[1] = false
		}
		rpdo.Synchronous = synchronous
		if transmissionType < CO_PDO_TRANSM_TYPE_SYNC_EVENT_LO {
			return ODR_INVALID_VALUE
		}

	case 5:
		// Envent timer
		eventTime := binary.LittleEndian.Uint16(data)
		rpdo.TimeoutTimeUs = uint32(eventTime) * 1000
		rpdo.TimeoutTimer = 0
	}

	return WriteEntryOriginal(stream, bufCopy, countWritten)
}

// Write RPDO or TPDO mapping parameter
func WriteEntry16xxOr1Axx(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || countWritten == nil || stream.Subindex > MAX_MAPPED_ENTRIES {
		return ODR_DEV_INCOMPAT
	}
	// Get the corresponding object, either TPDO or RPDO
	var pdo *PDOBase
	switch v := stream.Object.(type) {
	case *RPDO:
		pdo = &v.Base
	case *TPDO:
		pdo = &v.Base
	default:
		return ODR_DEV_INCOMPAT
	}
	// PDO must be disabled in order to allow mapping
	if pdo.Valid || pdo.MappedObjectsCount != 0 && stream.Subindex > 0 {
		return ODR_UNSUPP_ACCESS
	}
	if stream.Subindex == 0 {
		mappedObjectsCount := data[0]
		pdoDataLength := uint32(0)
		// Don't allow number greater than possible mapped objects
		if mappedObjectsCount > MAX_MAPPED_ENTRIES {
			return ODR_MAP_LEN
		}
		for i := 0; i < int(mappedObjectsCount); i++ {
			streamer := pdo.Streamers[i]
			dataLength := uint32(len(streamer.Stream.Data))
			mappedLength := streamer.Stream.DataOffset
			if mappedLength > dataLength {
				return ODR_NO_MAP
			}
			pdoDataLength += mappedLength
		}
		if pdoDataLength > uint32(MAX_PDO_LENGTH) {
			return ODR_MAP_LEN
		}
		if pdoDataLength == 0 && mappedObjectsCount > 0 {
			return ODR_INVALID_VALUE
		}
		pdo.DataLength = pdoDataLength
		pdo.MappedObjectsCount = mappedObjectsCount

	} else {
		ret := pdo.ConfigureMap(
			pdo.od,
			binary.LittleEndian.Uint32(data),
			uint32(stream.Subindex)-1,
			pdo.IsRPDO)
		if ret != nil {
			return ret
		}
	}
	return WriteEntryOriginal(stream, data, countWritten)
}

// Write TPDO communication parameter
func WriteEntry18xx(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || countWritten == nil || len(data) > 4 {
		return ODR_DEV_INCOMPAT
	}
	tpdo, ok := stream.Object.(*TPDO)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	pdo := &tpdo.Base
	bufCopy := make([]byte, len(data))
	copy(bufCopy, data)
	switch stream.Subindex {
	case 1:
		// COB id used by PDO
		cobId := binary.LittleEndian.Uint32(data)
		canId := cobId & 0x7FF
		valid := (cobId & 0x80000000) == 0
		/* bits 11...29 must be zero, PDO must be disabled on change,
		 * CAN_ID == 0 is not allowed, mapping must be configured before
		 * enabling the PDO */

		if (cobId&0x3FFFF800) != 0 ||
			valid && pdo.Valid && canId != uint32(pdo.ConfiguredIdent) ||
			valid && isIDRestricted(uint16(canId)) ||
			valid && pdo.MappedObjectsCount == 0 {
			return ODR_INVALID_VALUE
		}

		// Parameter changed ?
		if valid != pdo.Valid || canId != uint32(pdo.ConfiguredIdent) {
			// If default id is written store to OD without node id
			if canId == uint32(pdo.PreDefinedIdent) {
				binary.LittleEndian.PutUint32(bufCopy, cobId&0xFFFFFF80)
			}
			if !valid {
				canId = 0
			}
			txBuffer, err := pdo.Canmodule.UpdateTxBuffer(
				pdo.BufferIdx,
				canId,
				false,
				uint8(pdo.DataLength),
				tpdo.TransmissionType <= CO_PDO_TRANSM_TYPE_SYNC_240)
			if txBuffer == nil || err != nil {
				return ODR_DEV_INCOMPAT
			}
			tpdo.TxBuffer = txBuffer
			pdo.Valid = valid
			pdo.ConfiguredIdent = uint16(canId)
		}

	case 2:
		// Transmission type
		transmissionType := data[0]
		if transmissionType > CO_PDO_TRANSM_TYPE_SYNC_240 && transmissionType < CO_PDO_TRANSM_TYPE_SYNC_EVENT_LO {
			return ODR_INVALID_VALUE
		}
		tpdo.TxBuffer.SyncFlag = transmissionType <= CO_PDO_TRANSM_TYPE_SYNC_240
		tpdo.SyncCounter = 255
		tpdo.TransmissionType = transmissionType
		tpdo.SendRequest = true
		tpdo.InhibitTimer = 0
		tpdo.EventTimer = 0

	case 3:
		//Inhibit time
		if pdo.Valid {
			return ODR_INVALID_VALUE
		}
		inhibitTime := binary.LittleEndian.Uint16(data)
		tpdo.InhibitTimeUs = uint32(inhibitTime) * 100
		tpdo.InhibitTimer = 0

	case 5:
		// Envent timer
		eventTime := binary.LittleEndian.Uint16(data)
		tpdo.EventTimeUs = uint32(eventTime) * 1000
		tpdo.EventTimer = 0

	case 6:
		syncStartValue := data[0]
		if pdo.Valid || syncStartValue > 240 {
			return ODR_INVALID_VALUE
		}
		tpdo.SyncStartValue = syncStartValue

	}
	return WriteEntryOriginal(stream, bufCopy, countWritten)

}
