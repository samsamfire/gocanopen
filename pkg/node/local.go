package node

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/emergency"
	"github.com/samsamfire/gocanopen/pkg/heartbeat"
	"github.com/samsamfire/gocanopen/pkg/nmt"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/samsamfire/gocanopen/pkg/pdo"
	"github.com/samsamfire/gocanopen/pkg/sdo"
	s "github.com/samsamfire/gocanopen/pkg/sync"
	t "github.com/samsamfire/gocanopen/pkg/time"
)

// A [LocalNode] is a CiA 301 compliant CANopen node
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
	SYNC               *s.SYNC
	EMCY               *emergency.EMCY
	TIME               *t.TIME
}

func (node *LocalNode) Reset() error {
	if node.NMT != nil {
		node.NMT.Reset()
	}
	return nil
}

// Process canopen objects that are not RT
// Does not process SYNC and PDOs
func (node *LocalNode) ProcessMain(enableGateway bool, timeDifferenceUs uint32) uint8 {

	// Process all objects
	NMTState := node.NMT.GetInternalState()
	NMTisPreOrOperational := (NMTState == nmt.StatePreOperational) || (NMTState == nmt.StateOperational)

	node.BusManager.Process()
	node.EMCY.Process(NMTisPreOrOperational, timeDifferenceUs)
	reset := node.NMT.Process(&NMTState, timeDifferenceUs)

	// Update NMTisPreOrOperational
	NMTisPreOrOperational = (NMTState == nmt.StatePreOperational) || (NMTState == nmt.StateOperational)

	node.HBConsumer.Process(NMTisPreOrOperational, timeDifferenceUs)

	if node.TIME != nil {
		node.TIME.Process(NMTisPreOrOperational, timeDifferenceUs)
	}

	return reset

}

func (node *LocalNode) Servers() []*sdo.SDOServer {
	return node.SDOServers
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
	for i := range pdo.MaxRpdoNumber {
		entry14xx := node.GetOD().Index(od.EntryRPDOCommunicationStart + i)
		entry16xx := node.GetOD().Index(od.EntryRPDOMappingStart + i)
		preDefinedIdent := uint16(0)
		pdoOffset := i % 4
		nodeIdOffset := i / 4
		preDefinedIdent = 0x200 + pdoOffset*0x100 + uint16(node.id) + nodeIdOffset
		rpdo, err := pdo.NewRPDO(
			node.BusManager,
			node.logger,
			node.GetOD(),
			node.EMCY,
			node.SYNC,
			entry14xx,
			entry16xx,
			preDefinedIdent,
		)
		if err != nil {
			node.logger.Warn("no more RPDO after", "nb", i-1)
			break
		} else {
			node.RPDOs = append(node.RPDOs, rpdo)
		}
	}
	// Do the same for TPDOS
	for i := range pdo.MaxTpdoNumber {
		entry18xx := node.GetOD().Index(od.EntryTPDOCommunicationStart + i)
		entry1Axx := node.GetOD().Index(od.EntryTPDOMappingStart + i)
		preDefinedIdent := uint16(0)
		pdoOffset := i % 4
		nodeIdOffset := i / 4
		preDefinedIdent = 0x180 + pdoOffset*0x100 + uint16(node.id) + nodeIdOffset
		tpdo, err := pdo.NewTPDO(
			node.BusManager,
			node.logger,
			node.GetOD(),
			node.EMCY,
			node.SYNC,
			entry18xx,
			entry1Axx,
			preDefinedIdent,
		)
		if err != nil {
			node.logger.Warn("no more TPDO after", "nb", i-1)
			break
		} else {
			node.TPDOs = append(node.TPDOs, tpdo)
		}

	}

	// Register NMT state change callback for RPDOs & TPDOs
	_ = node.NMT.AddStateChangeCallback(func(state uint8) {
		isOperational := state == nmt.StateOperational
		for _, rpdo := range node.RPDOs {
			rpdo.SetOperational(isOperational)
		}
		for _, tpdo := range node.TPDOs {
			tpdo.SetOperational(isOperational)
		}
	})

	return nil
}

// Create a new local node
func NewLocalNode(
	bm *canopen.BusManager,
	logger *slog.Logger,
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
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("id", nodeId)
	base, err := newBaseNode(bm, logger, odict, nodeId)
	if err != nil {
		return nil, err
	}
	node := &LocalNode{BaseNode: base}
	node.NodeIdUnconfigured = false
	node.od = odict
	node.id = nodeId

	if emcy == nil {
		emergency, err := emergency.NewEMCY(
			bm,
			logger,
			nodeId,
			odict.Index(od.EntryErrorRegister),
			odict.Index(od.EntryCobIdEMCY),
			odict.Index(od.EntryInhibitTimeEMCY),
			odict.Index(od.EntryManufacturerStatusRegister),
			nil,
		)
		if err != nil {
			logger.Error("init failed [EMCY] producer", "error", err)
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
			logger,
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
			logger.Error("init failed [NMT]", "error", err)
			return nil, err
		} else {
			node.NMT = nmt
			logger.Info("[NMT] initialized from OD")
		}
	} else {
		node.NMT = nm
		logger.Info("[NMT] initialized from parameters")
	}

	// Initialize HB consumer
	hbCons, err := heartbeat.NewHBConsumer(bm, logger, emcy, odict.Index(od.EntryConsumerHeartbeatTime))
	if err != nil {
		logger.Error("init failed [HBConsumer]", "error", err)
		return nil, err
	} else {
		node.HBConsumer = hbCons
	}
	logger.Info("[HBConsumer] initialized")

	// Initialize SDO server
	// For now only one server
	entry1200 := odict.Index(od.EntrySDOServerParameter)
	sdoServers := make([]*sdo.SDOServer, 0)
	if entry1200 == nil {
		logger.Warn("no [SDOServer] initialized")
	} else {
		server, err := sdo.NewSDOServer(bm, logger, odict, nodeId, sdoServerTimeoutMs, entry1200)
		if err != nil {
			logger.Error("init failed [SDOServer]", "error", err)
			return nil, err
		} else {
			sdoServers = append(sdoServers, server)
			node.SDOServers = sdoServers
			logger.Info("[SDOServer] initialized")
		}
		// Register NMT state change callback for sdo servers
		_ = node.NMT.AddStateChangeCallback(server.OnNmtStateChange)
	}

	// Initialize SDO clients if any
	// For now only one client
	entry1280 := odict.Index(od.EntrySDOClientParameter)
	sdoClients := make([]*sdo.SDOClient, 0)
	if entry1280 == nil {
		logger.Warn("no [SDOClient] initialized")
	} else {

		client, err := sdo.NewSDOClient(bm, logger, odict, nodeId, sdoClientTimeoutMs, entry1280)
		if err != nil {
			logger.Error("init failed [SDOClient]", "error", err)
		} else {
			sdoClients = append(sdoClients, client)
			logger.Info("[SDOClient] initialized")
		}
		node.SDOclients = sdoClients
	}

	// Initialize TIME
	time, err := t.NewTIME(bm, logger, odict.Index(od.EntryCobIdTIME), 1000) // hardcoded for now
	if err != nil {
		node.logger.Error("init failed [TIME]", "error", err)
	} else {
		node.TIME = time
	}

	// Initialize SYNC
	sync, err := s.NewSYNC(
		bm,
		logger,
		emcy,
		odict.Index(od.EntryCobIdSYNC),
		odict.Index(od.EntryCommunicationCyclePeriod),
		odict.Index(od.EntrySynchronousWindowLength),
		odict.Index(od.EntrySynchronousCounterOverflow),
	)
	if err != nil {
		node.logger.Error("init failed [SYNC]", "error", err)
	} else {
		node.SYNC = sync
	}
	_ = node.NMT.AddStateChangeCallback(func(state uint8) {
		isPreOrOperational := state == nmt.StateOperational
		node.SYNC.SetOperational(isPreOrOperational)
	})

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
				node.logger.Warn("error reading EDS format, default to ASCII", "error", err)
				format = 0
			}
		}
		switch format {
		case od.FormatEDSAscii:
			node.logger.Info("EDS is downloadable via object 0x1021 in ASCII format")
			odict.AddReader(edsStore.Index, edsStore.Name, odict.NewReaderSeeker())
		case od.FormatEDSZipped:
			node.logger.Info("EDS is downloadable via object 0x1021 in Zipped format")
			compressed, err := createInMemoryZip("compressed.eds", odict.NewReaderSeeker())
			if err != nil {
				node.logger.Error("failed to compress EDS", "error", err)
				return nil, err
			}
			odict.AddReader(edsStore.Index, edsStore.Name, bytes.NewReader(compressed))
		default:
			return nil, fmt.Errorf("invalid EDS storage format %v", format)
		}
	}

	err = node.initPDO()

	return node, err
}

// Create an in memory zip representation of an io.Reader.
// This can be used to increase transfer speeds in block transfers
// for example.
func createInMemoryZip(filename string, r io.ReadSeeker) ([]byte, error) {

	if r == nil {
		return nil, fmt.Errorf("expecting a reader %v", r)
	}

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
