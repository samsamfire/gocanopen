package canopen

import (
	log "github.com/sirupsen/logrus"
)

/*
Common parts between RPDOs and TPDOs
*/

const (
	MAX_PDO_LENGTH     uint8 = 8
	MAX_MAPPED_ENTRIES uint8 = 8
	OD_FLAGS_PDO_SIZE  uint8 = 32
	RPDO_BUFFER_COUNT  uint8 = 2
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
	od                          *ObjectDictionary
	em                          *EM
	busManager                  *BusManager
	Valid                       bool
	DataLength                  uint32
	MappedObjectsCount          uint8
	Streamers                   [MAX_MAPPED_ENTRIES]ObjectStreamer
	FlagPDOByte                 [OD_FLAGS_PDO_SIZE]*byte
	FlagPDOBitmask              [OD_FLAGS_PDO_SIZE]byte
	IsRPDO                      bool
	PreDefinedIdent             uint16
	ConfiguredIdent             uint16
	ExtensionMappingParam       *Extension
	ExtensionCommunicationParam *Extension
	BufferIdx                   int
}

func (base *PDOCommon) attribute() ODA {
	if base.IsRPDO {
		return ODA_RPDO
	} else {
		return ODA_TRPDO
	}
}

func (base *PDOCommon) Type() string {
	if base.IsRPDO {
		return "RPDO"
	} else {
		return "TPDO"
	}
}

// Configure a PDO map (this is done on startup and can also be done dynamically)
func (pdo *PDOCommon) ConfigureMap(od *ObjectDictionary, mapParam uint32, mapIndex uint32, isRPDO bool) error {
	index := uint16(mapParam >> 16)
	subindex := byte(mapParam >> 8)
	mappedLengthBits := byte(mapParam)
	mappedLength := mappedLengthBits >> 3
	streamer := &pdo.Streamers[mapIndex]

	// Total PDO length should be smaller than the max possible size
	if mappedLength > MAX_PDO_LENGTH {
		log.Warnf("[%v][%x|%x] mapped parameter is too long", pdo.Type(), index, subindex)
		return ODR_MAP_LEN
	}
	// Dummy entries map to "fake" entries
	if index < 0x20 && subindex == 0 {
		streamer.stream.Data = make([]byte, mappedLength)
		streamer.stream.DataOffset = uint32(mappedLength)
		streamer.write = WriteDummy
		streamer.read = ReadDummy
		return nil
	}
	// Get entry in OD
	entry := od.Index(index)
	streamerCopy, ret := entry.CreateStreamer(subindex, false)
	if ret != nil {
		log.Warnf("[%v][%x|%x] mapping failed : %v", pdo.Type(), index, subindex, ret)
		return ret
	}

	// Check correct attribute, length, and alignment
	switch {
	case streamerCopy.stream.Attribute&pdo.attribute() == 0:
		log.Warnf("[%v][%x|%x] mapping failed : attribute error", pdo.Type(), index, subindex)
		return ODR_NO_MAP
	case (mappedLengthBits & 0x07) != 0:
		log.Warnf("[%v][%x|%x] mapping failed : alignment error", pdo.Type(), index, subindex)
		return ODR_NO_MAP
	case streamerCopy.stream.DataLength < uint32(mappedLength):
		log.Warnf("[%v][%x|%x] mapping failed : length error", pdo.Type(), index, subindex)
		return ODR_NO_MAP
	default:
	}

	streamer = streamerCopy
	streamer.stream.DataOffset = uint32(mappedLength)

	if isRPDO {
		return nil
	}
	if uint32(subindex) < (uint32(OD_FLAGS_PDO_SIZE)*8) && entry.Extension != nil {
		pdo.FlagPDOByte[mapIndex] = &entry.Extension.flagsPDO[subindex>>3]
		pdo.FlagPDOBitmask[mapIndex] = 1 << (subindex & 0x07)
	} else {
		pdo.FlagPDOByte[mapIndex] = nil
	}
	return nil

}

// Initialize mapping objects for PDOs, this is done once on startup
func (pdo *PDOCommon) InitMapping(od *ObjectDictionary, entry *Entry, isRPDO bool, erroneoursMap *uint32) error {
	pdoDataLength := uint32(0)
	mappedObjectsCount := uint8(0)

	// Get number of mapped objects
	ret := entry.GetUint8(0, &mappedObjectsCount)
	if ret != nil {
		log.Errorf("[%v][%x|%x] reading nb mapped objects failed : %v", pdo.Type(), entry.Index, 0, ret)
		return ErrOdParameters
	}

	// Iterate over all the mapping objects
	for i := range pdo.Streamers {
		streamer := &pdo.Streamers[i]
		mapParam := uint32(0)
		ret := entry.GetUint32(uint8(i)+1, &mapParam)
		if ret == ODR_SUB_NOT_EXIST {
			continue
		}
		if ret != nil {
			log.Errorf("[%v][%x|%x] reading mapped object failed : %v", pdo.Type(), entry.Index, i+1, ret)
			return ErrOdParameters
		}
		ret = pdo.ConfigureMap(od, mapParam, uint32(i), isRPDO)
		if ret != nil {
			// Init failed, but not critical
			streamer.stream.Data = make([]byte, 0)
			streamer.stream.DataOffset = 0xFF
			if *erroneoursMap == 0 {
				*erroneoursMap = mapParam
			}
		}
		if i < int(mappedObjectsCount) {
			pdoDataLength += streamer.stream.DataOffset
		}
	}

	if pdoDataLength > uint32(MAX_PDO_LENGTH) || (pdoDataLength == 0 && mappedObjectsCount > 0) {
		if *erroneoursMap == 0 {
			*erroneoursMap = 1
		}
	}
	if *erroneoursMap == 0 {
		pdo.DataLength = uint32(pdoDataLength)
		pdo.MappedObjectsCount = mappedObjectsCount
	}
	return nil
}
