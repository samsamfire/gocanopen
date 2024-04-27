package pdo

import (
	"sync"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/emergency"
	"github.com/samsamfire/gocanopen/pkg/od"
	log "github.com/sirupsen/logrus"
)

// Common base between TPDOs and RPDOs

const (
	MAX_PDO_LENGTH    uint8 = 8
	RPDO_BUFFER_COUNT uint8 = 2
)

const (
	TRANSMISSION_TYPE_SYNC_ACYCLIC  = 0    // synchronous (acyclic)
	TRANSMISSION_TYPE_SYNC_1        = 1    // synchronous (cyclic every sync)
	TRANSMISSION_TYPE_SYNC_240      = 0xF0 // synchronous (cyclic every 240-th sync)
	TRANSMISSION_TYPE_SYNC_EVENT_LO = 0xFE // event-driven, lower value (manufacturer specific)
	TRANSMISSION_TYPE_SYNC_EVENT_HI = 0xFF // event-driven, higher value (device profile and application profile specific)
)

// Common to TPDO & RPDO
type PDOCommon struct {
	mu             sync.Mutex
	od             *od.ObjectDictionary
	emcy           *emergency.EMCY
	streamers      [od.PDO_MAX_MAPPED_ENTRIES]od.Streamer
	Valid          bool
	dataLength     uint32
	nbMapped       uint8
	flagPDOByte    [od.OD_FLAGS_PDO_SIZE]*byte
	flagPDOBitmask [od.OD_FLAGS_PDO_SIZE]byte
	IsRPDO         bool
	predefinedId   uint16
	configuredId   uint16
}

func (base *PDOCommon) attribute() uint8 {
	if base.IsRPDO {
		return od.ATTRIBUTE_RPDO
	} else {
		return od.ATTRIBUTE_TPDO
	}
}

func (base *PDOCommon) Type() string {
	if base.IsRPDO {
		return "RPDO"
	} else {
		return "TPDO"
	}
}

// Configure a PDO map (this is done on startup and can also be done dynamically when writing to special objects)
func (pdo *PDOCommon) configureMap(mapParam uint32, mapIndex uint32, isRPDO bool) error {
	index := uint16(mapParam >> 16)
	subIndex := byte(mapParam >> 8)
	mappedLengthBits := byte(mapParam)
	mappedLength := mappedLengthBits >> 3
	streamer := &pdo.streamers[mapIndex]

	// Total PDO length should be smaller than the max possible size
	if mappedLength > MAX_PDO_LENGTH {
		log.Warnf("[%v] mapped parameter [x%x|x%x] is too long", pdo.Type(), index, subIndex)
		return od.ODR_MAP_LEN
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
		return od.ODR_NO_MAP
	case (mappedLengthBits & 0x07) != 0:
		log.Warnf("[%v] mapping failed [x%x|x%x] : alignment error", pdo.Type(), index, subIndex)
		return od.ODR_NO_MAP
	case streamerCopy.DataLength < uint32(mappedLength):
		log.Warnf("[%v] mapping failed [x%x|x%x] : length error", pdo.Type(), index, subIndex)
		return od.ODR_NO_MAP
	default:
	}
	streamer.SetStream(streamerCopy.Stream)
	streamer.SetReader(streamerCopy.Reader())
	streamer.SetWriter(streamerCopy.Writer())
	streamer.DataOffset = uint32(mappedLength)

	if isRPDO {
		return nil
	}
	if uint32(subIndex) < (uint32(od.OD_FLAGS_PDO_SIZE)*8) && entry.Extension() != nil {
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
		if ret == od.ODR_SUB_NOT_EXIST {
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

	if pdoDataLength > uint32(MAX_PDO_LENGTH) || (pdoDataLength == 0 && mappedObjectsCount > 0) {
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
