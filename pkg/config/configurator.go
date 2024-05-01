package config

import "github.com/samsamfire/gocanopen/pkg/sdo"

// NodeConfigurator provides helper methods for
// reading / updating CANopen reserved configuration objects
// i.e. objects between 0x1000 and 0x2000.
// No EDS files need to be loaded for configuring these parameters
type NodeConfigurator struct {
	*sdo.SDOClient
	nodeId uint8
	RPDO   *PDOConfig
	TPDO   *PDOConfig
	SYNC   *SYNCConfig
	HB     *HBConfig
	TIME   *TIMEConfig
}

// Create a new [NodeConfigurator] for given ID and SDOClient
func NewNodeConfigurator(nodeId uint8, client *sdo.SDOClient) *NodeConfigurator {
	configurator := NodeConfigurator{SDOClient: client, nodeId: nodeId}
	configurator.RPDO = NewRPDOConfigurator(nodeId, client)
	configurator.TPDO = NewTPDOConfigurator(nodeId, client)
	configurator.SYNC = NewSYNCConfigurator(nodeId, client)
	configurator.HB = NewHBConfigurator(nodeId, client)
	configurator.TIME = NewTIMEConfigurator(nodeId, client)
	return &configurator
}

func (config *NodeConfigurator) ReadManufacturerDeviceName() (string, error) {
	raw := make([]byte, 256)
	n, err := config.ReadRaw(config.nodeId, 0x1008, 0, raw)
	if err != nil {
		return "", err
	}
	return string(raw[:n]), nil
}

func (config *NodeConfigurator) ReadManufacturerHardwareVersion() (string, error) {
	raw := make([]byte, 256)
	n, err := config.ReadRaw(config.nodeId, 0x1009, 0, raw)
	if err != nil {
		return "", err
	}
	return string(raw[:n]), nil
}

func (config *NodeConfigurator) ReadManufacturerSoftwareVersion() (string, error) {
	raw := make([]byte, 256)
	n, err := config.ReadRaw(config.nodeId, 0x100A, 0, raw)
	if err != nil {
		return "", err
	}
	return string(raw[:n]), nil
}

type Identity struct {
	VendorId       uint32
	ProductCode    uint32
	RevisionNumber uint32
	SerialNumber   uint32
}

// Read identity object (0x1018, mandatory)
func (config *NodeConfigurator) ReadIdentity() (*Identity, error) {
	// Vendor ID is the only mandatory field
	vendorId, err := config.ReadUint32(config.nodeId, 0x1018, 1)
	if err != nil {
		return nil, err
	}
	productCode, _ := config.ReadUint32(config.nodeId, 0x1018, 2)
	revisionNumber, _ := config.ReadUint32(config.nodeId, 0x1018, 3)
	serialNumber, _ := config.ReadUint32(config.nodeId, 0x1018, 4)
	return &Identity{
		VendorId:       vendorId,
		ProductCode:    productCode,
		RevisionNumber: revisionNumber,
		SerialNumber:   serialNumber,
	}, nil

}

type ManufacturerInformation struct {
	ManufacturerDeviceName      string
	ManufacturerHardwareVersion string
	ManufacturerSoftwareVersion string
}

// Read manufacturer objects (0x1008,0x1009,0x100A, these are all optional)
func (config *NodeConfigurator) ReadManufacturerInformation() ManufacturerInformation {
	info := ManufacturerInformation{}
	info.ManufacturerDeviceName, _ = config.ReadManufacturerDeviceName()
	info.ManufacturerHardwareVersion, _ = config.ReadManufacturerHardwareVersion()
	info.ManufacturerSoftwareVersion, _ = config.ReadManufacturerSoftwareVersion()
	return info
}
