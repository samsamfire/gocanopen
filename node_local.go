package canopen

import (
	"errors"
	"os"

	log "github.com/sirupsen/logrus"
)

// A LocalNode is a CiA 301 compliant CANopen node
// It supports all the standard CANopen objects.
// These objects will be loaded depending on the given EDS file.
// For configuration of the different CANopen objects see [NodeConfigurator].
type LocalNode struct {
	*BaseNode
	NodeIdUnconfigured bool
	NMT                *NMT
	HBConsumer         *HBConsumer
	SDOclients         []*SDOClient
	SDOServers         []*SDOServer
	TPDOs              []*TPDO
	RPDOs              []*RPDO
	SYNC               *SYNC
	EMCY               *EMCY
	TIME               *TIME
}

func (node *LocalNode) ProcessTPDO(syncWas bool, timeDifferenceUs uint32, timerNextUs *uint32) {
	if node.NodeIdUnconfigured {
		return
	}
	nmtIsOperational := node.NMT.GetInternalState() == NMT_OPERATIONAL
	for _, tpdo := range node.TPDOs {
		tpdo.process(timeDifferenceUs, timerNextUs, nmtIsOperational, syncWas)
	}
}

func (node *LocalNode) ProcessRPDO(syncWas bool, timeDifferenceUs uint32, timerNextUs *uint32) {
	if node.NodeIdUnconfigured {
		return
	}
	nmtIsOperational := node.NMT.GetInternalState() == NMT_OPERATIONAL
	for _, rpdo := range node.RPDOs {
		rpdo.process(timeDifferenceUs, timerNextUs, nmtIsOperational, syncWas)
	}
}

func (node *LocalNode) ProcessSync(timeDifferenceUs uint32, timerNextUs *uint32) bool {
	syncWas := false
	sync := node.SYNC
	if !node.NodeIdUnconfigured && sync != nil {

		nmtState := node.NMT.GetInternalState()
		nmtIsPreOrOperational := nmtState == NMT_PRE_OPERATIONAL || nmtState == NMT_OPERATIONAL
		syncProcess := sync.process(nmtIsPreOrOperational, timeDifferenceUs, timerNextUs)

		switch syncProcess {
		case syncRxOrTx:
			syncWas = true
		case syncPassedWindow:
		default:
		}
	}
	return syncWas
}

// Process canopen objects that are not RT
// Does not process SYNC and PDOs
func (node *LocalNode) ProcessMain(enableGateway bool, timeDifferenceUs uint32, timerNextUs *uint32) uint8 {
	// Process all objects
	NMTState := node.NMT.GetInternalState()
	NMTisPreOrOperational := (NMTState == NMT_PRE_OPERATIONAL) || (NMTState == NMT_OPERATIONAL)

	node.busManager.process()
	node.EMCY.process(NMTisPreOrOperational, timeDifferenceUs, timerNextUs)
	reset := node.NMT.process(&NMTState, timeDifferenceUs, timerNextUs)
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

func (node *LocalNode) MainCallback() {
	if node.mainCallback != nil {
		node.mainCallback(node)
	}
}

// Initialize all PDOs
func (node *LocalNode) initPDO() error {
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
		entry14xx := node.GetOD().Index(0x1400 + i)
		entry16xx := node.GetOD().Index(0x1600 + i)
		preDefinedIdent := uint16(0)
		pdoOffset := i % 4
		nodeIdOffset := i / 4
		preDefinedIdent = 0x200 + pdoOffset*0x100 + uint16(node.id) + nodeIdOffset
		rpdo, err := NewRPDO(node.busManager, node.GetOD(), node.EMCY, node.SYNC, entry14xx, entry16xx, preDefinedIdent)
		if err != nil {
			log.Warnf("[NODE][RPDO] no more RPDO after RPDO %v", i-1)
			break
		} else {
			node.RPDOs = append(node.RPDOs, rpdo)
		}
	}
	// Do the same for TPDOS
	for i := uint16(0); i < 512; i++ {
		entry18xx := node.GetOD().Index(0x1800 + i)
		entry1Axx := node.GetOD().Index(0x1A00 + i)
		preDefinedIdent := uint16(0)
		pdoOffset := i % 4
		nodeIdOffset := i / 4
		preDefinedIdent = 0x180 + pdoOffset*0x100 + uint16(node.id) + nodeIdOffset
		tpdo, err := NewTPDO(node.busManager, node.GetOD(), node.EMCY, node.SYNC, entry18xx, entry1Axx, preDefinedIdent)
		if err != nil {
			log.Warnf("[NODE][TPDO] no more TPDO after TPDO %v", i-1)
			break
		} else {
			node.TPDOs = append(node.TPDOs, tpdo)
		}

	}

	return nil
}

// Create a new local node
func newLocalNode(
	bm *busManager,
	od *ObjectDictionary,
	nmt *NMT,
	emergency *EMCY,
	nodeId uint8,
	nmtControl uint16,
	firstHbTimeMs uint16,
	sdoServerTimeoutMs uint32,
	sdoClientTimeoutMs uint32,
	blockTransferEnabled bool,
	statusBits *Entry,

) (*LocalNode, error) {

	if bm == nil || od == nil {
		return nil, errors.New("need at least busManager and od parameters")
	}
	base, err := newBaseNode(bm, od, nodeId)
	if err != nil {
		return nil, err
	}
	node := &LocalNode{BaseNode: base}
	node.NodeIdUnconfigured = false
	node.od = od
	node.exitBackground = make(chan bool)
	node.exit = make(chan bool)
	node.id = nodeId
	node.state = NODE_INIT

	if emergency == nil {
		emergency, err := NewEM(
			bm,
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
		node.EMCY = emergency
	} else {
		node.EMCY = emergency
	}
	emergency = node.EMCY

	// NMT object can either be supplied or created with automatically with an OD entry
	if nmt == nil {
		nmt, err := NewNMT(
			bm,
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
	hbCons, err := NewHBConsumer(bm, emergency, od.Index(0x1016))
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
		server, err := NewSDOServer(bm, od, nodeId, sdoServerTimeoutMs, entry1200)
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

		client, err := NewSDOClient(bm, od, nodeId, sdoClientTimeoutMs, entry1280)
		if err != nil {
			log.Errorf("[NODE][SDO CLIENT] error when initializing SDO client object %v", err)
		} else {
			sdoClients = append(node.SDOclients, client)
			log.Infof("[NODE][SDO CLIENT] initialized for node x%x", nodeId)
		}
		node.SDOclients = sdoClients
	}

	//Initialize TIME
	time, err := NewTIME(bm, od.Index(0x1012), 1000) // hardcoded for now
	if err != nil {
		log.Errorf("[NODE][TIME] error when initializing TIME object %v", err)
	} else {
		node.TIME = time
	}

	//Initialize SYNC
	sync, err := NewSYNC(
		bm,
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
