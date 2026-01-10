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
	*canopen.BusManager
	mu               s.Mutex
	sync             *sync.SYNC
	pdo              *PDOCommon
	txBuffer         canopen.Frame
	transmissionType uint8
	sendRequest      bool
	syncStartValue   uint8
	syncCounter      uint8
	inhibitTimeUs    uint32
	eventTimeUs      uint32
	timerInhibit     *time.Timer
	timerEvent       *time.Timer
	inhibitActive    bool
	isOperational    bool
	syncCh           chan uint8
}

func (tpdo *TPDO) syncHandler() {
	for range tpdo.syncCh {
		tpdo.mu.Lock()
		isSyncAcyclic := tpdo.transmissionType == TransmissionTypeSyncAcyclic

		// Send synchronous acyclic tpdo
		if isSyncAcyclic && tpdo.sendRequest {
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
	tpdo.sendRequest = true
	return nil
}

func (tpdo *TPDO) configureCOBID(entry18xx *od.Entry, predefinedIdent uint16, erroneousMap uint32) (canId uint16, e error) {
	tpdo.mu.Lock()
	defer tpdo.mu.Unlock()

	pdo := tpdo.pdo
	cobId, err := entry18xx.Uint32(od.SubPdoCobId)
	if err != nil {
		tpdo.pdo.logger.Error("reading failed",
			"index", fmt.Errorf("x%x", entry18xx.Index),
			"subindex", od.SubPdoCobId,
			"error", err,
		)
		return 0, canopen.ErrOdParameters
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
		pdo.emcy.ErrorReport(emergency.EmPDOWrongMapping, emergency.ErrProtocolError, errorInfo)
	}
	if !valid {
		canId = 0
	}
	// If default canId is stored in od add node id
	if canId != 0 && canId == (predefinedIdent&0xFF80) {
		canId = predefinedIdent
	}
	tpdo.txBuffer = canopen.NewFrame(uint32(canId), 0, uint8(pdo.dataLength))
	pdo.Valid = valid
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
	tpdo.sendRequest = false
	tpdo.restartEventTimer()
	tpdo.startInhibitTimer()
	return tpdo.Send(tpdo.txBuffer)
}

func (tpdo *TPDO) checkAndSend() {
	tpdo.mu.Lock()
	if tpdo.inhibitActive {
		tpdo.sendRequest = true
		tpdo.mu.Unlock()
		return
	}
	tpdo.mu.Unlock()
	_ = tpdo.send()
}

// Send TPDO asynchronously, next time it is processed
// This only works for event driven TPDOs
func (tpdo *TPDO) SendAsync() {
	tpdo.checkAndSend()
}

func (tpdo *TPDO) SetOperational(operational bool) {
	tpdo.mu.Lock()
	defer tpdo.mu.Unlock()
	tpdo.isOperational = operational
	if operational {
		tpdo.restartEventTimer()
	} else {
		// Stop timers
		if tpdo.timerEvent != nil {
			tpdo.timerEvent.Stop()
		}
		if tpdo.timerInhibit != nil {
			tpdo.timerInhibit.Stop()
		}
		tpdo.inhibitActive = false
	}
}

func (tpdo *TPDO) startInhibitTimer() {
	if tpdo.inhibitTimeUs == 0 {
		return
	}
	tpdo.inhibitActive = true
	duration := time.Duration(tpdo.inhibitTimeUs) * time.Microsecond
	if tpdo.timerInhibit == nil {
		tpdo.timerInhibit = time.AfterFunc(duration, tpdo.inhibitHandler)
	} else {
		tpdo.timerInhibit.Reset(duration)
	}
}

func (tpdo *TPDO) inhibitHandler() {
	tpdo.mu.Lock()
	active := tpdo.isOperational
	req := tpdo.sendRequest
	tpdo.inhibitActive = false
	tpdo.mu.Unlock()

	if active && req {
		_ = tpdo.send()
	}
}

func (tpdo *TPDO) restartEventTimer() {
	if tpdo.eventTimeUs == 0 {
		return
	}
	duration := time.Duration(tpdo.eventTimeUs) * time.Microsecond
	if tpdo.timerEvent == nil {
		tpdo.timerEvent = time.AfterFunc(duration, tpdo.eventHandler)
	} else {
		tpdo.timerEvent.Reset(duration)
	}
}

func (tpdo *TPDO) eventHandler() {
	tpdo.mu.Lock()
	tpdo.sendRequest = true
	inhibit := tpdo.inhibitActive
	operational := tpdo.isOperational
	tpdo.mu.Unlock()

	if operational && !inhibit {
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

	tpdo := &TPDO{BusManager: bm}

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
	canId, err := tpdo.configureCOBID(entry18xx, predefinedIdent, erroneousMap)
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
	tpdo.inhibitTimeUs = uint32(inhibitTime) * 100

	// Configure event timer (not mandatory)
	eventTime, err := entry18xx.Uint16(od.SubPdoEventTimer)
	if err != nil {
		tpdo.pdo.logger.Warn("reading event timer failed",
			"index", entry18xx.Index,
			"subindex", od.SubPdoEventTimer,
			"error", err,
		)

	}
	tpdo.eventTimeUs = uint32(eventTime) * 1000

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
