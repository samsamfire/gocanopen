package canopen

// BaseGateway implements all the basic gateway features defined by CiA 309
// CiA 309 currently defines 4 types:
// CiA 309-2 : Modbus TCP
// CiA 309-3 : ASCII
// CiA 309-4 : Profinet
// CiA 309-5 : HTTP / Websocket
// Each gateway maps its own parsing logic to this base gateway
type BaseGateway struct {
	network        *Network
	defaultNetwork uint16
	defaultNodeId  uint8
}

type GatewayVersion struct {
	vendorId            string
	productCode         string
	revisionNumber      string
	serialNumber        string
	gatewayClass        string
	protocolVersion     string
	implementationClass string
}

// Set default network to use
func (gw *BaseGateway) SetDefaultNetwork(id uint16) error {
	gw.defaultNetwork = id
	return nil
}

// Set default node Id to use
func (gw *BaseGateway) SetDefaultNodeId(id uint8) error {
	gw.defaultNodeId = id
	return nil
}

// Get gateway version information
func (gw *BaseGateway) GetVersion() (GatewayVersion, error) {
	return GatewayVersion{}, nil
}

// Broadcast nmt command to one or all nodes
func (gw *BaseGateway) NMTCommand(id uint8, command NMTCommand) error {
	return gw.network.Command(id, command)
}

// Set SDO timeout
func (gw *BaseGateway) SetSDOTimeout(timeoutMs uint32) error {
	// TODO : maybe add mutex in case ongoing transfer
	gw.network.sdoClient.timeoutTimeUs = timeoutMs * 1000
	gw.network.sdoClient.timeoutTimeBlockTransferUs = timeoutMs * 1000
	return nil
}
