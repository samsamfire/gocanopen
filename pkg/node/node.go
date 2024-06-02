package node

import (
	"sync"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/config"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/samsamfire/gocanopen/pkg/sdo"
)

const (
	NODE_INIT     uint8 = 0
	NODE_RUNNING  uint8 = 1
	NODE_RESETING uint8 = 2
	NODE_EXIT     uint8 = 3
)

type Node interface {
	ProcessTPDO(syncWas bool, timeDifferenceUs uint32, timerNextUs *uint32)
	ProcessRPDO(syncWas bool, timeDifferenceUs uint32, timerNextUs *uint32)
	ProcessSYNC(timeDifferenceUs uint32, timerNextUs *uint32) bool
	ProcessMain(enableGateway bool, timeDifferenceUs uint32, timerNextUs *uint32) uint8
	GetOD() *od.ObjectDictionary
	GetID() uint8
	GetState() uint8
	SetState(newState uint8)
	GetExitBackground() chan bool
	SetExitBackground(exit bool) // Exit background processing
	GetExit() chan bool
	SetExit(exit bool) // Exit node processing
	MainCallback()
	Wg() *sync.WaitGroup
}

type BaseNode struct {
	*canopen.BusManager
	*sdo.SDOClient
	mu             sync.Mutex
	od             *od.ObjectDictionary
	mainCallback   func(node Node)
	state          uint8
	id             uint8
	wgBackground   *sync.WaitGroup
	exitBackground chan bool
	exit           chan bool
}

func newBaseNode(
	bm *canopen.BusManager,
	odict *od.ObjectDictionary,
	nodeId uint8,
) (*BaseNode, error) {
	base := &BaseNode{
		BusManager:     bm,
		od:             odict,
		id:             nodeId,
		wgBackground:   &sync.WaitGroup{},
		exitBackground: make(chan bool),
		exit:           make(chan bool),
		state:          NODE_INIT,
	}
	sdoClient, err := sdo.NewSDOClient(bm, odict, nodeId, sdo.ClientTimeoutMs, nil)
	if err != nil {
		return nil, err
	}
	base.SDOClient = sdoClient
	return base, nil
}

func (node *BaseNode) GetOD() *od.ObjectDictionary {
	return node.od
}
func (node *BaseNode) GetID() uint8 {
	return node.id
}

func (node *BaseNode) GetState() uint8 {
	node.mu.Lock()
	defer node.mu.Unlock()
	return node.state
}

func (node *BaseNode) SetState(newState uint8) {
	node.mu.Lock()
	defer node.mu.Unlock()
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

func (node *BaseNode) Wg() *sync.WaitGroup {
	return node.wgBackground
}

func (node *BaseNode) SetMainCallback(mainCallback func(node Node)) {
	node.mainCallback = mainCallback
}

func (node *BaseNode) Configurator() *config.NodeConfigurator {
	return config.NewNodeConfigurator(node.id, node.SDOClient)
}
