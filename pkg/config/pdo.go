package config

import (
	"errors"

	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/samsamfire/gocanopen/pkg/pdo"
	"github.com/samsamfire/gocanopen/pkg/sdo"
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

func (conf *NodeConfigurator) getType(pdoNb uint16) string {
	if pdoNb <= 256 {
		return "RPDO"
	}
	return "TPDO"
}

func (conf *NodeConfigurator) getMappingIndex(pdoNb uint16) uint16 {
	if pdoNb <= pdo.MaxRpdoNumber {
		return od.EntryRPDOMappingStart + pdoNb - 1
	}
	return od.EntryTPDOMappingStart + pdoNb - pdo.MaxRpdoNumber - 1

}

func (conf *NodeConfigurator) getCommunicationIndex(pdoNb uint16) uint16 {
	if pdoNb <= pdo.MaxRpdoNumber {
		return od.EntryRPDOCommunicationStart + pdoNb - 1
	}
	return od.EntryTPDOCommunicationStart + pdoNb - pdo.MaxRpdoNumber - 1
}

func (config *NodeConfigurator) ReadCobIdPDO(pdoNb uint16) (uint32, error) {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	return config.client.ReadUint32(config.nodeId, pdoCommIndex, 1)
}

func (config *NodeConfigurator) ReadEnabledPDO(pdoNb uint16) (bool, error) {
	cobId, err := config.ReadCobIdPDO(pdoNb)
	if err != nil {
		return false, err
	}
	return (cobId>>31)&0b1 == 0, nil
}

func (config *NodeConfigurator) ReadTransmissionType(pdoNb uint16) (uint8, error) {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	return config.client.ReadUint8(config.nodeId, pdoCommIndex, 2)
}

func (config *NodeConfigurator) ReadInhibitTime(pdoNb uint16) (uint16, error) {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	return config.client.ReadUint16(config.nodeId, pdoCommIndex, 3)
}

func (config *NodeConfigurator) ReadEventTimer(pdoNb uint16) (uint16, error) {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	return config.client.ReadUint16(config.nodeId, pdoCommIndex, 5)
}

func (config *NodeConfigurator) ReadNbMappings(pdoNb uint16) (uint8, error) {
	pdoMappingIndex := config.getMappingIndex(pdoNb)
	return config.client.ReadUint8(config.nodeId, pdoMappingIndex, 0)
}

func (config *NodeConfigurator) ReadMappings(pdoNb uint16) ([]PDOMappingParameter, error) {
	pdoMappingIndex := config.getMappingIndex(pdoNb)
	mappings := make([]PDOMappingParameter, 0)
	nbMappings, err := config.ReadNbMappings(pdoNb)
	if err != nil {
		return nil, err
	}
	for i := range nbMappings {
		rawMap, err := config.client.ReadUint32(config.nodeId, pdoMappingIndex, i+1)
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

// Reads configuration of a single PDO
func (config *NodeConfigurator) ReadConfigurationPDO(pdoNb uint16) (PDOConfigurationParameter, error) {
	conf := PDOConfigurationParameter{}
	var err error
	cobId, err := config.ReadCobIdPDO(pdoNb)
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
	config.logger.Debug("read configuration",
		"type", config.getType(pdoNb),
		"pdoNb", pdoNb,
		"conf", conf,
	)
	return conf, err
}

// Reads configuration of a range of PDOs
func (config *NodeConfigurator) ReadConfigurationRangePDO(
	pdoStartNb uint16, pdoEndNb uint16,
) ([]PDOConfigurationParameter, error) {

	if pdoStartNb < pdo.MinPdoNumber || pdoEndNb > pdo.MaxPdoNumber {
		return nil, errors.New("pdo number or length is incorrect")
	}
	pdos := make([]PDOConfigurationParameter, 0)
	for pdoNb := pdoStartNb; pdoNb <= pdoEndNb; pdoNb++ {
		conf, err := config.ReadConfigurationPDO(pdoNb)
		if err != nil && err == sdo.AbortNotExist {
			config.logger.Debug("no more pdo",
				"type", config.getType(pdoNb),
				"pdoNb", pdoNb,
			)
			break
		} else if err != nil {
			config.logger.Error("failed to read configuration",
				"type", config.getType(pdoNb),
				"pdoNb", pdoNb,
				"error", err,
			)
			return pdos, err
		}
		pdos = append(pdos, conf)
	}
	return pdos, nil

}

// Reads complete PDO configuration (RPDO, TPDO)
// Returns RPDOs and TPDOs configurations in two seperate lists
func (config *NodeConfigurator) ReadConfigurationAllPDO() (
	rpdos []PDOConfigurationParameter, tpdos []PDOConfigurationParameter, err error,
) {
	// Read all RPDOs
	rpdos, err = config.ReadConfigurationRangePDO(pdo.MinRpdoNumber, pdo.MaxRpdoNumber)
	if err != nil {
		return rpdos, tpdos, err
	}
	// Read all TPDOs
	tpdos, err = config.ReadConfigurationRangePDO(pdo.MinTpdoNumber, pdo.MaxTpdoNumber)
	return rpdos, tpdos, err
}

// Disable PDO
func (config *NodeConfigurator) DisablePDO(pdoNb uint16) error {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	cobId, err := config.ReadCobIdPDO(pdoNb)
	if err != nil {
		return err
	}
	cobId |= (1 << 31)
	return config.client.WriteRaw(config.nodeId, pdoCommIndex, 1, cobId, false)
}

// Enable PDO
func (config *NodeConfigurator) EnablePDO(pdoNb uint16) error {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	cobId, err := config.ReadCobIdPDO(pdoNb)
	if err != nil {
		return err
	}
	mask := ^(uint32(1) << 31)
	cobId &= mask
	return config.client.WriteRaw(config.nodeId, pdoCommIndex, 1, cobId, false)
}

func (config *NodeConfigurator) WriteCanIdPDO(pdoNb uint16, canId uint16) error {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	cobId, err := config.ReadCobIdPDO(pdoNb)
	if err != nil {
		return err
	}
	cobId &= 0xFFFFF800 // clear cobid bits
	cobId |= uint32(canId)
	return config.client.WriteRaw(config.nodeId, pdoCommIndex, 1, cobId, false)
}

func (config *NodeConfigurator) WriteTransmissionType(pdoNb uint16, transType uint8) error {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	return config.client.WriteRaw(config.nodeId, pdoCommIndex, 2, transType, false)
}

func (config *NodeConfigurator) WriteInhibitTime(pdoNb uint16, inhibitTime uint16) error {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	return config.client.WriteRaw(config.nodeId, pdoCommIndex, 3, inhibitTime, false)
}

func (config *NodeConfigurator) WriteEventTimer(pdoNb uint16, eventTimer uint16) error {
	pdoCommIndex := config.getCommunicationIndex(pdoNb)
	return config.client.WriteRaw(config.nodeId, pdoCommIndex, 5, eventTimer, false)
}

// Clear all the PDO mappings
// Technically clearing the actual map entries is not necessary but I find it cleaner
func (config *NodeConfigurator) ClearMappings(pdoNb uint16) error {
	pdoMappingIndex := config.getMappingIndex(pdoNb)
	// First clear nb of mapped entries
	err := config.client.WriteRaw(config.nodeId, pdoMappingIndex, 0, uint8(0), false)
	if err != nil {
		return err
	}
	// Then clear entries
	for i := range od.MaxMappedEntriesPdo {
		err := config.client.WriteRaw(config.nodeId, pdoMappingIndex, i+1, uint32(0), false)
		if err != nil {
			return err
		}
	}
	return nil
}

// Write new PDO mapping
// Takes a list of objects to map and will fill them up in the given order
// This will first clear the current mapping
func (config *NodeConfigurator) WriteMappings(pdoNb uint16, mappings []PDOMappingParameter) error {
	pdoMappingIndex := config.getMappingIndex(pdoNb)
	err := config.ClearMappings(pdoNb)
	if err != nil {
		return err
	}
	// Update with new mapping
	for sub, mapping := range mappings {
		rawMap := uint32(mapping.Index)<<16 + uint32(mapping.Subindex)<<8 + uint32(mapping.LengthBits)
		err := config.client.WriteRaw(config.nodeId, pdoMappingIndex, uint8(sub)+1, rawMap, false)
		if err != nil {
			return err
		}
	}
	// Update number of mapped objects
	return config.client.WriteRaw(config.nodeId, pdoMappingIndex, 0, uint8(len(mappings)), false)
}

// Update hole configuration
func (config *NodeConfigurator) WriteConfigurationPDO(pdoNb uint16, conf PDOConfigurationParameter) error {
	config.logger.Debug("updating configuration",
		"type", config.getType(pdoNb),
		"pdoNb", pdoNb,
		"conf", conf,
	)
	err := config.WriteCanIdPDO(pdoNb, conf.CanId)
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
