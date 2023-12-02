package canopen

import (
	"encoding/binary"
	"errors"
	"fmt"
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
	client, err := NewSDOClient(busManager, nil, 0, nil)
	network.sdoClient = client
	return err
}

// Disconnects from CAN bus and stops cleanly everything
func (network *Network) Disconnect() {

}

// Process CANopen stack, this is blocking
func (network *Network) Process() error {
	var wg sync.WaitGroup
	for id := range network.Nodes {
		wg.Add(1)
		log.Infof("[NETWORK][x%x] adding node to nodes being processed", id)
		go func(node *Node) {
			defer wg.Done()
			// These are timer values and can be adjusted
			startBackground := time.Now()
			backgroundPeriod := time.Duration(10 * time.Millisecond)
			startMain := time.Now()
			mainPeriod := time.Duration(1 * time.Millisecond)
			for {
				switch node.State {
				case NODE_INIT:
					// TODO : init node
					log.Infof("[NETWORK][x%x] starting node background process", node.id)
					go func() {
						for {
							select {
							case <-node.exit:
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
					state := node.Process(false, timeDifferenceUs, nil)
					// <-- Add application code HERE
					time.Sleep(mainPeriod)
					if state == RESET_APP || state == RESET_COMM {
						node.State = NODE_RESETING
					}
				case NODE_RESETING:
					node.exit <- true
					node.State = NODE_INIT

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

// Read an entry from a remote node
// index and subindex can either be strings or integers
// this method requires the corresponding node OD to be loaded
func (network *Network) Read(nodeId uint8, index any, subindex any) (value any, e error) {
	od, err := network.GetOD(nodeId)
	if err != nil {
		return nil, err
	}
	// Find corresponding Variable inside OD
	// This will be used to determine information on the expected value
	odVar, e := od.Index(index).SubIndex(subindex)
	if e != nil {
		return nil, e
	}
	data := make([]byte, odVar.DataLength)
	nbRead, e := network.sdoClient.ReadRaw(nodeId, odVar.Index, odVar.SubIndex, data)
	if e != SDO_ABORT_NONE {
		return nil, e
	}
	return decode(data[:nbRead], odVar.DataType)
}

// Read an entry from a remote node
// this method does not require corresponding OD to be loaded
// value will be read as a raw byte slice
// does not support block transfer
func (network *Network) ReadRaw(nodeId uint8, index uint16, subIndex uint8, data []byte) (int, error) {
	return network.sdoClient.ReadRaw(nodeId, index, subIndex, data)
}

// Write an entry to a remote node
// index and subindex can either be strings or integers
// this method requires the corresponding node OD to be loaded
// value should correspond to the expected datatype
func (network *Network) Write(nodeId uint8, index any, subindex any, value any) error {
	od, err := network.GetOD(nodeId)
	if err != nil {
		return err
	}
	// Find corresponding Variable inside OD
	// This will be used to determine information on the expected value
	odVar, e := od.Index(index).SubIndex(subindex)
	if e != nil {
		return e
	}
	// TODO : maybe check data type with the current OD ?
	var encoded []byte
	switch val := value.(type) {
	case uint8:
		encoded = []byte{val}
	case int8:
		encoded = []byte{byte(val)}
	case uint16:
		encoded = make([]byte, 2)
		binary.LittleEndian.PutUint16(encoded, val)
	case int16:
		encoded = make([]byte, 2)
		binary.LittleEndian.PutUint16(encoded, uint16(val))
	case uint32:
		encoded = make([]byte, 4)
		binary.LittleEndian.PutUint32(encoded, val)
	case int32:
		encoded = make([]byte, 4)
		binary.LittleEndian.PutUint32(encoded, uint32(val))
	case uint64:
		encoded = make([]byte, 8)
		binary.LittleEndian.PutUint64(encoded, val)
	case int64:
		encoded = make([]byte, 8)
		binary.LittleEndian.PutUint64(encoded, uint64(val))
	case string:
		encoded = []byte(val)
	default:
		return ODR_TYPE_MISMATCH
	}
	e = network.sdoClient.WriteRaw(nodeId, odVar.Index, odVar.SubIndex, encoded, false)
	if e == SDO_ABORT_NONE {
		return nil
	}
	return e
}

// Write an entry to a remote node
// this method does not require corresponding OD to be loaded
// value will be written as a raw byte slice
// does not support block transfer
func (network *Network) WriteRaw(nodeId uint8, index uint16, subIndex uint8, data []byte) error {
	return network.sdoClient.WriteRaw(nodeId, index, subIndex, data, false)
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
		odNode, err = ParseEDS(odType, nodeId)
		if err != nil {
			return nil, err
		}
	case ObjectDictionary:
		odNode = &odType
	default:
		return nil, fmt.Errorf("expecting string or ObjectDictionary got : %T", od)
	}
	// Create and initialize a CANopen node
	node := &Node{Config: nil, BusManager: network.busManager, NMT: nil}
	err = node.Init(nil, nil, odNode, nil, NMT_STARTUP_TO_OPERATIONAL, 500, 1000, 1000, true, nodeId)
	if err != nil {
		return nil, err
	}
	err = node.InitPDO()
	if err != nil {
		return nil, err
	}
	network.Nodes[nodeId] = node
	return node, nil
}

// Add a remote node with a given OD
// OD can be a path, ObjectDictionary or nil
// This function will load and parse Object dictionnary (OD) into memory
// If already present, OD will be overwritten
// User can then access the node via OD naming
// A same OD can be used for multiple nodes
// Loading from node using object x1020 is not supported yet
func (network *Network) AddNode(nodeId uint8, od any) error {
	var odNode *ObjectDictionary
	var err error
	if nodeId < 1 || nodeId > 127 {
		return fmt.Errorf("nodeId should be between 1 and 127, value given : %v", nodeId)
	}
	switch odType := od.(type) {
	case string:
		odNode, err = ParseEDS(odType, nodeId)
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
