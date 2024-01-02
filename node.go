package canopen

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
	MainCallback   func(args ...any)
	state          uint8
	id             uint8
	exitBackground chan bool
	exit           chan bool
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
}

type NodeConfigurator struct {
	RPDO PDOConfigurator
	TPDO PDOConfigurator
	SYNC SYNCConfigurator
	HB   HBConfigurator
	NMT  NMTConfigurator
	// Others to come
}

func NewNodeConfigurator(nodeId uint8, client *SDOClient) NodeConfigurator {
	configurator := NodeConfigurator{}
	configurator.RPDO = *NewRPDOConfigurator(nodeId, client)
	configurator.TPDO = *NewTPDOConfigurator(nodeId, client)
	configurator.SYNC = *NewSYNCConfigurator(nodeId, client)
	configurator.HB = *NewHBConfigurator(nodeId, client)
	configurator.NMT = *NewNMTConfigurator(nodeId, client)
	return configurator
}
