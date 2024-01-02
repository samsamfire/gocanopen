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
	*busManager
	pdo           PDOCommon
	RxNew         [RPDO_BUFFER_COUNT]bool
	RxData        [RPDO_BUFFER_COUNT][MAX_PDO_LENGTH]byte
	ReceiveError  uint8
	sync          *SYNC
	Synchronous   bool
	TimeoutTimeUs uint32
	TimeoutTimer  uint32
}

func (rpdo *RPDO) Handle(frame Frame) {
	pdo := &rpdo.pdo
	err := rpdo.ReceiveError

	if !pdo.Valid {
		return
	}

	if frame.DLC >= uint8(pdo.dataLength) {
		// Indicate if errors in PDO length
		if frame.DLC == uint8(pdo.dataLength) {
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
		if rpdo.Synchronous && rpdo.sync != nil && rpdo.sync.RxToggle {
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
	pdo := &rpdo.pdo
	cobId, ret := entry14xx.Uint32(1)
	if ret != nil {
		log.Errorf("[RPDO][%x|%x] reading %v failed : %v", entry14xx.Index, 1, entry14xx.Name, ret)
		return 0, ErrOdParameters
	}
	valid := (cobId & 0x80000000) == 0
	canId = cobId & 0x7FF
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
		pdo.em.ErrorReport(emPDOWrongMapping, emErrProtocolError, errorInfo)
	}
	if !valid {
		canId = 0
	}
	// If default canId is stored in od add node id
	if canId != 0 && canId == (predefinedIdent&0xFF80) {
		canId = predefinedIdent
	}
	ret = rpdo.Subscribe(canId, 0x7FF, false, rpdo)
	if ret != nil {
		return 0, ret
	}
	pdo.Valid = valid
	return canId, nil
}

func (rpdo *RPDO) process(timeDifferenceUs uint32, timerNext *uint32, nmtIsOperational bool, syncWas bool) {
	pdo := &rpdo.pdo
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
		errorCode := emErrPdoLength
		if rpdo.ReceiveError != CO_RPDO_RX_SHORT {
			errorCode = emErrPdoLengthExc
		}
		pdo.em.Error(setError, emRPDOWrongLength, uint16(errorCode), pdo.dataLength)
		if setError {
			rpdo.ReceiveError = CO_RPDO_RX_ACK_ERROR
		} else {
			rpdo.ReceiveError = CO_RPDO_RX_ACK_NO_ERROR
		}
	}
	// Get the correct rx buffer
	bufNo := uint8(0)
	if rpdo.Synchronous && rpdo.sync != nil && !rpdo.sync.RxToggle {
		bufNo = 1
	}
	// Copy RPDO into OD variables
	rpdoReceived := false
	for rpdo.RxNew[bufNo] {
		rpdoReceived = true
		dataRPDO := rpdo.RxData[bufNo][:]
		rpdo.RxNew[bufNo] = false
		for i := 0; i < int(pdo.nbMapped); i++ {
			streamer := &pdo.streamers[i]
			dataOffset := &streamer.stream.DataOffset
			mappedLength := *dataOffset
			dataLength := streamer.stream.DataLength
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
			*dataOffset = 0
			_, err := streamer.Write(buffer)
			if err != nil {
				log.Warnf("[RPDO][%x] failed to write to OD on RPDO reception because %v", rpdo.pdo.configuredId, err)
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
			pdo.em.ErrorReset(emRPDOTimeOut, rpdo.TimeoutTimer)
		}
		rpdo.TimeoutTimer = 1
	} else if rpdo.TimeoutTimer > 0 && rpdo.TimeoutTimer < rpdo.TimeoutTimeUs {
		rpdo.TimeoutTimer += timeDifferenceUs
		if rpdo.TimeoutTimer > rpdo.TimeoutTimeUs {
			pdo.em.ErrorReport(emRPDOTimeOut, emErrRpdoTimeout, rpdo.TimeoutTimer)
		}
	}
	if timerNext != nil && rpdo.TimeoutTimer < rpdo.TimeoutTimeUs {
		diff := rpdo.TimeoutTimeUs - rpdo.TimeoutTimer
		if *timerNext > diff {
			*timerNext = diff
		}
	}

}

func NewRPDO(
	bm *busManager,
	od *ObjectDictionary,
	em *EM,
	sync *SYNC,
	entry14xx *Entry,
	entry16xx *Entry,
	predefinedIdent uint16,
) (*RPDO, error) {
	if od == nil || entry14xx == nil || entry16xx == nil || bm == nil {
		return nil, ErrIllegalArgument
	}
	rpdo := &RPDO{busManager: bm}
	// Configure mapping parameters
	erroneousMap := uint32(0)
	pdo, err := NewPDO(od, entry16xx, true, em, &erroneousMap)
	rpdo.pdo = *pdo
	if err != nil {
		return nil, err
	}
	rpdo.pdo = *pdo
	// Configure communication params
	canId, err := rpdo.configureCOBID(entry14xx, uint32(predefinedIdent), erroneousMap)
	if err != nil {
		return nil, err
	}
	// Configure transmission type
	transmissionType, ret := entry14xx.Uint8(2)
	if ret != nil {
		log.Errorf("[RPDO][%x|%x] reading transmission type failed : %v", entry14xx.Index, 2, ret)
		return nil, ErrOdParameters
	}
	rpdo.sync = sync
	rpdo.Synchronous = transmissionType <= TRANSMISSION_TYPE_SYNC_240

	// Configure event timer
	eventTime, ret := entry14xx.Uint16(5)
	if ret != nil {
		log.Warnf("[RPDO][%x|%x] reading event timer failed : %v", entry14xx.Index, 5, ret)
	}
	rpdo.TimeoutTimeUs = uint32(eventTime) * 1000
	pdo.IsRPDO = true
	pdo.od = od
	pdo.predefinedId = predefinedIdent
	pdo.configuredId = uint16(canId)
	entry14xx.AddExtension(rpdo, ReadEntry14xxOr18xx, WriteEntry14xx)
	entry16xx.AddExtension(rpdo, ReadEntryDefault, WriteEntry16xxOr1Axx)
	log.Debugf("[RPDO][%x] Finished initializing | canId : %v | valid : %v | event timer : %v | synchronous : %v",
		entry14xx.Index,
		canId,
		pdo.Valid,
		eventTime,
		rpdo.Synchronous,
	)
	return rpdo, nil
}
