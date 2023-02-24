package canopen

import log "github.com/sirupsen/logrus"

type Configuration struct{}

const (
	NMT_SERVICE_ID       uint16 = 0
	EMERGENCY_SERVICE_ID uint16 = 0x80
	HEARTBEAT_SERVICE_ID uint16 = 0x700
	SDO_SERVER_ID        uint16 = 0x580
	SDO_CLIENT_ID        uint16 = 0x600
)

type Node struct {
	Config             *Configuration
	CANModule          *CANModule
	NMT                *NMT
	SDOclients         []*SDOClient
	SDOServers         []*SDOServer
	TPDOs              []*TPDO
	RPDOs              []*RPDO
	SYNC               *SYNC
	EM                 *EM
	TIME               *TIME
	NodeIdUnconfigured bool
}

/* Create a new canopen management object */
func NewNode(configuration *Configuration) *Node {
	return &Node{Config: configuration}
}

/* Process SRDO part */
func (node *Node) ProcessSRDO(time_difference_us uint32) (timer_next_us uint32) {
	// Process SRDO object
	return 0
}

/* Process TPDO */
func (node *Node) ProcessTPDO(syncWas bool, timeDifferenceUs uint32, timerNextUs *uint32) {
	// Process TPDO object
	if node.NodeIdUnconfigured {
		return
	}
	nmtIsOperational := node.NMT.GetInternalState() == CO_NMT_OPERATIONAL
	for _, tpdo := range node.TPDOs {
		tpdo.Process(timeDifferenceUs, timerNextUs, nmtIsOperational, syncWas)
	}
}

/* Process RPDO */
func (node *Node) ProcessRPDO(sync_was bool, time_difference_us uint32) (timer_next_us uint32) {
	// Process RPDO object
	return 0
}

/* Process SYNC */
func (node *Node) ProcessSYNC(timeDifferenceUs uint32, timerNextUs *uint32) bool {
	syncWas := false
	sync := node.SYNC
	if !node.NodeIdUnconfigured && sync != nil {

		nmtState := node.NMT.GetInternalState()
		nmtIsPreOrOperational := nmtState == CO_NMT_PRE_OPERATIONAL || nmtState == CO_NMT_OPERATIONAL
		syncProcess := sync.Process(nmtIsPreOrOperational, timeDifferenceUs, timerNextUs)

		switch syncProcess {
		case CO_SYNC_NONE:
		case CO_SYNC_RX_TX:
			syncWas = true
		case CO_SYNC_PASSED_WINDOW:
			node.CANModule.ClearSyncPDOs()
		}
	}
	return syncWas
}

/* Process all objects */
func (node *Node) Process(enable_gateway bool, time_difference_us uint32, timer_next_us *uint32) uint8 {
	// Process all objects
	reset := CO_RESET_NOT
	NMTState := node.NMT.GetInternalState()
	NMTisPreOrOperational := (NMTState == CO_NMT_PRE_OPERATIONAL) || (NMTState == CO_NMT_OPERATIONAL)

	// CAN stuff to process
	node.CANModule.Process()
	node.EM.Process(NMTisPreOrOperational, time_difference_us, timer_next_us)
	reset = node.NMT.Process(&NMTState, time_difference_us, timer_next_us)
	// Update NMTisPreOrOperational
	NMTisPreOrOperational = (NMTState == CO_NMT_PRE_OPERATIONAL) || (NMTState == CO_NMT_OPERATIONAL)

	// Process SDO servers
	for _, server := range node.SDOServers {
		server.Process(NMTisPreOrOperational, time_difference_us, timer_next_us)
	}
	// Process TIME object
	node.TIME.Process(NMTisPreOrOperational, time_difference_us)

	return reset

}

/*Initialize all PDOs*/
func (node *Node) InitPDO(od *ObjectDictionary, nodeId uint8) error {
	if nodeId < 1 || nodeId > 127 || node.NodeIdUnconfigured {
		if node.NodeIdUnconfigured {
			return CO_ERROR_NODE_ID_UNCONFIGURED_LSS
		} else {
			return CO_ERROR_ILLEGAL_ARGUMENT
		}
	}
	// Iterate over all the possible entries : there can be a maximum of 512 maps
	// Break loops as soon as entry doesn't exist (don't allow holes in mapping)
	for i := uint16(0); i < 512; i++ {
		entry14xx := od.Find(0x1400 + i)
		entry16xx := od.Find(0x1600 + i)
		preDefinedIdent := uint16(0)
		pdoOffset := i % 4
		nodeIdOffset := i / 4
		preDefinedIdent = 0x200 + pdoOffset*0x100 + uint16(nodeId) + nodeIdOffset
		rpdo := RPDO{}
		err := rpdo.Init(od, node.EM, node.SYNC, preDefinedIdent, entry14xx, entry16xx, node.CANModule)
		if err != nil {
			log.Warnf("Failed to Initialize RPDO%v, stopping there", i)
			break
		} else {
			log.Infof("Initialized RPDO%v", i)
			node.RPDOs = append(node.RPDOs, &rpdo)
		}
	}
	// Do the same for TPDOS
	for i := uint16(0); i < 512; i++ {
		entry18xx := od.Find(0x1800 + i)
		entry1Axx := od.Find(0x1A00 + i)
		preDefinedIdent := uint16(0)
		pdoOffset := i % 4
		nodeIdOffset := i / 4
		preDefinedIdent = 0x180 + pdoOffset*0x100 + uint16(nodeId) + nodeIdOffset
		tpdo := TPDO{}
		err := tpdo.Init(od, node.EM, node.SYNC, preDefinedIdent, entry18xx, entry1Axx, node.CANModule)
		if err != nil {
			log.Warnf("Failed to Initialize TPDO%v, stopping there", i)
			break
		} else {
			log.Infof("Initialized TPDO%v", i)
			node.TPDOs = append(node.TPDOs, &tpdo)
		}

	}

	return nil
}

/*Initialize CANopen stack */
func (node *Node) Init(
	nmt *NMT,
	emergency *EM,
	od *ObjectDictionary,
	statusBits *Entry,
	nmtControl uint16,
	firstHbTimeMs uint16,
	sdoServerTimeoutMs uint16,
	sdoClientTimeoutMs uint16,
	blockTransferEnabled bool,
	nodeId uint8,

) error {
	var err error
	node.NodeIdUnconfigured = false

	if emergency == nil {
		node.EM = &EM{}
	} else {
		node.EM = emergency
	}
	// Initialize EM object
	err = node.EM.Init(
		node.CANModule,
		od.Find(0x1001),
		od.Find(0x1014),
		od.Find(0x1015),
		od.Find(0x1003),
		nil,
		nodeId,
	)
	if err != nil {
		log.Errorf("[EMERGENCY producer] error when initializing emergency producer %v", err)
		return CO_ERROR_OD_PARAMETERS
	}

	// NMT object can either be supplied or created with OD entry
	if nmt == nil {
		node.NMT = &NMT{}
	} else {
		node.NMT = nmt
	}
	// Initialize NMT
	entry1017 := od.Find(0x1017)
	if entry1017 == nil {
		return CO_ERROR_OD_PARAMETERS
	}
	err = node.NMT.Init(entry1017, nil, nodeId, nmtControl, firstHbTimeMs, node.CANModule, NMT_SERVICE_ID, NMT_SERVICE_ID, HEARTBEAT_SERVICE_ID+uint16(nodeId))
	if err != nil {
		log.Errorf("Error when initializing NMT object %v", err)
		return err
	} else {
		log.Infof("NMT initialized for node x%x", nodeId)
	}

	// Initialize SDO server
	// For now only one server
	entry1200 := od.Find(0x1200)
	if entry1200 == nil {
		log.Warnf("No SDO servers initialized in node x%x", nodeId)
	} else {
		node.SDOServers = make([]*SDOServer, 0)
		server := &SDOServer{}
		err = server.Init(od, entry1200, nodeId, sdoServerTimeoutMs, node.CANModule)
		if err != nil {
			log.Errorf("Error when initializing SDO server object %v", err)
			return err
		}
		node.SDOServers = append(node.SDOServers, server)
		log.Infof("SDO server initialized for node x%x", nodeId)
	}

	// Initialize SDO clients if any
	// For now only one client
	entry1280 := od.Find(0x1280)
	if entry1280 == nil {
		log.Info("No SDO clients initialized in node")
	} else {
		node.SDOclients = make([]*SDOClient, 0)
		client := &SDOClient{}
		err = client.Init(od, entry1280, nodeId, node.CANModule)
		if err != nil {
			log.Errorf("Error when initializing SDO client object %v", err)
		}
		node.SDOclients = append(node.SDOclients, client)
		log.Infof("SDO client initialized for node x%x", nodeId)
	}
	//Initialize TIME
	time := &TIME{}
	node.TIME = time
	err = time.Init(od.Find(0x1012), node.CANModule, 1000)
	if err != nil {
		log.Errorf("[TIME] Error when initializing TIME object %v", err)
	}

	//Initialize SYNC
	sync := &SYNC{}
	err = sync.Init(&EM{}, od.Find(0x1005), od.Find(0x1006), od.Find(0x1007), od.Find(0x1019), node.CANModule)
	if err != nil {
		log.Errorf("Error when initialising SYNC object %v", err)
	}
	node.SYNC = sync
	return nil
}
