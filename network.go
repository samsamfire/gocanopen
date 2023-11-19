package canopen

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

type Network struct {
	Nodes           map[uint8]*Node
	bus             *Bus
	busManager      *BusManager
	sdoClient       *SDOClient // Network master has an sdo client to read/write nodes on network
	nmtMasterTxBuff *BufferTxFrame
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

// Create bus instance and add basic functionality : SDO, NMT
func (network *Network) Connect(canInterface any, channel any, bitrate int) error {
	// If no bus was given during NewNetwork call, then default to socketcan
	var bus Bus
	busManager := network.busManager
	if busManager.Bus == nil {
		channelStr, ok := channel.(string)
		if !ok {
			return fmt.Errorf("expecting a string for the can channel")
		}
		var err error
		switch canInterface {
		case "socketcan", "":
			bus, err = NewSocketcanBus(channelStr)
		case "virtualcan":
			bus = NewVirtualCanBus(channelStr)
		default:
			return fmt.Errorf("%v is not a supported bus interface :(", err)
		}
		if err != nil {
			return fmt.Errorf("could not connect to can channel %v , because %v", channel, err)
		}
		busManager.Bus = bus
	} else {
		bus = busManager.Bus
	}
	// Init bus, connect and subscribe to CAN message reception
	e := bus.Connect()
	if e != nil {
		return e
	}
	bus.Subscribe(busManager)
	// Add SDO client to network by default
	client := &SDOClient{}
	e = client.Init(nil, nil, 0, busManager)
	if e != nil {
		return e
	}
	network.sdoClient = client
	// Add NMT tx buffer, for sending NMT commands
	network.nmtMasterTxBuff, e = busManager.InsertTxBuffer(uint32(NMT_SERVICE_ID), false, 2, false)
	if e != nil {
		return e
	}
	return e
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
								syncWas := node.ProcessSYNC(timeDifferenceUs, nil)
								node.ProcessTPDO(syncWas, timeDifferenceUs, nil)
								node.ProcessRPDO(syncWas, timeDifferenceUs, nil)
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

// This function will load and parse Object dictionnary (OD) into memory
// If already present, OD will be overwritten
// User can then access the node via OD naming
// A same OD can be used for multiple nodes
// An OD can also be downloaded from remote nodes via block transfer
// A callback function is available if any custom format is used for EDS custom storage
func (network *Network) LoadOD(nodeId uint8, edsPath string, edsCustomStorageCallback *func()) error {
	if edsPath == "" {
		return fmt.Errorf("loading EDS from node is not implemented yet")
	}
	if nodeId < 1 || nodeId > 127 {
		return fmt.Errorf("nodeId should be between 1 and 127, value given : %v", nodeId)
	}
	od, e := ParseEDS(edsPath, nodeId)
	if e != nil {
		return e
	}
	network.odMap[nodeId] = &ObjectDictionaryInformation{nodeId: nodeId, od: od, edsPath: edsPath}
	return nil
}

// Check if OD exists for the given node
func (network *Network) IsODLoaded(nodeId uint8) bool {
	_, odLoaded := network.odMap[nodeId]
	return odLoaded
}

// Read an entry from a remote node
// index and subindex can either be strings or integers
// this method requires the corresponding node OD to be loaded
func (network *Network) Read(nodeId uint8, index any, subindex any) (value any, e error) {
	if !network.IsODLoaded(nodeId) {
		return nil, ODR_OD_MISSING
	}
	// Find corresponding Variable inside OD
	// This will be used to determine information on the expected value
	odVar, e := network.odMap[nodeId].od.Index(index).SubIndex(subindex)
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
	if !network.IsODLoaded(nodeId) {
		return ODR_OD_MISSING
	}
	// Find corresponding Variable inside OD
	// This will be used to determine information on the expected value
	odVar, e := network.odMap[nodeId].od.Index(index).SubIndex(subindex)
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
	network.nmtMasterTxBuff.Data[0] = uint8(nmtCommand)
	network.nmtMasterTxBuff.Data[1] = nodeId
	log.Debugf("[NMT] sending nmt command : %v to node(s) %v (x%x)", NMT_COMMAND_MAP[nmtCommand], nodeId, nodeId)
	return network.busManager.Send((*network.nmtMasterTxBuff))
}

// Add a local node to the network given an Object Dictionary
func (network *Network) AddNodeFromOD(nodeId uint8, objectDictionary *ObjectDictionary) error {
	// Create and initialize a CANopen node
	node := &Node{Config: nil, BusManager: network.busManager, NMT: nil}
	err := node.Init(nil, nil, objectDictionary, nil, NMT_STARTUP_TO_OPERATIONAL, 500, 1000, 1000, true, nodeId)
	if err != nil {
		return err
	}
	err = node.InitPDO(objectDictionary, nodeId)
	if err != nil {
		return nil
	}
	network.Nodes[nodeId] = node
	return nil
}

// Add a local node to the network given an EDS path
func (network *Network) AddNodeFromEDS(nodeId uint8, eds string) error {
	od, err := ParseEDS(eds, nodeId)
	if err != nil {
		return err
	}
	return network.AddNodeFromOD(nodeId, od)
}
