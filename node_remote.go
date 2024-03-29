package canopen

import (
	"errors"

	log "github.com/sirupsen/logrus"
)

// A RemoteNode is a bit different from a [LocalNode].
// It is a local representation of a remote node on the CAN bus
// and does not have the same standard CiA objects.
// Its goal is to simplify master control by providing some general
// features :
//   - SDOClient for reading / writing to remote node with given EDS
//   - RPDO for updating a local OD with the TPDOs from the remote node
//   - SYNC consumer
//
// A RemoteNode has the same id as the remote node that it controls
// however, being a direct local representation it may only be accessed
// locally.
type RemoteNode struct {
	*BaseNode
	remoteOd  *ObjectDictionary // Remote node od, this does not change
	sdoClient *SDOClient        // A unique sdoClient shared between localCtrl & remoteCtrl
	rpdos     []*RPDO           // Local RPDOs (corresponds to remote TPDOs)
	tpdos     []*TPDO           // Local TPDOs (corresponds to remote RPDOs)
	sync      *SYNC             // Sync consumer (for synchronous PDOs)
}

func (node *RemoteNode) ProcessTPDO(syncWas bool, timeDifferenceUs uint32, timerNextUs *uint32) {
	if node.GetState() == NODE_RUNNING {
		for _, tpdo := range node.tpdos {
			tpdo.process(timeDifferenceUs, timerNextUs, true, syncWas)
		}
	}
}

func (node *RemoteNode) ProcessRPDO(syncWas bool, timeDifferenceUs uint32, timerNextUs *uint32) {
	if node.GetState() == NODE_RUNNING {
		for _, rpdo := range node.rpdos {
			rpdo.process(timeDifferenceUs, timerNextUs, true, syncWas)
		}
	}
}

func (node *RemoteNode) ProcessSync(timeDifferenceUs uint32, timerNextUs *uint32) bool {
	syncWas := false
	sync := node.sync
	if sync != nil {
		syncProcess := sync.process(node.GetState() == NODE_RUNNING, timeDifferenceUs, timerNextUs)

		switch syncProcess {
		case syncNone, syncRxOrTx:
			syncWas = true
		case syncPassedWindow:
		}
	}
	return syncWas
}

func (node *RemoteNode) ProcessMain(enableGateway bool, timeDifferenceUs uint32, timerNextUs *uint32) uint8 {
	return RESET_NOT
}

func (node *RemoteNode) MainCallback() {
	if node.mainCallback != nil {
		node.mainCallback(node)
	}
}

// Create a remote node
func newRemoteNode(
	bm *busManager,
	remoteOd *ObjectDictionary,
	remoteNodeId uint8,
	useLocal bool,
) (*RemoteNode, error) {
	if bm == nil {
		return nil, errors.New("need at least busManager")
	}
	if remoteOd == nil {
		remoteOd = NewOD()
	}
	base, err := newBaseNode(bm, remoteOd, remoteNodeId)
	if err != nil {
		return nil, err
	}
	node := &RemoteNode{BaseNode: base}
	node.baseSdoClient.nodeId = 0 // Change the SDO client node id to 0 as not a real node
	node.remoteOd = remoteOd

	// Create a new SDO client for the remote node & for local access
	client, err := NewSDOClient(bm, remoteOd, 0, DEFAULT_SDO_CLIENT_TIMEOUT_MS, nil)
	if err != nil {
		log.Errorf("[NODE][SDO CLIENT] error when initializing SDO client object %v", err)
		return nil, err
	}
	node.sdoClient = client
	// Create a new SYNC object
	node.od.AddSYNC()
	//Initialize SYNC
	sync, err := NewSYNC(
		bm,
		nil,
		node.od.Index(0x1005),
		node.od.Index(0x1006),
		node.od.Index(0x1007),
		node.od.Index(0x1019),
	)
	if err != nil {
		log.Errorf("[NODE][SYNC] error when initialising SYNC object %v", err)
		return nil, err
	}
	node.sync = sync
	err = node.InitPDOs(useLocal)
	return node, err
}

// Initialize PDOs according to either local OD mapping or remote OD mapping
// A TPDO corresponds to an RPDO and vice-versa
func (node *RemoteNode) InitPDOs(useLocal bool) error {
	// Iterate over all the possible entries : there can be a maximum of 512 maps
	// Break loops when an entry doesn't exist (don't allow holes in mapping)
	var pdoConfigurators []*PDOConfigurator

	localRPDOConfigurator := NewRPDOConfigurator(0, node.sdoClient)
	localTPDOConfigurator := NewTPDOConfigurator(0, node.sdoClient)

	if useLocal {
		pdoConfigurators = []*PDOConfigurator{localRPDOConfigurator, localTPDOConfigurator}
	} else {
		pdoConfigurators = []*PDOConfigurator{
			NewRPDOConfigurator(node.id, node.sdoClient),
			NewTPDOConfigurator(node.id, node.sdoClient),
		}
	}

	// Read TPDO & RPDO configurations
	// RPDO becomes TPDO & vice versa
	allPdoConfigurations := make([][]PDOConfiguration, 0)

	for _, configurator := range pdoConfigurators {
		pdoConfigurations := make([]PDOConfiguration, 0)
		for pdoNb := uint16(1); pdoNb <= 512; pdoNb++ {
			conf, err := configurator.ReadConfiguration(pdoNb)
			if err != nil && err == SDO_ABORT_NOT_EXIST {
				log.Warnf("[NODE][PDO] no more PDO after PDO nb %v", pdoNb-1)
				break
			} else if err != nil {
				log.Errorf("[NODE][PDO] unable to read configuration : %v", err)
				return err
			}
			pdoConfigurations = append(pdoConfigurations, conf)
		}
		allPdoConfigurations = append(allPdoConfigurations, pdoConfigurations)
	}

	rpdoConfigurations := allPdoConfigurations[0]
	tpdoConfigurations := allPdoConfigurations[1]
	for i, configuration := range tpdoConfigurations {
		err := node.od.AddRPDO(uint16(i + 1))
		if err != nil {
			return err
		}
		err = localRPDOConfigurator.Disable(uint16(i) + 1)
		if err != nil {
			return err
		}
		err = localRPDOConfigurator.WriteConfiguration(uint16(i)+1, configuration)
		if err != nil {
			return err
		}
		rpdo, err := NewRPDO(
			node.busManager,
			node.od,
			nil,
			node.sync,
			node.GetOD().Index(0x1400+i),
			node.GetOD().Index(0x1600+i),
			0,
		)
		if err != nil {
			return err
		}
		node.rpdos = append(node.rpdos, rpdo)
		err = localRPDOConfigurator.Enable(uint16(i) + 1) // This can fail but not critical
		if err != nil {
			log.Warnf("[NODE] failed to initialize RPDO %v : %v", uint16(i)+1, err)
		}
	}
	for i, configuration := range rpdoConfigurations {
		err := node.od.AddTPDO(uint16(i + 1))
		if err != nil {
			return err
		}
		err = localTPDOConfigurator.Disable(uint16(i) + 1)
		if err != nil {
			return err
		}
		err = localTPDOConfigurator.WriteConfiguration(uint16(i)+1, configuration)
		if err != nil {
			return err
		}
		tpdo, err := NewTPDO(
			node.busManager,
			node.od,
			nil,
			node.sync,
			node.GetOD().Index(0x1800+i),
			node.GetOD().Index(0x1A00+i),
			0,
		)
		if err != nil {
			return err
		}
		node.tpdos = append(node.tpdos, tpdo)
	}

	return nil

}
