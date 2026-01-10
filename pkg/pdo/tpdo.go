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
	SyncCounterReset        = 255
	SyncCounterWaitForStart = 254
)

type TPDO struct {
	bm                    *canopen.BusManager
	mu                    s.Mutex
	sync                  *sync.SYNC
	pdo                   *PDOCommon
	txBuffer              canopen.Frame
	transmissionType      uint8
	sendRequestAsyncEvent bool
	syncStartValue        uint8
	syncCounter           uint8
	timeLastSend          time.Time
	timeInhibit           time.Duration
	timeEvent             time.Duration
	timerEvent            *time.Timer
	isOperational         bool
	syncCh                chan uint8
}

// Process TPDOs on SYNC reception
func (tpdo *TPDO) syncHandler() {
	for range tpdo.syncCh {
		tpdo.mu.Lock()
		isAsyclic := tpdo.transmissionType == TransmissionTypeSyncAcyclic

		// Send synchronous acyclic tpdo
		if isAsyclic && tpdo.sendRequestAsyncEvent {
			tpdo.mu.Unlock()
			_ = tpdo.send()
			continue
		}

		// Send synchronous cyclic TPDOs
		if tpdo.syncCounter == SyncCounterReset {
			if tpdo.sync.CounterOverflow() != 0 && tpdo.syncStartValue != 0 {
				tpdo.syncCounter = SyncCounterWaitForStart
			} else {
				tpdo.syncCounter = tpdo.transmissionType
			}
		}

		// If sync start value is used , start first TPDO
		// after sync with matched syncstartvalue
		switch tpdo.syncCounter {

		case SyncCounterWaitForStart:
			if tpdo.sync.Counter() == tpdo.syncStartValue {
				tpdo.syncCounter = tpdo.transmissionType
				tpdo.mu.Unlock()
				_ = tpdo.send()
				continue
			}

		case 1:
			tpdo.syncCounter = tpdo.transmissionType
			tpdo.mu.Unlock()
			_ = tpdo.send()
			continue

		default:
			tpdo.syncCounter--
		}
		tpdo.mu.Unlock()
	}
}

func (tpdo *TPDO) configureTransmissionType(entry18xx *od.Entry) error {
	tpdo.mu.Lock()
	defer tpdo.mu.Unlock()

	transmissionType, err := entry18xx.Uint8(od.SubPdoTransmissionType)
	if err != nil {
		tpdo.pdo.logger.Error("reading failed",
			"index", fmt.Errorf("x%x", entry18xx.Index),
			"subindex", od.SubPdoTransmissionType,
			"error", err,
		)
		return canopen.ErrOdParameters
	}
	if transmissionType < TransmissionTypeSyncEventLo && transmissionType > TransmissionTypeSync240 {
		transmissionType = TransmissionTypeSyncEventLo
	}
	tpdo.transmissionType = transmissionType
	tpdo.sendRequestAsyncEvent = false
	return nil
}

func (tpdo *TPDO) configureCobId(entry18xx *od.Entry, predefinedIdent uint16, erroneousMap uint32) (canId uint16, e error) {
	tpdo.mu.Lock()
	defer tpdo.mu.Unlock()

	pdo := tpdo.pdo
	canId, err := pdo.configureCobId(entry18xx, predefinedIdent, erroneousMap)
	if err != nil {
		return 0, err
	}
	tpdo.txBuffer = canopen.NewFrame(uint32(canId), 0, uint8(pdo.dataLength))
	pdo.Valid = canId != 0
	return canId, nil

}

func (tpdo *TPDO) send() error {
	tpdo.mu.Lock()
	defer tpdo.mu.Unlock()

	pdo := tpdo.pdo
	if !pdo.Valid {
		return nil
	}

	totalNbRead := 0
	var err error

	for i := range pdo.nbMapped {
		streamer := &pdo.streamers[i]
		mappedLength := streamer.DataOffset
		streamer.DataOffset = 0
		_, err = streamer.Read(tpdo.txBuffer.Data[totalNbRead:])
		if err != nil {
			tpdo.pdo.logger.Warn("failed to send", "cobId", pdo.configuredId, "error", err)
			return err
		}
		streamer.DataOffset = mappedLength
		totalNbRead += int(mappedLength)
	}
	tpdo.sendRequestAsyncEvent = false
	tpdo.restartEventTimer()
	tpdo.timeLastSend = time.Now()
	return tpdo.bm.Send(tpdo.txBuffer)
}

// Send TPDO asynchronously
// Rate is limited by the inhibit time, which can be 0
func (tpdo *TPDO) SendAsync() {
	tpdo.mu.Lock()
	defer tpdo.mu.Unlock()

	isAcyclic := tpdo.transmissionType == TransmissionTypeSyncAcyclic
	if isAcyclic {
		tpdo.sendRequestAsyncEvent = true
		return
	}

	// If no inhibit, send straight away
	if tpdo.timeInhibit == 0 {
		_ = tpdo.send()
		return
	}

	// If inhibit, we must cancel any event timers
	// and send only after the inhibit time is elapsed
	remaining := time.Since(tpdo.timeLastSend) - tpdo.timeInhibit
	if tpdo.timerEvent == nil {
		tpdo.timerEvent = time.AfterFunc(max(0, remaining), tpdo.eventHandler)
	} else {
		tpdo.timerEvent.Reset(max(0, remaining))
	}
	tpdo.send()
}

func (tpdo *TPDO) SetOperational(operational bool) {
	tpdo.mu.Lock()
	defer tpdo.mu.Unlock()

	tpdo.isOperational = operational
	if operational {
		tpdo.restartEventTimer()
		return
	}

	// Stop timers
	if tpdo.timerEvent != nil {
		tpdo.timerEvent.Stop()
	}

}

func (tpdo *TPDO) restartEventTimer() {
	if tpdo.timeEvent == 0 {
		return
	}
	// Event timer is used, the next send is limited by the inhibit time
	if tpdo.timerEvent == nil {
		tpdo.timerEvent = time.AfterFunc(max(tpdo.timeEvent, tpdo.timeInhibit), tpdo.eventHandler)
	} else {
		tpdo.timerEvent.Reset(max(tpdo.timeEvent, tpdo.timeInhibit))
	}
}

func (tpdo *TPDO) eventHandler() {
	tpdo.mu.Lock()
	operational := tpdo.isOperational
	tpdo.mu.Unlock()

	if operational {
		_ = tpdo.send()
	}
}

// Create a new TPDO
func NewTPDO(
	bm *canopen.BusManager,
	logger *slog.Logger,
	odict *od.ObjectDictionary,
	emcy *emergency.EMCY,
	sync *sync.SYNC,
	entry18xx *od.Entry,
	entry1Axx *od.Entry,
	predefinedIdent uint16,

) (*TPDO, error) {
	if odict == nil || entry18xx == nil || entry1Axx == nil || bm == nil || emcy == nil {
		return nil, canopen.ErrIllegalArgument
	}

	tpdo := &TPDO{bm: bm}

	// Configure mapping parameters
	erroneousMap := uint32(0)
	pdo, err := NewPDO(odict, logger, entry1Axx, false, emcy, &erroneousMap)
	if err != nil {
		return nil, err
	}
	tpdo.pdo = pdo
	// Configure transmission type
	err = tpdo.configureTransmissionType(entry18xx)
	if err != nil {
		return nil, err
	}
	// Configure COB ID
	canId, err := tpdo.configureCobId(entry18xx, predefinedIdent, erroneousMap)
	if err != nil {
		return nil, err
	}
	// Configure inhibit time (not mandatory)
	inhibitTime, err := entry18xx.Uint16(od.SubPdoInhibitTime)
	if err != nil {
		tpdo.pdo.logger.Warn("reading inhibit time failed",
			"index", fmt.Sprintf("x%x", entry18xx.Index),
			"subindex", od.SubPdoInhibitTime,
			"error", err,
		)
	}
	tpdo.timeInhibit = time.Duration(inhibitTime) * 100 * time.Microsecond

	// Configure event timer (not mandatory)
	eventTime, err := entry18xx.Uint16(od.SubPdoEventTimer)
	if err != nil {
		tpdo.pdo.logger.Warn("reading event timer failed",
			"index", entry18xx.Index,
			"subindex", od.SubPdoEventTimer,
			"error", err,
		)

	}
	tpdo.timeEvent = time.Duration(eventTime) * 1000 * time.Microsecond

	// Configure sync start value (not mandatory)
	tpdo.syncStartValue, err = entry18xx.Uint8(od.SubPdoSyncStart)
	if err != nil {
		tpdo.pdo.logger.Warn("reading sync start failed",
			"index", entry18xx.Index,
			"subindex", od.SubPdoSyncStart,
			"error", err,
		)
	}
	tpdo.sync = sync
	tpdo.syncCounter = SyncCounterReset

	// Configure OD extensions
	pdo.IsRPDO = false
	pdo.predefinedId = predefinedIdent
	pdo.configuredId = canId
	entry18xx.AddExtension(tpdo, readEntry14xxOr18xx, writeEntry18xx)
	entry1Axx.AddExtension(tpdo, od.ReadEntryDefault, writeEntry16xxOr1Axx)
	tpdo.pdo.logger.Debug("finished initializing",
		"canId", canId,
		"valid", pdo.Valid,
		"inhibit time", inhibitTime,
		"event time", eventTime,
		"transmission type", tpdo.transmissionType,
	)
	if tpdo.transmissionType < TransmissionTypeSyncEventLo && tpdo.sync != nil {
		tpdo.syncCh = tpdo.sync.Subscribe()
		go tpdo.syncHandler()
	}
	return tpdo, nil
}
