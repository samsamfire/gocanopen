package canopen

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

type Network struct {
	Nodes      map[uint8]*Node
	busManager *BusManager
	sdoClient  *SDOClient // Network master has an sdo client to read/write nodes on network
	// An sdo client does not have to be linked to a specific node
	odMap map[uint8]*ObjectDictionaryInformation
}

type ObjectDictionaryInformation struct {
	nodeId  uint8
	od      *ObjectDictionary
	edsPath string
}

func NewNetwork(bus Bus) Network {
	return Network{Nodes: map[uint8]*Node{}, busManager: NewBusManager(bus), odMap: map[uint8]*ObjectDictionaryInformation{}}
}

// Connects to network and initialize master functionnality
// Custom CAN backend is possible using "Bus" interface
// Otherwise it expects an interface name, channel and bitrate
func (network *Network) Connect(args ...any) error {
	if len(args) < 3 && network.busManager.Bus == nil {
		return errors.New("either provide custom backend, or provide interface, channel and bitrate")
	}
	var bus Bus
	var err error
	busManager := network.busManager
	if busManager.Bus == nil {
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
		bus, err = createBusInternal(canInterface, channel, bitrate)
		if err != nil {
			return err
		}
		busManager.Bus = bus
	} else {
		bus = busManager.Bus
	}
	// Connect to CAN bus and subscribe to CAN message reception
	err = bus.Connect(args)
	if err != nil {
		return err
	}
	err = bus.Subscribe(busManager)
	if err != nil {
		return err
	}
	// Add SDO client to network by default
	client, err := NewSDOClient(busManager, nil, 0, DEFAULT_SDO_CLIENT_TIMEOUT_MS, nil)
	network.sdoClient = client
	return err
}

// Disconnects from CAN bus and stops cleanly everything
func (network *Network) Disconnect() {
	log.Infof("[NETWORK] disconnecting from network")
	for _, node := range network.Nodes {
		node.exit <- true
	}
	network.busManager.Bus.Disconnect()

}

// Process CANopen stack, this is blocking
func (network *Network) Process() error {
	var wg sync.WaitGroup
	for id := range network.Nodes {
		wg.Add(1)
		log.Infof("[NETWORK][x%x] adding node to nodes being processed", id)
		go func(node *Node) {
			defer wg.Done()
			var nodeWg sync.WaitGroup
			// These are timer values and can be adjusted
			startBackground := time.Now()
			backgroundPeriod := time.Duration(10 * time.Millisecond)
			startMain := time.Now()
			mainPeriod := time.Duration(1 * time.Millisecond)
			for {
				switch node.State {
				case NODE_INIT:
					log.Infof("[NETWORK][x%x] starting node background process", node.id)
					nodeWg.Add(1)
					go func() {
						defer nodeWg.Done()
						for {
							select {
							case <-node.reset:
								log.Infof("[NETWORK][x%x] exited node background process", node.id)
								return
							default:
								elapsed := time.Since(startBackground)
								startBackground = time.Now()
								timeDifferenceUs := uint32(elapsed.Microseconds())
								syncWas := node.processSync(timeDifferenceUs, nil)
								node.processTPDO(syncWas, timeDifferenceUs, nil)
								node.processRPDO(syncWas, timeDifferenceUs, nil)
								time.Sleep(backgroundPeriod)
							}
						}
					}()
					node.State = NODE_RUNNING

				case NODE_RUNNING:
					elapsed := time.Since(startMain)
					startMain = time.Now()
					timeDifferenceUs := uint32(elapsed.Microseconds())
					state := node.processMain(false, timeDifferenceUs, nil)
					// <-- Add application code HERE
					time.Sleep(mainPeriod)
					if state == RESET_APP || state == RESET_COMM {
						node.State = NODE_RESETING
					} else {
						select {
						case <-node.exit:
							log.Infof("[NETWORK][x%x] request to stop local node", node.id)
							node.State = NODE_EXIT
						default:
						}
					}
				case NODE_RESETING:
					node.reset <- true
					node.State = NODE_INIT

				case NODE_EXIT:
					node.reset <- true
					nodeWg.Wait()

				}
			}
		}(network.Nodes[id])
	}
	wg.Wait()
	return nil
}

// Get OD for a specific node id
func (network *Network) GetOD(nodeId uint8) (*ObjectDictionary, error) {
	_, odLoaded := network.odMap[nodeId]
	if odLoaded {
		return network.odMap[nodeId].od, nil
	}
	// Look in local nodes
	_, odLoaded = network.Nodes[nodeId]
	if odLoaded {
		return network.Nodes[nodeId].OD, nil
	}
	return nil, ODR_OD_MISSING
}

// Send NMT commands to remote nodes
// Id 0 is used as a broadcast command i.e. affects all nodes
func (network *Network) Command(nodeId uint8, nmtCommand NMTCommand) error {
	if nodeId > 127 || (nmtCommand != NMT_ENTER_OPERATIONAL &&
		nmtCommand != NMT_ENTER_PRE_OPERATIONAL &&
		nmtCommand != NMT_ENTER_STOPPED &&
		nmtCommand != NMT_RESET_COMMUNICATION &&
		nmtCommand != NMT_RESET_NODE) {
		return ErrIllegalArgument
	}
	frame := NewFrame(uint32(NMT_SERVICE_ID), 0, 2)
	frame.Data[0] = uint8(nmtCommand)
	frame.Data[1] = nodeId
	log.Debugf("[NMT] sending nmt command : %v to node(s) %v (x%x)", NMT_COMMAND_MAP[nmtCommand], nodeId, nodeId)
	return network.busManager.Send(frame)
}

// Create a local node with a given OD
// Can be either a string : path to OD
// Or it can be an OD object
func (network *Network) CreateNode(nodeId uint8, od any) (*Node, error) {
	var odNode *ObjectDictionary
	var err error
	switch odType := od.(type) {
	case string:
		odNode, err = ParseEDSFromFile(odType, nodeId)
		if err != nil {
			return nil, err
		}
	case ObjectDictionary:
		odNode = &odType
	default:
		return nil, fmt.Errorf("expecting string or ObjectDictionary got : %T", od)
	}
	// Create and initialize a "local" CANopen node
	node, err := NewNode(
		network.busManager,
		odNode,
		nil, // Use definition from OD
		nil, // Use definition from OD
		nodeId,
		NMT_STARTUP_TO_OPERATIONAL,
		500,
		SDO_CLIENT_TIMEOUT, // Not changeable currently
		SDO_SERVER_TIMEOUT, // Not changeable currently
		true,
		nil,
	)
	if err != nil {
		return nil, err
	}
	// Return created node and add it to network
	network.Nodes[nodeId] = node
	return node, nil
}

// Add a remote node with a given OD
// OD can be a path, ObjectDictionary or nil
// This function will load and parse Object dictionnary (OD) into memory
// If already present, OD will be overwritten
// User can then access the node via OD naming
// A same OD can be used for multiple nodes
func (network *Network) AddNode(nodeId uint8, od any) error {
	var odNode *ObjectDictionary
	var err error
	if nodeId < 1 || nodeId > 127 {
		return fmt.Errorf("nodeId should be between 1 and 127, value given : %v", nodeId)
	}
	switch odType := od.(type) {
	case string:
		odNode, err = ParseEDSFromFile(odType, nodeId)
		if err != nil {
			return err
		}
		network.odMap[nodeId] = &ObjectDictionaryInformation{nodeId: nodeId, od: odNode, edsPath: odType}
	case ObjectDictionary:
		odNode = &odType
		network.odMap[nodeId] = &ObjectDictionaryInformation{nodeId: nodeId, od: odNode, edsPath: ""}
	default:
		return fmt.Errorf("expecting string or ObjectDictionary got : %T", od)
	}

	return nil

}

// Same as AddNode, except od is downloaded from remote node
// Optional callback can be provided to perform unzip, untar etc if a specific storage format is used
func (network *Network) AddNodeFromSDO(
	nodeId uint8,
	formatHandlerCallback func(formatType uint8, reader io.Reader) (*ObjectDictionary, error),
) error {
	rawEds, err := network.sdoClient.ReadAll(nodeId, 0x1021, 0)
	if err != nil {
		return err
	}
	edsFormat := []byte{0}
	_, err = network.sdoClient.ReadRaw(nodeId, 0x1022, 0, edsFormat)
	switch formatHandlerCallback {
	case nil:
		// No callback & format is not specified or
		// Storage format is 0
		// Use default ASCII format
		if err != nil || (err == nil && edsFormat[0] == 0) {
			od, err := ParseEDSFromRaw(rawEds, nodeId)
			if err != nil {
				return err
			}
			network.odMap[nodeId] = &ObjectDictionaryInformation{nodeId: nodeId, od: od, edsPath: ""}
			return nil
		} else {
			return fmt.Errorf("supply a handler for the format : %v", edsFormat[0])
		}
	default:
		odReader := bytes.NewBuffer(rawEds)
		od, err := formatHandlerCallback(edsFormat[0], odReader)
		if err != nil {
			return nil
		}
		network.odMap[nodeId] = &ObjectDictionaryInformation{nodeId: nodeId, od: od, edsPath: ""}
		return nil
	}
}
