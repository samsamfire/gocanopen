package node

import (
	"sync"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/config"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/samsamfire/gocanopen/pkg/sdo"
	log "github.com/sirupsen/logrus"
)

const (
	NodeInit     uint8 = 0
	NodeRunning  uint8 = 1
	NodeReseting uint8 = 2
	NodeExit     uint8 = 3
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
	Export(filename string) error
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
		state:          NodeInit,
	}
	sdoClient, err := sdo.NewSDOClient(bm, odict, nodeId, sdo.DefaultClientTimeout, nil)
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

// Export EDS file with current state
func (node *BaseNode) Export(filename string) error {
	countRead := 0
	countErrors := 0
	for index, entry := range node.GetOD().Entries() {
		if entry.ObjectType == od.ObjectTypeDOMAIN {
			log.Warnf("skipping domain object %x", index)
			continue
		}
		for j := range uint8(entry.SubCount()) {
			buffer := make([]byte, 100)
			n, err := node.ReadRaw(index, j, buffer)
			if err != nil {
				countErrors++
				log.Warnf("failed to read remote value %x|%x : %v", index, j, err)
				continue
			}
			err = entry.WriteExactly(j, buffer[:n], true)
			if err != nil {
				log.Warnf("failed to write remote value to local od %x|%x : %v", index, j, err)
				countErrors++
				continue
			}
			countRead++
		}
	}
	log.Infof("dump successful, read : %v, errors : %v", countRead, countErrors)
	return od.ExportEDS(node.GetOD(), false, filename)
}
