package config

type Identity struct {
	VendorId       uint32
	ProductCode    uint32
	RevisionNumber uint32
	SerialNumber   uint32
}

type ManufacturerInformation struct {
	ManufacturerDeviceName      string
	ManufacturerHardwareVersion string
	ManufacturerSoftwareVersion string
}

// Read identity object (0x1018, mandatory)
func (config *NodeConfigurator) ReadIdentity() (*Identity, error) {
	// Vendor ID is the only mandatory field
	vendorId, err := config.client.ReadUint32(config.nodeId, 0x1018, 1)
	if err != nil {
		return nil, err
	}
	productCode, _ := config.client.ReadUint32(config.nodeId, 0x1018, 2)
	revisionNumber, _ := config.client.ReadUint32(config.nodeId, 0x1018, 3)
	serialNumber, _ := config.client.ReadUint32(config.nodeId, 0x1018, 4)
	return &Identity{
		VendorId:       vendorId,
		ProductCode:    productCode,
		RevisionNumber: revisionNumber,
		SerialNumber:   serialNumber,
	}, nil
}

// Read manufacturer device name
func (config *NodeConfigurator) ReadManufacturerDeviceName() (string, error) {
	raw := make([]byte, 256)
	n, err := config.client.ReadRaw(config.nodeId, 0x1008, 0, raw)
	if err != nil {
		return "", err
	}
	return string(raw[:n]), nil
}

// Read Manufacturer hardware version
func (config *NodeConfigurator) ReadManufacturerHardwareVersion() (string, error) {
	raw := make([]byte, 256)
	n, err := config.client.ReadRaw(config.nodeId, 0x1009, 0, raw)
	if err != nil {
		return "", err
	}
	return string(raw[:n]), nil
}

// Read manufacturer software version
func (config *NodeConfigurator) ReadManufacturerSoftwareVersion() (string, error) {
	raw := make([]byte, 256)
	n, err := config.client.ReadRaw(config.nodeId, 0x100A, 0, raw)
	if err != nil {
		return "", err
	}
	return string(raw[:n]), nil
}

// Read manufacturer objects (0x1008,0x1009,0x100A, these are all optional)
func (config *NodeConfigurator) ReadManufacturerInformation() ManufacturerInformation {
	info := ManufacturerInformation{}
	info.ManufacturerDeviceName, _ = config.ReadManufacturerDeviceName()
	info.ManufacturerHardwareVersion, _ = config.ReadManufacturerHardwareVersion()
	info.ManufacturerSoftwareVersion, _ = config.ReadManufacturerSoftwareVersion()
	return info
}
