package config

import (
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/samsamfire/gocanopen/pkg/sdo"
	log "github.com/sirupsen/logrus"
)

type PDOMappingParameter struct {
	Index      uint16
	Subindex   uint8
	LengthBits uint8
}

// Holds a PDO configuration
type PDOConfigurationParameter struct {
	CanId            uint16
	TransmissionType uint8
	InhibitTime      uint16
	EventTimer       uint16
	Mappings         []PDOMappingParameter
}

type PDOConfig struct {
	*sdo.SDOClient
	nodeId      uint8  // Node id being controlled
	indexOffset uint16 // 0 for RPDO, 0x200 for TPDO
	isRPDO      bool   // Useful for logging
}

// PDO configurator is used for configurating node PDOs
// It has helper functions for accessing the common PDO mandatory objects
func NewRPDOConfigurator(nodeId uint8, sdoClient *sdo.SDOClient) *PDOConfig {
	return &PDOConfig{nodeId: nodeId, SDOClient: sdoClient, indexOffset: 0, isRPDO: true}
}

func NewTPDOConfigurator(nodeId uint8, sdoClient *sdo.SDOClient) *PDOConfig {
	return &PDOConfig{nodeId: nodeId, SDOClient: sdoClient, indexOffset: 0x400, isRPDO: false}
}

func (conf *PDOConfig) getType() string {
	if conf.isRPDO {
		return "RPDO"
	}
	return "TPDO"
}

func (conf *PDOConfig) getMappingIndex(pdoNb uint16) uint16 {
	return conf.indexOffset + od.BASE_RPDO_MAPPING_INDEX + pdoNb - 1
}

func (conf *PDOConfig) getCommunicationIndex(pdoNb uint16) uint16 {
	return conf.indexOffset + od.BASE_RPDO_COMMUNICATION_INDEX + pdoNb - 1
}

func (config *PDOConfig) ReadCobId(pdoNb uint16) (uint32, error) {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	return config.ReadUint32(config.nodeId, pdoCommIndex, 1)
}

func (config *PDOConfig) ReadEnabled(pdoNb uint16) (bool, error) {
	cobId, err := config.ReadCobId(pdoNb)
	if err != nil {
		return false, err
	}
	return (cobId>>31)&0b1 == 0, nil
}

func (config *PDOConfig) ReadTransmissionType(pdoNb uint16) (uint8, error) {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	return config.ReadUint8(config.nodeId, pdoCommIndex, 2)
}

func (config *PDOConfig) ReadInhibitTime(pdoNb uint16) (uint16, error) {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	return config.ReadUint16(config.nodeId, pdoCommIndex, 3)
}

func (config *PDOConfig) ReadEventTimer(pdoNb uint16) (uint16, error) {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	return config.ReadUint16(config.nodeId, pdoCommIndex, 5)
}

func (config *PDOConfig) ReadNbMappings(pdoNb uint16) (uint8, error) {
	pdoMappingIndex := config.getMappingIndex(pdoNb)
	return config.ReadUint8(config.nodeId, pdoMappingIndex, 0)
}

func (config *PDOConfig) ReadMappings(pdoNb uint16) ([]PDOMappingParameter, error) {
	pdoMappingIndex := config.getMappingIndex(pdoNb)
	mappings := make([]PDOMappingParameter, 0)
	nbMappings, err := config.ReadNbMappings(pdoNb)
	if err != nil {
		return nil, err
	}
	for i := uint8(0); i < nbMappings; i++ {
		rawMap, err := config.ReadUint32(config.nodeId, pdoMappingIndex, uint8(i)+1)
		if err != nil {
			return nil, err
		}
		mapping := PDOMappingParameter{}
		mapping.LengthBits = uint8(rawMap)
		mapping.Subindex = uint8(rawMap >> 8)
		mapping.Index = uint16(rawMap >> 16)
		mappings = append(mappings, mapping)
	}
	return mappings, nil
}

// Reads complete PDO configuration
func (config *PDOConfig) ReadConfiguration(pdoNb uint16) (PDOConfigurationParameter, error) {
	conf := PDOConfigurationParameter{}
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
func (config *PDOConfig) Disable(pdoNb uint16) error {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	cobId, err := config.ReadCobId(pdoNb)
	if err != nil {
		return err
	}
	cobId |= (1 << 31)
	return config.WriteRaw(config.nodeId, pdoCommIndex, 1, cobId, false)
}

// Enable PDO
func (config *PDOConfig) Enable(pdoNb uint16) error {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	cobId, err := config.ReadCobId(pdoNb)
	if err != nil {
		return err
	}
	mask := ^(uint32(1) << 31)
	cobId &= mask
	return config.WriteRaw(config.nodeId, pdoCommIndex, 1, cobId, false)
}

func (config *PDOConfig) WriteCanId(pdoNb uint16, canId uint16) error {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	cobId, err := config.ReadCobId(pdoNb)
	if err != nil {
		return err
	}
	cobId &= 0xFFFFF800 // clear cobid bits
	cobId |= uint32(canId)
	return config.WriteRaw(config.nodeId, pdoCommIndex, 1, cobId, false)
}

func (config *PDOConfig) WriteTransmissionType(pdoNb uint16, transType uint8) error {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	return config.WriteRaw(config.nodeId, pdoCommIndex, 2, transType, false)
}

func (config *PDOConfig) WriteInhibitTime(pdoNb uint16, inhibitTime uint16) error {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	return config.WriteRaw(config.nodeId, pdoCommIndex, 3, inhibitTime, false)
}

func (config *PDOConfig) WriteEventTimer(pdoNb uint16, eventTimer uint16) error {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	return config.WriteRaw(config.nodeId, pdoCommIndex, 5, eventTimer, false)
}

// Clear all the PDO mappings
// Technically clearing the actual map entries is not necessary but I find it cleaner
func (config *PDOConfig) ClearMappings(pdoNb uint16) error {
	pdoMappingIndex := config.getMappingIndex(pdoNb)
	// First clear nb of mapped entries
	err := config.WriteRaw(config.nodeId, pdoMappingIndex, 0, uint8(0), false)
	if err != nil {
		return err
	}
	// Then clear entries
	for i := uint8(1); i <= od.PDO_MAX_MAPPED_ENTRIES; i++ {
		err := config.WriteRaw(config.nodeId, pdoMappingIndex, i, uint32(0), false)
		if err != nil {
			return err
		}
	}
	return nil
}

// Write new PDO mapping
// Takes a list of objects to map and will fill them up in the given order
// This will first clear the current mapping
func (config *PDOConfig) WriteMappings(pdoNb uint16, mappings []PDOMappingParameter) error {
	pdoMappingIndex := config.getMappingIndex(pdoNb)
	err := config.ClearMappings(pdoNb)
	if err != nil {
		return err
	}
	// Update with new mapping
	for sub, mapping := range mappings {
		rawMap := uint32(mapping.Index)<<16 + uint32(mapping.Subindex)<<8 + uint32(mapping.LengthBits)
		err := config.WriteRaw(config.nodeId, pdoMappingIndex, uint8(sub)+1, rawMap, false)
		if err != nil {
			return err
		}
	}
	// Update number of mapped objects
	return config.WriteRaw(config.nodeId, pdoMappingIndex, 0, uint8(len(mappings)), false)
}

// Update hole configuration
func (config *PDOConfig) WriteConfiguration(pdoNb uint16, conf PDOConfigurationParameter) error {
	log.Debugf("[CONFIGURATOR][%s%v] updating configuration : %+v", config.getType(), pdoNb, conf)
	err := config.WriteCanId(pdoNb, conf.CanId)
	if err != nil {
		return err
	}
	err = config.WriteTransmissionType(pdoNb, conf.TransmissionType)
	if err != nil {
		return err
	}
	err = config.WriteEventTimer(pdoNb, conf.EventTimer)
	if err != nil {
		return err
	}
	err = config.WriteInhibitTime(pdoNb, conf.InhibitTime)
	if err != nil {
		return err
	}
	return config.WriteMappings(pdoNb, conf.Mappings)
}
