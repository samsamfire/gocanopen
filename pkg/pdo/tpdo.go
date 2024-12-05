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
	inhibitTimer     uint32
	eventTimer       uint32
}

// Process [TPDO] state machine and TX CAN frames
// This should be called periodically
func (tpdo *TPDO) Process(timeDifferenceUs uint32, timerNextUs *uint32, nmtIsOperational bool, syncWas bool) error {
	tpdo.mu.Lock()

	pdo := tpdo.pdo
	if !pdo.Valid || !nmtIsOperational {
		tpdo.sendRequest = true
		tpdo.inhibitTimer = 0
		tpdo.eventTimer = 0
		tpdo.syncCounter = 255
		tpdo.mu.Unlock()
		return nil
	}

	if tpdo.transmissionType == TransmissionTypeSyncAcyclic || tpdo.transmissionType >= TransmissionTypeSyncEventLo {
		if tpdo.eventTimeUs != 0 {
			if tpdo.eventTimer > timeDifferenceUs {
				tpdo.eventTimer -= timeDifferenceUs
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
			for i := range pdo.nbMapped {
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
	if tpdo.transmissionType >= TransmissionTypeSyncEventLo {
		if tpdo.inhibitTimer > timeDifferenceUs {
			tpdo.inhibitTimer -= timeDifferenceUs
		} else {
			tpdo.inhibitTimer = 0
		}
		if tpdo.sendRequest && tpdo.inhibitTimer == 0 {
			tpdo.mu.Unlock()
			_ = tpdo.send()
			tpdo.mu.Lock()
		}
		if tpdo.sendRequest && timerNextUs != nil && *timerNextUs > tpdo.inhibitTimer {
			*timerNextUs = tpdo.inhibitTimer
		}
	} else if tpdo.sync != nil && syncWas {

		// Send synchronous acyclic tpdo
		if tpdo.transmissionType == TransmissionTypeSyncAcyclic &&
			tpdo.sendRequest {
			tpdo.mu.Unlock()
			return tpdo.send()
		}
		// Send synchronous cyclic TPDOs
		if tpdo.syncCounter == 255 {
			if tpdo.sync.CounterOverflow() != 0 && tpdo.syncStartValue != 0 {
				// Sync start value used
				tpdo.syncCounter = 254
			} else {
				tpdo.syncCounter = tpdo.transmissionType/2 + 1
			}
		}
		// If sync start value is used , start first TPDO
		// after sync with matched syncstartvalue
		switch tpdo.syncCounter {
		case 254:
			if tpdo.sync.Counter() == tpdo.syncStartValue {
				tpdo.syncCounter = tpdo.transmissionType
				tpdo.mu.Unlock()
				return tpdo.send()
			}
		case 1:
			tpdo.syncCounter = tpdo.transmissionType
			tpdo.mu.Unlock()
			return tpdo.send()

		default:
			tpdo.syncCounter--
		}

	}
	tpdo.mu.Unlock()
	return nil
}

func (tpdo *TPDO) configureTransmissionType(entry18xx *od.Entry) error {
	tpdo.mu.Lock()
	defer tpdo.mu.Unlock()

	transmissionType, err := entry18xx.Uint8(2)
	if err != nil {
		tpdo.pdo.logger.Error("reading failed",
			"index", fmt.Errorf("x%x", entry18xx.Index),
			"subindex", 2,
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
	cobId, err := entry18xx.Uint32(1)
	if err != nil {
		tpdo.pdo.logger.Error("reading failed",
			"index", fmt.Errorf("x%x", entry18xx.Index),
			"subindex", 1,
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
	eventDriven := tpdo.transmissionType == TransmissionTypeSyncAcyclic || tpdo.transmissionType >= uint8(TransmissionTypeSyncEventLo)

	totalNbRead := 0
	var err error

	for i := range pdo.nbMapped {
		streamer := &pdo.streamers[i]
		mappedLength := streamer.DataOffset
		dataLength := int(streamer.DataLength)
		if dataLength > int(MaxPdoLength) {
			dataLength = int(MaxPdoLength)
		}

		streamer.DataOffset = 0
		_, err = streamer.Read(tpdo.txBuffer.Data[totalNbRead:])
		if err != nil {
			tpdo.pdo.logger.Warn("failed to send", "cobId", pdo.configuredId, "error", err)
			return err
		}
		streamer.DataOffset = mappedLength
		// Add to tpdo frame only up to mapped length
		flagPDOByte := pdo.flagPDOByte[i]
		if flagPDOByte != nil && eventDriven {
			*flagPDOByte |= pdo.flagPDOBitmask[i]
		}
		totalNbRead += int(mappedLength)
	}
	tpdo.sendRequest = false
	tpdo.eventTimer = tpdo.eventTimeUs
	tpdo.inhibitTimer = tpdo.inhibitTimeUs
	return tpdo.Send(tpdo.txBuffer)
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
	inhibitTime, err := entry18xx.Uint16(3)
	if err != nil {
		tpdo.pdo.logger.Warn("reading inhibit time failed",
			"index", fmt.Sprintf("x%x", entry18xx.Index),
			"subindex", 3,
			"error", err,
		)
	}
	tpdo.inhibitTimeUs = uint32(inhibitTime) * 100

	// Configure event timer (not mandatory)
	eventTime, err := entry18xx.Uint16(5)
	if err != nil {
		tpdo.pdo.logger.Warn("reading event timer failed",
			"index", entry18xx.Index,
			"subindex", 5,
			"error", err,
		)

	}
	tpdo.eventTimeUs = uint32(eventTime) * 1000

	// Configure sync start value (not mandatory)
	tpdo.syncStartValue, err = entry18xx.Uint8(6)
	if err != nil {
		tpdo.pdo.logger.Warn("reading sync start failed",
			"index", entry18xx.Index,
			"subindex", 6,
			"error", err,
		)
	}
	tpdo.sync = sync
	tpdo.syncCounter = 255

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
	return tpdo, nil
}
