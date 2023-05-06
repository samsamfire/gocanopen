package canopen

import (
	log "github.com/sirupsen/logrus"
)

/*
Common parts between RPDOs and TPDOs
*/

const (
	MAX_PDO_LENGTH     uint8 = 8
	MAX_MAPPED_ENTRIES uint8 = 8
	OD_FLAGS_PDO_SIZE  uint8 = 32
	RPDO_BUFFER_COUNT  uint8 = 2
)

const (
	CO_PDO_TRANSM_TYPE_SYNC_ACYCLIC  = 0    /**< synchronous (acyclic) */
	CO_PDO_TRANSM_TYPE_SYNC_1        = 1    /**< synchronous (cyclic every sync) */
	CO_PDO_TRANSM_TYPE_SYNC_240      = 0xF0 /**< synchronous (cyclic every 240-th sync) */
	CO_PDO_TRANSM_TYPE_SYNC_EVENT_LO = 0xFE /**< event-driven, lower value (manufacturer specific),  */
	CO_PDO_TRANSM_TYPE_SYNC_EVENT_HI = 0xFF /**< event-driven, higher value (device profile and application profile specific) */
)

// Common to TPDO & RPDO
type PDOCommon struct {
	od                          *ObjectDictionary
	em                          *EM
	busManager                  *BusManager
	Valid                       bool
	DataLength                  uint32
	MappedObjectsCount          uint8
	Streamers                   [MAX_MAPPED_ENTRIES]ObjectStreamer
	FlagPDOByte                 [OD_FLAGS_PDO_SIZE]*byte
	FlagPDOBitmask              [OD_FLAGS_PDO_SIZE]byte
	IsRPDO                      bool
	PreDefinedIdent             uint16
	ConfiguredIdent             uint16
	ExtensionMappingParam       Extension
	ExtensionCommunicationParam Extension
	BufferIdx                   int
}

// Configure a PDO map
func (base *PDOCommon) ConfigureMap(od *ObjectDictionary, mapParam uint32, mapIndex uint32, isRPDO bool) error {
	index := uint16(mapParam >> 16)
	subindex := byte(mapParam >> 8)
	mappedLengthBits := byte(mapParam)
	mappedLength := mappedLengthBits >> 3
	streamer := &base.Streamers[mapIndex]

	// Total PDO length should be smaller than max possible size
	if mappedLength > MAX_PDO_LENGTH {
		log.Warnf("[PDO][%x|%x] mapped parameter is too long", index, subindex)
		return ODR_MAP_LEN
	}
	// Dummy entries map to "fake" entries
	if index < 0x20 && subindex == 0 {
		streamer.Stream.Data = make([]byte, mappedLength)
		streamer.Stream.DataOffset = uint32(mappedLength)
		streamer.Write = WriteDummy
		streamer.Read = ReadDummy
		return nil
	}
	// Get entry in OD
	streamerCopy := ObjectStreamer{}
	entry := od.Index(index)
	ret := entry.Sub(subindex, false, &streamerCopy)
	if ret != nil {
		log.Debugf("[PDO] Couldn't get object x%x:x%x, because %v", index, subindex, ret)
		return ret
	}

	// Check access attributes, byte alignement and length
	var testAttribute ODA
	if isRPDO {
		testAttribute = ODA_RPDO
	} else {
		testAttribute = ODA_TPDO
	}
	if streamerCopy.Stream.Attribute&testAttribute == 0 ||
		(mappedLengthBits&0x07) != 0 ||
		int(streamerCopy.Stream.DataLength) < int(mappedLength) {
		log.Debugf("[PDO] couldn't map x%x:x%x (can be because of attribute, invalid size ... etc) %v, %v, %v",
			index,
			subindex,
			streamerCopy.Stream.Attribute&testAttribute == 0,
			(mappedLengthBits&0x07) != 0,
			int(streamerCopy.Stream.DataLength) < int(mappedLength),
		)
		return ODR_NO_MAP
	}
	*streamer = streamerCopy
	streamer.Stream.DataOffset = uint32(mappedLength)
	if !isRPDO {
		if uint32(subindex) < (uint32(OD_FLAGS_PDO_SIZE)*8) && entry.Extension != nil {
			base.FlagPDOByte[mapIndex] = &entry.Extension.flagsPDO[subindex>>3]
			base.FlagPDOBitmask[mapIndex] = 1 << (subindex & 0x07)
		} else {
			base.FlagPDOByte[mapIndex] = nil
		}
	}
	return nil

}

func (base *PDOCommon) InitMapping(od *ObjectDictionary, entry *Entry, isRPDO bool, erroneoursMap *uint32) error {
	pdoDataLength := 0
	mappedObjectsCount := uint8(0)

	// Get number of mapped objects
	ret := entry.GetUint8(0, &mappedObjectsCount)
	if ret != nil {
		log.Errorf("entry x%x, couldn't read number of mapped objects : %v", entry.Index, ret)
		return CO_ERROR_OD_PARAMETERS
	}

	// Iterate over all possible objects
	for i := range base.Streamers {
		streamer := &base.Streamers[i]
		mapParam := uint32(0)
		ret := entry.GetUint32(uint8(i)+1, &mapParam)
		if ret == ODR_SUB_NOT_EXIST {
			continue
		}
		if ret != nil {
			log.Errorf("[PDO] entry x%x, couldn't read mapping parameter subindex %v, because : %v", entry.Index, i+1, ret)
			return CO_ERROR_OD_PARAMETERS
		}
		ret = base.ConfigureMap(od, mapParam, uint32(i), isRPDO)
		if ret != nil {
			// Couldn't initialize the mapping
			streamer.Stream.Data = make([]byte, 0)
			streamer.Stream.DataOffset = 0xFF
			if *erroneoursMap == 0 {
				*erroneoursMap = mapParam
			}
			log.Warnf("[PDO] failed to initialize mapping parameter x%x,%x, because %v", entry.Index, i+1, ret)
		}
		if i < int(mappedObjectsCount) {
			pdoDataLength += int(streamer.Stream.DataOffset)
		}

	}
	if pdoDataLength > int(MAX_PDO_LENGTH) || (pdoDataLength == 0 && mappedObjectsCount > 0) {
		if *erroneoursMap == 0 {
			*erroneoursMap = 1
		}
	}
	if *erroneoursMap == 0 {
		base.DataLength = uint32(pdoDataLength)
		base.MappedObjectsCount = mappedObjectsCount
	}
	return nil
}

func (tpdo *TPDO) Init(
	od *ObjectDictionary,
	em *EM, sync *SYNC,
	predefinedIdent uint16,
	entry18xx *Entry,
	entry1Axx *Entry,
	busManager *BusManager) error {

	pdo := &tpdo.PDO
	if od == nil || em == nil || entry18xx == nil || entry1Axx == nil || busManager == nil {
		return CO_ERROR_ILLEGAL_ARGUMENT
	}
	// Clear TPDO
	*tpdo = TPDO{}
	pdo.em = em
	pdo.busManager = busManager
	// Configure mapping parameters
	erroneousMap := uint32(0)
	ret := pdo.InitMapping(od, entry1Axx, false, &erroneousMap)
	if ret != nil {
		return ret
	}
	// Configure transmission type
	transmissionType := uint8(CO_PDO_TRANSM_TYPE_SYNC_EVENT_LO)
	ret = entry18xx.GetUint8(2, &transmissionType)
	if ret != nil {
		return CO_ERROR_OD_PARAMETERS
	}
	if transmissionType < CO_PDO_TRANSM_TYPE_SYNC_EVENT_LO && transmissionType > CO_PDO_TRANSM_TYPE_SYNC_240 {
		transmissionType = CO_PDO_TRANSM_TYPE_SYNC_EVENT_LO
	}
	tpdo.TransmissionType = transmissionType
	tpdo.SendRequest = true

	// Configure COB-ID
	cobId := uint32(0)
	ret = entry18xx.GetUint32(1, &cobId)
	if ret != nil {
		return CO_ERROR_OD_PARAMETERS
	}
	valid := (cobId & 0x80000000) == 0
	canId := uint16(cobId & 0x7FF)
	if valid && (pdo.MappedObjectsCount == 0 || canId == 0) {
		valid = false
		if erroneousMap == 0 {
			erroneousMap = 1
		}
	}
	// if erroneousMap != 0 {
	// 	// TODO send emergency
	// }
	if !valid {
		canId = 0
	}
	// If default canId is stored in od add node id
	if canId != 0 && canId == (predefinedIdent&0xFF80) {
		canId = predefinedIdent
	}

	var err error
	tpdo.TxBuffer, pdo.BufferIdx, _ = pdo.busManager.InsertTxBuffer(uint32(canId), false, uint8(pdo.DataLength), tpdo.TransmissionType <= CO_PDO_TRANSM_TYPE_SYNC_240)
	if tpdo.TxBuffer == nil || err != nil {
		return CO_ERROR_ILLEGAL_ARGUMENT
	}
	pdo.Valid = valid
	// Configure inhibit time and event timer
	inhibitTime := uint16(0)
	eventTime := uint16(0)
	ret = entry18xx.GetUint16(3, &inhibitTime)
	if ret != nil {
		log.Warnf("[PDO] error reading inhibit time %v", ret)
	}
	ret = entry18xx.GetUint16(5, &eventTime)
	if ret != nil {
		log.Warnf("[PDO] error reading event time %v", ret)
	}
	tpdo.InhibitTimeUs = uint32(inhibitTime) * 100
	tpdo.EventTimeUs = uint32(eventTime) * 1000

	// Configure sync start value
	tpdo.SyncStartValue = 0
	ret = entry18xx.GetUint8(6, &tpdo.SyncStartValue)
	if ret != nil {
		log.Warnf("[PDO] error reading sync start %v", ret)
	}
	tpdo.Sync = sync
	tpdo.SyncCounter = 255

	// Configure OD extensions
	pdo.IsRPDO = false
	pdo.od = od
	pdo.busManager = busManager
	pdo.PreDefinedIdent = predefinedIdent
	pdo.ConfiguredIdent = canId
	pdo.ExtensionCommunicationParam.Object = tpdo
	pdo.ExtensionCommunicationParam.Read = ReadEntry14xxOr18xx
	pdo.ExtensionCommunicationParam.Write = WriteEntry18xx
	pdo.ExtensionMappingParam.Object = tpdo
	pdo.ExtensionMappingParam.Read = ReadEntryOriginal
	pdo.ExtensionMappingParam.Write = WriteEntry16xxOr1Axx
	entry18xx.AddExtension(&pdo.ExtensionCommunicationParam)
	entry1Axx.AddExtension(&pdo.ExtensionMappingParam)
	log.Debugf("[TPDO] Configuration parameter : canId : %v | valid : %v | inhibit : %v | event timer : %v | transmission type : %v",
		canId,
		valid,
		inhibitTime,
		eventTime,
		transmissionType,
	)
	return nil

}

// Send TPDO object
func (tpdo *TPDO) Send() error {
	pdo := &tpdo.PDO
	eventDriven := tpdo.TransmissionType == CO_PDO_TRANSM_TYPE_SYNC_ACYCLIC || tpdo.TransmissionType >= uint8(CO_PDO_TRANSM_TYPE_SYNC_EVENT_LO)
	dataTPDO := make([]byte, 0)
	for i := 0; i < int(pdo.MappedObjectsCount); i++ {
		streamer := &pdo.Streamers[i]
		stream := &streamer.Stream
		mappedLength := streamer.Stream.DataOffset
		dataLength := int(stream.DataLength)
		if dataLength > int(MAX_PDO_LENGTH) {
			dataLength = int(MAX_PDO_LENGTH)
		}

		stream.DataOffset = 0
		countRead := uint16(0)
		buffer := make([]byte, dataLength)
		streamer.Read(stream, buffer, &countRead)
		stream.DataOffset = mappedLength
		// Add to tpdo frame only up to mapped length
		dataTPDO = append(dataTPDO, buffer[:mappedLength]...)

		flagPDOByte := pdo.FlagPDOByte[i]
		if flagPDOByte != nil && eventDriven {
			*flagPDOByte |= pdo.FlagPDOBitmask[i]
		}
	}
	tpdo.SendRequest = false
	tpdo.EventTimer = tpdo.EventTimeUs
	tpdo.InhibitTimer = tpdo.InhibitTimeUs
	// Copy data to the buffer & send
	copy(tpdo.TxBuffer.Data[:], dataTPDO)
	return pdo.busManager.Send(*tpdo.TxBuffer)
}

func (tpdo *TPDO) Process(timeDifferenceUs uint32, timerNextUs *uint32, nmtIsOperational bool, syncWas bool) {

	pdo := &tpdo.PDO
	if !pdo.Valid || !nmtIsOperational {
		tpdo.SendRequest = true
		tpdo.InhibitTimer = 0
		tpdo.EventTimer = 0
		tpdo.SyncCounter = 255
		return
	}

	if tpdo.TransmissionType == CO_PDO_TRANSM_TYPE_SYNC_ACYCLIC || tpdo.TransmissionType >= CO_PDO_TRANSM_TYPE_SYNC_EVENT_LO {
		if tpdo.EventTimeUs != 0 {
			if tpdo.EventTimer > timeDifferenceUs {
				tpdo.EventTimer = tpdo.EventTimer - timeDifferenceUs
			} else {
				tpdo.EventTimer = 0
			}
			if tpdo.EventTimer == 0 {
				tpdo.SendRequest = true
			}
			if timerNextUs != nil && *timerNextUs > tpdo.EventTimer {
				*timerNextUs = tpdo.EventTimer
			}
		}
		// Check for tpdo send requests
		if !tpdo.SendRequest {
			for i := 0; i < int(pdo.MappedObjectsCount); i++ {
				flagPDOByte := pdo.FlagPDOByte[i]
				if flagPDOByte != nil {
					if (*flagPDOByte & pdo.FlagPDOBitmask[i]) == 0 {
						tpdo.SendRequest = true
					}
				}
			}
		}
	}
	// Send PDO by application request or event timer
	if tpdo.TransmissionType >= CO_PDO_TRANSM_TYPE_SYNC_EVENT_LO {
		if tpdo.InhibitTimer > timeDifferenceUs {
			tpdo.InhibitTimer = tpdo.InhibitTimer - timeDifferenceUs
		} else {
			tpdo.InhibitTimer = 0
		}
		if tpdo.SendRequest && tpdo.InhibitTimer == 0 {
			tpdo.Send()
		}
		if tpdo.SendRequest && timerNextUs != nil && *timerNextUs > tpdo.InhibitTimer {
			*timerNextUs = tpdo.InhibitTimer
		}
	} else if tpdo.Sync != nil && syncWas {

		// Send synchronous acyclic tpdo
		if tpdo.TransmissionType == CO_PDO_TRANSM_TYPE_SYNC_ACYCLIC &&
			tpdo.SendRequest {
			tpdo.Send()
			return
		}
		// Send synchronous cyclic TPDOs
		if tpdo.SyncCounter == 255 {
			if tpdo.Sync.CounterOverflowValue != 0 && tpdo.SyncStartValue != 0 {
				// Sync start value used

				tpdo.SyncCounter = 254
			} else {
				tpdo.SyncCounter = tpdo.TransmissionType/2 + 1
			}
		}
		// If sync start value is used , start first TPDO
		//after sync with matched syncstartvalue
		switch tpdo.SyncCounter {
		case 254:
			if tpdo.Sync.Counter == tpdo.SyncStartValue {
				tpdo.SyncCounter = tpdo.TransmissionType
				tpdo.Send()
			}
		case 1:
			tpdo.SyncCounter = tpdo.TransmissionType
			tpdo.Send()

		default:
			tpdo.SyncCounter--
		}

	}

}
