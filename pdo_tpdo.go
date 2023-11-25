package canopen

import log "github.com/sirupsen/logrus"

type TPDO struct {
	PDO              PDOCommon
	txBuffer         Frame
	TransmissionType uint8
	SendRequest      bool
	Sync             *SYNC
	SyncStartValue   uint8
	SyncCounter      uint8
	InhibitTimeUs    uint32
	EventTimeUs      uint32
	InhibitTimer     uint32
	EventTimer       uint32
}

func (tpdo *TPDO) configureTransmissionType(entry18xx *Entry) error {
	transmissionType := uint8(TRANSMISSION_TYPE_SYNC_EVENT_LO)
	ret := entry18xx.GetUint8(2, &transmissionType)
	if ret != nil {
		log.Errorf("[TPDO][%x|%x] reading %v failed : %v", entry18xx.Index, 2, entry18xx.Name, ret)
		return ErrOdParameters
	}
	if transmissionType < TRANSMISSION_TYPE_SYNC_EVENT_LO && transmissionType > TRANSMISSION_TYPE_SYNC_240 {
		transmissionType = TRANSMISSION_TYPE_SYNC_EVENT_LO
	}
	tpdo.TransmissionType = transmissionType
	tpdo.SendRequest = true
	return nil
}

func (tpdo *TPDO) configureCOBID(entry18xx *Entry, predefinedIdent uint16, erroneousMap uint32) (canId uint16, e error) {
	pdo := &tpdo.PDO
	cobId := uint32(0)
	ret := entry18xx.GetUint32(1, &cobId)
	if ret != nil {
		log.Errorf("[TPDO][%x|%x] reading %v failed : %v", entry18xx.Index, 1, entry18xx.Name, ret)
		return 0, ErrOdParameters
	}
	valid := (cobId & 0x80000000) == 0
	canId = uint16(cobId & 0x7FF)
	if valid && (pdo.MappedObjectsCount == 0 || canId == 0) {
		valid = false
		if erroneousMap == 0 {
			erroneousMap = 1
		}
	}
	if erroneousMap != 0 {
		errorInfo := erroneousMap
		if erroneousMap == 1 {
			errorInfo = cobId
		}
		pdo.em.ErrorReport(CO_EM_PDO_WRONG_MAPPING, CO_EMC_PROTOCOL_ERROR, errorInfo)
	}
	if !valid {
		canId = 0
	}
	// If default canId is stored in od add node id
	if canId != 0 && canId == (predefinedIdent&0xFF80) {
		canId = predefinedIdent
	}
	tpdo.txBuffer = NewFrame(uint32(canId), 0, uint8(pdo.DataLength))
	pdo.Valid = valid
	return canId, nil

}

// Called when creating node
func (tpdo *TPDO) Init(

	od *ObjectDictionary,
	em *EM, sync *SYNC,
	predefinedIdent uint16,
	entry18xx *Entry,
	entry1Axx *Entry,
	busManager *BusManager) error {

	pdo := &tpdo.PDO
	if od == nil || em == nil || entry18xx == nil || entry1Axx == nil || busManager == nil {
		return ErrIllegalArgument
	}

	// Reset TPDO entirely
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
	ret = tpdo.configureTransmissionType(entry18xx)
	if ret != nil {
		return ret
	}
	// Configure COB ID
	canId, err := tpdo.configureCOBID(entry18xx, predefinedIdent, erroneousMap)
	if err != nil {
		return err
	}
	// Configure inhibit timer (not mandatory)
	inhibitTime := uint16(0)
	ret = entry18xx.GetUint16(3, &inhibitTime)
	if ret != nil {
		log.Warnf("[TPDO][%x|%x] reading inhibit timer failed : %v", entry18xx.Index, 3, ret)
	}
	tpdo.InhibitTimeUs = uint32(inhibitTime) * 100

	// Configure event timer (not mandatory)
	eventTime := uint16(0)
	ret = entry18xx.GetUint16(5, &eventTime)
	if ret != nil {
		log.Warnf("[TPDO][%x|%x] reading event timer failed : %v", entry18xx.Index, 5, ret)
	}
	tpdo.EventTimeUs = uint32(eventTime) * 1000

	// Configure sync start value (not mandatory)
	tpdo.SyncStartValue = 0
	ret = entry18xx.GetUint8(6, &tpdo.SyncStartValue)
	if ret != nil {
		log.Warnf("[TPDO][%x|%x] reading sync start failed : %v", entry18xx.Index, 6, ret)
	}
	tpdo.Sync = sync
	tpdo.SyncCounter = 255

	// Configure OD extensions
	pdo.IsRPDO = false
	pdo.od = od
	pdo.busManager = busManager
	pdo.PreDefinedIdent = predefinedIdent
	pdo.ConfiguredIdent = canId
	pdo.ExtensionCommunicationParam = entry18xx.AddExtension(tpdo, ReadEntry14xxOr18xx, WriteEntry18xx)
	pdo.ExtensionMappingParam = entry1Axx.AddExtension(tpdo, ReadEntryOriginal, WriteEntry16xxOr1Axx)
	log.Debugf("[TPDO][%x] Finished initializing | canId : %v | valid : %v | inhibit : %v | event timer : %v | transmission type : %v",
		entry18xx.Index,
		canId,
		pdo.Valid,
		inhibitTime,
		eventTime,
		tpdo.TransmissionType,
	)
	return nil

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

	if tpdo.TransmissionType == TRANSMISSION_TYPE_SYNC_ACYCLIC || tpdo.TransmissionType >= TRANSMISSION_TYPE_SYNC_EVENT_LO {
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
	if tpdo.TransmissionType >= TRANSMISSION_TYPE_SYNC_EVENT_LO {
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
		if tpdo.TransmissionType == TRANSMISSION_TYPE_SYNC_ACYCLIC &&
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

// Send TPDO object
func (tpdo *TPDO) Send() error {
	pdo := &tpdo.PDO
	eventDriven := tpdo.TransmissionType == TRANSMISSION_TYPE_SYNC_ACYCLIC || tpdo.TransmissionType >= uint8(TRANSMISSION_TYPE_SYNC_EVENT_LO)
	dataTPDO := make([]byte, 0)
	for i := 0; i < int(pdo.MappedObjectsCount); i++ {
		streamer := &pdo.Streamers[i]
		mappedLength := streamer.stream.DataOffset
		dataLength := int(streamer.stream.DataLength)
		if dataLength > int(MAX_PDO_LENGTH) {
			dataLength = int(MAX_PDO_LENGTH)
		}

		streamer.stream.DataOffset = 0
		buffer := make([]byte, dataLength)
		_, err := streamer.Read(buffer)
		if err != nil {
			log.Warnf("[TPDO]sending TPDO cob id %x failed : %v", pdo.ConfiguredIdent, err)
			return err
		}
		streamer.stream.DataOffset = mappedLength
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
	copy(tpdo.txBuffer.Data[:], dataTPDO)
	return pdo.busManager.Send(tpdo.txBuffer)
}
