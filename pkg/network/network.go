// This package is a pure golang implementation of the CANopen protocol
package network

import (
	"errors"
	"fmt"
	"sync"
	"time"

	canopen "github.com/samsamfire/gocanopen"
	can "github.com/samsamfire/gocanopen/pkg/can"
	"github.com/samsamfire/gocanopen/pkg/config"
	"github.com/samsamfire/gocanopen/pkg/nmt"
	n "github.com/samsamfire/gocanopen/pkg/node"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/samsamfire/gocanopen/pkg/sdo"
	log "github.com/sirupsen/logrus"
)

var ErrIdConflict = errors.New("id already exists on network, this will create conflicts")

// A Network is the main object of this package
// It should be created before doint anything else
// It acts as scheduler for locally created CANopen nodes
// But can also be used for controlling remote CANopen nodes
type Network struct {
	*canopen.BusManager
	*sdo.SDOClient
	nodes     map[uint8]n.Node
	wgProcess sync.WaitGroup
	// Network has an its own SDOClient
	odMap map[uint8]*ObjectDictionaryInformation
}

type ObjectDictionaryInformation struct {
	nodeId  uint8
	od      *od.ObjectDictionary
	edsPath string
}

// Create a new Network using the given CAN bus
func NewNetwork(bus can.Bus) Network {
	return Network{
		nodes:      map[uint8]n.Node{},
		BusManager: canopen.NewBusManager(bus),
		odMap:      map[uint8]*ObjectDictionaryInformation{},
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
	var bus can.Bus
	var err error
	if network.BusManager == nil {
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
		bus, err = can.NewBus(canInterface, channel, bitrate)
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
	client, err := sdo.NewSDOClient(network.BusManager, nil, 0, sdo.DEFAULT_SDO_CLIENT_TIMEOUT_MS, nil)
	network.SDOClient = client
	return err
}

// Disconnects from the CAN bus and stops processing
// of CANopen stack
func (network *Network) Disconnect() {
	for _, node := range network.nodes {
		node.SetExit(true)
	}
	network.wgProcess.Wait()
	network.BusManager.Bus().Disconnect()
}

// Launch goroutine that handles CANopen stack processing of a node
func (network *Network) launchNodeProcess(node n.Node) {
	log.Infof("[NETWORK][x%x] adding node to nodes being processed %T", node.GetID(), node)
	network.wgProcess.Add(1)
	go func(node n.Node) {
		defer network.wgProcess.Done()
		// These are timer values and can be adjusted
		startBackground := time.Now()
		backgroundPeriod := time.Duration(10 * time.Millisecond)
		startMain := time.Now()
		mainPeriod := time.Duration(1 * time.Millisecond)
		for {
			switch node.GetState() {
			case n.NODE_INIT:
				log.Infof("[NETWORK][x%x] starting node background process", node.GetID())
				node.Wg().Add(1)
				go func() {
					defer node.Wg().Done()
					for {
						select {
						case <-node.GetExitBackground():
							log.Infof("[NETWORK][x%x] exited node background process", node.GetID())
							return
						default:
							elapsed := time.Since(startBackground)
							startBackground = time.Now()
							timeDifferenceUs := uint32(elapsed.Microseconds())
							syncWas := node.ProcessSync(timeDifferenceUs, nil)
							node.ProcessTPDO(syncWas, timeDifferenceUs, nil)
							node.ProcessRPDO(syncWas, timeDifferenceUs, nil)
							time.Sleep(backgroundPeriod)
						}
					}
				}()
				node.SetState(n.NODE_RUNNING)

			case n.NODE_RUNNING:
				elapsed := time.Since(startMain)
				startMain = time.Now()
				timeDifferenceUs := uint32(elapsed.Microseconds())
				state := node.ProcessMain(false, timeDifferenceUs, nil)
				node.MainCallback()
				time.Sleep(mainPeriod)
				if state == nmt.RESET_APP || state == nmt.RESET_COMM {
					node.SetState(n.NODE_RESETING)
				}
				select {
				case <-node.GetExit():
					log.Infof("[NETWORK][x%x] received exit request", node.GetID())
					node.SetState(n.NODE_EXIT)
				default:

				}
			case n.NODE_RESETING:
				node.SetExitBackground(true)
				node.SetState(n.NODE_INIT)

			case n.NODE_EXIT:
				node.SetExitBackground(true)
				node.Wg().Wait()
				log.Infof("[NETWORK][x%x] complete exit", node.GetID())
				return
			}
		}
	}(node)
}

// Get OD for a specific node id
func (network *Network) GetOD(nodeId uint8) (*od.ObjectDictionary, error) {
	_, odLoaded := network.odMap[nodeId]
	if odLoaded {
		return network.odMap[nodeId].od, nil
	}
	// Look in local nodes
	_, odLoaded = network.nodes[nodeId]
	if odLoaded {
		return network.nodes[nodeId].GetOD(), nil
	}
	return nil, od.ODR_OD_MISSING
}

// Command can be used to send an NMT command to a specific nodeId
// nodeId = 0 is used as a broadcast command i.e. affects all nodes
// on the network
//
//	network.Command(0,NMT_RESET_NODE) // resets all nodes
//	network.Command(12,NMT_RESET_NODE) // resets nodeId 12
func (network *Network) Command(nodeId uint8, nmtCommand nmt.NMTCommand) error {
	if nodeId > 127 || (nmtCommand != nmt.NMT_ENTER_OPERATIONAL &&
		nmtCommand != nmt.NMT_ENTER_PRE_OPERATIONAL &&
		nmtCommand != nmt.NMT_ENTER_STOPPED &&
		nmtCommand != nmt.NMT_RESET_COMMUNICATION &&
		nmtCommand != nmt.NMT_RESET_NODE) {
		return canopen.ErrIllegalArgument
	}
	frame := can.NewFrame(uint32(nmt.SERVICE_ID), 0, 2)
	frame.Data[0] = uint8(nmtCommand)
	frame.Data[1] = nodeId
	log.Debugf("[NMT] sending nmt command : %v to node(s) %v (x%x)", nmt.CommandDescription[nmtCommand], nodeId, nodeId)
	return network.Send(frame)
}

// Create a [LocalNode] a CiA 301 compliant node with a given OD
// od can be either a string : path to OD or an OD object.
// Processing is started immediately after creating the node.
// By default, node automatically goes to operational state if no errors are detected.
// First heartbeat, if enabled is started after 500ms
func (network *Network) CreateLocalNode(nodeId uint8, odict any) (*n.LocalNode, error) {
	var odNode *od.ObjectDictionary
	var err error
	switch odType := odict.(type) {
	case string:
		odNode, err = od.Parse(odType, nodeId)
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
		odNode,
		nil, // Use definition from OD
		nil, // Use definition from OD
		nodeId,
		nmt.StartupToOperational,
		500,
		sdo.SDO_CLIENT_TIMEOUT, // Not changeable currently
		sdo.SDO_SERVER_TIMEOUT, // Not changeable currently
		true,
		nil,
	)
	if err != nil {
		return nil, err
	}
	// Add to network, launch routine
	if _, ok := network.nodes[nodeId]; ok {
		return nil, ErrIdConflict
	}
	network.nodes[nodeId] = node
	network.launchNodeProcess(node)
	return node, nil
}

// Add a [RemoteNode] with a given OD for master control
// od can be either a string : path to OD or OD object
// useLocal is used to define whether the supplied OD should be used
// or the remote node should be read to create PDO mapping
// If remote nodes PDO mapping is static and known, use useLocal = true
// otherwise, if PDO mapping is dynamic, use useLocal = false
func (network *Network) AddRemoteNode(nodeId uint8, odict any, useLocal bool) (*n.RemoteNode, error) {
	var odNode *od.ObjectDictionary
	var err error
	if nodeId < 1 || nodeId > 127 {
		return nil, fmt.Errorf("nodeId should be between 1 and 127, value given : %v", nodeId)
	}
	switch odType := odict.(type) {
	case string:
		odNode, err = od.Parse(odType, nodeId)
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

	node, err := n.NewRemoteNode(network.BusManager, odNode, nodeId, useLocal)
	if err != nil {
		return nil, err
	}
	if _, ok := network.nodes[nodeId]; ok {
		return nil, ErrIdConflict
	}
	network.nodes[nodeId] = node
	network.launchNodeProcess(node)
	return node, nil
}

// RemoveNode gracefully exits any running go routine for this node
// It also removes any object associated with the node, including OD
func (network *Network) RemoveNode(nodeId uint8) {
	node, ok := network.nodes[nodeId]
	if !ok {
		return
	}
	node.SetExit(true)
	node.Wg().Wait()
	delete(network.nodes, nodeId)
}

// Configurator creates a [NodeConfigurator] object for a given id
// using the networks internal sdo client
func (network *Network) Configurator(nodeId uint8) *config.NodeConfigurator {
	return config.NewNodeConfigurator(nodeId, network.SDOClient)
}
