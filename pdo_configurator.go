package canopen

import log "github.com/sirupsen/logrus"

const BASE_RPDO_COMMUNICATION_INDEX = uint16(0x1400)
const BASE_RPDO_MAPPING_INDEX = uint16(0x1600)
const BASE_TPDO_COMMUNICATION_INDEX = uint16(0x1800)
const BASE_TPDO_MAPPING_INDEX = uint16(0x1A00)

type PDOMapping struct {
	Index      uint16
	Subindex   uint8
	LengthBits uint8
}

// Holds a PDO configuration
type PDOConfiguration struct {
	CanId            uint16
	TransmissionType uint8
	InhibitTime      uint16
	EventTimer       uint16
	Mappings         []PDOMapping
}

type PDOConfigurator struct {
	nodeId      uint8      // Node id being controlled
	sdoClient   *SDOClient // Standard sdo client
	indexOffset uint16     // 0 for RPDO, 0x200 for TPDO
	isRPDO      bool       // Useful for logging
}

// PDO configurator is used for configurating node PDOs
// It has helper functions for accessing the common PDO mandatory objects
func NewRPDOConfigurator(nodeId uint8, sdoClient *SDOClient) *PDOConfigurator {
	return &PDOConfigurator{nodeId: nodeId, sdoClient: sdoClient, indexOffset: 0, isRPDO: true}
}

func NewTPDOConfigurator(nodeId uint8, sdoClient *SDOClient) *PDOConfigurator {
	return &PDOConfigurator{nodeId: nodeId, sdoClient: sdoClient, indexOffset: 0x400, isRPDO: false}
}

func (conf *PDOConfigurator) getType() string {
	if conf.isRPDO {
		return "RPDO"
	}
	return "TPDO"
}

func (conf *PDOConfigurator) getMappingIndex(pdoNb uint16) uint16 {
	return conf.indexOffset + BASE_RPDO_MAPPING_INDEX + pdoNb - 1
}

func (conf *PDOConfigurator) getCommunicationIndex(pdoNb uint16) uint16 {
	return conf.indexOffset + BASE_RPDO_COMMUNICATION_INDEX + pdoNb - 1
}

func (config *PDOConfigurator) ReadCobId(pdoNb uint16) (uint32, error) {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	return config.sdoClient.ReadUint32(config.nodeId, pdoCommIndex, 1)
}

func (config *PDOConfigurator) ReadEnabled(pdoNb uint16) (bool, error) {
	cobId, err := config.ReadCobId(pdoNb)
	if err != nil {
		return false, err
	}
	return (cobId>>31)&0b1 == 0, nil
}

func (config *PDOConfigurator) ReadTransmissionType(pdoNb uint16) (uint8, error) {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	return config.sdoClient.ReadUint8(config.nodeId, pdoCommIndex, 2)
}

func (config *PDOConfigurator) ReadInhibitTime(pdoNb uint16) (uint16, error) {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	return config.sdoClient.ReadUint16(config.nodeId, pdoCommIndex, 3)
}

func (config *PDOConfigurator) ReadEventTimer(pdoNb uint16) (uint16, error) {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	return config.sdoClient.ReadUint16(config.nodeId, pdoCommIndex, 5)
}

func (config *PDOConfigurator) ReadNbMappings(pdoNb uint16) (uint8, error) {
	pdoMappingIndex := config.getMappingIndex(pdoNb)
	return config.sdoClient.ReadUint8(config.nodeId, pdoMappingIndex, 0)
}

func (config *PDOConfigurator) ReadMappings(pdoNb uint16) ([]PDOMapping, error) {
	pdoMappingIndex := config.getMappingIndex(pdoNb)
	mappings := make([]PDOMapping, 0)
	nbMappings, err := config.ReadNbMappings(pdoNb)
	if err != nil {
		return nil, err
	}
	for i := uint8(0); i < nbMappings; i++ {
		rawMap, err := config.sdoClient.ReadUint32(config.nodeId, pdoMappingIndex, uint8(i)+1)
		if err != nil {
			return nil, err
		}
		mapping := PDOMapping{}
		mapping.LengthBits = uint8(rawMap)
		mapping.Subindex = uint8(rawMap >> 8)
		mapping.Index = uint16(rawMap >> 16)
		mappings = append(mappings, mapping)
	}
	return mappings, nil
}

// Reads complete PDO configuration
func (config *PDOConfigurator) ReadConfiguration(pdoNb uint16) (PDOConfiguration, error) {
	conf := PDOConfiguration{}
	var err error
	cobId, err := config.ReadCobId(pdoNb)
	if err != nil {
		return conf, err
	}
	conf.CanId = uint16(cobId & 0x7FF)
	conf.TransmissionType, err = config.ReadTransmissionType(pdoNb)
	if err != nil {
		return conf, err
	}
	// Optional
	conf.InhibitTime, _ = config.ReadInhibitTime(pdoNb)
	// Optional
	conf.EventTimer, _ = config.ReadEventTimer(pdoNb)
	conf.Mappings, err = config.ReadMappings(pdoNb)
	log.Debugf("[CONFIGURATOR][%s%v] read configuration : %+v", config.getType(), pdoNb, conf)
	return conf, err
}

// Disable PDO
func (config *PDOConfigurator) Disable(pdoNb uint16) error {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	cobId, err := config.ReadCobId(pdoNb)
	if err != nil {
		return err
	}
	cobId |= (1 << 31)
	return config.sdoClient.WriteRaw(config.nodeId, pdoCommIndex, 1, cobId, false)
}

// Enable PDO
func (config *PDOConfigurator) Enable(pdoNb uint16) error {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	cobId, err := config.ReadCobId(pdoNb)
	if err != nil {
		return err
	}
	mask := ^(uint32(1) << 31)
	cobId &= mask
	return config.sdoClient.WriteRaw(config.nodeId, pdoCommIndex, 1, cobId, false)
}

func (config *PDOConfigurator) WriteCanId(pdoNb uint16, canId uint16) error {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	cobId, err := config.ReadCobId(pdoNb)
	if err != nil {
		return err
	}
	cobId &= 0xFFFFF800 // clear cobid bits
	cobId |= uint32(canId)
	return config.sdoClient.WriteRaw(config.nodeId, pdoCommIndex, 1, cobId, false)
}

func (config *PDOConfigurator) WriteTransmissionType(pdoNb uint16, transType uint8) error {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	return config.sdoClient.WriteRaw(config.nodeId, pdoCommIndex, 2, transType, false)
}

func (config *PDOConfigurator) WriteInhibitTime(pdoNb uint16, inhibitTime uint16) error {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	return config.sdoClient.WriteRaw(config.nodeId, pdoCommIndex, 3, inhibitTime, false)
}

func (config *PDOConfigurator) WriteEventTimer(pdoNb uint16, eventTimer uint16) error {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	return config.sdoClient.WriteRaw(config.nodeId, pdoCommIndex, 5, eventTimer, false)
}

// Clear all the PDO mappings
// Technically clearing the actual map entries is not necessary but I find it cleaner
func (config *PDOConfigurator) ClearMappings(pdoNb uint16) error {
	pdoMappingIndex := config.getMappingIndex(pdoNb)
	// First clear nb of mapped entries
	err := config.sdoClient.WriteRaw(config.nodeId, pdoMappingIndex, 0, uint8(0), false)
	if err != nil {
		return err
	}
	// Then clear entries
	for i := uint8(1); i <= MAX_MAPPED_ENTRIES; i++ {
		err := config.sdoClient.WriteRaw(config.nodeId, pdoMappingIndex, i, uint32(0), false)
		if err != nil {
			return err
		}
	}
	return nil
}

// Write new PDO mapping
// Takes a list of objects to map and will fill them up in the given order
// This will first clear the current mapping
func (config *PDOConfigurator) WriteMappings(pdoNb uint16, mappings []PDOMapping) error {
	pdoMappingIndex := config.getMappingIndex(pdoNb)
	err := config.ClearMappings(pdoNb)
	if err != nil {
		return err
	}
	// Update with new mapping
	for sub, mapping := range mappings {
		rawMap := uint32(mapping.Index)<<16 + uint32(mapping.Subindex)<<8 + uint32(mapping.LengthBits)
		err := config.sdoClient.WriteRaw(config.nodeId, pdoMappingIndex, uint8(sub)+1, rawMap, false)
		if err != nil {
			return err
		}
	}
	// Update number of mapped objects
	return config.sdoClient.WriteRaw(config.nodeId, pdoMappingIndex, 0, uint8(len(mappings)), false)
}

// Update hole configuration
func (config *PDOConfigurator) WriteConfiguration(pdoNb uint16, conf PDOConfiguration) error {
	log.Debugf("[CONFIGURATOR][%s%v] updating configuration : %+v", config.getType(), pdoNb, conf)
	err := config.WriteCanId(pdoNb, conf.CanId)
	if err != nil {
		return err
	}
	err = config.WriteTransmissionType(pdoNb, conf.TransmissionType)
	if err != nil {
		return err
	}
	config.WriteEventTimer(pdoNb, conf.EventTimer)
	config.WriteInhibitTime(pdoNb, conf.InhibitTime)
	return config.WriteMappings(pdoNb, conf.Mappings)
}
