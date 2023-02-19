package canopen

// This file regroups special functions that are executed when reading or writing to object dictionary

import (
	"encoding/binary"

	log "github.com/sirupsen/logrus"
)

// [RPDO][TPDO] write method that fakes writing an OD variable
func WriteDummy(stream *Stream, data []byte, countWritten *uint16) error {
	if countWritten != nil {
		*countWritten = uint16(len(data))
	}
	return nil
}

// [RPDO][TPDO] read method that fakes reading an OD variable
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

// [RPDO][TPDO] get communication parameter
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

// [RPDO] update communication parameter
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

// [RPDO][TPDO] update mapping parameter
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

// [TPDO] update communication parameter
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

// [SYNC] Update cob id & if should be producer
func WriteEntry1005(stream *Stream, data []byte, countWritten *uint16) error {
	log.Debug("[SYNC] Writing in extension entry 1005")
	// Expect a uint32 and subindex 0 and no nill pointers
	if stream == nil || data == nil || stream.Subindex != 0 || countWritten == nil || len(data) != 4 {
		return ODR_DEV_INCOMPAT
	}
	sync, ok := stream.Object.(*SYNC)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	cobIdSync := binary.LittleEndian.Uint32(data)
	var canId uint16 = uint16(cobIdSync & 0x7FF)
	isProducer := (cobIdSync & 0x40000000) != 0
	if (cobIdSync&0xBFFFF800) != 0 || isIDRestricted(canId) || (sync.IsProducer && isProducer && canId != sync.Ident) {
		return ODR_INVALID_VALUE
	}
	// Reconfigure the receive and transmit buffers only if changed
	if canId != sync.Ident {
		err := sync.CANModule.UpdateRxBuffer(sync.CANRxBuffIndex, uint32(canId), 0x7FF, false, sync)
		if err != nil {
			return ODR_DEV_INCOMPAT
		}
		var frameSize uint8 = 0
		if sync.CounterOverflowValue != 0 {
			frameSize = 1
		}
		sync.CANTxBuff, err = sync.CANModule.UpdateTxBuffer(sync.CANTxBuffIndex, uint32(canId), false, frameSize, false)
		if sync.CANTxBuff == nil || err != nil {
			return ODR_DEV_INCOMPAT
		}
		sync.Ident = canId
	}
	// Reset in case sync is producer
	sync.IsProducer = isProducer
	if isProducer {
		log.Info("SYNC is now a producer")
		sync.Counter = 0
		sync.Timer = 0
	}
	return WriteEntryOriginal(stream, data, countWritten)
}

// [HB Consumer] Update heartbeat consumer
func WriteEntry1016(stream *Stream, data []byte, countWritten *uint16) error {
	consumer, ok := stream.Object.(*HBConsumer)
	if !ok {
		return ODR_DEV_INCOMPAT
	}

	if stream == nil || stream.Subindex < 1 || int(stream.Subindex) > len(consumer.MonitoredNodes) {
		return ODR_DEV_INCOMPAT
	}
	var hbConsValue uint32
	nodeId := uint8(hbConsValue>>16) & 0xFF
	time := hbConsValue & 0xFFFF
	ret := consumer.InitEntry(stream.Subindex-1, nodeId, uint16(time))
	if ret != nil {
		return ODR_PAR_INCOMPAT
	}
	return WriteEntryOriginal(stream, data, countWritten)
}

// [NMT] update heartbeat period
func WriteEntry1017(stream *Stream, data []byte, countWritten *uint16) error {
	if stream.Subindex != 0 || data == nil || len(data) != 2 || countWritten == nil || stream == nil {
		return ODR_DEV_INCOMPAT
	}
	nmt, ok := stream.Object.(*NMT)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	nmt.HearbeatProducerTimeUs = uint32(binary.LittleEndian.Uint16(data)) * 1000
	nmt.HearbeatProducerTimer = 0
	return WriteEntryOriginal(stream, data, countWritten)
}

// [SYNC] update synchronous counter overflow
func WriteEntry1019(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || countWritten == nil || len(data) != 1 {
		return ODR_DEV_INCOMPAT
	}
	sync, ok := stream.Object.(*SYNC)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	syncCounterOverflow := data[0]
	if syncCounterOverflow == 1 || syncCounterOverflow > 240 {
		return ODR_INVALID_VALUE
	}
	OD1006Period := binary.LittleEndian.Uint32(*sync.OD1006Period)
	if OD1006Period != 0 {
		return ODR_DATA_DEV_STATE
	}
	var nbBytes = uint8(0)
	if syncCounterOverflow != 0 {
		nbBytes = 1
	}
	var err error
	sync.CANTxBuff, err = sync.CANModule.UpdateTxBuffer(sync.CANTxBuffIndex, uint32(sync.Ident), false, nbBytes, false)
	if sync.CANTxBuff == nil || err != nil {
		sync.IsProducer = false
		return ODR_DEV_INCOMPAT
	}
	sync.CounterOverflowValue = syncCounterOverflow
	return WriteEntryOriginal(stream, data, countWritten)
}
