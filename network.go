package canopen

import (
	"fmt"
)

type Network struct {
	nodes      []Node
	bus        *Bus
	busManager *BusManager
	SDOClient  *SDOClient // Network master has an sdo client to read/write nodes on network
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
	network.SDOClient = client
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
	odVar, e := odInfo.od.Index(index).SubIndex(subindex)
	if e != nil {
		return nil, e
	}
	data := make([]byte, 10)
	_, e = network.SDOClient.ReadRaw(nodeId, odVar.Index, odVar.SubIndex, data)
	if e == SDO_ABORT_NONE {
		return data, nil
	}
	return data, e

}
