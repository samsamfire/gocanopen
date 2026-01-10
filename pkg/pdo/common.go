package pdo

import (
	"fmt"
	"log/slog"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/emergency"
	"github.com/samsamfire/gocanopen/pkg/od"
)

const (
	MaxPdoLength   uint8 = 8
	MinPdoNumber         = uint16(1)
	MaxRpdoNumber        = uint16(512)
	MaxTpdoNumber        = MaxRpdoNumber
	MaxPdoNumber         = MaxRpdoNumber + MaxTpdoNumber
	IndexFirstRpdo       = 1
	IndexFirstTpdo       = MaxRpdoNumber + 1
)

const (
	TransmissionTypeSyncAcyclic = 0    // synchronous (acyclic)
	TransmissionTypeSync1       = 1    // synchronous (cyclic every sync)
	TransmissionTypeSync240     = 0xF0 // synchronous (cyclic every 240-th sync)
	TransmissionTypeSyncEventLo = 0xFE // event-driven, lower value (manufacturer specific)
	TransmissionTypeSyncEventHi = 0xFF // event-driven, higher value (device profile and application profile specific)
)

// Common to TPDO & RPDO
type PDOCommon struct {
	od           *od.ObjectDictionary
	logger       *slog.Logger
	emcy         *emergency.EMCY
	streamers    [od.MaxMappedEntriesPdo]od.Streamer
	Valid        bool
	dataLength   uint32
	nbMapped     uint8
	IsRPDO       bool
	predefinedId uint16
	configuredId uint16
}

func (base *PDOCommon) attribute() uint8 {
	if base.IsRPDO {
		return od.AttributeRpdo
	}
	return od.AttributeTpdo
}

func (base *PDOCommon) Type() string {
	if base.IsRPDO {
		return "RPDO"
	}
	return "TPDO"
}

// Configure a PDO map (this is done on startup and can also be done dynamically when writing to special objects)
func (pdo *PDOCommon) configureMap(mapParam uint32, mapIndex uint32, isRPDO bool) error {
	index := uint16(mapParam >> 16)
	subIndex := byte(mapParam >> 8)
	mappedLengthBits := byte(mapParam)
	mappedLength := mappedLengthBits >> 3
	streamer := &pdo.streamers[mapIndex]

	// Total PDO length should be smaller than the max possible size
	if mappedLength > MaxPdoLength {
		pdo.logger.Warn("mapped parameter is too long",
			"index", fmt.Sprintf("x%x", index),
			"subindex", fmt.Sprintf("x%x", subIndex),
			"length", mappedLength,
		)
		return od.ErrMapLen
	}
	// Dummy entries map to "fake" entries
	if index < 0x20 && subIndex == 0 {
		streamer.ResetData(uint32(mappedLength), uint32(mappedLength))
		streamer.SetWriter(WriteDummy)
		streamer.SetReader(ReadDummy)
		return nil
	}
	// Get entry in OD
	streamerCopy, err := pdo.od.Streamer(index, subIndex, false)
	if err != nil {
		pdo.logger.Warn("mapping failed",
			"index", fmt.Sprintf("x%x", index),
			"subindex", fmt.Sprintf("x%x", subIndex),
			"error", err,
		)
		return err
	}

	// Check correct attribute, length, and alignment
	switch {
	case !streamerCopy.HasAttribute(pdo.attribute()):
		pdo.logger.Warn("mapping failed : attribute error",
			"index", fmt.Sprintf("x%x", index),
			"subindex", fmt.Sprintf("x%x", subIndex),
		)
		return od.ErrNoMap
	case (mappedLengthBits & 0x07) != 0:
		pdo.logger.Warn("mapping failed : alignment error",
			"index", fmt.Sprintf("x%x", index),
			"subindex", fmt.Sprintf("x%x", subIndex),
		)
		return od.ErrNoMap
	case streamerCopy.DataLength < uint32(mappedLength):
		pdo.logger.Warn("mapping failed : length error",
			"index", fmt.Sprintf("x%x", index),
			"subindex", fmt.Sprintf("x%x", subIndex),
		)
		return od.ErrNoMap
	default:
	}
	streamer.SetStream(streamerCopy.Stream)
	streamer.SetReader(streamerCopy.Reader())
	streamer.SetWriter(streamerCopy.Writer())
	streamer.DataOffset = uint32(mappedLength)

	if isRPDO {
		return nil
	}
	pdo.logger.Info("update mapping successful",
		"index", fmt.Sprintf("x%x", index),
		"subindex", fmt.Sprintf("x%x", subIndex),
	)
	return nil

}

func (pdo *PDOCommon) configureCobId(entry *od.Entry, predefinedIdent uint16, erroneousMap uint32) (canId uint16, err error) {

	cobId, err := entry.Uint32(od.SubPdoCobId)
	if err != nil {
		pdo.logger.Error("reading COB-ID failed",
			"index", fmt.Errorf("x%x", entry.Index),
			"subindex", od.SubPdoCobId,
			"error", err,
		)
		return 0, canopen.ErrOdParameters
	}
	valid := (cobId & CobIdValidBit) == 0
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
	return canId, err
}

// Create and initialize a common PDO object
func NewPDO(
	odict *od.ObjectDictionary,
	logger *slog.Logger,
	entry *od.Entry,
	isRPDO bool,
	em *emergency.EMCY,
	erroneoursMap *uint32,
) (*PDOCommon, error) {

	pdo := &PDOCommon{}
	pdo.od = odict
	pdo.emcy = em
	pdo.IsRPDO = isRPDO

	if logger == nil {
		logger = slog.Default()
	}

	if pdo.IsRPDO {
		pdo.logger = logger.With("service", "RPDO")
	} else {
		pdo.logger = logger.With("service", "TPDO")
	}

	pdoDataLength := uint32(0)

	// Get number of mapped objects
	mappedObjectsCount, err := entry.Uint8(0)
	if err != nil {
		pdo.logger.Error("reading nb mapped objects failed",
			"index", fmt.Sprintf("x%x", entry.Index),
			"subindex", fmt.Sprintf("x%x", 0),
			"error", err,
		)
		return nil, canopen.ErrOdParameters
	}

	// Iterate over all the mapping objects
	for i := range pdo.streamers {
		streamer := &pdo.streamers[i]
		mapParam, err := entry.Uint32(uint8(i) + 1)
		if err == od.ErrSubNotExist {
			continue
		}
		if err != nil {
			pdo.logger.Error("reading mapped objects failed",
				"index", fmt.Sprintf("x%x", entry.Index),
				"subindex", fmt.Sprintf("x%x", i+1),
				"error", err,
			)
			return nil, canopen.ErrOdParameters
		}
		err = pdo.configureMap(mapParam, uint32(i), isRPDO)
		if err != nil {
			// Init failed, but not critical
			streamer.ResetData(0, 0xFF)
			if *erroneoursMap == 0 {
				*erroneoursMap = mapParam
			}
		}
		if i < int(mappedObjectsCount) {
			pdoDataLength += streamer.DataOffset
		}
	}

	if pdoDataLength > uint32(MaxPdoLength) || (pdoDataLength == 0 && mappedObjectsCount > 0) {
		if *erroneoursMap == 0 {
			*erroneoursMap = 1
		}
	}
	if *erroneoursMap == 0 {
		pdo.dataLength = uint32(pdoDataLength)
		pdo.nbMapped = mappedObjectsCount
	}
	return pdo, nil
}
