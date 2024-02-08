package canopen

type syncConfigurator struct {
	nodeId    uint8
	sdoClient *SDOClient
}

func newSYNCConfigurator(nodeId uint8, sdoClient *SDOClient) syncConfigurator {
	return syncConfigurator{nodeId: nodeId, sdoClient: sdoClient}
}

func (config *syncConfigurator) ReadCobId() (cobId uint32, err error) {
	return config.sdoClient.ReadUint32(config.nodeId, 0x1005, 0x0)
}

func (config *syncConfigurator) ReadCounterOverflow() (uint8, error) {
	return config.sdoClient.ReadUint8(config.nodeId, 0x1019, 0x0)
}

func (config *syncConfigurator) ReadCommunicationPeriod() (uint32, error) {
	return config.sdoClient.ReadUint32(config.nodeId, 0x1006, 0)
}

func (config *syncConfigurator) ReadWindowLengthPdos() (uint32, error) {
	return config.sdoClient.ReadUint32(config.nodeId, 0x1007, 0)
}

func (config *syncConfigurator) ProducerEnable() error {
	// Changing COB-ID is not allowed if already producer, read first
	cobId, err := config.ReadCobId()
	if err != nil {
		return err
	}
	cobId |= (1 << 30)
	return config.sdoClient.WriteRaw(config.nodeId, 0x1005, 0x0, cobId, false)
}

func (config *syncConfigurator) ProducerDisable() error {
	// Changing COB-ID is not allowed if already producer, read first
	cobId, err := config.ReadCobId()
	if err != nil {
		return err
	}
	mask := ^(uint32(1) << 30)
	cobId &= mask
	return config.sdoClient.WriteRaw(config.nodeId, 0x1005, 0x0, cobId, false)
}

// Change sync can id, sync should be disabled before changing this
func (config *syncConfigurator) WriteCanId(canId uint16) error {
	return config.sdoClient.WriteRaw(config.nodeId, 0x1005, 0x0, uint32(canId), false)
}

// Sync should have communication period of 0 before changing this
func (config *syncConfigurator) WriteCounterOverflow(counter uint8) error {
	return config.sdoClient.WriteRaw(config.nodeId, 0x1019, 0x0, counter, false)
}

func (config *syncConfigurator) WriteCommunicationPeriod(periodUs uint32) error {
	return config.sdoClient.WriteRaw(config.nodeId, 0x1006, 0, periodUs, false)
}

func (config *syncConfigurator) WriteWindowLengthPdos(windowPeriodUs uint32) error {
	return config.sdoClient.WriteRaw(config.nodeId, 0x1007, 0, windowPeriodUs, false)
}
