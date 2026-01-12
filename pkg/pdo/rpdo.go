package pdo

import (
	"fmt"
	"log/slog"
	s "sync"
	"time"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/emergency"
	"github.com/samsamfire/gocanopen/pkg/nmt"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/samsamfire/gocanopen/pkg/sync"
)

const (
// Codes used for reporting specific RPDO length errors
)

type RPDO struct {
	bm            *canopen.BusManager
	mu            s.Mutex
	pdo           *PDOCommon
	rxData        []byte
	sync          *sync.SYNC
	synchronous   bool
	timeoutRx     time.Duration
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

	// Do not process if invalid or NMT not operational
	if !rpdo.pdo.Valid || !rpdo.isOperational {
		return
	}

	// Check for correct legnth, process any errors
	if !rpdo.validateFrameLength(frame.DLC) {
		return
	}

	// Timeout timer logic, if enabled
	rpdo.restartTimeoutTimer()
	if rpdo.inTimeout {
		rpdo.pdo.emcy.ErrorReset(emergency.EmRPDOTimeOut, 0)
		rpdo.inTimeout = false
	}

	// If async, copy data to OD straight away
	if !rpdo.synchronous {
		rpdo.copyDataToOd(frame.Data[:])
	}

	// Buffer RX data
	rpdo.rxData = frame.Data[:]
}

func (rpdo *RPDO) syncHandler() {
	for range rpdo.syncCh {
		rpdo.mu.Lock()
		if rpdo.rxData != nil {
			data := rpdo.rxData
			rpdo.copyDataToOd(data)
			rpdo.rxData = nil
		}
		rpdo.mu.Unlock()
	}
}

// Start subscription & timers
func (rpdo *RPDO) Start() error {
	rpdo.mu.Lock()
	defer rpdo.mu.Unlock()

	if rpdo.rxCancel == nil {
		rxCancel, err := rpdo.bm.Subscribe(uint32(rpdo.pdo.configuredId), 0x7FF, false, rpdo)
		rpdo.rxCancel = rxCancel
		if err != nil {
			return err
		}
	}

	if rpdo.synchronous && rpdo.sync != nil {
		rpdo.syncCh = rpdo.sync.Subscribe()
		go rpdo.syncHandler()
	}

	return nil
}

// Stop any subscriptions & timers
func (rpdo *RPDO) Stop() {
	rpdo.mu.Lock()
	defer rpdo.mu.Unlock()

	if rpdo.rxCancel != nil {
		rpdo.rxCancel()
		rpdo.rxCancel = nil
	}

	if rpdo.timer != nil {
		rpdo.timer.Stop()
	}

	if rpdo.sync != nil && rpdo.syncCh != nil {
		rpdo.sync.Unsubscribe(rpdo.syncCh)
		rpdo.syncCh = nil
	}

	rpdo.rxData = nil
	rpdo.inTimeout = false
}

// Check the frame DLC and manages the receiveError state.
// Returns true if the frame should be processed, false otherwise.
func (rpdo *RPDO) validateFrameLength(dlc uint8) bool {
	expectedLength := uint8(rpdo.pdo.dataLength)

	if dlc == expectedLength {
		rpdo.pdo.emcy.Error(false, emergency.EmRPDOWrongLength, emergency.ErrNoError, 0)
		return true
	}

	errorCode := emergency.ErrPdoLength
	if dlc > expectedLength {
		errorCode = emergency.ErrPdoLengthExc
	}

	rpdo.pdo.emcy.Error(true, emergency.EmRPDOWrongLength, uint16(errorCode), rpdo.pdo.dataLength)
	return false
}

func (rpdo *RPDO) restartTimeoutTimer() {
	if rpdo.timeoutRx == 0 {
		return
	}
	if rpdo.timer == nil {
		rpdo.timer = time.AfterFunc(rpdo.timeoutRx, rpdo.timeoutHandler)
	} else {
		rpdo.timer.Reset(rpdo.timeoutRx)
	}
}

func (rpdo *RPDO) timeoutHandler() {
	rpdo.mu.Lock()
	defer rpdo.mu.Unlock()

	if !rpdo.isOperational {
		return
	}

	rpdo.inTimeout = true
	rpdo.pdo.emcy.ErrorReport(emergency.EmRPDOTimeOut, emergency.ErrRpdoTimeout, 0)
}

func (rpdo *RPDO) OnStateChange(state uint8) {
	operational := state == nmt.StateOperational
	rpdo.mu.Lock()
	rpdo.isOperational = operational
	rpdo.mu.Unlock()

	if operational {
		rpdo.Start()
	} else {
		rpdo.Stop()
	}
}

func (rpdo *RPDO) copyDataToOd(data []byte) {
	// Assumes rpdo.mu is locked by caller
	pdo := rpdo.pdo
	offset := uint32(0)

	for i := 0; i < int(pdo.nbMapped); i++ {
		streamer := &pdo.streamers[i]

		// Determine the slice of data for this object
		end := offset + streamer.DataLength
		if end > uint32(len(data)) {
			// Should not happen if dataLength is consistent with nbMapped/MaxPdoLength
			break
		}

		streamer.DataOffset = 0
		if _, err := streamer.Write(data[offset:end]); err != nil {
			rpdo.pdo.logger.Warn("failed to write to OD on RPDO reception",
				"configured id", rpdo.pdo.configuredId,
				"error", err,
			)
		}

		streamer.DataOffset = streamer.DataLength
		offset = end
	}
}

func (rpdo *RPDO) configureCobId(entry14xx *od.Entry, predefinedIdent uint16, erroneousMap uint32) (canId uint16, e error) {
	rpdo.mu.Lock()
	defer rpdo.mu.Unlock()

	pdo := rpdo.pdo
	canId, err := pdo.configureCobId(entry14xx, predefinedIdent, erroneousMap)
	if err != nil {
		return 0, err
	}
	pdo.Valid = canId != 0
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
	rpdo := &RPDO{bm: bm}
	// Configure mapping parameters
	erroneousMap := uint32(0)
	pdo, err := NewPDO(odict, logger, entry16xx, true, emcy, &erroneousMap)
	if err != nil {
		return nil, err
	}
	rpdo.pdo = pdo
	// Configure communication params
	canId, err := rpdo.configureCobId(entry14xx, predefinedIdent, erroneousMap)
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
	rpdo.timeoutRx = time.Duration(eventTime) * time.Millisecond
	pdo.IsRPDO = true
	pdo.od = odict
	pdo.predefinedId = predefinedIdent
	pdo.configuredId = uint16(canId)
	entry14xx.AddExtension(rpdo, readEntry14xxOr18xx, writeEntry14xx)
	entry16xx.AddExtension(rpdo, od.ReadEntryDefault, writeEntry16xxOr1Axx)
	err = rpdo.Start()
	rpdo.pdo.logger.Debug("finished initializing",
		"canId", canId,
		"valid", pdo.Valid,
		"event time", eventTime,
		"synchronous", rpdo.synchronous,
	)
	return rpdo, err
}
