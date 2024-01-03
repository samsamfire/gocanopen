package canopen

import (
	"encoding/binary"
	"math"
	"sync"
)

const (
	NMT_SERVICE_ID       uint16 = 0
	EMERGENCY_SERVICE_ID uint16 = 0x80
	HEARTBEAT_SERVICE_ID uint16 = 0x700
	SDO_SERVER_ID        uint16 = 0x580
	SDO_CLIENT_ID        uint16 = 0x600
)

const (
	NODE_INIT     uint8 = 0
	NODE_RUNNING  uint8 = 1
	NODE_RESETING uint8 = 2
	NODE_EXIT     uint8 = 3
)

type BaseNode struct {
	*busManager
	od             *ObjectDictionary
	baseSdoClient  *SDOClient
	mainCallback   func(node Node)
	state          uint8
	id             uint8
	wgBackground   *sync.WaitGroup
	exitBackground chan bool
	exit           chan bool
}

func newBaseNode(
	bm *busManager,
	od *ObjectDictionary,
	nodeId uint8,
) (*BaseNode, error) {
	base := &BaseNode{
		busManager:     bm,
		od:             od,
		id:             nodeId,
		wgBackground:   &sync.WaitGroup{},
		exitBackground: make(chan bool),
		exit:           make(chan bool),
		state:          NODE_INIT,
	}
	sdoClient, err := NewSDOClient(bm, od, 0, DEFAULT_SDO_CLIENT_TIMEOUT_MS, nil)
	if err != nil {
		return nil, err
	}
	base.baseSdoClient = sdoClient
	return base, nil
}

func (node *BaseNode) GetOD() *ObjectDictionary {
	return node.od
}
func (node *BaseNode) GetID() uint8 {
	return node.id
}

func (node *BaseNode) GetState() uint8 {
	return node.state
}

func (node *BaseNode) SetState(newState uint8) {
	node.state = newState
}

func (node *BaseNode) GetExitBackground() chan bool {
	return node.exitBackground
}

func (node *BaseNode) SetExitBackground(exit bool) {
	node.exitBackground <- exit
}

func (node *BaseNode) GetExit() chan bool {
	return node.exit
}

func (node *BaseNode) SetExit(exit bool) {
	node.exit <- exit
}

func (node *BaseNode) wg() *sync.WaitGroup {
	return node.wgBackground
}

func (node *BaseNode) SetMainCallback(mainCallback func(node Node)) {
	node.mainCallback = mainCallback
}

// Helper function for reading a remote node entry as bytes
func (node *BaseNode) readBytes(index any, subindex any) ([]byte, uint8, error) {

	// Find corresponding Variable inside OD
	// This will be used to determine information on the expected value
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return nil, 0, err
	}
	data := make([]byte, odVar.DataLength())
	nbRead, err := node.baseSdoClient.ReadRaw(node.id, entry.Index, odVar.SubIndex, data)
	if err != nil {
		return nil, 0, err
	}
	return data[:nbRead], odVar.DataType, nil
}

// Read an entry using a base sdo client
// index and subindex can either be strings or integers
// this method requires the corresponding node OD to be loaded
// Returned value can be either string, uint64, int64 or float64
func (node *BaseNode) Read(index any, subindex any) (value any, e error) {
	data, dataType, err := node.readBytes(index, subindex)
	if err != nil {
		return nil, err
	}
	return decode(data, dataType)
}

// Same as Read but enforces the returned type as uint64
func (node *BaseNode) ReadUint(index any, subindex any) (value uint64, e error) {
	data, dataType, err := node.readBytes(index, subindex)
	if err != nil {
		return 0, err
	}
	e = checkSize(len(data), dataType)
	if e != nil {
		return 0, e
	}
	// Cast to correct type
	switch dataType {
	case BOOLEAN, UNSIGNED8:
		return uint64(data[0]), nil
	case UNSIGNED16:
		return uint64(binary.LittleEndian.Uint16(data)), nil
	case UNSIGNED32:
		return uint64(binary.LittleEndian.Uint32(data)), nil
	case UNSIGNED64:
		return uint64(binary.LittleEndian.Uint64(data)), nil
	default:
		return 0, ODR_TYPE_MISMATCH
	}
}

// Same as Read but enforces the returned type as int64
func (node *BaseNode) ReadInt(index any, subindex any) (value int64, e error) {
	data, dataType, err := node.readBytes(index, subindex)
	if err != nil {
		return 0, err
	}
	e = checkSize(len(data), dataType)
	if e != nil {
		return 0, e
	}
	// Cast to correct type
	switch dataType {
	case BOOLEAN, INTEGER8:
		return int64(data[0]), nil
	case INTEGER16:
		return int64(int16(binary.LittleEndian.Uint16(data))), nil
	case INTEGER32:
		return int64(int32(binary.LittleEndian.Uint32(data))), nil
	case INTEGER64:
		return int64(binary.LittleEndian.Uint64(data)), nil
	default:
		return 0, ODR_TYPE_MISMATCH
	}
}

// Same as Read but enforces the returned type as float
func (node *BaseNode) ReadFloat(index any, subindex any) (value float64, e error) {
	data, dataType, err := node.readBytes(index, subindex)
	if err != nil {
		return 0, err
	}
	e = checkSize(len(data), dataType)
	if e != nil {
		return 0, e
	}
	// Cast to correct type
	switch dataType {
	case REAL32:
		parsed := binary.LittleEndian.Uint32(data)
		return float64(math.Float32frombits(parsed)), nil
	case REAL64:
		parsed := binary.LittleEndian.Uint64(data)
		return math.Float64frombits(parsed), nil
	default:
		return 0, ODR_TYPE_MISMATCH
	}
}

// Same as Read but enforces the returned type as string
func (node *BaseNode) ReadString(index any, subindex any) (value string, e error) {
	data, dataType, err := node.readBytes(index, subindex)
	if err != nil {
		return "", err
	}
	e = checkSize(len(data), dataType)
	if e != nil {
		return "", e
	}
	// Cast to correct type
	switch dataType {
	case OCTET_STRING, VISIBLE_STRING, UNICODE_STRING:
		return string(data), nil
	default:
		return "", ODR_TYPE_MISMATCH
	}
}

// Read an entry from a remote node
// this method does not require corresponding OD to be loaded
// value will be read as a raw byte slice
// does not support block transfer
func (node *BaseNode) ReadRaw(index uint16, subIndex uint8, data []byte) (int, error) {
	return node.baseSdoClient.ReadRaw(node.id, index, subIndex, data)
}

// Write an entry to a remote node
// index and subindex can either be strings or integers
// this method requires the corresponding node OD to be loaded
// value should correspond to the expected datatype
func (node *BaseNode) Write(index any, subindex any, value any) error {
	// Find corresponding Variable inside OD
	// This will be used to determine information on the expected value
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return err
	}

	err = node.baseSdoClient.WriteRaw(node.id, entry.Index, odVar.SubIndex, value, false)
	if err != nil {
		return err
	}
	return nil
}

// Write an entry to a remote node
// this method does not require corresponding OD to be loaded
// value will be written as a raw byte slice
// does not support block transfer
func (node *BaseNode) WriteRaw(index uint16, subIndex uint8, data []byte) error {
	return node.baseSdoClient.WriteRaw(node.id, index, subIndex, data, false)
}

type Node interface {
	ProcessTPDO(syncWas bool, timeDifferenceUs uint32, timerNextUs *uint32)
	ProcessRPDO(syncWas bool, timeDifferenceUs uint32, timerNextUs *uint32)
	ProcessSync(timeDifferenceUs uint32, timerNextUs *uint32) bool
	ProcessMain(enableGateway bool, timeDifferenceUs uint32, timerNextUs *uint32) uint8
	GetOD() *ObjectDictionary
	GetID() uint8
	GetState() uint8
	SetState(newState uint8)
	GetExitBackground() chan bool
	SetExitBackground(exit bool) // Exit background processing
	GetExit() chan bool
	SetExit(exit bool) // Exit node processing
	MainCallback()
	wg() *sync.WaitGroup
}

// NodeConfigurator provides helper functions for configuring
// a CANopen node using SDO and the definitions provided by
// CiA 301.
// These configurations being standard, they do not require any
// EDS file.
type NodeConfigurator struct {
	RPDO PDOConfigurator
	TPDO PDOConfigurator
	SYNC SYNCConfigurator
	HB   HBConfigurator
	NMT  NMTConfigurator
	// Others to come
}

// Create a new [NodeConfigurator] given a specific ID and an SDOClient
func NewNodeConfigurator(nodeId uint8, client *SDOClient) NodeConfigurator {
	configurator := NodeConfigurator{}
	configurator.RPDO = *NewRPDOConfigurator(nodeId, client)
	configurator.TPDO = *NewTPDOConfigurator(nodeId, client)
	configurator.SYNC = *NewSYNCConfigurator(nodeId, client)
	configurator.HB = *NewHBConfigurator(nodeId, client)
	configurator.NMT = *NewNMTConfigurator(nodeId, client)
	return configurator
}
