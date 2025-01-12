// This package is a pure golang implementation of the CANopen protocol
package network

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"

	canopen "github.com/samsamfire/gocanopen"
	can "github.com/samsamfire/gocanopen/pkg/can"
	_ "github.com/samsamfire/gocanopen/pkg/can/all"
	"github.com/samsamfire/gocanopen/pkg/config"
	"github.com/samsamfire/gocanopen/pkg/nmt"
	n "github.com/samsamfire/gocanopen/pkg/node"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/samsamfire/gocanopen/pkg/sdo"
)

var (
	ErrIdConflict      = errors.New("id already exists on network, this will create conflicts")
	ErrIdRange         = errors.New("id is out of range")
	ErrNotFound        = errors.New("node id not found on network, add or create it first")
	ErrInvalidNodeType = errors.New("invalid node type")
	ErrNoNodesFound    = errors.New("no nodes found on network when performing SDO scan")
)

const (
	nodeIdMin = uint8(1)
	nodeIdMax = uint8(126)
)

// A Network is the main object of this package
// It should be created before doing anything else
// It acts as scheduler for locally created CANopen nodes
// But can also be used for controlling remote CANopen nodes
type Network struct {
	*canopen.BusManager
	*sdo.SDOClient
	controllers map[uint8]*n.NodeProcessor
	// Network has an its own SDOClient
	odMap    map[uint8]*ObjectDictionaryInformation
	odParser od.Parser
	logger   *slog.Logger
}

type ObjectDictionaryInformation struct {
	nodeId  uint8
	od      *od.ObjectDictionary
	edsPath string
}

// Create a new CAN bus with given interface
func NewBus(canInterfaceName string, channel string, bitrate int) (canopen.Bus, error) {
	createInterface, ok := can.AvailableInterfaces[canInterfaceName]
	if !ok {
		if slices.Contains(can.ImplementedInterfaces, canInterfaceName) {
			return nil, fmt.Errorf("not enabled : %v, check build flags for project", canInterfaceName)
		} else {
			return nil, fmt.Errorf("not supported : %v", canInterfaceName)
		}
	}
	return createInterface(channel)
}

// Create a new Network using the given CAN bus
func NewNetwork(bus canopen.Bus) Network {
	return Network{
		controllers: map[uint8]*n.NodeProcessor{},
		BusManager:  canopen.NewBusManager(bus),
		odMap:       map[uint8]*ObjectDictionaryInformation{},
		odParser:    od.Parse,
		logger:      slog.Default(),
	}
}

// Connects to CAN bus, this should be called before anything else.
// Custom CAN backend is possible using a custom "Bus" interface.
// Otherwise it expects an interface name, channel and bitrate.
// Currently only socketcan and virtualcan are supported.
func (network *Network) Connect(args ...any) error {
	if len(args) < 3 && network.Bus() == nil {
		return errors.New("either provide custom backend, or provide interface, channel and bitrate")
	}
	var bus canopen.Bus
	var err error
	if network.Bus() == nil {
		canInterface, ok := args[0].(string)
		if !ok {
			return fmt.Errorf("expecting string for interface got : %v", args[0])
		}
		channel, ok := args[1].(string)
		if !ok {
			return fmt.Errorf("expecting string for channel got : %v", args[1])
		}
		bitrate, ok := args[2].(int)
		if !ok {
			return fmt.Errorf("expecting int for bitrate got : %v", args[2])
		}
		bus, err = NewBus(canInterface, channel, bitrate)
		if err != nil {
			return err
		}
		network.SetBus(bus)
	} else {
		bus = network.Bus()
	}
	// Connect to CAN bus and subscribe to CAN message reception
	err = bus.Connect(args)
	if err != nil {
		return err
	}
	err = bus.Subscribe(network.BusManager)
	if err != nil {
		return err
	}
	// Add SDO client to network by default
	client, err := sdo.NewSDOClient(network.BusManager, network.logger, nil, 0, sdo.DefaultClientTimeout, nil)
	network.SDOClient = client
	return err
}

// Disconnects from the CAN bus and stops processing
// of CANopen stack
func (network *Network) Disconnect() {
	// Stop processing for everyone then wait for everyone
	// This is done in two steps because there can be a delay
	// between stop & wait.
	for _, controller := range network.controllers {
		controller.Stop()
	}
	for _, controller := range network.controllers {
		controller.Wait()
	}
	_ = network.BusManager.Bus().Disconnect()
}

// Get OD for a specific node id
func (network *Network) GetOD(nodeId uint8) (*od.ObjectDictionary, error) {
	_, odLoaded := network.odMap[nodeId]
	if odLoaded {
		return network.odMap[nodeId].od, nil
	}
	// Look in local nodes
	_, odLoaded = network.controllers[nodeId]
	if odLoaded {
		return network.controllers[nodeId].GetNode().GetOD(), nil
	}
	return nil, od.ErrOdMissing
}

// Read object dictionary using object 1021 (EDS storage) of a remote node
// Optional callback can be provided to perform manufacturer specific parsing
// in case a custom format is used (format type != 0).
// By default, regular uncompressed ASCII will be used (format type of 0).
func (network *Network) ReadEDS(nodeId uint8, edsFormatHandler od.EDSFormatHandler) (*od.ObjectDictionary, error) {
	// Read EDS and format in memory
	rawEds, err := network.ReadAll(nodeId, 0x1021, 0)
	if err != nil {
		return nil, err
	}
	edsFormat, err := network.ReadUint8(nodeId, 0x1022, 0)
	if err != nil {
		// Don't fail if format is not specified, consider it to be ASCII
		network.logger.Warn("read EDS format failed, defaulting to ASCII", "id", nodeId, "error", err)
		edsFormat = 0
	}
	// Use ascii format handler as default if non given
	if edsFormatHandler == nil {
		edsFormatHandler = od.DefaultEDSFormatHandler
	}
	odReader := bytes.NewBuffer(rawEds)
	return edsFormatHandler(nodeId, edsFormat, odReader)
}

// Command can be used to send an NMT command to a specific nodeId
// nodeId = 0 is used as a broadcast command i.e. affects all nodes
// on the network
//
//	network.Command(0,nmt.CommandResetNode) // resets all nodes
//	network.Command(12,nmt.CommandResetNode) // resets nodeId 12
func (network *Network) Command(nodeId uint8, nmtCommand nmt.Command) error {
	if nodeId > 127 || (nmtCommand != nmt.CommandEnterOperational &&
		nmtCommand != nmt.CommandEnterPreOperational &&
		nmtCommand != nmt.CommandEnterStopped &&
		nmtCommand != nmt.CommandResetCommunication &&
		nmtCommand != nmt.CommandResetNode) {
		return canopen.ErrIllegalArgument
	}
	frame := canopen.NewFrame(uint32(nmt.ServiceId), 0, 2)
	frame.Data[0] = uint8(nmtCommand)
	frame.Data[1] = nodeId
	network.logger.Info("[TX] nmt command to node(s)", "command", nmt.CommandDescription[nmtCommand], "id", nodeId)
	return network.Send(frame)
}

// Create a [LocalNode] a CiA 301 compliant node with a given OD
// OD can be either a string : path to OD or an OD object.
// Processing is started immediately after creating the node.
// By default, node automatically goes to operational state if no errors are detected.
// First heartbeat, if enabled is started after 500ms
func (network *Network) CreateLocalNode(nodeId uint8, odict any) (*n.LocalNode, error) {
	var odNode *od.ObjectDictionary
	var err error

	if nodeId < nodeIdMin || nodeId > nodeIdMax {
		return nil, ErrIdRange
	}

	switch odType := odict.(type) {
	case string:
		odNode, err = network.odParser(odType, nodeId)
		if err != nil {
			return nil, err
		}
	case od.ObjectDictionary:
		odNode = &odType
	case *od.ObjectDictionary:
		odNode = odType
	default:
		return nil, fmt.Errorf("expecting string or ObjectDictionary got : %T", odict)
	}
	// Create and initialize a "local" CANopen node
	node, err := n.NewLocalNode(
		network.BusManager,
		network.logger,
		odNode,
		nil, // Use definition from OD
		nil, // Use definition from OD
		nodeId,
		nmt.StartupToOperational,
		500,
		sdo.DefaultClientTimeout, // Not changeable currently
		sdo.DefaultServerTimeout, // Not changeable currently
		true,
		nil,
	)
	if err != nil {
		return nil, err
	}
	// Add to network, launch routine for managing this node
	// Automatically
	controller, err := network.AddNode(node)
	if err != nil {
		return nil, err
	}
	err = controller.Start(context.Background())
	if err != nil {
		return nil, err
	}
	return node, nil
}

// Add a [RemoteNode] with a given OD for master control
// od can be either a string : path to OD or OD object
// useLocal is used to define whether the supplied OD should be used
// or the remote node should be read to create PDO mapping
// If remote nodes PDO mapping is static and known, use useLocal = true
// otherwise, if PDO mapping is dynamic, use useLocal = false
func (network *Network) AddRemoteNode(nodeId uint8, odict any) (*n.RemoteNode, error) {
	var odNode *od.ObjectDictionary
	var err error

	if nodeId < nodeIdMin || nodeId > nodeIdMax {
		return nil, ErrIdRange
	}

	switch odType := odict.(type) {
	case string:
		odNode, err = network.odParser(odType, nodeId)
		if err != nil {
			return nil, err
		}
		network.odMap[nodeId] = &ObjectDictionaryInformation{nodeId: nodeId, od: odNode, edsPath: odType}
	case od.ObjectDictionary:
		odNode = &odType
		network.odMap[nodeId] = &ObjectDictionaryInformation{nodeId: nodeId, od: odNode, edsPath: ""}
	case *od.ObjectDictionary:
		odNode = odType
		network.odMap[nodeId] = &ObjectDictionaryInformation{nodeId: nodeId, od: odNode, edsPath: ""}
	case nil:
		odNode = nil

	default:
		return nil, fmt.Errorf("expecting string or ObjectDictionary got : %T", odict)
	}

	node, err := n.NewRemoteNode(network.BusManager, network.logger, odNode, nodeId)
	if err != nil {
		return nil, err
	}

	// Add to network, launch routine for managing this node
	// Automatically
	controller, err := network.AddNode(node)
	if err != nil {
		return nil, err
	}
	err = controller.Start(context.Background())
	if err != nil {
		return nil, err
	}
	return node, nil
}

// Add any node to the network and return a node controller which can be used
// To control high level node behaviour (starting, stopping the node)
func (network *Network) AddNode(node n.Node) (*n.NodeProcessor, error) {
	controller := n.NewNodeProcessor(node, network.logger)
	_, ok := network.controllers[node.GetID()]
	if ok {
		return nil, ErrIdConflict
	}
	network.controllers[node.GetID()] = controller
	return controller, nil
}

// RemoveNode gracefully exits any running go routine for this node
// It also removes any object associated with the node, including OD
func (network *Network) RemoveNode(nodeId uint8) error {
	node, ok := network.controllers[nodeId]
	if !ok {
		return ErrNotFound
	}
	err := node.Stop()
	if err != nil {
		return err
	}
	node.Wait()
	delete(network.controllers, nodeId)
	return nil
}

// Get a remote node object in network, based on its id
func (network *Network) Remote(nodeId uint8) (*n.RemoteNode, error) {
	ctrl, ok := network.controllers[nodeId]
	if !ok {
		return nil, ErrNotFound
	}
	remote, ok := ctrl.GetNode().(*n.RemoteNode)
	if !ok {
		return nil, ErrInvalidNodeType
	}
	return remote, nil
}

// Get a local node object in network, based on its id
func (network *Network) Local(nodeId uint8) (*n.LocalNode, error) {
	ctrl, ok := network.controllers[nodeId]
	if !ok {
		return nil, ErrNotFound
	}
	remote, ok := ctrl.GetNode().(*n.LocalNode)
	if !ok {
		return nil, ErrInvalidNodeType
	}
	return remote, nil
}

// Configurator creates a [NodeConfigurator] object for a given id
// using the networks internal sdo client
func (network *Network) Configurator(nodeId uint8) *config.NodeConfigurator {
	return config.NewNodeConfigurator(nodeId, network.logger, network.SDOClient)
}

// NodeInformation contains manufacturer information and identity object
type NodeInformation struct {
	config.ManufacturerInformation
	config.Identity
}

// Scan network for nodes via SDO, and return map of found node ids and respective
// node information. Scanning is done in parallel and requires that the scanned
// nodes have an SDO server and that the identity object is implemented (0x1018) which
// is mandatory per CiA standard.
func (network *Network) Scan(timeoutMs uint32) (map[uint8]NodeInformation, error) {
	// Create multiple sdo clients to speed up discovery
	clients := make([]*sdo.SDOClient, 0)
	for i := nodeIdMin; i <= nodeIdMax; i++ {
		client, err := sdo.NewSDOClient(network.BusManager, network.logger, nil, i, timeoutMs, nil)
		if err != nil {
			return nil, err
		}
		clients = append(clients, client)
	}
	wg := sync.WaitGroup{}
	mu := sync.Mutex{}
	scan := make(map[uint8]NodeInformation)
	wg.Add(len(clients))
	// Scanning is done in parallel to speed up discovery
	// As the limiting factor is the SDO round-trip time which
	// can take up to timeoutMs to complete
	for i, client := range clients {
		nodeId := uint8(i + 1)
		go func(client *sdo.SDOClient) {
			defer wg.Done()
			config := config.NewNodeConfigurator(nodeId, network.logger, client)
			identity, err := config.ReadIdentity()
			if err != nil {
				// Failure to respond to ReadIdentity means node doesn't exist
				// Or that it does not implement a mandatory object so it will
				// be considered as not found
				return
			}
			manufacturerInfo := config.ReadManufacturerInformation()
			mu.Lock()
			defer mu.Unlock()
			scan[nodeId] = NodeInformation{
				ManufacturerInformation: manufacturerInfo,
				Identity:                *identity,
			}
		}(client)
	}
	wg.Wait()
	return scan, nil
}

func (network *Network) SetLogger(logger *slog.Logger) {
	network.logger = logger
}

func (network *Network) SetParser(parser od.Parser) {
	network.odParser = parser
}
