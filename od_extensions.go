package canopen

// This file regroups OD extensions that are executed when reading or writing to object dictionary

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

// [EMERGENCY] read emergency history
func ReadEntry1003(stream *Stream, data []byte, countRead *uint16) error {
	if stream == nil || data == nil || countRead == nil ||
		(len(data) < 4 && stream.Subindex > 0) ||
		len(data) < 1 {
		return ODR_DEV_INCOMPAT
	}
	em, ok := stream.Object.(*EM)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	if len(em.Fifo) < 2 {
		return ODR_DEV_INCOMPAT
	}
	if stream.Subindex == 0 {
		data[0] = em.FifoCount
		*countRead = 1
		return nil
	}
	if stream.Subindex > em.FifoCount {
		return ODR_NO_DATA
	}
	// Most recent error is in subindex 1 and stored behind fifoWrPtr
	index := int(em.FifoWrPtr) - int(stream.Subindex)
	if index >= len(em.Fifo) {
		return ODR_DEV_INCOMPAT
	}
	if index < 0 {
		index += len(em.Fifo)
	}
	binary.LittleEndian.PutUint32(data, em.Fifo[index].msg)
	*countRead = 4
	return nil
}

// [EMERGENCY] clear emergency history
func WriteEntry1003(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || stream.Subindex != 0 || data == nil || len(data) != 1 || countWritten == nil {
		return ODR_DEV_INCOMPAT
	}
	if data[0] != 0 {
		return ODR_INVALID_VALUE
	}
	em, ok := stream.Object.(*EM)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	// Clear error history
	em.FifoCount = 0
	*countWritten = 1
	return nil
}

// [SYNC] update cob id & if should be producer
func WriteEntry1005(stream *Stream, data []byte, countWritten *uint16) error {
	// Expect a uint32 and subindex 0 and no nill pointers
	if stream == nil || data == nil || stream.Subindex != 0 || countWritten == nil || len(data) != 4 {
		return ODR_DEV_INCOMPAT
	}
	sync, ok := stream.Object.(*SYNC)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	cobIdSync := binary.LittleEndian.Uint32(data)
	canId := uint16(cobIdSync & 0x7FF)
	isProducer := (cobIdSync & 0x40000000) != 0
	if (cobIdSync&0xBFFFF800) != 0 || isIDRestricted(canId) || (sync.IsProducer && isProducer && canId != sync.Ident) {
		return ODR_INVALID_VALUE
	}
	// Reconfigure the receive and transmit buffers only if changed
	if canId != sync.Ident {
		err := sync.BusManager.InsertRxBuffer(uint32(canId), 0x7FF, false, sync)
		if err != nil {
			return ODR_DEV_INCOMPAT
		}
		var frameSize uint8 = 0
		if sync.CounterOverflowValue != 0 {
			frameSize = 1
		}
		sync.txBuffer, err = sync.BusManager.InsertTxBuffer(uint32(canId), false, frameSize, false)
		if sync.txBuffer == nil || err != nil {
			return ODR_DEV_INCOMPAT
		}
		sync.Ident = canId
	}
	// Reset in case sync is producer
	sync.IsProducer = isProducer
	if isProducer {
		sync.Counter = 0
		sync.Timer = 0
	}
	log.Debugf("[SYNC] cob-id request : %x | producer request : %v", cobIdSync, isProducer)
	return WriteEntryOriginal(stream, data, countWritten)
}

// [TIME] update cob id & if should be producer
func WriteEntry1012(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || stream.Subindex != 0 || countWritten == nil || len(data) != 4 {
		return ODR_DEV_INCOMPAT
	}
	time, ok := stream.Object.(*TIME)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	cobIdTimestamp := binary.LittleEndian.Uint32(data)
	var canId = uint16(cobIdTimestamp & 0x7FF)
	if (cobIdTimestamp&0x3FFFF800) != 0 || isIDRestricted(canId) {
		return ODR_INVALID_VALUE
	}
	time.IsConsumer = (cobIdTimestamp & 0x80000000) != 0
	time.IsProducer = (cobIdTimestamp & 0x40000000) != 0

	return WriteEntryOriginal(stream, data, countWritten)
}

// [EMERGENCY] read emergency cob id
func ReadEntry1014(stream *Stream, data []byte, countRead *uint16) error {
	if stream == nil || data == nil || countRead == nil || len(data) < 4 || stream.Subindex != 0 {
		return ODR_DEV_INCOMPAT
	}
	em, ok := stream.Object.(*EM)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	var canId uint16
	if em.ProducerIdent == EMERGENCY_SERVICE_ID {
		canId = EMERGENCY_SERVICE_ID + uint16(em.NodeId)
	} else {
		canId = em.ProducerIdent
	}
	var cobId uint32
	if em.ProducerEnabled {
		cobId = 0
	} else {
		cobId = 0x80000000
	}
	cobId |= uint32(canId)
	binary.LittleEndian.PutUint32(data, cobId)
	*countRead = 4
	return nil
}

// [EMERGENCY] update emergency producer cob id
func WriteEntry1014(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || countWritten == nil || len(data) != 4 || stream.Subindex != 0 {
		return ODR_DEV_INCOMPAT
	}
	em, ok := stream.Object.(*EM)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	// Check written value, cob id musn't change when enabled
	cobId := binary.LittleEndian.Uint32(data)
	newCanId := cobId & 0x7FF
	var currentCanId uint16
	if em.ProducerIdent == EMERGENCY_SERVICE_ID {
		currentCanId = EMERGENCY_SERVICE_ID + uint16(em.NodeId)
	} else {
		currentCanId = em.ProducerIdent
	}
	newEnabled := (cobId&uint32(currentCanId)) == 0 && newCanId != 0
	if cobId&0x7FFFF800 != 0 || isIDRestricted(uint16(newCanId)) ||
		(em.ProducerEnabled && newEnabled && newCanId != uint32(currentCanId)) {
		return ODR_INVALID_VALUE
	}
	em.ProducerEnabled = newEnabled
	if newCanId == uint32(EMERGENCY_SERVICE_ID+uint16(em.NodeId)) {
		em.ProducerIdent = EMERGENCY_SERVICE_ID
	} else {
		em.ProducerIdent = uint16(newCanId)
	}

	if newEnabled {
		var err error
		em.txBuffer, err = em.busManager.InsertTxBuffer(
			newCanId,
			false,
			8,
			false,
		)
		if em.txBuffer == nil || err != nil {
			return ODR_DEV_INCOMPAT
		}
	}
	return WriteEntryOriginal(stream, data, countWritten)

}

// [EMERGENCY] update inhibite time
func WriteEntry1015(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || stream.Subindex != 0 || data == nil || len(data) != 2 || countWritten == nil {
		return ODR_DEV_INCOMPAT
	}
	em, ok := stream.Object.(*EM)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	em.InhibitEmTimeUs = uint32(binary.LittleEndian.Uint16(data)) * 100
	em.InhibitEmTimer = 0

	return WriteEntryOriginal(stream, data, countWritten)

}

// [HB Consumer] update heartbeat consumer
func WriteEntry1016(stream *Stream, data []byte, countWritten *uint16) error {
	consumer, ok := stream.Object.(*HBConsumer)
	if !ok {
		return ODR_DEV_INCOMPAT
	}

	if stream == nil || stream.Subindex < 1 ||
		int(stream.Subindex) > len(consumer.monitoredNodes) ||
		len(data) != 4 {
		return ODR_DEV_INCOMPAT
	}

	hbConsValue := binary.LittleEndian.Uint32(data)
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
	nmt.hearbeatProducerTimeUs = uint32(binary.LittleEndian.Uint16(data)) * 1000
	nmt.hearbeatProducerTimer = 0
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
	sync.txBuffer, err = sync.BusManager.InsertTxBuffer(uint32(sync.Ident), false, nbBytes, false)
	if sync.txBuffer == nil || err != nil {
		sync.IsProducer = false
		return ODR_DEV_INCOMPAT
	}
	sync.CounterOverflowValue = syncCounterOverflow
	return WriteEntryOriginal(stream, data, countWritten)
}

// [SDO server] update server parameters
func WriteEntry1201(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || countWritten == nil {
		return ODR_DEV_INCOMPAT
	}
	server, ok := stream.Object.(*SDOServer)
	if !ok {
		return ODR_DEV_INCOMPAT
	}

	switch stream.Subindex {

	case 0:
		return ODR_READONLY
	// cob id client to server
	case 1:
		cobId := binary.LittleEndian.Uint32(data)
		canId := uint16(cobId & 0x7FF)
		canIdCurrent := uint16(server.CobIdClientToServer & 0x7FF)
		valid := (cobId & 0x80000000) == 0
		if (cobId&0x3FFFF800) != 0 ||
			(valid && server.Valid && canId != canIdCurrent) ||
			(valid && isIDRestricted(canId)) {
			return ODR_INVALID_VALUE
		}
		server.InitRxTx(
			server.BusManager,
			cobId,
			server.CobIdServerToClient,
		)
	// cob id server to client
	case 2:
		cobId := binary.LittleEndian.Uint32(data)
		canId := uint16(cobId & 0x7FF)
		canIdCurrent := uint16(server.CobIdServerToClient & 0x7FF)
		valid := (cobId & 0x80000000) == 0
		if (cobId&0x3FFFF800) != 0 ||
			(valid && server.Valid && canId != canIdCurrent) ||
			(valid && isIDRestricted(canId)) {
			return ODR_INVALID_VALUE
		}
		server.InitRxTx(
			server.BusManager,
			server.CobIdClientToServer,
			cobId,
		)
		// node id of server
	case 3:
		if len(data) != 1 {
			return ODR_TYPE_MISMATCH
		}
		nodeId := data[0]
		if nodeId < 1 || nodeId > 127 {
			return ODR_INVALID_VALUE
		}
		server.NodeId = nodeId // ??

	default:
		return ODR_SUB_NOT_EXIST

	}
	return WriteEntryOriginal(stream, data, countWritten)
}

// [SDO Client] update parameters
func WriteEntry1280(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || countWritten == nil {
		return ODR_DEV_INCOMPAT
	}
	client, ok := stream.Object.(*SDOClient)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	switch stream.Subindex {
	case 0:
		return ODR_READONLY
	// cob id client to server
	case 1:
		cobId := binary.LittleEndian.Uint32(data)
		canId := uint16(cobId & 0x7FF)
		canIdCurrent := uint16(client.CobIdClientToServer & 0x7FF)
		valid := (cobId & 0x80000000) == 0
		if (cobId&0x3FFFF800) != 0 ||
			(valid && client.Valid && canId != canIdCurrent) ||
			(valid && isIDRestricted(canId)) {
			return ODR_INVALID_VALUE
		}
		client.Setup(cobId, client.CobIdServerToClient, client.NodeIdServer)
	// cob id server to client
	case 2:
		cobId := binary.LittleEndian.Uint32(data)
		canId := uint16(cobId & 0x7FF)
		canIdCurrent := uint16(client.CobIdServerToClient & 0x7FF)
		valid := (cobId & 0x80000000) == 0
		if (cobId&0x3FFFF800) != 0 ||
			(valid && client.Valid && canId != canIdCurrent) ||
			(valid && isIDRestricted(canId)) {
			return ODR_INVALID_VALUE
		}
		client.Setup(cobId, client.CobIdClientToServer, client.NodeIdServer)
	// node id of server
	case 3:
		if len(data) != 1 {
			return ODR_TYPE_MISMATCH
		}
		nodeId := data[0]
		if nodeId > 127 {
			return ODR_INVALID_VALUE
		}
		client.NodeIdServer = nodeId

	default:
		return ODR_SUB_NOT_EXIST

	}
	return WriteEntryOriginal(stream, data, countWritten)
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
	pdo := &rpdo.PDO
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
			err := pdo.busManager.InsertRxBuffer(
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
		if transmissionType > TRANSMISSION_TYPE_SYNC_240 && transmissionType < TRANSMISSION_TYPE_SYNC_EVENT_LO {
			return ODR_INVALID_VALUE
		}
		synchronous := transmissionType <= TRANSMISSION_TYPE_SYNC_240
		// Remove old message from second buffer
		if rpdo.Synchronous != synchronous {
			rpdo.RxNew[1] = false
		}
		rpdo.Synchronous = synchronous
		if transmissionType < TRANSMISSION_TYPE_SYNC_EVENT_LO {
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

// [RPDO][TPDO] get communication parameter
func ReadEntry14xxOr18xx(stream *Stream, data []byte, countRead *uint16) error {
	err := ReadEntryOriginal(stream, data, countRead)
	// Add node id when reading subindex 1
	if err == nil && stream.Subindex == 1 && *countRead == 4 {
		// Get the corresponding object, either TPDO or RPDO
		var pdo *PDOCommon
		switch v := stream.Object.(type) {
		case *RPDO:
			pdo = &v.PDO
		case *TPDO:
			pdo = &v.PDO
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

// [RPDO][TPDO] update mapping parameter
func WriteEntry16xxOr1Axx(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || countWritten == nil || stream.Subindex > MAX_MAPPED_ENTRIES {
		return ODR_DEV_INCOMPAT
	}
	// Get the corresponding object, either TPDO or RPDO
	var pdo *PDOCommon
	switch v := stream.Object.(type) {
	case *RPDO:
		pdo = &v.PDO
	case *TPDO:
		pdo = &v.PDO
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
			dataLength := streamer.stream.DataLength
			mappedLength := streamer.stream.DataOffset
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
	pdo := &tpdo.PDO
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
			var err error
			tpdo.txBuffer, err = pdo.busManager.InsertTxBuffer(
				canId,
				false,
				uint8(pdo.DataLength),
				tpdo.TransmissionType <= TRANSMISSION_TYPE_SYNC_240,
			)
			if tpdo.txBuffer == nil || err != nil {
				return ODR_DEV_INCOMPAT
			}
			pdo.Valid = valid
			pdo.ConfiguredIdent = uint16(canId)
		}

	case 2:
		// Transmission type
		transmissionType := data[0]
		if transmissionType > TRANSMISSION_TYPE_SYNC_240 && transmissionType < TRANSMISSION_TYPE_SYNC_EVENT_LO {
			return ODR_INVALID_VALUE
		}
		tpdo.txBuffer.SyncFlag = transmissionType <= TRANSMISSION_TYPE_SYNC_240
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
		// Event timer
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
