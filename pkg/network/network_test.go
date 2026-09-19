package network

import (
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	canopen "github.com/samsamfire/gocanopen/v2"
	"github.com/samsamfire/gocanopen/v2/pkg/can/virtual"
	"github.com/samsamfire/gocanopen/v2/pkg/config"
	"github.com/samsamfire/gocanopen/v2/pkg/od"
	"github.com/samsamfire/gocanopen/v2/pkg/pdo"
	"github.com/samsamfire/gocanopen/v2/pkg/sdo"
	"github.com/stretchr/testify/assert"
)

const NodeIdTest uint8 = 0x30

func CreateNetworkEmptyTest() *Network {
	canBus, _ := NewBus("virtual", "localhost:18888", 0)
	bus := canBus.(*virtual.Bus)
	bus.SetReceiveOwn(true)
	network := NewNetwork(bus)
	network.SetLogger(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))
	e := network.Connect()
	if e != nil {
		panic(e)
	}
	return &network
}

func CreateNetworkTest() *Network {
	network := CreateNetworkEmptyTest()
	_, err := network.CreateLocalNode(NodeIdTest, od.Default())
	if err != nil {
		panic(err)
	}
	return network
}

func TestReadEDS(t *testing.T) {
	network := CreateNetworkTest()
	network2 := CreateNetworkEmptyTest()
	defer network2.Disconnect()
	defer network.Disconnect()
	_, err := network.CreateLocalNode(NodeIdTest+1, "../../testdata/test_zipped_format.eds")
	assert.Nil(t, err)

	t.Run("local node ascii format", func(t *testing.T) {
		od, err := network.ReadEDS(NodeIdTest, od.DefaultEDSFormatHandler)
		assert.Nil(t, err)
		assert.NotNil(t, od.Index(0x1021))
	})
	t.Run("local node zipped format local", func(t *testing.T) {
		assert.Nil(t, err)
		od, err := network.ReadEDS(NodeIdTest+1, od.DefaultEDSFormatHandler)
		assert.Nil(t, err)
		assert.NotNil(t, od.Index(0x1021))
	})
	t.Run("local node zipped format remote", func(t *testing.T) {
		od, err := network2.ReadEDS(NodeIdTest+1, od.DefaultEDSFormatHandler)
		assert.Nil(t, err)
		assert.NotNil(t, od.Index(0x1021))
	})
	t.Run("remote node", func(t *testing.T) {
		od, err := network2.ReadEDS(NodeIdTest, nil)
		assert.Nil(t, err)
		assert.NotNil(t, od.Index(0x1021))
	})
	t.Run("with invalid format handler", func(t *testing.T) {
		local, _ := network.Local(NodeIdTest)
		// Replace EDS format with another value
		_, err := local.GetOD().AddVariableType(0x1022, "Storage Format", od.UNSIGNED8, od.AttributeSdoRw, "0x10")
		assert.Nil(t, err)
		_, err = network2.ReadEDS(NodeIdTest, nil)
		assert.Equal(t, od.ErrEdsFormat, err)
	})
}

func TestAddRemoveNodes(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	t.Run("remove node", func(t *testing.T) {
		err := network.RemoveNode(0x12)
		assert.Equal(t, ErrNotFound, err)
		err = network.RemoveNode(NodeIdTest)
		assert.Nil(t, err)
		_, err = network.CreateLocalNode(NodeIdTest, od.Default())
		assert.Len(t, network.controllers, 1)
		assert.Nil(t, err)
		err = network.RemoveNode(NodeIdTest)
		assert.Nil(t, err)
		assert.Len(t, network.controllers, 0)
	})
	t.Run("add node", func(t *testing.T) {
		// Test creating multiple nodes with same id
		assert.Len(t, network.controllers, 0)
		_, err := network.CreateLocalNode(NodeIdTest, od.Default())
		assert.Nil(t, err)
		_, err = network.CreateLocalNode(NodeIdTest, od.Default())
		assert.Equal(t, ErrIdConflict, err)
		// Test adding multiple nodes with same id
		_, err = network.AddRemoteNode(NodeIdTest, od.Default())
		assert.NotEmpty(t, ErrIdConflict, err)
	})

}

// Creates a network with a local node whose RPDO1 mapping record declares less
// application objects than od.MaxMappedEntriesPdo
func CreateNetworkShortMappingTest(nbMappingSubs uint8) *Network {
	odict := od.Default()
	pdoMap := od.NewRecord()
	_, _ = pdoMap.AddSubObject(od.SubPdoNbMappings,
		"Number of mapped application objects in PDO", od.UNSIGNED8, od.AttributeSdoRw, "0x0")
	for i := range nbMappingSubs {
		_, _ = pdoMap.AddSubObject(i+1,
			fmt.Sprintf("Application object %d", i+1), od.UNSIGNED32, od.AttributeSdoRw, "0x0")
	}
	odict.AddVariableList(od.EntryRPDOMappingStart, "RPDO mapping parameter", pdoMap)

	network := CreateNetworkEmptyTest()
	_, err := network.CreateLocalNode(NodeIdTest, odict)
	if err != nil {
		panic(err)
	}
	return network
}

// The number of mapped objects (0x1600 sub0) has to be checked against the number of
// mapping sub entries the OD actually declares, and not only against od.MaxMappedEntriesPdo.
func TestRPDONbMappedAboveDeclaredSubEntries(t *testing.T) {
	const nbMappingSubs = 4
	const rpdoCanId = 0x255

	net := CreateNetworkShortMappingTest(nbMappingSubs)
	otherNet := CreateNetworkEmptyTest()
	defer net.Disconnect()
	defer otherNet.Disconnect()

	local, err := net.Local(NodeIdTest)
	assert.Nil(t, err)

	c := net.Configurator(NodeIdTest)
	assert.Nil(t, c.DisablePDO(1))
	assert.Nil(t, c.WriteCanIdPDO(1, rpdoCanId))
	assert.Nil(t, c.WriteTransmissionType(1, pdo.TransmissionTypeSyncEventHi))

	// Map a single UNSIGNED32, the mapping sub entries 5 to 8 do not exist in this OD
	assert.Nil(t, net.WriteRaw(NodeIdTest, od.EntryRPDOMappingStart, od.SubPdoNbMappings, uint8(0), false))
	assert.Nil(t, net.WriteRaw(NodeIdTest, od.EntryRPDOMappingStart, 1, uint32(0x20070020), false))
	assert.Nil(t, net.WriteRaw(NodeIdTest, od.EntryRPDOMappingStart, od.SubPdoNbMappings, uint8(1), false))

	// Announcing more mapped objects than the OD declares should be rejected
	err = net.WriteRaw(NodeIdTest, od.EntryRPDOMappingStart, od.SubPdoNbMappings, uint8(nbMappingSubs+1), false)
	assert.Equal(t, sdo.AbortMapLen, err)

	assert.Nil(t, c.EnablePDO(1))
	time.Sleep(100 * time.Millisecond)

	// Send PDO and check local OD updated
	assert.Nil(t, otherNet.Send(canopen.Frame{ID: rpdoCanId, DLC: 4, Data: [8]byte{0x11, 0x22, 0x33, 0x44}}))
	time.Sleep(100 * time.Millisecond)

	val, err := local.ReadUint32("UNSIGNED32 value", 0)
	assert.Nil(t, err)
	assert.EqualValues(t, 0x44332211, val)
}

// Mapped object should be read with the mapped length and not with the object length.
func TestTPDOPartiallyMappedObject(t *testing.T) {
	net := CreateNetworkTest()
	otherNet := CreateNetworkEmptyTest()
	defer net.Disconnect()
	defer otherNet.Disconnect()

	local, err := net.Local(NodeIdTest)
	assert.Nil(t, err)

	tpdo1 := pdo.MaxRpdoNumber + 1
	canId := uint32(0x180 + int(NodeIdTest))

	collector := &FrameCollector{}
	_, err = otherNet.Subscribe(canId, 0x7FF, false, collector)
	assert.Nil(t, err)

	assert.Nil(t, local.WriteAnyExact("UNSIGNED8 value", 0, uint8(0x42)))
	assert.Nil(t, local.WriteAnyExact("UNSIGNED64 value", 0, uint64(0x1122334455667788)))

	c := local.Configurator()
	assert.Nil(t, c.ProducerDisableSYNC())
	assert.Nil(t, c.WriteCommunicationPeriod(0))
	assert.Nil(t, c.DisablePDO(tpdo1))
	err = c.WriteConfigurationPDO(tpdo1,
		config.PDOConfigurationParameter{
			CanId:            uint16(canId),
			TransmissionType: 1, // Sync every cycle
			Mappings: []config.PDOMappingParameter{
				{Index: 0x2005, Subindex: 0, LengthBits: 8},
				// Only the first byte of the UNSIGNED64 is mapped, the object
				// is bigger than the space left inside the frame
				{Index: 0x201B, Subindex: 0, LengthBits: 8},
			},
		})
	assert.Nil(t, err)
	assert.Nil(t, c.EnablePDO(tpdo1))
	time.Sleep(100 * time.Millisecond)
	collector.Clear()

	// Send SYNC
	assert.Nil(t, otherNet.Send(canopen.Frame{ID: 0x80, DLC: 0}))
	time.Sleep(100 * time.Millisecond)

	frames := collector.GetFrames(canId)
	assert.Len(t, frames, 1)
	if len(frames) > 0 {
		assert.EqualValues(t, 2, frames[0].DLC)
		assert.EqualValues(t, 0x42, frames[0].Data[0])
		// First byte of the UNSIGNED64
		assert.EqualValues(t, 0x88, frames[0].Data[1])
	}
}

func TestRPDOPartiallyMappedObject(t *testing.T) {
	const rpdoCanId = 0x255

	net := CreateNetworkTest()
	otherNet := CreateNetworkEmptyTest()
	defer net.Disconnect()
	defer otherNet.Disconnect()

	local, err := net.Local(NodeIdTest)
	assert.Nil(t, err)

	assert.Nil(t, local.WriteAnyExact("UNSIGNED32 value", 0, uint32(0)))
	assert.Nil(t, local.WriteAnyExact("UNSIGNED8 value", 0, uint8(0)))

	c := local.Configurator()
	assert.Nil(t, c.ProducerDisableSYNC())
	assert.Nil(t, c.WriteCommunicationPeriod(0))
	assert.Nil(t, c.DisablePDO(1))
	err = c.WriteConfigurationPDO(1,
		config.PDOConfigurationParameter{
			CanId:            rpdoCanId,
			TransmissionType: pdo.TransmissionTypeSyncEventHi,
			Mappings: []config.PDOMappingParameter{
				// Only the first byte of the UNSIGNED32 is mapped
				{Index: 0x2007, Subindex: 0, LengthBits: 8},
				{Index: 0x2005, Subindex: 0, LengthBits: 8},
			},
		})
	assert.Nil(t, err)
	assert.Nil(t, c.EnablePDO(1))
	time.Sleep(100 * time.Millisecond)

	assert.Nil(t, otherNet.Send(canopen.Frame{ID: rpdoCanId, DLC: 2, Data: [8]byte{0xAA, 0xBB}}))
	time.Sleep(100 * time.Millisecond)

	// Each object gets the bytes that are mapped to it, and only those
	valU32, err := local.ReadUint32("UNSIGNED32 value", 0)
	assert.Nil(t, err)
	assert.EqualValues(t, 0xAA, valU32)
	valU8, err := local.ReadUint8("UNSIGNED8 value", 0)
	assert.Nil(t, err)
	assert.EqualValues(t, 0xBB, valU8)
}

// RPDO that carries more bytes than are mapped is processed, the
// surplus bytes being ignored. Too short frames are discarded.
func TestRPDOOverlongFrame(t *testing.T) {
	const rpdoCanId = 0x255
	emcyCanId := 0x80 + uint32(NodeIdTest)

	net := CreateNetworkTest()
	otherNet := CreateNetworkEmptyTest()
	defer net.Disconnect()
	defer otherNet.Disconnect()

	local, err := net.Local(NodeIdTest)
	assert.Nil(t, err)

	emcyCollector := &FrameCollector{}
	_, err = otherNet.Subscribe(emcyCanId, 0x7FF, false, emcyCollector)
	assert.Nil(t, err)

	c := local.Configurator()
	assert.Nil(t, c.ProducerDisableSYNC())
	assert.Nil(t, c.WriteCommunicationPeriod(0))
	assert.Nil(t, c.DisablePDO(1))
	err = c.WriteConfigurationPDO(1,
		config.PDOConfigurationParameter{
			CanId:            rpdoCanId,
			TransmissionType: pdo.TransmissionTypeSyncEventHi,
			Mappings: []config.PDOMappingParameter{
				{Index: 0x2005, Subindex: 0, LengthBits: 8},
			},
		})
	assert.Nil(t, err)
	assert.Nil(t, c.EnablePDO(1))
	time.Sleep(100 * time.Millisecond)

	assert.Nil(t, local.WriteAnyExact("UNSIGNED8 value", 0, uint8(0)))
	emcyCollector.Clear()

	// A single byte is mapped but the producer pads its frame up to 8 bytes
	assert.Nil(t, otherNet.Send(canopen.Frame{
		ID:   rpdoCanId,
		DLC:  8,
		Data: [8]byte{0x42, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
	}))
	time.Sleep(100 * time.Millisecond)

	val, err := local.ReadUint8("UNSIGNED8 value", 0)
	assert.Nil(t, err)
	assert.EqualValues(t, 0x42, val)

	// The length error is still reported
	frames := emcyCollector.GetFrames(emcyCanId)
	assert.Len(t, frames, 1)
	if len(frames) > 0 {
		// ErrPdoLengthExc = 0x8220. Little endian : 20 82
		assert.EqualValues(t, 0x20, frames[0].Data[0])
		assert.EqualValues(t, 0x82, frames[0].Data[1])
	}

	// A frame that is too short is still discarded
	assert.Nil(t, otherNet.Send(canopen.Frame{ID: rpdoCanId, DLC: 0}))
	time.Sleep(100 * time.Millisecond)
	val, err = local.ReadUint8("UNSIGNED8 value", 0)
	assert.Nil(t, err)
	assert.EqualValues(t, 0x42, val)
}
