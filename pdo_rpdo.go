package canopen

import log "github.com/sirupsen/logrus"

const (
	CO_RPDO_RX_ACK_NO_ERROR = 0  /* No error */
	CO_RPDO_RX_ACK_ERROR    = 1  /* Error is acknowledged */
	CO_RPDO_RX_ACK          = 10 /* Auxiliary value */
	CO_RPDO_RX_OK           = 11 /* Correct RPDO received, not acknowledged */
	CO_RPDO_RX_SHORT        = 12 /* Too short RPDO received, not acknowledged */
	CO_RPDO_RX_LONG         = 13 /* Too long RPDO received, not acknowledged */
)

type RPDO struct {
	PDO           PDOCommon
	RxNew         [RPDO_BUFFER_COUNT]bool
	RxData        [RPDO_BUFFER_COUNT][MAX_PDO_LENGTH]byte
	ReceiveError  uint8
	Sync          *SYNC
	Synchronous   bool
	TimeoutTimeUs uint32
	TimeoutTimer  uint32
}

func (rpdo *RPDO) Handle(frame Frame) {
	pdo := &rpdo.PDO
	err := rpdo.ReceiveError

	if !pdo.Valid {
		return
	}

	if frame.DLC >= uint8(pdo.DataLength) {
		// Indicate if errors in PDO length
		if frame.DLC == uint8(pdo.DataLength) {
			if err == CO_RPDO_RX_ACK_ERROR {
				err = CO_RPDO_RX_OK
			}
		} else {
			if err == CO_RPDO_RX_ACK_NO_ERROR {
				err = CO_RPDO_RX_LONG
			}
		}
		// Determine where to copy the message
		bufNo := 0
		if rpdo.Synchronous && rpdo.Sync != nil && rpdo.Sync.RxToggle {
			bufNo = 1
		}
		rpdo.RxData[bufNo] = frame.Data
		rpdo.RxNew[bufNo] = true

	} else if err == CO_RPDO_RX_ACK_NO_ERROR {
		err = CO_RPDO_RX_SHORT
	}
	rpdo.ReceiveError = err

}

func (rpdo *RPDO) configureCOBID(entry14xx *Entry, predefinedIdent uint32, erroneousMap uint32) (canId uint32, e error) {
	cobId := uint32(0)
	pdo := &rpdo.PDO
	ret := entry14xx.GetUint32(1, &cobId)
	if ret != nil {
		log.Errorf("[RPDO][%x|%x] reading %v failed : %v", entry14xx.Index, 1, entry14xx.Name, ret)
		return 0, CO_ERROR_OD_PARAMETERS
	}
	valid := (cobId & 0x80000000) == 0
	canId = cobId & 0x7FF
	if valid && pdo.MappedObjectsCount == 0 || canId == 0 {
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
	pdo.BufferIdx, ret = pdo.busManager.InsertRxBuffer(canId, 0x7FF, false, rpdo)
	if ret != nil {
		return 0, ret
	}
	pdo.Valid = valid
	return canId, nil
}

func (rpdo *RPDO) Init(od *ObjectDictionary,
	em *EM,
	sync *SYNC,
	predefinedIdent uint16,
	entry14xx *Entry,
	entry16xx *Entry,
	busManager *BusManager) error {
	pdo := &rpdo.PDO
	if od == nil || em == nil || entry14xx == nil || entry16xx == nil || busManager == nil {
		return CO_ERROR_ILLEGAL_ARGUMENT
	}

	// Reset RPDO entirely
	*rpdo = RPDO{}
	pdo.em = em
	pdo.busManager = busManager

	// Configure mapping params
	erroneousMap := uint32(0)
	ret := pdo.InitMapping(od, entry16xx, true, &erroneousMap)
	if ret != nil {
		return ret
	}
	// Configure communication params
	canId, err := rpdo.configureCOBID(entry14xx, uint32(predefinedIdent), erroneousMap)
	if err != nil {
		return err
	}
	// Configure transmission type
	transmissionType := uint8(TRANSMISSION_TYPE_SYNC_EVENT_LO)
	ret = entry14xx.GetUint8(2, &transmissionType)
	if ret != nil {
		log.Errorf("[RPDO][%x|%x] reading transmission type failed : %v", entry14xx.Index, 2, ret)
		return CO_ERROR_OD_PARAMETERS
	}
	rpdo.Sync = sync
	rpdo.Synchronous = transmissionType <= TRANSMISSION_TYPE_SYNC_240

	// Configure event timer
	eventTime := uint16(0)
	ret = entry14xx.GetUint16(5, &eventTime)
	if ret != nil {
		log.Errorf("[RPDO][%x|%x] reading event timer failed : %v", entry14xx.Index, 5, ret)
	}
	rpdo.TimeoutTimeUs = uint32(eventTime) * 1000
	pdo.IsRPDO = true
	pdo.od = od
	pdo.PreDefinedIdent = predefinedIdent
	pdo.ConfiguredIdent = uint16(canId)
	pdo.ExtensionCommunicationParam.Object = rpdo
	pdo.ExtensionCommunicationParam.Read = ReadEntry14xxOr18xx
	pdo.ExtensionCommunicationParam.Write = WriteEntry14xx
	pdo.ExtensionMappingParam.Object = rpdo
	pdo.ExtensionMappingParam.Read = ReadEntryOriginal
	pdo.ExtensionMappingParam.Write = WriteEntry16xxOr1Axx
	entry14xx.AddExtension(&pdo.ExtensionCommunicationParam)
	entry16xx.AddExtension(&pdo.ExtensionMappingParam)
	return nil
}

func (rpdo *RPDO) Process(timeDifferenceUs uint32, timerNext *uint32, nmtIsOperational bool, syncWas bool) {
	pdo := &rpdo.PDO
	if !pdo.Valid || !nmtIsOperational || (!syncWas && rpdo.Synchronous) {
		// not valid and op, clear can receive flags & timeouttimer
		if !pdo.Valid || !nmtIsOperational {
			rpdo.RxNew[0] = false
			rpdo.RxNew[1] = false
			rpdo.TimeoutTimer = 0
		}
		return

	}
	// Check errors in length of received messages
	if rpdo.ReceiveError > CO_RPDO_RX_ACK {
		setError := rpdo.ReceiveError != CO_RPDO_RX_OK
		errorCode := CO_EMC_PDO_LENGTH
		if rpdo.ReceiveError != CO_RPDO_RX_SHORT {
			errorCode = CO_EMC_PDO_LENGTH_EXC
		}
		pdo.em.Error(setError, CO_EM_RPDO_WRONG_LENGTH, uint16(errorCode), pdo.DataLength)
		if setError {
			rpdo.ReceiveError = CO_RPDO_RX_ACK_ERROR
		} else {
			rpdo.ReceiveError = CO_RPDO_RX_ACK_NO_ERROR
		}
	}
	// Get the correct rx buffer
	bufNo := uint8(0)
	if rpdo.Synchronous && rpdo.Sync != nil && !rpdo.Sync.RxToggle {
		bufNo = 1
	}
	// Copy RPDO into OD variables
	rpdoReceived := false
	for rpdo.RxNew[bufNo] {
		rpdoReceived = true
		dataRPDO := rpdo.RxData[bufNo][:]
		rpdo.RxNew[bufNo] = false
		for i := 0; i < int(pdo.MappedObjectsCount); i++ {
			streamer := &pdo.Streamers[i]
			dataOffset := &streamer.Stream.DataOffset
			mappedLength := *dataOffset
			dataLength := streamer.Stream.DataLength
			if dataLength > uint32(MAX_PDO_LENGTH) {
				dataLength = uint32(MAX_PDO_LENGTH)
			}
			// Prepare for writing into OD
			var buffer []byte
			buffer, dataRPDO = dataRPDO[:mappedLength], dataRPDO[mappedLength:]
			if dataLength > uint32(mappedLength) {
				// Append zeroes up to 8 bytes
				buffer = append(buffer, make([]byte, int(MAX_PDO_LENGTH)-len(buffer))...)
			}
			var countWritten uint16
			*dataOffset = 0
			err := streamer.Write(buffer, &countWritten)
			if err != nil {
				log.Warnf("[RPDO][%x] failed to write to OD on RPDO reception because %v", rpdo.PDO.ConfiguredIdent, err)
			}
			*dataOffset = mappedLength

		}
	}
	if rpdo.TimeoutTimeUs <= 0 {
		return
	}
	//Check timeouts
	if rpdoReceived {
		if rpdo.TimeoutTimer > rpdo.TimeoutTimeUs {
			pdo.em.ErrorReset(CO_EM_RPDO_TIME_OUT, rpdo.TimeoutTimer)
		}
		rpdo.TimeoutTimer = 1
	} else if rpdo.TimeoutTimer > 0 && rpdo.TimeoutTimer < rpdo.TimeoutTimeUs {
		rpdo.TimeoutTimer += timeDifferenceUs
		if rpdo.TimeoutTimer > rpdo.TimeoutTimeUs {
			pdo.em.ErrorReport(CO_EM_RPDO_TIME_OUT, CO_EMC_RPDO_TIMEOUT, rpdo.TimeoutTimer)
		}
	}
	if timerNext != nil && rpdo.TimeoutTimer < rpdo.TimeoutTimeUs {
		diff := rpdo.TimeoutTimeUs - rpdo.TimeoutTimer
		if *timerNext > diff {
			*timerNext = diff
		}
	}

}
