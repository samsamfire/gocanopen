package gateway

import (
	"github.com/samsamfire/gocanopen/pkg/network"
	"github.com/samsamfire/gocanopen/pkg/nmt"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/samsamfire/gocanopen/pkg/sdo"
	log "github.com/sirupsen/logrus"
)

// BaseGateway implements the basic gateway features defined by CiA 309
// CiA 309 currently defines 4 types:
// CiA 309-2 : Modbus TCP
// CiA 309-3 : ASCII
// CiA 309-4 : Profinet
// CiA 309-5 : HTTP / Websocket
// Each gateway maps its own parsing logic to this base gateway
type BaseGateway struct {
	network        *network.Network
	defaultNetwork uint16
	defaultNodeId  uint8
	sdoBuffer      []byte
}

func NewBaseGateway(network *network.Network, defaultNetwork uint16, defaultNodeId uint8, sdoUploadBufferSize int) *BaseGateway {
	return &BaseGateway{
		network:        network,
		defaultNetwork: defaultNetwork,
		defaultNodeId:  defaultNodeId,
		sdoBuffer:      make([]byte, sdoUploadBufferSize),
	}
}

type GatewayVersion struct {
	VendorId            string
	ProductCode         string
	RevisionNumber      string
	SerialNumber        string
	GatewayClass        string
	ProtocolVersion     string
	ImplementationClass string
}

// Set default network to use
func (gw *BaseGateway) SetDefaultNetworkId(id uint16) error {
	gw.defaultNetwork = id
	return nil
}

// Get the default network
func (gw *BaseGateway) DefaultNetworkId() uint16 {
	return gw.defaultNetwork
}

// Set default node Id to use
func (gw *BaseGateway) SetDefaultNodeId(id uint8) error {
	gw.defaultNodeId = id
	return nil
}

// Get default node Id
func (gw *BaseGateway) DefaultNodeId() uint8 {
	return gw.defaultNodeId
}

// Get gateway version information
func (gw *BaseGateway) GetVersion() (GatewayVersion, error) {
	// TODO : gateway should be a node and be able to read specific values
	return GatewayVersion{
		VendorId:            "0x0",
		ProductCode:         "0x0",
		RevisionNumber:      "0x0",
		SerialNumber:        "0x0",
		GatewayClass:        "2",
		ProtocolVersion:     "02.01",
		ImplementationClass: "01.00",
	}, nil
}

// Broadcast nmt command to one or all nodes
func (gw *BaseGateway) NMTCommand(id uint8, command nmt.NMTCommand) error {
	return gw.network.Command(id, command)
}

// Set SDO timeout
func (gw *BaseGateway) SetSDOTimeout(timeoutMs uint32) error {
	// TODO : maybe add mutex in case ongoing transfer
	gw.network.SDOClient.SetTimeout(timeoutMs)
	gw.network.SDOClient.SetTimeoutBlockTransfer(timeoutMs)
	log.Debugf("[HTTP][SERVER] changing sdo client timeout to %vms", timeoutMs)
	return nil
}

// Access SDO read buffer
func (gw *BaseGateway) Buffer() []byte {
	return gw.sdoBuffer
}

// Read SDO
func (gw *BaseGateway) ReadSDO(nodeId uint8, index uint16, subindex uint8) (int, error) {
	return gw.network.ReadRaw(nodeId, index, subindex, gw.sdoBuffer)
}

// Write SDO
func (gw *BaseGateway) WriteSDO(nodeId uint8, index uint16, subindex uint8, value string, datatype uint8) error {
	encodedValue, err := od.EncodeFromString(value, datatype, 0)
	if err != nil {
		return sdo.AbortTypeMismatch
	}
	return gw.network.WriteRaw(nodeId, index, subindex, encodedValue, false)
}

// Disconnect from network
func (gw *BaseGateway) Disconnect() {
	gw.network.Disconnect()
}
