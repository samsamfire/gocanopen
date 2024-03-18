package pdo

import (
	canopen "github.com/samsamfire/gocanopen"
	can "github.com/samsamfire/gocanopen/pkg/can"
	"github.com/samsamfire/gocanopen/pkg/emergency"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/samsamfire/gocanopen/pkg/sync"
	log "github.com/sirupsen/logrus"
)

const (
	rpdoRxAckNoError = 0  // No error
	rpdoRxAckError   = 1  // Error is acknowledged
	rpdoRxAck        = 10 // Auxiliary value
	rpdoRxOk         = 11 // Correct RPDO received, not acknowledged
	rpdoRxShort      = 12 // Too short RPDO received, not acknowledged
	rpdoRxLong       = 13 // Too long RPDO received, not acknowledged
)

type RPDO struct {
	*canopen.BusManager
	pdo           PDOCommon
	rxNew         [RPDO_BUFFER_COUNT]bool
	rxData        [RPDO_BUFFER_COUNT][MAX_PDO_LENGTH]byte
	receiveError  uint8
	sync          *sync.SYNC
	synchronous   bool
	timeoutTimeUs uint32
	timeoutTimer  uint32
}

func (rpdo *RPDO) Handle(frame can.Frame) {
	pdo := &rpdo.pdo
	err := rpdo.receiveError

	if !pdo.Valid {
		return
	}

	if frame.DLC >= uint8(pdo.dataLength) {
		// Indicate if errors in PDO length
		if frame.DLC == uint8(pdo.dataLength) {
			if err == rpdoRxAckError {
				err = rpdoRxOk
			}
		} else {
			if err == rpdoRxAckNoError {
				err = rpdoRxLong
			}
		}
		// Determine where to copy the message
		bufNo := 0
		if rpdo.synchronous && rpdo.sync != nil && rpdo.sync.RxToggle {
			bufNo = 1
		}
		rpdo.rxData[bufNo] = frame.Data
		rpdo.rxNew[bufNo] = true

	} else if err == rpdoRxAckNoError {
		err = rpdoRxShort
	}
	rpdo.receiveError = err

}

func (rpdo *RPDO) configureCOBID(entry14xx *od.Entry, predefinedIdent uint32, erroneousMap uint32) (canId uint32, e error) {
	pdo := &rpdo.pdo
	cobId, ret := entry14xx.Uint32(1)
	if ret != nil {
		log.Errorf("[RPDO][%x|%x] reading %v failed : %v", entry14xx.Index, 1, entry14xx.Name, ret)
		return 0, canopen.ErrOdParameters
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
		pdo.emcy.ErrorReport(emergency.EmPDOWrongMapping, emergency.ErrProtocolError, errorInfo)
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

func (rpdo *RPDO) Process(timeDifferenceUs uint32, timerNext *uint32, nmtIsOperational bool, syncWas bool) {
	pdo := &rpdo.pdo
	if !pdo.Valid || !nmtIsOperational || (!syncWas && rpdo.synchronous) {
		// not valid and op, clear can receive flags & timeouttimer
		if !pdo.Valid || !nmtIsOperational {
			rpdo.rxNew[0] = false
			rpdo.rxNew[1] = false
			rpdo.timeoutTimer = 0
		}
		return

	}
	// Check errors in length of received messages
	if rpdo.receiveError > rpdoRxAck {
		setError := rpdo.receiveError != rpdoRxOk
		errorCode := emergency.ErrPdoLength
		if rpdo.receiveError != rpdoRxShort {
			errorCode = emergency.ErrPdoLengthExc
		}
		pdo.emcy.Error(setError, emergency.EmRPDOWrongLength, uint16(errorCode), pdo.dataLength)
		if setError {
			rpdo.receiveError = rpdoRxAckError
		} else {
			rpdo.receiveError = rpdoRxAckNoError
		}
	}
	// Get the correct rx buffer
	bufNo := uint8(0)
	if rpdo.synchronous && rpdo.sync != nil && !rpdo.sync.RxToggle {
		bufNo = 1
	}
	// Copy RPDO into OD variables
	rpdoReceived := false
	for rpdo.rxNew[bufNo] {
		rpdoReceived = true
		dataRPDO := rpdo.rxData[bufNo][:]
		rpdo.rxNew[bufNo] = false
		for i := 0; i < int(pdo.nbMapped); i++ {
			streamer := &pdo.streamers[i]
			mappedLength := streamer.DataOffset()
			dataLength := streamer.DataLength()
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
			streamer.SetDataOffset(0)
			_, err := streamer.Write(buffer)
			if err != nil {
				log.Warnf("[RPDO][%x] failed to write to OD on RPDO reception because %v", rpdo.pdo.configuredId, err)
			}
			streamer.SetDataOffset(mappedLength)

		}
	}
	if rpdo.timeoutTimeUs <= 0 {
		return
	}
	// Check timeouts
	if rpdoReceived {
		if rpdo.timeoutTimer > rpdo.timeoutTimeUs {
			pdo.emcy.ErrorReset(emergency.EmRPDOTimeOut, rpdo.timeoutTimer)
		}
		rpdo.timeoutTimer = 1
	} else if rpdo.timeoutTimer > 0 && rpdo.timeoutTimer < rpdo.timeoutTimeUs {
		rpdo.timeoutTimer += timeDifferenceUs
		if rpdo.timeoutTimer > rpdo.timeoutTimeUs {
			pdo.emcy.ErrorReport(emergency.EmRPDOTimeOut, emergency.ErrRpdoTimeout, rpdo.timeoutTimer)
		}
	}
	if timerNext != nil && rpdo.timeoutTimer < rpdo.timeoutTimeUs {
		diff := rpdo.timeoutTimeUs - rpdo.timeoutTimer
		if *timerNext > diff {
			*timerNext = diff
		}
	}

}

func NewRPDO(
	bm *canopen.BusManager,
	odict *od.ObjectDictionary,
	em *emergency.EMCY,
	sync *sync.SYNC,
	entry14xx *od.Entry,
	entry16xx *od.Entry,
	predefinedIdent uint16,
) (*RPDO, error) {
	if odict == nil || entry14xx == nil || entry16xx == nil || bm == nil {
		return nil, canopen.ErrIllegalArgument
	}
	rpdo := &RPDO{BusManager: bm}
	// Configure mapping parameters
	erroneousMap := uint32(0)
	pdo, err := NewPDO(odict, entry16xx, true, em, &erroneousMap)
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
		return nil, canopen.ErrOdParameters
	}
	rpdo.sync = sync
	rpdo.synchronous = transmissionType <= TRANSMISSION_TYPE_SYNC_240

	// Configure event timer
	eventTime, ret := entry14xx.Uint16(5)
	if ret != nil {
		log.Warnf("[RPDO][%x|%x] reading event timer failed : %v", entry14xx.Index, 5, ret)
	}
	rpdo.timeoutTimeUs = uint32(eventTime) * 1000
	pdo.IsRPDO = true
	pdo.od = odict
	pdo.predefinedId = predefinedIdent
	pdo.configuredId = uint16(canId)
	entry14xx.AddExtension(rpdo, readEntry14xxOr18xx, writeEntry14xx)
	entry16xx.AddExtension(rpdo, od.ReadEntryDefault, writeEntry16xxOr1Axx)
	log.Debugf("[RPDO][%x] Finished initializing | canId : %v | valid : %v | event timer : %v | synchronous : %v",
		entry14xx.Index,
		canId,
		pdo.Valid,
		eventTime,
		rpdo.synchronous,
	)
	return rpdo, nil
}
