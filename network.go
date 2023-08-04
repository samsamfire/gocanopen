package canopen

import (
	"encoding/binary"
	"fmt"
)

type Network struct {
	nodes           []Node
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
	return Network{nodes: make([]Node, 0), busManager: &BusManager{Bus: bus}, odMap: map[uint8]*ObjectDictionaryInformation{}}
}

// Connect and process canopen stack in background
// This is the main method which should always be called
func (network *Network) ConnectAndProcess(can_interface any, channel any, bitrate int) error {
	// If no bus was given during NewNetwork call, then default to socketcan
	var bus Bus
	busManager := network.busManager
	if busManager.Bus == nil {
		channel_str, ok := channel.(string)
		if !ok {
			return fmt.Errorf("expecting a string for the can channel")
		}
		if can_interface != "socketcan" && can_interface != "" {
			return fmt.Errorf("only socketcan is supported currently")
		}
		b, e := NewSocketcanBus(channel_str)
		if e != nil {
			return fmt.Errorf("could not connect to socketcan channel %v , because %v", channel, e)
		}
		bus = &b
	} else {
		bus = busManager.Bus
	}
	// Init bus, connect and subscribe to CAN message reception
	busManager.Init(bus)
	bus.Subscribe(busManager)
	e := bus.Connect()
	if e != nil {
		return e
	}
	// Add SDO client to network by default
	client := &SDOClient{}
	e = client.Init(nil, nil, 0, busManager)
	if e != nil {
		return e
	}
	network.sdoClient = client
	// Add NMT tx buffer, for sending NMT commands
	network.nmtMasterTxBuff, _, e = busManager.InsertTxBuffer(uint32(NMT_SERVICE_ID), false, 2, false)
	if e != nil {
		return e
	}
	return e
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

// Read an entry from a remote node
// index and subindex can either be strings or integers
// this method requires the corresponding node OD to be loaded
func (network *Network) Read(nodeId uint8, index any, subindex any) (value any, e error) {
	odInfo, odLoaded := network.odMap[nodeId]
	if !odLoaded {
		return nil, ODR_OD_MISSING
	}
	// Find corresponding Variable inside OD
	// This will be used to determine information on the expected value
	odVar, e := odInfo.od.Index(index).SubIndex(subindex)
	if e != nil {
		return nil, e
	}
	data := make([]byte, 1000)
	nbRead, e := network.sdoClient.ReadRaw(nodeId, odVar.Index, odVar.SubIndex, data)
	if e != SDO_ABORT_NONE {
		return nil, e
	}
	return decode(data[:nbRead], odVar.DataType)

}

// Write an entry to a remote node
// index and subindex can either be strings or integers
// this method requires the corresponding node OD to be loaded
// value should correspond to the expected datatype
func (network *Network) Write(nodeId uint8, index any, subindex any, value any) error {
	odInfo, odLoaded := network.odMap[nodeId]
	if !odLoaded {
		return ODR_OD_MISSING
	}
	// Find corresponding Variable inside OD
	// This will be used to determine information on the expected value
	odVar, e := odInfo.od.Index(index).SubIndex(subindex)
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

// Send NMT commands to remote nodes
// Id 0 is used as a broadcast command i.e. affects all nodes
func (network *Network) Command(nodeId uint8, nmtCommand NMTCommand) error {
	if nodeId > 127 || (nmtCommand != NMT_ENTER_OPERATIONAL &&
		nmtCommand != NMT_ENTER_PRE_OPERATIONAL &&
		nmtCommand != NMT_ENTER_STOPPED &&
		nmtCommand != NMT_RESET_COMMUNICATION &&
		nmtCommand != NMT_RESET_NODE) {
		return CO_ERROR_ILLEGAL_ARGUMENT
	}
	network.nmtMasterTxBuff.Data[0] = uint8(nmtCommand)
	network.nmtMasterTxBuff.Data[1] = nodeId
	return network.busManager.Send((*network.nmtMasterTxBuff))
}
