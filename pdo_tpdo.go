package canopen

import log "github.com/sirupsen/logrus"

type TPDO struct {
	pdo              PDOCommon
	txBuffer         Frame
	TransmissionType uint8
	SendRequest      bool
	sync             *SYNC
	SyncStartValue   uint8
	SyncCounter      uint8
	InhibitTimeUs    uint32
	EventTimeUs      uint32
	InhibitTimer     uint32
	EventTimer       uint32
	busManager       *BusManager
}

func (tpdo *TPDO) configureTransmissionType(entry18xx *Entry) error {
	transmissionType := uint8(TRANSMISSION_TYPE_SYNC_EVENT_LO)
	ret := entry18xx.Uint8(2, &transmissionType)
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
	pdo := &tpdo.pdo
	cobId := uint32(0)
	ret := entry18xx.Uint32(1, &cobId)
	if ret != nil {
		log.Errorf("[TPDO][%x|%x] reading %v failed : %v", entry18xx.Index, 1, entry18xx.Name, ret)
		return 0, ErrOdParameters
	}
	valid := (cobId & 0x80000000) == 0
	canId = uint16(cobId & 0x7FF)
	if valid && (pdo.nbMapped == 0 || canId == 0) {
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
	tpdo.txBuffer = NewFrame(uint32(canId), 0, uint8(pdo.dataLength))
	pdo.Valid = valid
	return canId, nil

}

func (tpdo *TPDO) process(timeDifferenceUs uint32, timerNextUs *uint32, nmtIsOperational bool, syncWas bool) {

	pdo := &tpdo.pdo
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
			for i := 0; i < int(pdo.nbMapped); i++ {
				flagPDOByte := pdo.flagPDOByte[i]
				if flagPDOByte != nil {
					if (*flagPDOByte & pdo.flagPDOBitmask[i]) == 0 {
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
	} else if tpdo.sync != nil && syncWas {

		// Send synchronous acyclic tpdo
		if tpdo.TransmissionType == TRANSMISSION_TYPE_SYNC_ACYCLIC &&
			tpdo.SendRequest {
			tpdo.Send()
			return
		}
		// Send synchronous cyclic TPDOs
		if tpdo.SyncCounter == 255 {
			if tpdo.sync.CounterOverflowValue != 0 && tpdo.SyncStartValue != 0 {
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
			if tpdo.sync.Counter == tpdo.SyncStartValue {
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
	pdo := &tpdo.pdo
	eventDriven := tpdo.TransmissionType == TRANSMISSION_TYPE_SYNC_ACYCLIC || tpdo.TransmissionType >= uint8(TRANSMISSION_TYPE_SYNC_EVENT_LO)
	dataTPDO := make([]byte, 0)
	for i := 0; i < int(pdo.nbMapped); i++ {
		streamer := &pdo.streamers[i]
		mappedLength := streamer.stream.DataOffset
		dataLength := int(streamer.stream.DataLength)
		if dataLength > int(MAX_PDO_LENGTH) {
			dataLength = int(MAX_PDO_LENGTH)
		}

		streamer.stream.DataOffset = 0
		buffer := make([]byte, dataLength)
		_, err := streamer.Read(buffer)
		if err != nil {
			log.Warnf("[TPDO]sending TPDO cob id %x failed : %v", pdo.configuredId, err)
			return err
		}
		streamer.stream.DataOffset = mappedLength
		// Add to tpdo frame only up to mapped length
		dataTPDO = append(dataTPDO, buffer[:mappedLength]...)

		flagPDOByte := pdo.flagPDOByte[i]
		if flagPDOByte != nil && eventDriven {
			*flagPDOByte |= pdo.flagPDOBitmask[i]
		}
	}
	tpdo.SendRequest = false
	tpdo.EventTimer = tpdo.EventTimeUs
	tpdo.InhibitTimer = tpdo.InhibitTimeUs
	// Copy data to the buffer & send
	copy(tpdo.txBuffer.Data[:], dataTPDO)
	return tpdo.busManager.Send(tpdo.txBuffer)
}

// Create a new TPDO
func NewTPDO(
	busManager *BusManager,
	od *ObjectDictionary,
	em *EM,
	sync *SYNC,
	entry18xx *Entry,
	entry1Axx *Entry,
	predefinedIdent uint16,

) (*TPDO, error) {
	if od == nil || em == nil || entry18xx == nil || entry1Axx == nil || busManager == nil {
		return nil, ErrIllegalArgument
	}
	tpdo := &TPDO{}
	// Configure mapping parameters
	erroneousMap := uint32(0)
	pdo, err := NewPDO(od, entry1Axx, false, em, &erroneousMap)
	tpdo.pdo = *pdo
	tpdo.busManager = busManager
	if err != nil {
		return nil, err
	}
	// Configure transmission type
	err = tpdo.configureTransmissionType(entry18xx)
	if err != nil {
		return nil, err
	}
	// Configure COB ID
	canId, err := tpdo.configureCOBID(entry18xx, predefinedIdent, erroneousMap)
	if err != nil {
		return nil, err
	}
	// Configure inhibit timer (not mandatory)
	inhibitTime := uint16(0)
	err = entry18xx.Uint16(3, &inhibitTime)
	if err != nil {
		log.Warnf("[TPDO][%x|%x] reading inhibit timer failed : %v", entry18xx.Index, 3, err)
	}
	tpdo.InhibitTimeUs = uint32(inhibitTime) * 100

	// Configure event timer (not mandatory)
	eventTime := uint16(0)
	err = entry18xx.Uint16(5, &eventTime)
	if err != nil {
		log.Warnf("[TPDO][%x|%x] reading event timer failed : %v", entry18xx.Index, 5, err)
	}
	tpdo.EventTimeUs = uint32(eventTime) * 1000

	// Configure sync start value (not mandatory)
	tpdo.SyncStartValue = 0
	err = entry18xx.Uint8(6, &tpdo.SyncStartValue)
	if err != nil {
		log.Warnf("[TPDO][%x|%x] reading sync start failed : %v", entry18xx.Index, 6, err)
	}
	tpdo.sync = sync
	tpdo.SyncCounter = 255

	// Configure OD extensions
	pdo.IsRPDO = false
	pdo.predefinedId = predefinedIdent
	pdo.configuredId = canId
	entry18xx.AddExtension(tpdo, ReadEntry14xxOr18xx, WriteEntry18xx)
	entry1Axx.AddExtension(tpdo, ReadEntryDefault, WriteEntry16xxOr1Axx)

	log.Debugf("[TPDO][%x] Finished initializing | canId : %v | valid : %v | inhibit : %v | event timer : %v | transmission type : %v",
		entry18xx.Index,
		canId,
		pdo.Valid,
		inhibitTime,
		eventTime,
		tpdo.TransmissionType,
	)
	return tpdo, nil

}
