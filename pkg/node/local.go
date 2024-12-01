package node

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/emergency"
	"github.com/samsamfire/gocanopen/pkg/heartbeat"
	"github.com/samsamfire/gocanopen/pkg/nmt"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/samsamfire/gocanopen/pkg/pdo"
	"github.com/samsamfire/gocanopen/pkg/sdo"
	"github.com/samsamfire/gocanopen/pkg/sync"
	"github.com/samsamfire/gocanopen/pkg/time"
	log "github.com/sirupsen/logrus"
)

// A LocalNode is a CiA 301 compliant CANopen node
// It supports all the standard CANopen objects.
// These objects will be loaded depending on the given EDS file.
// For configuration of the different CANopen objects see [NodeConfigurator].
type LocalNode struct {
	*BaseNode
	NodeIdUnconfigured bool
	NMT                *nmt.NMT
	HBConsumer         *heartbeat.HBConsumer
	SDOclients         []*sdo.SDOClient
	SDOServers         []*sdo.SDOServer
	TPDOs              []*pdo.TPDO
	RPDOs              []*pdo.RPDO
	SYNC               *sync.SYNC
	EMCY               *emergency.EMCY
	TIME               *time.TIME
}

func (node *LocalNode) ProcessTPDO(syncWas bool, timeDifferenceUs uint32, timerNextUs *uint32) {
	if node.NodeIdUnconfigured {
		return
	}
	nmtIsOperational := node.NMT.GetInternalState() == nmt.StateOperational
	for _, tpdo := range node.TPDOs {
		tpdo.Process(timeDifferenceUs, timerNextUs, nmtIsOperational, syncWas)
	}
}

func (node *LocalNode) ProcessRPDO(syncWas bool, timeDifferenceUs uint32, timerNextUs *uint32) {
	if node.NodeIdUnconfigured {
		return
	}
	nmtIsOperational := node.NMT.GetInternalState() == nmt.StateOperational
	for _, rpdo := range node.RPDOs {
		rpdo.Process(timeDifferenceUs, timerNextUs, nmtIsOperational, syncWas)
	}
}

func (node *LocalNode) ProcessSYNC(timeDifferenceUs uint32, timerNextUs *uint32) bool {
	syncWas := false
	s := node.SYNC
	if !node.NodeIdUnconfigured && s != nil {

		nmtState := node.NMT.GetInternalState()
		nmtIsPreOrOperational := nmtState == nmt.StatePreOperational || nmtState == nmt.StateOperational
		syncProcess := s.Process(nmtIsPreOrOperational, timeDifferenceUs, timerNextUs)

		switch syncProcess {
		case sync.EventRxOrTx:
			syncWas = true
		case sync.EventPassedWindow:
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
	NMTisPreOrOperational := (NMTState == nmt.StatePreOperational) || (NMTState == nmt.StateOperational)

	node.BusManager.Process()
	node.EMCY.Process(NMTisPreOrOperational, timeDifferenceUs, timerNextUs)
	reset := node.NMT.Process(&NMTState, timeDifferenceUs, timerNextUs)

	// Update NMTisPreOrOperational
	NMTisPreOrOperational = (NMTState == nmt.StatePreOperational) || (NMTState == nmt.StateOperational)

	// Process SDO servers
	for _, server := range node.SDOServers {
		server.Process(NMTisPreOrOperational, timeDifferenceUs, timerNextUs)
	}
	node.HBConsumer.Process(NMTisPreOrOperational, timeDifferenceUs, timerNextUs)

	if node.TIME != nil {
		node.TIME.Process(NMTisPreOrOperational, timeDifferenceUs)
	}

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
			return canopen.ErrNodeIdUnconfiguredLSS
		} else {
			return canopen.ErrIllegalArgument
		}
	}
	// Iterate over all the possible entries : there can be a maximum of 512 maps
	// Break loops when an entry doesn't exist (don't allow holes in mapping)
	for i := range uint16(512) {
		entry14xx := node.GetOD().Index(od.EntryRPDOCommunicationStart + i)
		entry16xx := node.GetOD().Index(od.EntryRPDOMappingStart + i)
		preDefinedIdent := uint16(0)
		pdoOffset := i % 4
		nodeIdOffset := i / 4
		preDefinedIdent = 0x200 + pdoOffset*0x100 + uint16(node.id) + nodeIdOffset
		rpdo, err := pdo.NewRPDO(node.BusManager, node.GetOD(), node.EMCY, node.SYNC, entry14xx, entry16xx, preDefinedIdent)
		if err != nil {
			log.Warnf("[NODE][RPDO] no more RPDO after RPDO %v", i-1)
			break
		} else {
			node.RPDOs = append(node.RPDOs, rpdo)
		}
	}
	// Do the same for TPDOS
	for i := range uint16(512) {
		entry18xx := node.GetOD().Index(od.EntryTPDOCommunicationStart + i)
		entry1Axx := node.GetOD().Index(od.EntryTPDOMappingStart + i)
		preDefinedIdent := uint16(0)
		pdoOffset := i % 4
		nodeIdOffset := i / 4
		preDefinedIdent = 0x180 + pdoOffset*0x100 + uint16(node.id) + nodeIdOffset
		tpdo, err := pdo.NewTPDO(node.BusManager, node.GetOD(), node.EMCY, node.SYNC, entry18xx, entry1Axx, preDefinedIdent)
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
func NewLocalNode(
	bm *canopen.BusManager,
	odict *od.ObjectDictionary,
	nm *nmt.NMT,
	emcy *emergency.EMCY,
	nodeId uint8,
	nmtControl uint16,
	firstHbTimeMs uint16,
	sdoServerTimeoutMs uint32,
	sdoClientTimeoutMs uint32,
	blockTransferEnabled bool,
	statusBits *od.Entry,

) (*LocalNode, error) {

	if bm == nil || odict == nil {
		return nil, errors.New("need at least busManager and od parameters")
	}
	base, err := newBaseNode(bm, odict, nodeId)
	if err != nil {
		return nil, err
	}
	node := &LocalNode{BaseNode: base}
	node.NodeIdUnconfigured = false
	node.od = odict
	node.exitBackground = make(chan bool)
	node.exit = make(chan bool)
	node.id = nodeId
	node.state = NodeInit

	if emcy == nil {
		emergency, err := emergency.NewEMCY(
			bm,
			nodeId,
			odict.Index(od.EntryErrorRegister),
			odict.Index(od.EntryCobIdEMCY),
			odict.Index(od.EntryInhibitTimeEMCY),
			odict.Index(od.EntryManufacturerStatusRegister),
			nil,
		)
		if err != nil {
			log.Errorf("[NODE][EMERGENCY producer] error when initializing emergency producer %v", err)
			return nil, canopen.ErrOdParameters
		}
		node.EMCY = emergency
	} else {
		node.EMCY = emcy
	}
	emcy = node.EMCY

	// NMT object can either be supplied or created with automatically with an OD entry
	if nm == nil {
		nmt, err := nmt.NewNMT(
			bm,
			emcy,
			nodeId,
			nmtControl,
			firstHbTimeMs,
			nmt.ServiceId,
			nmt.ServiceId,
			heartbeat.ServiceId+uint16(nodeId),
			odict.Index(od.EntryProducerHeartbeatTime),
		)
		if err != nil {
			log.Errorf("[NODE][NMT] error when initializing NMT object %v", err)
			return nil, err
		} else {
			node.NMT = nmt
			log.Infof("[NODE][NMT] initialized from OD for node x%x", nodeId)
		}
	} else {
		node.NMT = nm
		log.Infof("[NODE][NMT] initialized for node x%x", nodeId)
	}

	// Initialize HB consumer
	hbCons, err := heartbeat.NewHBConsumer(bm, emcy, odict.Index(od.EntryConsumerHeartbeatTime))
	if err != nil {
		log.Errorf("[NODE][HB Consumer] error when initializing HB consummers %v", err)
		return nil, err
	} else {
		node.HBConsumer = hbCons
	}
	log.Infof("[NODE][HB Consumer] initialized for node x%x", nodeId)

	// Initialize SDO server
	// For now only one server
	entry1200 := odict.Index(od.EntrySDOServerParameter)
	sdoServers := make([]*sdo.SDOServer, 0)
	if entry1200 == nil {
		log.Warnf("[NODE][SDO SERVER] no sdo servers initialized for node x%x", nodeId)
	} else {
		server, err := sdo.NewSDOServer(bm, odict, nodeId, sdoServerTimeoutMs, entry1200)
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
	entry1280 := odict.Index(od.EntrySDOClientParameter)
	sdoClients := make([]*sdo.SDOClient, 0)
	if entry1280 == nil {
		log.Info("[NODE][SDO CLIENT] no SDO clients initialized for node")
	} else {

		client, err := sdo.NewSDOClient(bm, odict, nodeId, sdoClientTimeoutMs, entry1280)
		if err != nil {
			log.Errorf("[NODE][SDO CLIENT] error when initializing SDO client object %v", err)
		} else {
			sdoClients = append(sdoClients, client)
			log.Infof("[NODE][SDO CLIENT] initialized for node x%x", nodeId)
		}
		node.SDOclients = sdoClients
	}

	// Initialize TIME
	time, err := time.NewTIME(bm, odict.Index(od.EntryCobIdTIME), 1000) // hardcoded for now
	if err != nil {
		log.Errorf("[NODE][TIME] error when initializing TIME object %v", err)
	} else {
		node.TIME = time
	}

	// Initialize SYNC
	sync, err := sync.NewSYNC(
		bm,
		emcy,
		odict.Index(od.EntryCobIdSYNC),
		odict.Index(od.EntryCommunicationCyclePeriod),
		odict.Index(od.EntrySynchronousWindowLength),
		odict.Index(od.EntrySynchronousCounterOverflow),
	)
	if err != nil {
		log.Errorf("[NODE][SYNC] error when initialising SYNC object %v", err)
	} else {
		node.SYNC = sync
	}

	// Add EDS storage if supported, library supports either plain ascii
	// Or zipped format
	edsStore := odict.Index(od.EntryStoreEDS)
	edsFormat := odict.Index(od.EntryStorageFormat)
	if edsStore != nil {
		var format uint8
		if edsFormat == nil {
			format = 0
		} else {
			format, err = edsFormat.Uint8(0)
			if err != nil {
				log.Warnf("[NODE][EDS] error reading format for node x%x, default to ASCII, %v", nodeId, err)
				format = 0
			}
		}
		switch format {
		case od.FormatEDSAscii:
			log.Info("[NODE][EDS] EDS is downloadable via object 0x1021 in ASCII format")
			odict.AddReader(edsStore.Index, edsStore.Name, odict.Reader)
		case od.FormatEDSZipped:
			log.Info("[NODE][EDS] EDS is downloadable via object 0x1021 in Zipped format")
			compressed, err := createInMemoryZip("compressed.eds", odict.Reader)
			if err != nil {
				log.Errorf("[NODE][EDS] Failed to compress EDS %v", err)
				return nil, err
			}
			odict.AddReader(edsStore.Index, edsStore.Name, bytes.NewReader(compressed))
		default:
			return nil, fmt.Errorf("invalid eds storage format %v", format)
		}
	}
	err = node.initPDO()
	return node, err
}

// Create an in memory zip representation of an io.Reader.
// This can be used to increase transfer speeds in block transfers
// for example.
func createInMemoryZip(filename string, r io.ReadSeeker) ([]byte, error) {

	buffer := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buffer)
	// Create a file inside the zip
	writer, err := zipWriter.Create(filename)
	if err != nil {
		return nil, err
	}

	// Write the content to the file
	_, err = r.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}
	_, err = io.Copy(writer, r)
	if err != nil {
		return nil, err
	}

	// Close the zip writer to finalize the zip file
	err = zipWriter.Close()
	if err != nil {
		return nil, err
	}

	// Return the zip file as bytes
	return buffer.Bytes(), nil
}
