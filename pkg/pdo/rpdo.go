package pdo

import (
	"fmt"
	"log/slog"
	s "sync"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/emergency"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/samsamfire/gocanopen/pkg/sync"
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
	mu            s.Mutex
	pdo           *PDOCommon
	rxNew         [BufferCountRpdo]bool
	rxData        [BufferCountRpdo][MaxPdoLength]byte
	receiveError  uint8
	sync          *sync.SYNC
	synchronous   bool
	timeoutTimeUs uint32
	timeoutTimer  uint32
	rxCancel      func()
}

// Handle [RPDO] related RX CAN frames
func (rpdo *RPDO) Handle(frame canopen.Frame) {
	rpdo.mu.Lock()
	defer rpdo.mu.Unlock()
	pdo := rpdo.pdo
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
		if rpdo.synchronous && rpdo.sync != nil && rpdo.sync.RxToggle() {
			bufNo = 1
		}
		rpdo.rxData[bufNo] = frame.Data
		rpdo.rxNew[bufNo] = true

	} else if err == rpdoRxAckNoError {
		err = rpdoRxShort
	}
	rpdo.receiveError = err
}

// Process [RPDO] state machine and TX CAN frames
// This should be called periodically
func (rpdo *RPDO) Process(timeDifferenceUs uint32, nmtIsOperational bool, syncWas bool) {
	rpdo.mu.Lock()

	var buffer []byte
	pdo := rpdo.pdo

	if !pdo.Valid || !nmtIsOperational {
		rpdo.rxNew[0] = false
		rpdo.rxNew[1] = false
		rpdo.timeoutTimer = 0
		rpdo.mu.Unlock()
		return
	}

	if !syncWas && rpdo.synchronous {
		rpdo.mu.Unlock()
		return
	}

	// Check errors in length of received messages
	if rpdo.receiveError > rpdoRxAck {
		rpdo.processReceiveErrors(pdo)
	}

	// Get the correct rx buffer
	bufNo := uint8(0)
	if rpdo.synchronous && rpdo.sync != nil && !rpdo.sync.RxToggle() {
		bufNo = 1
	}

	// Handle timeout logic if RPDO not received
	if !rpdo.rxNew[bufNo] {
		if rpdo.timeoutTimeUs > 0 && rpdo.timeoutTimer > 0 && rpdo.timeoutTimer < rpdo.timeoutTimeUs {
			rpdo.timeoutTimer += timeDifferenceUs
			if rpdo.timeoutTimer > rpdo.timeoutTimeUs {
				pdo.emcy.ErrorReport(emergency.EmRPDOTimeOut, emergency.ErrRpdoTimeout, rpdo.timeoutTimer)
			}
		}
		rpdo.mu.Unlock()
		return
	}

	localData := rpdo.rxData[bufNo][:pdo.dataLength]
	rpdo.rxNew[bufNo] = false
	rpdo.mu.Unlock()

	// Handle timeout logic, reset if happened
	timeoutHappened := rpdo.timeoutTimer > rpdo.timeoutTimeUs
	if rpdo.timeoutTimeUs > 0 {
		rpdo.timeoutTimer = 1
	}

	if timeoutHappened && rpdo.timeoutTimeUs > 0 {
		pdo.emcy.ErrorReset(emergency.EmRPDOTimeOut, rpdo.timeoutTimer)
	}

	totalNbWritten := uint32(0)
	totalMappedLength := uint32(0)
	rpdo.rxNew[bufNo] = false

	for i := range pdo.nbMapped {
		streamer := &pdo.streamers[i]
		mappedLength := streamer.DataOffset
		dataLength := streamer.DataLength

		// Paranoid check
		totalMappedLength += mappedLength
		if totalMappedLength > uint32(MaxPdoLength) {
			break
		}

		if dataLength > uint32(MaxPdoLength) {
			dataLength = uint32(MaxPdoLength)
		}

		buffer = localData[totalNbWritten : totalNbWritten+mappedLength]
		if dataLength > uint32(mappedLength) {
			buffer = buffer[:cap(buffer)]
		}
		streamer.DataOffset = 0
		_, err := streamer.Write(buffer)
		if err != nil {
			rpdo.pdo.logger.Warn("failed to write to OD on RPDO reception",
				"configured id", rpdo.pdo.configuredId,
				"error", err,
			)
		}
		streamer.DataOffset = mappedLength
		totalNbWritten += mappedLength
	}
}

func (rpdo *RPDO) processReceiveErrors(pdo *PDOCommon) {
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

func (rpdo *RPDO) configureCOBID(entry14xx *od.Entry, predefinedIdent uint32, erroneousMap uint32) (canId uint32, e error) {
	rpdo.mu.Lock()
	defer rpdo.mu.Unlock()

	pdo := rpdo.pdo
	cobId, err := entry14xx.Uint32(1)
	if err != nil {
		rpdo.pdo.logger.Error("reading failed",
			"index", fmt.Errorf("x%x", entry14xx.Index),
			"subindex", 2,
			"error", err,
		)
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
	rxCancel, err := rpdo.Subscribe(canId, 0x7FF, false, rpdo)
	rpdo.rxCancel = rxCancel
	if err != nil {
		return 0, err
	}
	pdo.Valid = valid
	return canId, nil
}

func NewRPDO(
	bm *canopen.BusManager,
	logger *slog.Logger,
	odict *od.ObjectDictionary,
	emcy *emergency.EMCY,
	sync *sync.SYNC,
	entry14xx *od.Entry,
	entry16xx *od.Entry,
	predefinedIdent uint16,
) (*RPDO, error) {
	if odict == nil || entry14xx == nil || entry16xx == nil || bm == nil || emcy == nil {
		return nil, canopen.ErrIllegalArgument
	}
	rpdo := &RPDO{BusManager: bm}
	// Configure mapping parameters
	erroneousMap := uint32(0)
	pdo, err := NewPDO(odict, logger, entry16xx, true, emcy, &erroneousMap)
	if err != nil {
		return nil, err
	}
	rpdo.pdo = pdo
	// Configure communication params
	canId, err := rpdo.configureCOBID(entry14xx, uint32(predefinedIdent), erroneousMap)
	if err != nil {
		return nil, err
	}
	// Configure transmission type
	transmissionType, err := entry14xx.Uint8(2)
	if err != nil {
		rpdo.pdo.logger.Error("reading transmission type failed",
			"index", fmt.Errorf("x%x", entry14xx.Index),
			"subindex", 2,
			"error", err,
		)
		return nil, canopen.ErrOdParameters
	}
	rpdo.sync = sync
	rpdo.synchronous = transmissionType <= TransmissionTypeSync240

	// Configure event timer
	eventTime, err := entry14xx.Uint16(5)
	if err != nil {
		rpdo.pdo.logger.Error("reading event timer failed",
			"index", fmt.Errorf("x%x", entry14xx.Index),
			"subindex", 5,
			"error", err,
		)
	}
	rpdo.timeoutTimeUs = uint32(eventTime) * 1000
	pdo.IsRPDO = true
	pdo.od = odict
	pdo.predefinedId = predefinedIdent
	pdo.configuredId = uint16(canId)
	entry14xx.AddExtension(rpdo, readEntry14xxOr18xx, writeEntry14xx)
	entry16xx.AddExtension(rpdo, od.ReadEntryDefault, writeEntry16xxOr1Axx)
	rpdo.pdo.logger.Debug("finished initializing",
		"canId", canId,
		"valid", pdo.Valid,
		"event time", eventTime,
		"synchronous", rpdo.synchronous,
	)
	return rpdo, nil
}
