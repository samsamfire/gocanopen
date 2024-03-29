package canopen

import log "github.com/sirupsen/logrus"

type TPDO struct {
	*busManager
	sync             *SYNC
	pdo              PDOCommon
	txBuffer         Frame
	transmissionType uint8
	sendRequest      bool
	syncStartValue   uint8
	syncCounter      uint8
	inhibitTimeUs    uint32
	eventTimeUs      uint32
	inhibitTimer     uint32
	eventTimer       uint32
}

func (tpdo *TPDO) configureTransmissionType(entry18xx *Entry) error {
	transmissionType, ret := entry18xx.Uint8(2)
	if ret != nil {
		log.Errorf("[TPDO][%x|%x] reading %v failed : %v", entry18xx.Index, 2, entry18xx.Name, ret)
		return ErrOdParameters
	}
	if transmissionType < TRANSMISSION_TYPE_SYNC_EVENT_LO && transmissionType > TRANSMISSION_TYPE_SYNC_240 {
		transmissionType = TRANSMISSION_TYPE_SYNC_EVENT_LO
	}
	tpdo.transmissionType = transmissionType
	tpdo.sendRequest = true
	return nil
}

func (tpdo *TPDO) configureCOBID(entry18xx *Entry, predefinedIdent uint16, erroneousMap uint32) (canId uint16, e error) {
	pdo := &tpdo.pdo
	cobId, ret := entry18xx.Uint32(1)
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
		pdo.emcy.ErrorReport(emPDOWrongMapping, emErrProtocolError, errorInfo)
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
		tpdo.sendRequest = true
		tpdo.inhibitTimer = 0
		tpdo.eventTimer = 0
		tpdo.syncCounter = 255
		return
	}

	if tpdo.transmissionType == TRANSMISSION_TYPE_SYNC_ACYCLIC || tpdo.transmissionType >= TRANSMISSION_TYPE_SYNC_EVENT_LO {
		if tpdo.eventTimeUs != 0 {
			if tpdo.eventTimer > timeDifferenceUs {
				tpdo.eventTimer = tpdo.eventTimer - timeDifferenceUs
			} else {
				tpdo.eventTimer = 0
			}
			if tpdo.eventTimer == 0 {
				tpdo.sendRequest = true
			}
			if timerNextUs != nil && *timerNextUs > tpdo.eventTimer {
				*timerNextUs = tpdo.eventTimer
			}
		}
		// Check for tpdo send requests
		if !tpdo.sendRequest {
			for i := 0; i < int(pdo.nbMapped); i++ {
				flagPDOByte := pdo.flagPDOByte[i]
				if flagPDOByte != nil {
					if (*flagPDOByte & pdo.flagPDOBitmask[i]) == 0 {
						tpdo.sendRequest = true
					}
				}
			}
		}
	}
	// Send PDO by application request or event timer
	if tpdo.transmissionType >= TRANSMISSION_TYPE_SYNC_EVENT_LO {
		if tpdo.inhibitTimer > timeDifferenceUs {
			tpdo.inhibitTimer = tpdo.inhibitTimer - timeDifferenceUs
		} else {
			tpdo.inhibitTimer = 0
		}
		if tpdo.sendRequest && tpdo.inhibitTimer == 0 {
			tpdo.Send()
		}
		if tpdo.sendRequest && timerNextUs != nil && *timerNextUs > tpdo.inhibitTimer {
			*timerNextUs = tpdo.inhibitTimer
		}
	} else if tpdo.sync != nil && syncWas {

		// Send synchronous acyclic tpdo
		if tpdo.transmissionType == TRANSMISSION_TYPE_SYNC_ACYCLIC &&
			tpdo.sendRequest {
			tpdo.Send()
			return
		}
		// Send synchronous cyclic TPDOs
		if tpdo.syncCounter == 255 {
			if tpdo.sync.counterOverflow != 0 && tpdo.syncStartValue != 0 {
				// Sync start value used

				tpdo.syncCounter = 254
			} else {
				tpdo.syncCounter = tpdo.transmissionType/2 + 1
			}
		}
		// If sync start value is used , start first TPDO
		//after sync with matched syncstartvalue
		switch tpdo.syncCounter {
		case 254:
			if tpdo.sync.counter == tpdo.syncStartValue {
				tpdo.syncCounter = tpdo.transmissionType
				tpdo.Send()
			}
		case 1:
			tpdo.syncCounter = tpdo.transmissionType
			tpdo.Send()

		default:
			tpdo.syncCounter--
		}

	}

}

// Send TPDO object
func (tpdo *TPDO) Send() error {
	pdo := &tpdo.pdo
	eventDriven := tpdo.transmissionType == TRANSMISSION_TYPE_SYNC_ACYCLIC || tpdo.transmissionType >= uint8(TRANSMISSION_TYPE_SYNC_EVENT_LO)
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
	tpdo.sendRequest = false
	tpdo.eventTimer = tpdo.eventTimeUs
	tpdo.inhibitTimer = tpdo.inhibitTimeUs
	// Copy data to the buffer & send
	copy(tpdo.txBuffer.Data[:], dataTPDO)
	return tpdo.busManager.Send(tpdo.txBuffer)
}

// Create a new TPDO
func NewTPDO(
	bm *busManager,
	od *ObjectDictionary,
	em *EMCY,
	sync *SYNC,
	entry18xx *Entry,
	entry1Axx *Entry,
	predefinedIdent uint16,

) (*TPDO, error) {
	if od == nil || entry18xx == nil || entry1Axx == nil || bm == nil {
		return nil, ErrIllegalArgument
	}
	tpdo := &TPDO{busManager: bm}
	// Configure mapping parameters
	erroneousMap := uint32(0)
	pdo, err := NewPDO(od, entry1Axx, false, em, &erroneousMap)
	tpdo.pdo = *pdo
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
	// Configure inhibit time (not mandatory)
	inhibitTime, err := entry18xx.Uint16(3)
	if err != nil {
		log.Warnf("[TPDO][%x|%x] reading inhibit time failed : %v", entry18xx.Index, 3, err)
	}
	tpdo.inhibitTimeUs = uint32(inhibitTime) * 100

	// Configure event timer (not mandatory)
	eventTime, err := entry18xx.Uint16(5)
	if err != nil {
		log.Warnf("[TPDO][%x|%x] reading event timer failed : %v", entry18xx.Index, 5, err)
	}
	tpdo.eventTimeUs = uint32(eventTime) * 1000

	// Configure sync start value (not mandatory)
	tpdo.syncStartValue, err = entry18xx.Uint8(6)
	if err != nil {
		log.Warnf("[TPDO][%x|%x] reading sync start failed : %v", entry18xx.Index, 6, err)
	}
	tpdo.sync = sync
	tpdo.syncCounter = 255

	// Configure OD extensions
	pdo.IsRPDO = false
	pdo.predefinedId = predefinedIdent
	pdo.configuredId = canId
	entry18xx.AddExtension(tpdo, readEntry14xxOr18xx, writeEntry18xx)
	entry1Axx.AddExtension(tpdo, ReadEntryDefault, writeEntry16xxOr1Axx)

	log.Debugf("[TPDO][%x] Finished initializing | canId : %v | valid : %v | inhibit : %v | event timer : %v | transmission type : %v",
		entry18xx.Index,
		canId,
		pdo.Valid,
		inhibitTime,
		eventTime,
		tpdo.transmissionType,
	)
	return tpdo, nil

}
