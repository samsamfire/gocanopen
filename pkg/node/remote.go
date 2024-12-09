package node

import (
	"errors"
	"log/slog"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/config"
	"github.com/samsamfire/gocanopen/pkg/emergency"
	"github.com/samsamfire/gocanopen/pkg/nmt"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/samsamfire/gocanopen/pkg/pdo"
	"github.com/samsamfire/gocanopen/pkg/sdo"
	"github.com/samsamfire/gocanopen/pkg/sync"
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
	remoteOd *od.ObjectDictionary // Remote node od, this does not change
	client   *sdo.SDOClient       // A unique sdoClient shared between localCtrl & remoteCtrl
	rpdos    []*pdo.RPDO          // Local RPDOs (corresponds to remote TPDOs)
	tpdos    []*pdo.TPDO          // Local TPDOs (corresponds to remote RPDOs)
	sync     *sync.SYNC           // Sync consumer (for synchronous PDOs)
	emcy     *emergency.EMCY      // Emergency consumer (fake producer for logging internal errors)
}

func (node *RemoteNode) ProcessTPDO(syncWas bool, timeDifferenceUs uint32, timerNextUs *uint32) {

	node.mu.Lock()
	defer node.mu.Unlock()
	for _, tpdo := range node.tpdos {
		tpdo.Process(timeDifferenceUs, timerNextUs, true, syncWas)
	}

}

func (node *RemoteNode) ProcessRPDO(syncWas bool, timeDifferenceUs uint32, timerNextUs *uint32) {
	node.mu.Lock()
	defer node.mu.Unlock()
	for _, rpdo := range node.rpdos {
		rpdo.Process(timeDifferenceUs, timerNextUs, true, syncWas)
	}
}

func (node *RemoteNode) ProcessSYNC(timeDifferenceUs uint32, timerNextUs *uint32) bool {
	syncWas := false
	if node.sync != nil {
		event := node.sync.Process(true, timeDifferenceUs, timerNextUs)

		switch event {
		case sync.EventNone, sync.EventRxOrTx:
			syncWas = true
		case sync.EventPassedWindow:
		}
	}
	return syncWas
}

func (node *RemoteNode) ProcessMain(enableGateway bool, timeDifferenceUs uint32, timerNextUs *uint32) uint8 {
	return nmt.ResetNot
}

func (node *RemoteNode) Servers() []*sdo.SDOServer {
	return nil
}

// Create a remote node
func NewRemoteNode(
	bm *canopen.BusManager,
	logger *slog.Logger,
	remoteOd *od.ObjectDictionary,
	remoteNodeId uint8,
) (*RemoteNode, error) {
	if bm == nil {
		return nil, errors.New("need at least busManager")
	}
	if remoteOd == nil {
		remoteOd = od.NewOD()
	}
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("id", remoteNodeId)
	base, err := newBaseNode(bm, logger, remoteOd, remoteNodeId)
	if err != nil {
		return nil, err
	}
	node := &RemoteNode{BaseNode: base}
	node.SetNoId() // Change the SDO client node id to 0 as not a real node
	node.remoteOd = remoteOd

	// Create a new SDO client for the remote node & for local access
	client, err := sdo.NewSDOClient(bm, logger, remoteOd, 0, sdo.DefaultClientTimeout, nil)
	if err != nil {
		logger.Error("error when initializing SDO client object", "error", err)
		return nil, err
	}
	node.client = client
	// Create a new SYNC object
	node.od.AddSYNC()
	// Initialize SYNC
	sync, err := sync.NewSYNC(
		bm,
		logger,
		nil,
		node.od.Index(0x1005),
		node.od.Index(0x1006),
		node.od.Index(0x1007),
		node.od.Index(0x1019),
	)
	if err != nil {
		logger.Error("error when initialising SYNC object", "error", err)
		return nil, err
	}
	node.sync = sync

	// Add empty EMCY, only used for logging for now
	node.emcy = &emergency.EMCY{}

	return node, nil
}

// Initialize PDOs according to either local OD mapping or remote OD mapping
// A TPDO from the distant node corresponds to an RPDO on this node and vice-versa
func (node *RemoteNode) StartPDOs(useLocal bool) error {
	node.mu.Lock()
	defer node.mu.Unlock()

	var conf *config.NodeConfigurator

	localConf := config.NewNodeConfigurator(0, node.logger, node.client)

	if useLocal {
		conf = localConf
	} else {
		conf = config.NewNodeConfigurator(node.id, node.logger, node.client)
	}

	rpdos, tpdos, err := conf.ReadConfigurationAllPDO()
	if err != nil {
		return err
	}

	// Remote TPDOs become local RPDOs
	// Create CANopen RPDO objects
	for i, pdoConfig := range tpdos {
		err := node.od.AddRPDO(uint16(i) + 1)
		if err != nil {
			return err
		}
		err = localConf.DisablePDO(uint16(i) + 1)
		if err != nil {
			return err
		}
		err = localConf.WriteConfigurationPDO(uint16(i)+1, pdoConfig)
		if err != nil {
			return err
		}
		rpdo, err := pdo.NewRPDO(
			node.BusManager,
			node.logger,
			node.od,
			node.emcy, // Empty emergency object used for logging
			node.sync,
			node.GetOD().Index(0x1400+i),
			node.GetOD().Index(0x1600+i),
			0,
		)
		if err != nil {
			return err
		}
		node.rpdos = append(node.rpdos, rpdo)
		err = localConf.EnablePDO(uint16(i) + 1) // This can fail but not critical
		if err != nil {
			node.logger.Warn("failed to initialize RPDO", "nb", uint16(i)+1, "error", err)
		}
	}

	// Remote node RPDOs become local TPDOs
	// Create CANopen TPDO objects
	for i, pdoConfig := range rpdos {
		err := node.od.AddTPDO(uint16(i + 1))
		if err != nil {
			return err
		}
		err = localConf.DisablePDO(uint16(i) + 1 + pdo.MaxRpdoNumber)
		if err != nil {
			return err
		}
		err = localConf.WriteConfigurationPDO(uint16(i)+1+pdo.MaxRpdoNumber, pdoConfig)
		if err != nil {
			return err
		}
		tpdo, err := pdo.NewTPDO(
			node.BusManager,
			node.logger,
			node.od,
			node.emcy, // Empty emergency object used for logging
			node.sync,
			node.GetOD().Index(0x1800+i),
			node.GetOD().Index(0x1A00+i),
			0,
		)
		if err != nil {
			return err
		}
		node.tpdos = append(node.tpdos, tpdo)
		err = localConf.EnablePDO(uint16(i) + 1 + pdo.MaxRpdoNumber) // This can fail but not critical
		if err != nil {
			node.logger.Warn("failed to initialize RPDO", "nb", uint16(i)+1, "error", err)
		}
	}

	return nil
}
