package canopen

import (
	"errors"
	"os"

	log "github.com/sirupsen/logrus"
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

type Node struct {
	OD                 *ObjectDictionary
	BusManager         *BusManager
	NodeIdUnconfigured bool
	NMT                *NMT
	HBConsumer         *HBConsumer
	SDOclients         []*SDOClient
	SDOServers         []*SDOServer
	TPDOs              []*TPDO
	RPDOs              []*RPDO
	SYNC               *SYNC
	EM                 *EM
	TIME               *TIME
	MainCallback       func(args ...any)
	State              uint8
	id                 uint8
	reset              chan bool
	exit               chan bool
}

func (node *Node) processTPDO(syncWas bool, timeDifferenceUs uint32, timerNextUs *uint32) {
	if node.NodeIdUnconfigured {
		return
	}
	nmtIsOperational := node.NMT.GetInternalState() == NMT_OPERATIONAL
	for _, tpdo := range node.TPDOs {
		tpdo.process(timeDifferenceUs, timerNextUs, nmtIsOperational, syncWas)
	}
}

func (node *Node) processRPDO(syncWas bool, timeDifferenceUs uint32, timerNextUs *uint32) {
	if node.NodeIdUnconfigured {
		return
	}
	nmtIsOperational := node.NMT.GetInternalState() == NMT_OPERATIONAL
	for _, rpdo := range node.RPDOs {
		rpdo.process(timeDifferenceUs, timerNextUs, nmtIsOperational, syncWas)
	}
}

func (node *Node) processSync(timeDifferenceUs uint32, timerNextUs *uint32) bool {
	syncWas := false
	sync := node.SYNC
	if !node.NodeIdUnconfigured && sync != nil {

		nmtState := node.NMT.GetInternalState()
		nmtIsPreOrOperational := nmtState == NMT_PRE_OPERATIONAL || nmtState == NMT_OPERATIONAL
		syncProcess := sync.process(nmtIsPreOrOperational, timeDifferenceUs, timerNextUs)

		switch syncProcess {
		case CO_SYNC_NONE, CO_SYNC_RX_TX:
			syncWas = true
		case CO_SYNC_PASSED_WINDOW:
			node.BusManager.ClearSyncPDOs()
		}
	}
	return syncWas
}

// Process canopen objects that are not RT
// Does not process SYNC and PDOs
func (node *Node) processMain(enableGateway bool, timeDifferenceUs uint32, timerNextUs *uint32) uint8 {
	// Process all objects
	reset := RESET_NOT
	NMTState := node.NMT.GetInternalState()
	NMTisPreOrOperational := (NMTState == NMT_PRE_OPERATIONAL) || (NMTState == NMT_OPERATIONAL)

	node.BusManager.process()
	node.EM.process(NMTisPreOrOperational, timeDifferenceUs, timerNextUs)
	reset = node.NMT.process(&NMTState, timeDifferenceUs, timerNextUs)
	// Update NMTisPreOrOperational
	NMTisPreOrOperational = (NMTState == NMT_PRE_OPERATIONAL) || (NMTState == NMT_OPERATIONAL)

	// Process SDO servers
	for _, server := range node.SDOServers {
		server.process(NMTisPreOrOperational, timeDifferenceUs, timerNextUs)
	}
	node.HBConsumer.process(NMTisPreOrOperational, timeDifferenceUs, timerNextUs)
	node.TIME.process(NMTisPreOrOperational, timeDifferenceUs)

	return reset

}

// Initialize all PDOs
func (node *Node) initPDO() error {
	if node.id < 1 || node.id > 127 || node.NodeIdUnconfigured {
		if node.NodeIdUnconfigured {
			return ErrNodeIdUnconfiguredLSS
		} else {
			return ErrIllegalArgument
		}
	}
	// Iterate over all the possible entries : there can be a maximum of 512 maps
	// Break loops when an entry doesn't exist (don't allow holes in mapping)
	for i := uint16(0); i < 512; i++ {
		entry14xx := node.OD.Index(0x1400 + i)
		entry16xx := node.OD.Index(0x1600 + i)
		preDefinedIdent := uint16(0)
		pdoOffset := i % 4
		nodeIdOffset := i / 4
		preDefinedIdent = 0x200 + pdoOffset*0x100 + uint16(node.id) + nodeIdOffset
		rpdo, err := NewRPDO(node.BusManager, node.OD, node.EM, node.SYNC, entry14xx, entry16xx, preDefinedIdent)
		if err != nil {
			log.Warnf("[NODE][RPDO] no more RPDO after RPDO %v", i-1)
			break
		} else {
			node.RPDOs = append(node.RPDOs, rpdo)
		}
	}
	// Do the same for TPDOS
	for i := uint16(0); i < 512; i++ {
		entry18xx := node.OD.Index(0x1800 + i)
		entry1Axx := node.OD.Index(0x1A00 + i)
		preDefinedIdent := uint16(0)
		pdoOffset := i % 4
		nodeIdOffset := i / 4
		preDefinedIdent = 0x180 + pdoOffset*0x100 + uint16(node.id) + nodeIdOffset
		tpdo, err := NewTPDO(node.BusManager, node.OD, node.EM, node.SYNC, entry18xx, entry1Axx, preDefinedIdent)
		if err != nil {
			log.Warnf("[NODE][TPDO] no more TPDO after TPDO %v", i-1)
			break
		} else {
			node.TPDOs = append(node.TPDOs, tpdo)
		}

	}

	return nil
}

// Create a new node
func NewNode(
	busManager *BusManager,
	od *ObjectDictionary,
	nmt *NMT,
	emergency *EM,
	nodeId uint8,
	nmtControl uint16,
	firstHbTimeMs uint16,
	sdoServerTimeoutMs uint32,
	sdoClientTimeoutMs uint32,
	blockTransferEnabled bool,
	statusBits *Entry,

) (*Node, error) {

	if busManager == nil || od == nil {
		return nil, errors.New("need at least busManager and od parameters")
	}
	var err error
	node := &Node{}
	node.BusManager = busManager
	node.NodeIdUnconfigured = false
	node.OD = od
	node.exit = make(chan bool)
	node.reset = make(chan bool)
	node.id = nodeId
	node.State = NODE_INIT

	if emergency == nil {
		emergency, err := NewEM(
			busManager,
			nodeId,
			od.Index(0x1001),
			od.Index(0x1014),
			od.Index(0x1015),
			od.Index(0x1003),
			nil,
		)
		if err != nil {
			log.Errorf("[NODE][EMERGENCY producer] error when initializing emergency producer %v", err)
			return nil, ErrOdParameters
		}
		node.EM = emergency
	} else {
		node.EM = emergency
	}
	emergency = node.EM

	// NMT object can either be supplied or created with automatically with an OD entry
	if nmt == nil {
		nmt, err := NewNMT(
			busManager,
			emergency,
			nodeId,
			nmtControl,
			firstHbTimeMs,
			NMT_SERVICE_ID,
			NMT_SERVICE_ID,
			HEARTBEAT_SERVICE_ID+uint16(nodeId),
			od.Index(0x1017),
		)
		if err != nil {
			log.Errorf("[NODE][NMT] error when initializing NMT object %v", err)
			return nil, err
		} else {
			node.NMT = nmt
			log.Infof("[NODE][NMT] initialized from OD for node x%x", nodeId)
		}
	} else {
		node.NMT = nmt
		log.Infof("[NODE][NMT] initialized for node x%x", nodeId)
	}

	// Initialize HB consumer
	hbCons, err := NewHBConsumer(busManager, emergency, od.Index(0x1016))
	if err != nil {
		log.Errorf("[NODE][HB Consumer] error when initializing HB consummers %v", err)
		return nil, err
	} else {
		node.HBConsumer = hbCons
	}
	log.Infof("[NODE][HB Consumer] initialized for node x%x", nodeId)

	// Initialize SDO server
	// For now only one server
	entry1200 := od.Index(0x1200)
	sdoServers := make([]*SDOServer, 0)
	if entry1200 == nil {
		log.Warnf("[NODE][SDO SERVER] no sdo servers initialized for node x%x", nodeId)
	} else {
		server, err := NewSDOServer(busManager, od, nodeId, sdoServerTimeoutMs, entry1200)
		if err != nil {
			log.Errorf("[NODE][SDO SERVER] error when initializing SDO server object %v", err)
			return nil, err
		} else {
			sdoServers = append(sdoServers, server)
			node.SDOServers = sdoServers
			log.Infof("[NODE][SDO SERVER] initialized for node x%x", nodeId)
		}
	}

	// Initialize SDO clients if any
	// For now only one client
	entry1280 := od.Index(0x1280)
	sdoClients := make([]*SDOClient, 0)
	if entry1280 == nil {
		log.Info("[NODE][SDO CLIENT] no SDO clients initialized for node")
	} else {

		client, err := NewSDOClient(busManager, od, nodeId, sdoClientTimeoutMs, entry1280)
		if err != nil {
			log.Errorf("[NODE][SDO CLIENT] error when initializing SDO client object %v", err)
		} else {
			sdoClients = append(node.SDOclients, client)
			log.Infof("[NODE][SDO CLIENT] initialized for node x%x", nodeId)
		}
		node.SDOclients = sdoClients
	}

	//Initialize TIME
	time, err := NewTIME(busManager, od.Index(0x1012), 1000) // hardcoded for now
	if err != nil {
		log.Errorf("[NODE][TIME] error when initializing TIME object %v", err)
	} else {
		node.TIME = time
	}

	//Initialize SYNC
	sync, err := NewSYNC(
		busManager,
		emergency,
		od.Index(0x1005),
		od.Index(0x1006),
		od.Index(0x1007),
		od.Index(0x1019),
	)
	if err != nil {
		log.Errorf("[NODE][SYNC] error when initialising SYNC object %v", err)
	} else {
		node.SYNC = sync
	}

	//Add EDS storage if supported
	edsEntry := od.Index(0x1021)
	if edsEntry != nil {
		log.Info("[NODE][EDS] EDS is downloadable via object 0x1021")
		od.AddFile(edsEntry.Index, edsEntry.Name, od.filePath, os.O_RDONLY, os.O_RDONLY) // Don't allow to overwrite EDS
	}
	err = node.initPDO()
	if err != nil {
		return nil, err
	}
	return node, nil
}
