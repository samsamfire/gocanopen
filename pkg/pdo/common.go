package pdo

import (
	"sync"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/emergency"
	"github.com/samsamfire/gocanopen/pkg/od"
	log "github.com/sirupsen/logrus"
)

const (
	MaxPdoLength    uint8 = 8
	BufferCountRpdo uint8 = 2
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
	mu             sync.Mutex
	od             *od.ObjectDictionary
	emcy           *emergency.EMCY
	streamers      [od.MaxMappedEntriesPdo]od.Streamer
	Valid          bool
	dataLength     uint32
	nbMapped       uint8
	flagPDOByte    [od.FlagsPdoSize]*byte
	flagPDOBitmask [od.FlagsPdoSize]byte
	IsRPDO         bool
	predefinedId   uint16
	configuredId   uint16
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
		log.Warnf("[%v] mapped parameter [x%x|x%x] is too long", pdo.Type(), index, subIndex)
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
	entry := pdo.od.Index(index)
	streamerCopy, ret := od.NewStreamer(entry, subIndex, false)
	if ret != nil {
		log.Warnf("[%v] mapping failed [x%x|x%x] : %v", pdo.Type(), index, subIndex, ret)
		return ret
	}

	// Check correct attribute, length, and alignment
	switch {
	case !streamerCopy.HasAttribute(pdo.attribute()):
		log.Warnf("[%v] mapping failed [x%x|x%x] : attribute error", pdo.Type(), index, subIndex)
		return od.ErrNoMap
	case (mappedLengthBits & 0x07) != 0:
		log.Warnf("[%v] mapping failed [x%x|x%x] : alignment error", pdo.Type(), index, subIndex)
		return od.ErrNoMap
	case streamerCopy.DataLength < uint32(mappedLength):
		log.Warnf("[%v] mapping failed [x%x|x%x] : length error", pdo.Type(), index, subIndex)
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
	if uint32(subIndex) < (uint32(od.FlagsPdoSize)*8) && entry.Extension() != nil {
		pdo.flagPDOByte[mapIndex] = entry.FlagPDOByte(subIndex)
		pdo.flagPDOBitmask[mapIndex] = 1 << (subIndex & 0x07)
	} else {
		pdo.flagPDOByte[mapIndex] = nil
	}
	log.Infof("[%v] update mapping successful [x%x|x%x]", pdo.Type(), index, subIndex)
	return nil

}

// Create and initialize a common PDO object
func NewPDO(
	odict *od.ObjectDictionary,
	entry *od.Entry,
	isRPDO bool,
	em *emergency.EMCY,
	erroneoursMap *uint32,
) (*PDOCommon, error) {

	pdo := &PDOCommon{}
	pdo.od = odict
	pdo.emcy = em
	pdo.IsRPDO = isRPDO
	pdoDataLength := uint32(0)

	// Get number of mapped objects
	mappedObjectsCount, ret := entry.Uint8(0)
	if ret != nil {
		log.Errorf("[%v][%x|%x] reading nb mapped objects failed : %v", pdo.Type(), entry.Index, 0, ret)
		return nil, canopen.ErrOdParameters
	}

	// Iterate over all the mapping objects
	for i := range pdo.streamers {
		streamer := &pdo.streamers[i]
		mapParam, ret := entry.Uint32(uint8(i) + 1)
		if ret == od.ErrSubNotExist {
			continue
		}
		if ret != nil {
			log.Errorf("[%v][%x|%x] reading mapped object failed : %v", pdo.Type(), entry.Index, i+1, ret)
			return nil, canopen.ErrOdParameters
		}
		ret = pdo.configureMap(mapParam, uint32(i), isRPDO)
		if ret != nil {
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
