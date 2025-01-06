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
	"github.com/samsamfire/gocanopen/pkg/lss"
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
	LSSslave           *lss.LSSSlave
}

func (node *LocalNode) ProcessPDO(syncWas bool, timeDifferenceUs uint32) {
	if node.NodeIdUnconfigured {
		return
	}
	isOperational := node.NMT.GetInternalState() == nmt.StateOperational
	for _, tpdo := range node.TPDOs {
		tpdo.Process(timeDifferenceUs, isOperational, syncWas)
	}
	for _, rpdo := range node.RPDOs {
		rpdo.Process(timeDifferenceUs, isOperational, syncWas)
	}
}

func (node *LocalNode) ProcessSYNC(timeDifferenceUs uint32) bool {
	syncWas := false
	sy := node.SYNC
	if !node.NodeIdUnconfigured && sy != nil {

		nmtState := node.NMT.GetInternalState()
		nmtIsPreOrOperational := nmtState == nmt.StatePreOperational || nmtState == nmt.StateOperational
		syncProcess := sy.Process(nmtIsPreOrOperational, timeDifferenceUs)

		switch syncProcess {
		case s.EventRxOrTx:
			syncWas = true
		case s.EventPassedWindow:
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
	// Propagate NMT state to server
	for _, server := range node.SDOServers {
		server.SetNMTState(NMTState)
	}

	node.BusManager.Process()
	node.EMCY.Process(NMTisPreOrOperational, timeDifferenceUs, timerNextUs)
	reset := node.NMT.Process(&NMTState, timeDifferenceUs, timerNextUs)

	// Update NMTisPreOrOperational
	NMTisPreOrOperational = (NMTState == nmt.StatePreOperational) || (NMTState == nmt.StateOperational)

	node.HBConsumer.Process(NMTisPreOrOperational, timeDifferenceUs, timerNextUs)

	if node.TIME != nil {
		node.TIME.Process(NMTisPreOrOperational, timeDifferenceUs)
	}

	return reset

}

func (node *LocalNode) Servers() []*sdo.SDOServer {
	return node.SDOServers
}

func (node *LocalNode) LSSSlave() *lss.LSSSlave {
	return node.LSSslave
}

// Initialize all [pdo.RPDO] and [pdo.TPDO] objects
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
	for i := range uint16(512) {
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

	return nil
}

// Initialize [emergency.EMCY] object
func (node *LocalNode) initEMCY() error {

	emcy, err := emergency.NewEMCY(
		node.BusManager,
		node.logger,
		node.LSSslave.GetNodeIdActive(),
		node.od.Index(od.EntryErrorRegister),
		node.od.Index(od.EntryCobIdEMCY),
		node.od.Index(od.EntryInhibitTimeEMCY),
		node.od.Index(od.EntryManufacturerStatusRegister),
		nil,
	)
	if err != nil {
		node.logger.Error("init failed [EMCY] producer", "error", err)
		return canopen.ErrOdParameters
	}
	node.EMCY = emcy
	return nil
}

// Initialize [nmt.NMT] object, requires an EMCY object
func (node *LocalNode) initNMT(nmtControl uint16, firstHbTimeMs uint16) error {

	nodeIdActive := node.LSSslave.GetNodeIdActive()
	nm, err := nmt.NewNMT(
		node.BusManager,
		node.logger,
		node.EMCY,
		nodeIdActive,
		nmtControl,
		firstHbTimeMs,
		nmt.ServiceId,
		nmt.ServiceId,
		heartbeat.ServiceId+uint16(nodeIdActive),
		node.od.Index(od.EntryProducerHeartbeatTime),
	)
	if err != nil {
		node.logger.Error("init failed [NMT]", "error", err)
		return err
	}
	node.NMT = nm
	return nil
}

// Initialize [heartbeat.HBConsumer] object
func (node *LocalNode) initHBConsumer() error {

	hbCons, err := heartbeat.NewHBConsumer(
		node.BusManager,
		node.logger,
		node.EMCY,
		node.od.Index(od.EntryConsumerHeartbeatTime),
	)
	if err != nil {
		node.logger.Error("init failed [HBConsumer]", "error", err)
		return err
	}
	node.HBConsumer = hbCons
	return nil
}

// Initialize [sdo.SDOServer] object(s)
// Currently, only one server is supported (optionally)
func (node *LocalNode) initSDOServers(serverTimeoutMs uint32) error {
	entry1200 := node.od.Index(od.EntrySDOServerParameter)
	if entry1200 == nil {
		node.logger.Warn("no [SDOServer] initialized")
		return nil
	}
	sdoServers := make([]*sdo.SDOServer, 0)
	server, err := sdo.NewSDOServer(
		node.BusManager,
		node.logger,
		node.od,
		node.LSSslave.GetNodeIdActive(),
		serverTimeoutMs,
		entry1200,
	)
	if err != nil {
		node.logger.Error("init failed [SDOServer]", "error", err)
		return err
	}
	sdoServers = append(sdoServers, server)
	node.SDOServers = sdoServers
	return nil
}

// Initialize [sdo.SDOClient] object(s)
func (node *LocalNode) initSDOClients(clientTimeoutMs uint32) error {

	entry1280 := node.od.Index(od.EntrySDOClientParameter)
	if entry1280 == nil {
		node.logger.Warn("no [SDOClient] initialized")
		return nil
	}
	sdoClients := make([]*sdo.SDOClient, 0)
	client, err := sdo.NewSDOClient(
		node.BusManager,
		node.logger,
		node.od, node.LSSslave.GetNodeIdActive(),
		clientTimeoutMs,
		entry1280,
	)
	if err != nil {
		node.logger.Error("init failed [SDOClient]", "error", err)
		return err
	}
	sdoClients = append(sdoClients, client)
	node.SDOclients = sdoClients
	return nil
}

// Initialize [s.SYNC] object
func (node *LocalNode) initSYNC() error {

	sync, err := s.NewSYNC(
		node.BusManager,
		node.logger,
		node.EMCY,
		node.od.Index(od.EntryCobIdSYNC),
		node.od.Index(od.EntryCommunicationCyclePeriod),
		node.od.Index(od.EntrySynchronousWindowLength),
		node.od.Index(od.EntrySynchronousCounterOverflow),
	)
	if err != nil {
		node.logger.Error("init failed [SYNC]", "error", err)
		return err
	}
	node.SYNC = sync
	return nil
}

// Initialize [t.TIME] object
func (node *LocalNode) initTIME() error {

	time, err := t.NewTIME(
		node.BusManager,
		node.logger,
		node.od.Index(od.EntryCobIdTIME),
		1000,
	) // hardcoded for now
	if err != nil {
		node.logger.Error("init failed [TIME]", "error", err)
		return err
	}
	node.TIME = time
	return nil
}

// Initialize [lss.LSSSlave] object
func (node *LocalNode) initLSSSlave() error {

	slave, err := lss.NewLSSSlave(
		node.BusManager,
		node.logger,
		node.od.Index(od.EntryIdentityObject),
		node.LSSslave.GetNodeIdActive(),
	)
	if err != nil {
		node.logger.Error("init failed [LSSSlave]", "error", err)
		return err
	}
	node.LSSslave = slave
	return nil
}

// Initialize all CANopen components, this is will be called
// On node 'reset communication' NMT state machine
func (node *LocalNode) initAll(
	nmtControl uint16,
	firstHbTimeMs uint16,
	sdoServerTimeoutMs uint32,
	sdoClientTimeoutMs uint32,
) error {

	err := node.initLSSSlave()
	if err != nil {
		return err
	}

	err = node.initEMCY()
	if err != nil {
		return err
	}

	err = node.initNMT(nmtControl, firstHbTimeMs)
	if err != nil {
		return err
	}

	err = node.initHBConsumer()
	if err != nil {
		return err
	}

	err = node.initSDOServers(sdoServerTimeoutMs)
	if err != nil {
		return err
	}

	err = node.initSDOClients(sdoClientTimeoutMs)
	if err != nil {
		return err
	}

	err = node.initTIME()
	if err != nil {
		return err
	}

	err = node.initSYNC()
	if err != nil {
		return err
	}

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

	// Initialize all CANopen parts
	err = node.initAll(nmtControl, firstHbTimeMs, sdoServerTimeoutMs, sdoClientTimeoutMs)
	if err != nil {
		return nil, err
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
				node.logger.Warn("error reading EDS format, default to ASCII", "error", err)
				format = 0
			}
		}
		switch format {
		case od.FormatEDSAscii:
			node.logger.Info("EDS is downloadable via object 0x1021 in ASCII format")
			odict.AddReader(edsStore.Index, edsStore.Name, odict.Reader)
		case od.FormatEDSZipped:
			node.logger.Info("EDS is downloadable via object 0x1021 in Zipped format")
			compressed, err := createInMemoryZip("compressed.eds", odict.Reader)
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
