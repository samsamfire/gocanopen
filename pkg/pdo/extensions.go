package pdo

import (
	"encoding/binary"

	canopen "github.com/samsamfire/gocanopen"
	can "github.com/samsamfire/gocanopen/pkg/can"
	"github.com/samsamfire/gocanopen/pkg/od"
	log "github.com/sirupsen/logrus"
)

// [RPDO] update communication parameter
func writeEntry14xx(stream *od.Stream, data []byte, countWritten *uint16) error {
	log.Debug("[OD][EXTENSION][RPDO] updating communication parameter")
	if stream == nil || data == nil || countWritten == nil || len(data) > 4 {
		return od.ODR_DEV_INCOMPAT
	}
	rpdo, ok := stream.Object.(*RPDO)
	if !ok {
		return od.ODR_DEV_INCOMPAT
	}
	rpdo.mu.Lock()
	defer rpdo.mu.Unlock()

	pdo := rpdo.pdo
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
		log.Debugf("[OD][EXTENSION][%v] updating pdo cob-id, valid : %v, canId : x%x", pdo.Type(), valid, canId)
		if (cobId&0x3FFFF800) != 0 ||
			valid && pdo.Valid && canId != uint32(pdo.configuredId) ||
			valid && canopen.IsIDRestricted(uint16(canId)) ||
			valid && pdo.nbMapped == 0 {
			return od.ODR_INVALID_VALUE
		}

		// Parameter changed ?
		if valid != pdo.Valid || canId != uint32(pdo.configuredId) {
			// If default id is written store to OD without node id
			if canId == uint32(pdo.predefinedId) {
				binary.LittleEndian.PutUint32(bufCopy, cobId&0xFFFFFF80)
			}
			if !valid {
				canId = 0
			}
			err := rpdo.Subscribe(canId, 0x7FF, false, rpdo)
			if valid && err == nil {
				pdo.Valid = true
				pdo.configuredId = uint16(canId)
			} else {
				pdo.Valid = false
				rpdo.rxNew[0] = false
				rpdo.rxNew[1] = false
				if err != nil {
					return od.ODR_DEV_INCOMPAT
				}
			}
			log.Debugf("[OD][EXTENSION][%v] updated pdo with cobId : x%x, valid : %v", pdo.Type(), pdo.configuredId&0x7FF, pdo.Valid)
		}

	case 2:
		// Transmission type
		transmissionType := data[0]
		if transmissionType > TRANSMISSION_TYPE_SYNC_240 && transmissionType < TRANSMISSION_TYPE_SYNC_EVENT_LO {
			return od.ODR_INVALID_VALUE
		}
		synchronous := transmissionType <= TRANSMISSION_TYPE_SYNC_240
		// Remove old message from second buffer
		if rpdo.synchronous != synchronous {
			rpdo.rxNew[1] = false
		}
		rpdo.synchronous = synchronous
		log.Debugf("[OD][EXTENSION][%v] updated pdo transmission type to : %v", pdo.Type(), transmissionType)

	case 5:
		// Event timer
		eventTime := binary.LittleEndian.Uint16(data)
		rpdo.timeoutTimeUs = uint32(eventTime) * 1000
		rpdo.timeoutTimer = 0
		log.Debugf("[OD][EXTENSION][%v] updated pdo event timer to : %v us", pdo.Type(), eventTime)
	}

	return od.WriteEntryDefault(stream, bufCopy, countWritten)
}

// [RPDO][TPDO] get communication parameter
func readEntry14xxOr18xx(stream *od.Stream, data []byte, countRead *uint16) error {
	err := od.ReadEntryDefault(stream, data, countRead)
	// Add node id when reading subindex 1
	if err == nil && stream.Subindex == 1 && *countRead == 4 {
		// Get the corresponding object, either TPDO or RPDO
		var pdo *PDOCommon
		switch v := stream.Object.(type) {
		case *RPDO:
			v.mu.Lock()
			defer v.mu.Unlock()
			pdo = v.pdo
		case *TPDO:
			v.mu.Lock()
			defer v.mu.Unlock()
			pdo = v.pdo
		default:
			return od.ODR_DEV_INCOMPAT
		}
		cobId := binary.LittleEndian.Uint32(data)
		canId := uint16(cobId & 0x7FF)
		// Add ID if not contained
		if canId != 0 && canId == (pdo.predefinedId&0xFF80) {
			cobId = (cobId & 0xFFFF0000) | uint32(pdo.predefinedId)
		}
		// If PDO not valid, set bit 32
		if !pdo.Valid {
			cobId |= 0x80000000
		}
		binary.LittleEndian.PutUint32(data, cobId)
	}
	return err
}

// [RPDO][TPDO] update mapping parameter
func writeEntry16xxOr1Axx(stream *od.Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || countWritten == nil || stream.Subindex > od.PDO_MAX_MAPPED_ENTRIES {
		return od.ODR_DEV_INCOMPAT
	}
	// Get the corresponding object, either TPDO or RPDO
	var pdo *PDOCommon
	switch v := stream.Object.(type) {
	case *RPDO:
		v.mu.Lock()
		defer v.mu.Unlock()
		pdo = v.pdo
	case *TPDO:
		v.mu.Lock()
		defer v.mu.Unlock()
		pdo = v.pdo
	default:
		return od.ODR_DEV_INCOMPAT
	}
	log.Debugf("[OD][EXTENSION][%v] updating mapping parameter", pdo.Type())
	// PDO must be disabled in order to allow mapping
	if pdo.Valid || pdo.nbMapped != 0 && stream.Subindex > 0 {
		return od.ODR_UNSUPP_ACCESS
	}
	if stream.Subindex == 0 {
		mappedObjectsCount := data[0]
		pdoDataLength := uint32(0)
		// Don't allow number greater than possible mapped objects
		if mappedObjectsCount > od.PDO_MAX_MAPPED_ENTRIES {
			return od.ODR_MAP_LEN
		}
		for i := 0; i < int(mappedObjectsCount); i++ {
			streamer := pdo.streamers[i]
			dataLength := streamer.DataLength()
			mappedLength := streamer.DataOffset()
			if mappedLength > dataLength {
				return od.ODR_NO_MAP
			}
			pdoDataLength += mappedLength
		}
		if pdoDataLength > uint32(MAX_PDO_LENGTH) {
			return od.ODR_MAP_LEN
		}
		if pdoDataLength == 0 && mappedObjectsCount > 0 {
			return od.ODR_INVALID_VALUE
		}
		pdo.dataLength = pdoDataLength
		pdo.nbMapped = mappedObjectsCount
		log.Debugf("[OD][EXTENSION][%v] updated pdo number of mapped objects to : %v", pdo.Type(), mappedObjectsCount)

	} else {
		err := pdo.configureMap(binary.LittleEndian.Uint32(data), uint32(stream.Subindex)-1, pdo.IsRPDO)
		if err != nil {
			return err
		}
	}
	return od.WriteEntryDefault(stream, data, countWritten)
}

// [TPDO] update communication parameter
func writeEntry18xx(stream *od.Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || countWritten == nil || len(data) > 4 {
		return od.ODR_DEV_INCOMPAT
	}
	tpdo, ok := stream.Object.(*TPDO)
	if !ok {
		return od.ODR_DEV_INCOMPAT
	}
	tpdo.mu.Lock()
	defer tpdo.mu.Unlock()

	pdo := tpdo.pdo
	bufCopy := make([]byte, len(data))
	copy(bufCopy, data)
	switch stream.Subindex {
	case 1:
		// COB id used by PDO
		cobId := binary.LittleEndian.Uint32(data)
		canId := cobId & 0x7FF
		valid := (cobId & 0x80000000) == 0
		// - bits 11...29 must be zero
		// - PDO must be disabled on change
		// - CAN_ID == 0 is not allowed
		// - mapping must be configured before enabling the PDO
		log.Debugf("[OD][EXTENSION][%v] updating pdo cob-id, valid : %v, canId : x%x", pdo.Type(), valid, canId)

		if (cobId&0x3FFFF800) != 0 ||
			(valid && pdo.Valid && canId != uint32(pdo.configuredId)) ||
			(valid && canopen.IsIDRestricted(uint16(canId))) ||
			(valid && pdo.nbMapped == 0) {
			return od.ODR_INVALID_VALUE
		}

		// Parameter changed ?
		if valid != pdo.Valid || canId != uint32(pdo.configuredId) {
			// If default id is written store to OD without node id
			if canId == uint32(pdo.predefinedId) {
				binary.LittleEndian.PutUint32(bufCopy, cobId&0xFFFFFF80)
			}
			if !valid {
				canId = 0
			}
			tpdo.txBuffer = can.NewFrame(canId, 0, uint8(pdo.dataLength))
			pdo.Valid = valid
			pdo.configuredId = uint16(canId)
		}

	case 2:
		// Transmission type
		transmissionType := data[0]
		if transmissionType > TRANSMISSION_TYPE_SYNC_240 && transmissionType < TRANSMISSION_TYPE_SYNC_EVENT_LO {
			return od.ODR_INVALID_VALUE
		}
		tpdo.syncCounter = 255
		tpdo.transmissionType = transmissionType
		tpdo.sendRequest = true
		tpdo.inhibitTimer = 0
		tpdo.eventTimer = 0

	case 3:
		// Inhibit time
		if pdo.Valid {
			return od.ODR_INVALID_VALUE
		}
		inhibitTime := binary.LittleEndian.Uint16(data)
		tpdo.inhibitTimeUs = uint32(inhibitTime) * 100
		tpdo.inhibitTimer = 0

	case 5:
		// Event timer
		eventTime := binary.LittleEndian.Uint16(data)
		tpdo.eventTimeUs = uint32(eventTime) * 1000
		tpdo.eventTimer = 0

	case 6:
		syncStartValue := data[0]
		if pdo.Valid || syncStartValue > 240 {
			return od.ODR_INVALID_VALUE
		}
		tpdo.syncStartValue = syncStartValue

	}
	return od.WriteEntryDefault(stream, bufCopy, countWritten)

}

// [RPDO][TPDO] write method that fakes writing an OD variable
func WriteDummy(stream *od.Stream, data []byte, countWritten *uint16) error {
	if countWritten != nil {
		*countWritten = uint16(len(data))
	}
	return nil
}

// [RPDO][TPDO] read method that fakes reading an OD variable
func ReadDummy(stream *od.Stream, data []byte, countRead *uint16) error {
	if countRead == nil || data == nil || stream == nil {
		return od.ODR_DEV_INCOMPAT
	}
	if len(data) > len(stream.Data) {
		*countRead = uint16(len(stream.Data))
	} else {
		*countRead = uint16(len(data))
	}
	return nil
}
