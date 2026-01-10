package pdo

import (
	"fmt"
	"log/slog"
	s "sync"
	"time"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/emergency"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/samsamfire/gocanopen/pkg/sync"
)

const (
	rpdoRxAckNoError = 0  // No error
	rpdoRxAckError   = 1  // Error is acknowledged
	rpdoRxOk         = 11 // Correct RPDO received, not acknowledged
	rpdoRxShort      = 12 // Too short RPDO received, not acknowledged
	rpdoRxLong       = 13 // Too long RPDO received, not acknowledged
)

type RPDO struct {
	*canopen.BusManager
	mu            s.Mutex
	pdo           *PDOCommon
	rxData        [MaxPdoLength]byte
	rxNew         bool
	receiveError  uint8
	sync          *sync.SYNC
	synchronous   bool
	timeoutTimeUs uint32
	timer         *time.Timer
	inTimeout     bool
	isOperational bool
	rxCancel      func()
	syncCh        chan uint8
}

// Handle [RPDO] related RX CAN frames
func (rpdo *RPDO) Handle(frame canopen.Frame) {
	rpdo.mu.Lock()
	defer rpdo.mu.Unlock()

	// Don't process any further if PDO is not valid or NMT operational
	if !rpdo.pdo.Valid || !rpdo.isOperational {
		return
	}

	expectedLength := uint8(rpdo.pdo.dataLength)

	// If frame is too short, set error and don't process
	if frame.DLC < expectedLength {
		rpdo.receiveError = rpdoRxShort
		rpdo.processReceiveErrors(rpdo.pdo)
		return
	}

	// If frame is too long, set error but still process
	if frame.DLC > expectedLength {
		rpdo.receiveError = rpdoRxLong
		rpdo.processReceiveErrors(rpdo.pdo)
	}

	// If frame length is correct, clear any previous error
	if frame.DLC == expectedLength {
		if rpdo.receiveError != rpdoRxAckNoError {
			rpdo.receiveError = rpdoRxOk
			rpdo.processReceiveErrors(rpdo.pdo)
		}
	}

	// Reset timeout timer, if enabled
	if rpdo.timeoutTimeUs > 0 {
		if rpdo.timer != nil {
			rpdo.timer.Reset(time.Duration(rpdo.timeoutTimeUs) * time.Microsecond)
		} else {
			rpdo.timer = time.AfterFunc(time.Duration(rpdo.timeoutTimeUs)*time.Microsecond, rpdo.onTimeoutHandler)
		}
	}

	// Reset timeout error if it happened
	if rpdo.inTimeout {
		rpdo.pdo.emcy.ErrorReset(emergency.EmRPDOTimeOut, rpdo.timeoutTimeUs)
		rpdo.inTimeout = false
	}

	// For synchronous RPDOs, data is stored in an intermediate buffer, and
	// propagated to OD, only after a SYNC reception.
	// For asynchronous RPDOs, data is directly propagated to OD

	if rpdo.synchronous {
		rpdo.rxNew = true
		rpdo.rxData = frame.Data
		return
	}

	rpdo.copyDataToOd(rpdo.pdo, frame.Data)
}

// Callback on timeout event
func (rpdo *RPDO) onTimeoutHandler() {
	rpdo.mu.Lock()
	defer rpdo.mu.Unlock()

	if !rpdo.isOperational {
		return
	}

	rpdo.inTimeout = true
	rpdo.pdo.emcy.ErrorReport(emergency.EmRPDOTimeOut, emergency.ErrRpdoTimeout, rpdo.timeoutTimeUs)
}

func (rpdo *RPDO) SetOperational(operational bool) {
	rpdo.mu.Lock()
	defer rpdo.mu.Unlock()
	rpdo.isOperational = operational
	if !operational {
		if rpdo.timer != nil {
			rpdo.timer.Stop()
		}
		rpdo.rxNew = false
		rpdo.inTimeout = false
	}
}

func (rpdo *RPDO) syncHandler() {
	for range rpdo.syncCh {
		rpdo.mu.Lock()
		if rpdo.rxNew {
			data := rpdo.rxData
			rpdo.rxNew = false
			rpdo.mu.Unlock()
			rpdo.copyDataToOd(rpdo.pdo, data)
		} else {
			rpdo.mu.Unlock()
		}
	}
}

func (rpdo *RPDO) copyDataToOd(pdo *PDOCommon, data [8]byte) {

	var buffer []byte
	totalNbWritten := uint32(0)
	totalMappedLength := uint32(0)

	// Iterate over all the mapped objects and copy data from
	// received RPDO frame to OD via streamer objects.
	for i := range pdo.nbMapped {

		streamer := &pdo.streamers[i]
		mappedLength := streamer.DataOffset
		dataLength := streamer.DataLength

		// Paranoid check : the accumulated mapped length should never
		// exceed MaxPdoLength
		totalMappedLength += mappedLength
		if totalMappedLength > uint32(MaxPdoLength) {
			break
		}

		if dataLength > uint32(MaxPdoLength) {
			dataLength = uint32(MaxPdoLength)
		}

		buffer = data[totalNbWritten : totalNbWritten+mappedLength]
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

	// Additional check for total mapped length, should be equal to PDO data length
	// this should never happen unless software issue.
	if totalMappedLength > uint32(MaxPdoLength) || totalMappedLength != pdo.dataLength {
		pdo.emcy.ErrorReport(emergency.EmGenericSoftwareError, emergency.ErrSoftwareInternal, totalMappedLength)
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
	cobId, err := entry14xx.Uint32(od.SubPdoCobId)
	if err != nil {
		rpdo.pdo.logger.Error("reading failed",
			"index", fmt.Errorf("x%x", entry14xx.Index),
			"subindex", od.SubPdoCobId,
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
	transmissionType, err := entry14xx.Uint8(od.SubPdoTransmissionType)
	if err != nil {
		rpdo.pdo.logger.Error("reading transmission type failed",
			"index", fmt.Errorf("x%x", entry14xx.Index),
			"subindex", od.SubPdoTransmissionType,
			"error", err,
		)
		return nil, canopen.ErrOdParameters
	}
	rpdo.sync = sync
	rpdo.synchronous = transmissionType <= TransmissionTypeSync240

	// Configure event timer (not mandatory)
	eventTime, err := entry14xx.Uint16(od.SubPdoEventTimer)
	if err != nil {
		rpdo.pdo.logger.Warn("reading event timer failed",
			"index", fmt.Errorf("x%x", entry14xx.Index),
			"subindex", od.SubPdoEventTimer,
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
	if rpdo.synchronous && rpdo.sync != nil {
		rpdo.syncCh = rpdo.sync.Subscribe()
		go rpdo.syncHandler()
	}
	return rpdo, nil
}
