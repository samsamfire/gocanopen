package node

import (
	"log/slog"
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

// A [Node] handles the CANopen stack.
type Node interface {
	ProcessTPDO(syncWas bool, timeDifferenceUs uint32, timerNextUs *uint32)
	ProcessRPDO(syncWas bool, timeDifferenceUs uint32, timerNextUs *uint32)
	ProcessSYNC(timeDifferenceUs uint32, timerNextUs *uint32) bool
	ProcessMain(enableGateway bool, timeDifferenceUs uint32, timerNextUs *uint32) uint8
	GetOD() *od.ObjectDictionary
	GetID() uint8
	Export(filename string) error
	MainCallback()
}

type BaseNode struct {
	*canopen.BusManager
	*sdo.SDOClient
	logger       *slog.Logger
	mu           sync.Mutex
	od           *od.ObjectDictionary
	mainCallback func(node Node)
	id           uint8
	rxBuffer     []byte
}

func newBaseNode(
	bm *canopen.BusManager,
	logger *slog.Logger,
	odict *od.ObjectDictionary,
	nodeId uint8,
) (*BaseNode, error) {
	base := &BaseNode{
		BusManager: bm,
		od:         odict,
		id:         nodeId,
	}
	sdoClient, err := sdo.NewSDOClient(bm, odict, nodeId, sdo.DefaultClientTimeout, nil)
	if err != nil {
		return nil, err
	}
	base.SDOClient = sdoClient
	if logger == nil {
		base.logger = slog.Default()
	} else {
		base.logger = logger
	}
	base.rxBuffer = make([]byte, 1000)
	return base, nil
}

func (node *BaseNode) GetOD() *od.ObjectDictionary {
	return node.od
}
func (node *BaseNode) GetID() uint8 {
	return node.id
}

func (node *BaseNode) SetMainCallback(mainCallback func(node Node)) {
	node.mainCallback = mainCallback
}

func (node *BaseNode) Configurator() *config.NodeConfigurator {
	return config.NewNodeConfigurator(node.id, node.logger, node.SDOClient)
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
