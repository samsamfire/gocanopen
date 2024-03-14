package canopen

// This file regroups OD extensions that are executed when reading or writing to object dictionary

import (
	"encoding/binary"
	"io"
	"os"

	can "github.com/samsamfire/gocanopen/pkg/can"
	log "github.com/sirupsen/logrus"
)

// [EMCY] read emergency history
func readEntry1003(stream *Stream, data []byte, countRead *uint16) error {
	if stream == nil || data == nil || countRead == nil ||
		(len(data) < 4 && stream.Subindex > 0) ||
		len(data) < 1 {
		return ODR_DEV_INCOMPAT
	}
	em, ok := stream.Object.(*EMCY)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	if len(em.fifo) < 2 {
		return ODR_DEV_INCOMPAT
	}
	if stream.Subindex == 0 {
		data[0] = em.fifoCount
		*countRead = 1
		return nil
	}
	if stream.Subindex > em.fifoCount {
		return ODR_NO_DATA
	}
	// Most recent error is in subindex 1 and stored behind fifoWrPtr
	index := int(em.fifoWrPtr) - int(stream.Subindex)
	if index >= len(em.fifo) {
		return ODR_DEV_INCOMPAT
	}
	if index < 0 {
		index += len(em.fifo)
	}
	binary.LittleEndian.PutUint32(data, em.fifo[index].msg)
	*countRead = 4
	return nil
}

// [EMCY] clear emergency history
func writeEntry1003(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || stream.Subindex != 0 || data == nil || len(data) != 1 || countWritten == nil {
		return ODR_DEV_INCOMPAT
	}
	if data[0] != 0 {
		return ODR_INVALID_VALUE
	}
	em, ok := stream.Object.(*EMCY)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	// Clear error history
	em.fifoCount = 0
	*countWritten = 1
	return nil
}

// [SYNC] update cob id & if should be producer
func writeEntry1005(stream *Stream, data []byte, countWritten *uint16) error {
	log.Debugf("[OD][EXTENSION][SYNC] updating COB-ID SYNC")
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
	if (cobIdSync&0xBFFFF800) != 0 || isIDRestricted(canId) || (sync.isProducer && isProducer && canId != uint16(sync.cobId)) {
		return ODR_INVALID_VALUE
	}
	// Reconfigure the receive and transmit buffers only if changed
	if canId != uint16(sync.cobId) {
		err := sync.Subscribe(uint32(canId), 0x7FF, false, sync)
		if err != nil {
			return ODR_DEV_INCOMPAT
		}
		var frameSize uint8 = 0
		if sync.counterOverflow != 0 {
			frameSize = 1
		}
		log.Debugf("[OD][EXTENSION][SYNC] updated COB-ID SYNC to x%x (prev x%x)", canId, sync.cobId)
		sync.txBuffer = can.NewFrame(uint32(canId), 0, frameSize)
		sync.cobId = uint32(canId)
	}
	// Reset in case sync is producer
	sync.isProducer = isProducer
	if isProducer {
		log.Debug("[OD][EXTENSION][SYNC] SYNC is producer")
		sync.counter = 0
		sync.timer = 0
	} else {
		log.Debug("[OD][EXTENSION][SYNC] SYNC is not producer")
	}
	return WriteEntryDefault(stream, data, countWritten)
}

// [SYNC] update communication cycle period
func writeEntry1006(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || stream.Subindex != 0 || countWritten == nil || len(data) != 4 {
		return ODR_DEV_INCOMPAT
	}
	cyclePeriodUs := binary.LittleEndian.Uint32(data)
	log.Debugf("[OD][EXTENSION][SYNC] updating communication cycle period to %v us (%v ms)", cyclePeriodUs, cyclePeriodUs/1000)
	return WriteEntryDefault(stream, data, countWritten)
}

// [SYNC] update pdo synchronous window length
func writeEntry1007(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || stream.Subindex != 0 || countWritten == nil || len(data) != 4 {
		return ODR_DEV_INCOMPAT
	}
	windowLengthUs := binary.LittleEndian.Uint32(data)
	log.Debugf("[OD][EXTENSION][SYNC] updating synchronous window length to %v us (%v ms)", windowLengthUs, windowLengthUs/1000)
	return WriteEntryDefault(stream, data, countWritten)
}

// [TIME] update cob id & if should be producer
func writeEntry1012(stream *Stream, data []byte, countWritten *uint16) error {
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
	time.isConsumer = (cobIdTimestamp & 0x80000000) != 0
	time.isProducer = (cobIdTimestamp & 0x40000000) != 0

	return WriteEntryDefault(stream, data, countWritten)
}

// [EMCY] read emergency cob id
func readEntry1014(stream *Stream, data []byte, countRead *uint16) error {
	if stream == nil || data == nil || countRead == nil || len(data) < 4 || stream.Subindex != 0 {
		return ODR_DEV_INCOMPAT
	}
	em, ok := stream.Object.(*EMCY)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	var canId uint16
	if em.producerIdent == EMERGENCY_SERVICE_ID {
		canId = EMERGENCY_SERVICE_ID + uint16(em.nodeId)
	} else {
		canId = em.producerIdent
	}
	var cobId uint32
	if em.producerEnabled {
		cobId = 0
	} else {
		cobId = 0x80000000
	}
	cobId |= uint32(canId)
	binary.LittleEndian.PutUint32(data, cobId)
	*countRead = 4
	return nil
}

// [EMCY] update emergency producer cob id
func writeEntry1014(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || countWritten == nil || len(data) != 4 || stream.Subindex != 0 {
		return ODR_DEV_INCOMPAT
	}
	em, ok := stream.Object.(*EMCY)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	// Check written value, cob id musn't change when enabled
	cobId := binary.LittleEndian.Uint32(data)
	newCanId := cobId & 0x7FF
	var currentCanId uint16
	if em.producerIdent == EMERGENCY_SERVICE_ID {
		currentCanId = EMERGENCY_SERVICE_ID + uint16(em.nodeId)
	} else {
		currentCanId = em.producerIdent
	}
	newEnabled := (cobId&uint32(currentCanId)) == 0 && newCanId != 0
	if cobId&0x7FFFF800 != 0 || isIDRestricted(uint16(newCanId)) ||
		(em.producerEnabled && newEnabled && newCanId != uint32(currentCanId)) {
		return ODR_INVALID_VALUE
	}
	em.producerEnabled = newEnabled
	if newCanId == uint32(EMERGENCY_SERVICE_ID+uint16(em.nodeId)) {
		em.producerIdent = EMERGENCY_SERVICE_ID
	} else {
		em.producerIdent = uint16(newCanId)
	}

	if newEnabled {
		em.txBuffer = can.NewFrame(newCanId, 0, 8)
	}
	return WriteEntryDefault(stream, data, countWritten)

}

// [EMCY] update inhibite time
func writeEntry1015(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || stream.Subindex != 0 || data == nil || len(data) != 2 || countWritten == nil {
		return ODR_DEV_INCOMPAT
	}
	em, ok := stream.Object.(*EMCY)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	em.inhibitTimeUs = uint32(binary.LittleEndian.Uint16(data)) * 100
	em.inhibitTimer = 0

	return WriteEntryDefault(stream, data, countWritten)

}

// [HBConsumer] update heartbeat consumer
func writeEntry1016(stream *Stream, data []byte, countWritten *uint16) error {
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
	nodeId := uint8(hbConsValue >> 16)
	time := hbConsValue & 0xFFFF
	log.Debugf("[OD][EXTENSION][HB CONSUMER] will monitor x%x with period %v ms", nodeId, time)
	err := consumer.addHearbeatConsumerNode(stream.Subindex-1, nodeId, uint16(time))
	if err != nil {
		return ODR_PAR_INCOMPAT
	}
	return WriteEntryDefault(stream, data, countWritten)
}

// [NMT] update heartbeat period
func writeEntry1017(stream *Stream, data []byte, countWritten *uint16) error {
	if stream.Subindex != 0 || data == nil || len(data) != 2 || countWritten == nil || stream == nil {
		return ODR_DEV_INCOMPAT
	}
	nmt, ok := stream.Object.(*NMT)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	nmt.hearbeatProducerTimeUs = uint32(binary.LittleEndian.Uint16(data)) * 1000
	nmt.hearbeatProducerTimer = 0
	log.Debugf("[OD][EXTENSION][NMT] updated heartbeat period to %v ms", nmt.hearbeatProducerTimeUs/1000)
	return WriteEntryDefault(stream, data, countWritten)
}

// [SYNC] update synchronous counter overflow
func writeEntry1019(stream *Stream, data []byte, countWritten *uint16) error {
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
	communicationCyclePeriod := binary.LittleEndian.Uint32(sync.rawCommunicationCyclePeriod)
	if communicationCyclePeriod != 0 {
		return ODR_DATA_DEV_STATE
	}
	var nbBytes = uint8(0)
	if syncCounterOverflow != 0 {
		nbBytes = 1
	}
	sync.txBuffer = can.NewFrame(sync.cobId, 0, nbBytes)
	sync.counterOverflow = syncCounterOverflow
	log.Debugf("[OD][EXTENSION][SYNC] updated synchronous counter overflow to %v", syncCounterOverflow)
	return WriteEntryDefault(stream, data, countWritten)
}

// [SDO server] update server parameters
func writeEntry1201(stream *Stream, data []byte, countWritten *uint16) error {
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
		canIdCurrent := uint16(server.cobIdClientToServer & 0x7FF)
		valid := (cobId & 0x80000000) == 0
		if (cobId&0x3FFFF800) != 0 ||
			(valid && server.valid && canId != canIdCurrent) ||
			(valid && isIDRestricted(canId)) {
			return ODR_INVALID_VALUE
		}
		server.initRxTx(cobId, server.cobIdServerToClient)
	// cob id server to client
	case 2:
		cobId := binary.LittleEndian.Uint32(data)
		canId := uint16(cobId & 0x7FF)
		canIdCurrent := uint16(server.cobIdServerToClient & 0x7FF)
		valid := (cobId & 0x80000000) == 0
		if (cobId&0x3FFFF800) != 0 ||
			(valid && server.valid && canId != canIdCurrent) ||
			(valid && isIDRestricted(canId)) {
			return ODR_INVALID_VALUE
		}
		server.initRxTx(server.cobIdClientToServer, cobId)
	// node id of server
	case 3:
		if len(data) != 1 {
			return ODR_TYPE_MISMATCH
		}
		nodeId := data[0]
		if nodeId < 1 || nodeId > 127 {
			return ODR_INVALID_VALUE
		}
		server.nodeId = nodeId // ??

	default:
		return ODR_SUB_NOT_EXIST

	}
	return WriteEntryDefault(stream, data, countWritten)
}

// [SDO Client] update parameters
func writeEntry1280(stream *Stream, data []byte, countWritten *uint16) error {
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
		canIdCurrent := uint16(client.cobIdClientToServer & 0x7FF)
		valid := (cobId & 0x80000000) == 0
		if (cobId&0x3FFFF800) != 0 ||
			(valid && client.valid && canId != canIdCurrent) ||
			(valid && isIDRestricted(canId)) {
			return ODR_INVALID_VALUE
		}
		client.setupServer(cobId, client.cobIdServerToClient, client.nodeIdServer)
	// cob id server to client
	case 2:
		cobId := binary.LittleEndian.Uint32(data)
		canId := uint16(cobId & 0x7FF)
		canIdCurrent := uint16(client.cobIdServerToClient & 0x7FF)
		valid := (cobId & 0x80000000) == 0
		if (cobId&0x3FFFF800) != 0 ||
			(valid && client.valid && canId != canIdCurrent) ||
			(valid && isIDRestricted(canId)) {
			return ODR_INVALID_VALUE
		}
		client.setupServer(cobId, client.cobIdClientToServer, client.nodeIdServer)
	// node id of server
	case 3:
		if len(data) != 1 {
			return ODR_TYPE_MISMATCH
		}
		nodeId := data[0]
		if nodeId > 127 {
			return ODR_INVALID_VALUE
		}
		client.nodeIdServer = nodeId

	default:
		return ODR_SUB_NOT_EXIST

	}
	return WriteEntryDefault(stream, data, countWritten)
}

// [RPDO] update communication parameter
func writeEntry14xx(stream *Stream, data []byte, countWritten *uint16) error {
	log.Debug("[OD][EXTENSION][RPDO] updating communication parameter")
	if stream == nil || data == nil || countWritten == nil || len(data) > 4 {
		return ODR_DEV_INCOMPAT
	}
	rpdo, ok := stream.Object.(*RPDO)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	pdo := &rpdo.pdo
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
			valid && isIDRestricted(uint16(canId)) ||
			valid && pdo.nbMapped == 0 {
			return ODR_INVALID_VALUE
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
					return ODR_DEV_INCOMPAT
				}
			}
			log.Debugf("[OD][EXTENSION][%v] updated pdo with cobId : x%x, valid : %v", pdo.Type(), pdo.configuredId&0x7FF, pdo.Valid)
		}

	case 2:
		// Transmission type
		transmissionType := data[0]
		if transmissionType > TRANSMISSION_TYPE_SYNC_240 && transmissionType < TRANSMISSION_TYPE_SYNC_EVENT_LO {
			return ODR_INVALID_VALUE
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

	return WriteEntryDefault(stream, bufCopy, countWritten)
}

// [RPDO][TPDO] get communication parameter
func readEntry14xxOr18xx(stream *Stream, data []byte, countRead *uint16) error {
	err := ReadEntryDefault(stream, data, countRead)
	// Add node id when reading subindex 1
	if err == nil && stream.Subindex == 1 && *countRead == 4 {
		// Get the corresponding object, either TPDO or RPDO
		var pdo *PDOCommon
		switch v := stream.Object.(type) {
		case *RPDO:
			pdo = &v.pdo
		case *TPDO:
			pdo = &v.pdo
		default:
			return ODR_DEV_INCOMPAT
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
func writeEntry16xxOr1Axx(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || countWritten == nil || stream.Subindex > MAX_MAPPED_ENTRIES {
		return ODR_DEV_INCOMPAT
	}
	// Get the corresponding object, either TPDO or RPDO
	var pdo *PDOCommon
	switch v := stream.Object.(type) {
	case *RPDO:
		pdo = &v.pdo
	case *TPDO:
		pdo = &v.pdo
	default:
		return ODR_DEV_INCOMPAT
	}
	log.Debugf("[OD][EXTENSION][%v] updating mapping parameter", pdo.Type())
	// PDO must be disabled in order to allow mapping
	if pdo.Valid || pdo.nbMapped != 0 && stream.Subindex > 0 {
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
			streamer := pdo.streamers[i]
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
		pdo.dataLength = pdoDataLength
		pdo.nbMapped = mappedObjectsCount
		log.Debugf("[OD][EXTENSION][%v] updated pdo number of mapped objects to : %v", pdo.Type(), mappedObjectsCount)

	} else {
		err := pdo.configureMap(binary.LittleEndian.Uint32(data), uint32(stream.Subindex)-1, pdo.IsRPDO)
		if err != nil {
			return err
		}
	}
	return WriteEntryDefault(stream, data, countWritten)
}

// [TPDO] update communication parameter
func writeEntry18xx(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || countWritten == nil || len(data) > 4 {
		return ODR_DEV_INCOMPAT
	}
	tpdo, ok := stream.Object.(*TPDO)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	pdo := &tpdo.pdo
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
			(valid && isIDRestricted(uint16(canId))) ||
			(valid && pdo.nbMapped == 0) {
			return ODR_INVALID_VALUE
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
			return ODR_INVALID_VALUE
		}
		tpdo.syncCounter = 255
		tpdo.transmissionType = transmissionType
		tpdo.sendRequest = true
		tpdo.inhibitTimer = 0
		tpdo.eventTimer = 0

	case 3:
		//Inhibit time
		if pdo.Valid {
			return ODR_INVALID_VALUE
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
			return ODR_INVALID_VALUE
		}
		tpdo.syncStartValue = syncStartValue

	}
	return WriteEntryDefault(stream, bufCopy, countWritten)

}

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

type FileObject struct {
	FilePath  string
	WriteMode int
	ReadMode  int
	File      *os.File
}

// [SDO] Custom function for reading a file like object
func ReadEntryFileObject(stream *Stream, data []byte, countRead *uint16) error {
	if stream == nil || data == nil || countRead == nil || stream.Subindex != 0 || stream.Object == nil {
		return ODR_DEV_INCOMPAT
	}
	fileObject, ok := stream.Object.(*FileObject)
	if !ok {
		stream.DataOffset = 0
		return ODR_DEV_INCOMPAT
	}
	if stream.DataOffset == 0 {
		var err error
		log.Infof("[OD][EXTENSION][FILE] opening %v for reading", fileObject.FilePath)
		fileObject.File, err = os.OpenFile(fileObject.FilePath, fileObject.ReadMode, 0644)
		if err != nil {
			return ODR_DEV_INCOMPAT
		}
	}
	countReadInt, err := io.ReadFull(fileObject.File, data)

	switch err {
	case nil:
		*countRead = uint16(countReadInt)
		stream.DataOffset += uint32(countReadInt)
		return ODR_PARTIAL
	case io.EOF, io.ErrUnexpectedEOF:
		*countRead = uint16(countReadInt)
		log.Infof("[OD][EXTENSION][FILE] finished reading %v", fileObject.FilePath)
		fileObject.File.Close()
		return nil
	default:
		//unexpected error
		log.Errorf("[OD][EXTENSION][FILE] error reading file %v", err)
		fileObject.File.Close()
		return ODR_DEV_INCOMPAT

	}
}

// [SDO] Custom function for writing a file like object
func WriteEntryFileObject(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || countWritten == nil || stream.Subindex != 0 || stream.Object == nil {
		return ODR_DEV_INCOMPAT
	}
	fileObject, ok := stream.Object.(*FileObject)
	if !ok {
		stream.DataOffset = 0
		return ODR_DEV_INCOMPAT
	}
	if stream.DataOffset == 0 {
		var err error
		log.Infof("[OD][EXTENSION][FILE] opening %v for writing", fileObject.FilePath)
		fileObject.File, err = os.OpenFile(fileObject.FilePath, fileObject.WriteMode, 0644)
		if err != nil {
			return ODR_DEV_INCOMPAT
		}
	}

	countWrittenInt, err := fileObject.File.Write(data)
	if err == nil {
		*countWritten = uint16(countWrittenInt)
		stream.DataOffset += uint32(countWrittenInt)
		if stream.DataLength == stream.DataOffset {
			log.Infof("[OD][EXTENSION][FILE] finished writing %v", fileObject.FilePath)
			fileObject.File.Close()
			return nil
		} else {
			return ODR_PARTIAL
		}
	} else {
		log.Errorf("[OD][EXTENSION][FILE] error writing file %v", err)
		fileObject.File.Close()
		return ODR_DEV_INCOMPAT
	}

}
