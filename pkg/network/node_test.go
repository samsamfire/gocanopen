package network

import (
	"testing"
	"time"

	"github.com/samsamfire/gocanopen/pkg/node"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/stretchr/testify/assert"
)

var SDO_UNSIGNED_READ_MAP = map[string]uint64{
	"UNSIGNED8 value":  uint64(0x10),
	"UNSIGNED16 value": uint64(0x1111),
	"UNSIGNED32 value": uint64(0x22222222),
	"UNSIGNED64 value": uint64(0x55555555),
}

var SDO_INTEGER_READ_MAP = map[string]int64{
	"INTEGER8 value":  int64(0x33),
	"INTEGER16 value": int64(0x4444),
	"INTEGER32 value": int64(0x55555555),
	"INTEGER64 value": int64(0x55555555),
}

var SDO_FLOAT_READ_MAP = map[string]float64{
	"REAL32 value": float64(0.1),
	"REAL64 value": float64(0.55),
}

func TestAddRemoteNode(t *testing.T) {
	network := CreateNetworkTest()
	networkRemote := CreateNetworkEmptyTest()
	defer network.Disconnect()
	defer networkRemote.Disconnect()
	node, err := networkRemote.AddRemoteNode(NodeIdTest, od.Default())
	assert.Nil(t, err)
	assert.NotNil(t, node)
	err = node.StartPDOs(true)
	assert.Nil(t, err, err)
}

func TestCreateLocalNode(t *testing.T) {
	network := CreateNetworkTest()
	networkRemote := CreateNetworkEmptyTest()
	defer network.Disconnect()
	defer networkRemote.Disconnect()
	_, err := networkRemote.CreateLocalNode(0x90, od.Default())
	assert.Equal(t, ErrIdRange, err)
}

func TestReadLocal(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	local, err := network.Local(NodeIdTest)
	assert.Nil(t, err)
	t.Run("Read any", func(t *testing.T) {
		for indexName, key := range SDO_UNSIGNED_READ_MAP {
			val, _ := local.Read(indexName, "")
			assert.Equal(t, key, val)
		}
		for indexName, key := range SDO_INTEGER_READ_MAP {
			val, _ := local.Read(indexName, "")
			assert.Equal(t, key, val)
		}
		for indexName, key := range SDO_FLOAT_READ_MAP {
			val, _ := local.Read(indexName, "")
			assert.InDelta(t, key, val, 1e-5)
		}

	})
	t.Run("Read Uint", func(t *testing.T) {
		for indexName, key := range SDO_UNSIGNED_READ_MAP {
			val, _ := local.ReadUint(indexName, "")
			assert.Equal(t, key, val)
		}
		_, err := local.ReadUint("INTEGER8 value", "")
		assert.Equal(t, od.ErrTypeMismatch, err)
	})

	t.Run("Read Uint", func(t *testing.T) {
		for indexName, key := range SDO_UNSIGNED_READ_MAP {
			val, _ := local.ReadUint(indexName, "")
			assert.Equal(t, key, val)
		}
		_, err := local.ReadUint("INTEGER8 value", "")
		assert.Equal(t, od.ErrTypeMismatch, err)
	})

	t.Run("Read Int", func(t *testing.T) {
		for indexName, key := range SDO_INTEGER_READ_MAP {
			val, _ := local.ReadInt(indexName, "")
			assert.Equal(t, key, val)
		}
		_, err := local.ReadInt("UNSIGNED8 value", "")
		assert.Equal(t, od.ErrTypeMismatch, err)
	})

	t.Run("Read Float", func(t *testing.T) {
		for indexName, key := range SDO_FLOAT_READ_MAP {
			val, _ := local.ReadFloat(indexName, "")
			assert.InDelta(t, key, val, 0.01)
		}
		_, err := local.ReadFloat("UNSIGNED8 value", "")
		assert.Equal(t, od.ErrTypeMismatch, err)
	})

	t.Run("Read String", func(t *testing.T) {
		val, err := local.ReadString("VISIBLE STRING value", "")
		assert.Equal(t, "AStringCannotBeLongerThanTheDefaultValue", val)
		assert.Equal(t, nil, err, err)
	})

	t.Run("Write any", func(t *testing.T) {
		err = local.Write("REAL32 value", "", float32(1500.1))
		assert.Nil(t, err)
		val, _ := local.ReadFloat("REAL32 value", "")
		assert.InDelta(t, 1500.1, val, 0.01)
	})

}

func TestReadRemote(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	network2 := CreateNetworkEmptyTest()
	defer network2.Disconnect()
	remote, err := network2.AddRemoteNode(NodeIdTest, od.Default())
	assert.Nil(t, err)
	t.Run("Read any", func(t *testing.T) {
		for indexName, key := range SDO_UNSIGNED_READ_MAP {
			val, _ := remote.Read(indexName, "")
			assert.Equal(t, key, val)
		}
		for indexName, key := range SDO_INTEGER_READ_MAP {
			val, _ := remote.Read(indexName, "")
			assert.Equal(t, key, val)
		}
		for indexName, key := range SDO_FLOAT_READ_MAP {
			val, _ := remote.Read(indexName, "")
			assert.InDelta(t, key, val, 1e-5)
		}

	})
	t.Run("Read Uint", func(t *testing.T) {
		for indexName, key := range SDO_UNSIGNED_READ_MAP {
			val, _ := remote.ReadUint(indexName, "")
			assert.Equal(t, key, val)
		}
		_, err := remote.ReadUint("INTEGER8 value", "")
		assert.Equal(t, od.ErrTypeMismatch, err)
	})

	t.Run("Read Int", func(t *testing.T) {
		for indexName, key := range SDO_INTEGER_READ_MAP {
			val, _ := remote.ReadInt(indexName, "")
			assert.Equal(t, key, val)
		}
		_, err := remote.ReadInt("UNSIGNED8 value", "")
		assert.Equal(t, od.ErrTypeMismatch, err)
	})

	t.Run("Read Float", func(t *testing.T) {
		for indexName, key := range SDO_FLOAT_READ_MAP {
			val, _ := remote.ReadFloat(indexName, "")
			assert.InDelta(t, key, val, 0.01)
		}
		_, err := remote.ReadFloat("UNSIGNED8 value", "")
		assert.Equal(t, od.ErrTypeMismatch, err)
	})

	t.Run("Read String", func(t *testing.T) {
		val, err := remote.ReadString("VISIBLE STRING value", "")
		assert.Equal(t, "AStringCannotBeLongerThanTheDefaultValue", val)
		assert.Equal(t, nil, err, err)
	})

	t.Run("Write any", func(t *testing.T) {
		network2 := CreateNetworkEmptyTest()
		defer network2.Disconnect()
		remote, err := network2.AddRemoteNode(NodeIdTest, od.Default())
		assert.Nil(t, err)
		err = remote.Write("REAL32 value", "", float32(1500.1))
		assert.Nil(t, err)
		val, _ := remote.ReadFloat("REAL32 value", "")
		assert.InDelta(t, 1500.1, val, 0.01)
	})

}

func TestRemoteNodeRPDO(t *testing.T) {
	network := CreateNetworkTest()
	networkRemote := CreateNetworkEmptyTest()
	defer network.Disconnect()
	defer networkRemote.Disconnect()
	remoteNode, err := networkRemote.AddRemoteNode(NodeIdTest, od.Default())
	configurator := network.Configurator(NodeIdTest)
	configurator.EnablePDO(1 + 256)
	assert.Nil(t, err)
	assert.NotNil(t, remoteNode)
	err = network.WriteRaw(NodeIdTest, 0x2002, 0, []byte{10}, false)
	assert.Nil(t, err)
	time.Sleep(500 * time.Millisecond)
	read := make([]byte, 1)
	remoteNode.SDOClient.ReadRaw(0, 0x2002, 0x0, read)
	// assert.Equal(t, node.NodeRunning, remoteNode.GetState())
	assert.Equal(t, []byte{0x33}, read)
}

func TestRemoteNodeRPDOUsingRemote(t *testing.T) {
	network := CreateNetworkTest()
	networkRemote := CreateNetworkEmptyTest()
	defer network.Disconnect()
	defer networkRemote.Disconnect()
	remoteNode, err := networkRemote.AddRemoteNode(NodeIdTest, od.Default())
	assert.Nil(t, err)
	// Setup remote node PDOs by reading configuration from remote
	err = remoteNode.StartPDOs(false)
	assert.Nil(t, err)
	// Enable real node TPDO nb 1
	configurator := network.Configurator(NodeIdTest)
	err = configurator.EnablePDO(1 + 256)
	assert.Nil(t, err)
	assert.NotNil(t, remoteNode)
	// Write value to remote node
	err = network.WriteRaw(NodeIdTest, 0x2002, 0, []byte{10}, false)
	assert.Nil(t, err)
	time.Sleep(1000 * time.Millisecond)
	read := make([]byte, 1)
	// Check that value received from remote node was correctly updated in internal OD
	remoteNode.SDOClient.ReadRaw(0, 0x2002, 0x0, read)
	// assert.Equal(t, node.NodeRunning, remoteNode.GetState())
	assert.Equal(t, []byte{10}, read)
}

func TestTimeSynchronization(t *testing.T) {
	const slaveId = 0x66
	network := CreateNetworkTest()
	defer network.Disconnect()

	// Set master node as time producer with interval 100ms
	masterNode, _ := network.Local(NodeIdTest)
	masterNode.TIME.SetProducerIntervalMs(100)
	masterNode.Configurator().ProducerDisableTIME()

	time.Sleep(200 * time.Millisecond)

	// Create 10 slave nodes that will update there internal time
	slaveNodes := make([]*node.LocalNode, 0)
	for i := range 10 {
		odict := od.Default()
		err := odict.Index(od.EntryCobIdTIME).PutUint32(0, 0x100, true)
		assert.Nil(t, err)
		slaveNode, err := network.CreateLocalNode(slaveId+uint8(i), odict)
		assert.Nil(t, err)
		err = slaveNode.Configurator().ProducerDisableTIME()
		assert.Nil(t, err)
		err = slaveNode.Configurator().ConsumerEnableTIME()
		assert.Nil(t, err)
		// Set internal time of slave to now - 24h (wrong time)
		slaveNode.TIME.SetInternalTime(time.Now().Add(24 * time.Hour))
		slaveNodes = append(slaveNodes, slaveNode)
	}

	// Check that time difference between slaves and master is 24h
	for _, slaveNode := range slaveNodes {
		timeDiff := slaveNode.TIME.InternalTime().Sub(masterNode.TIME.InternalTime())
		assert.InDelta(t, 24, timeDiff.Hours(), 1)
	}
	// Start publishing time
	err := masterNode.Configurator().ProducerEnableTIME()
	assert.Nil(t, err)
	// After enabling producer, time should be updated inside all slave nodes
	time.Sleep(150 * time.Millisecond)
	for _, slaveNode := range slaveNodes {
		timeDiff := slaveNode.TIME.InternalTime().Sub(masterNode.TIME.InternalTime())
		assert.InDelta(t, 0, timeDiff.Milliseconds(), 5)
	}
}

func TestScan(t *testing.T) {
	network := CreateNetworkEmptyTest()
	network2 := CreateNetworkEmptyTest()
	defer network.Disconnect()
	defer network2.Disconnect()
	scan, err := network.Scan(100)
	assert.Len(t, scan, 0)
	assert.Nil(t, err)
	// Create some local nodes
	for i := range 10 {
		_, err := network.CreateLocalNode(uint8(i)+1, od.Default())
		assert.Nil(t, err)
	}
	// Scan from local
	scan, err = network.Scan(100)
	assert.Len(t, scan, 10)
	assert.Nil(t, err)
	// Scan from remote
	scan, err = network2.Scan(100)
	assert.Len(t, scan, 10)
	assert.Nil(t, err)
}

func TestExport(t *testing.T) {
	network := CreateNetworkEmptyTest()
	network2 := CreateNetworkEmptyTest()
	defer network.Disconnect()
	defer network2.Disconnect()

	// Create a local node
	network.CreateLocalNode(0x20, od.Default())
	remote, err := network2.AddRemoteNode(0x20, od.Default())
	assert.Nil(t, err)
	tempdir := t.TempDir()
	t.Run("dump successful", func(t *testing.T) {
		err = remote.Export(tempdir + "/dumped.eds")
		assert.Nil(t, err)
	})
	t.Run("load from dump", func(t *testing.T) {
		_, err := network2.AddRemoteNode(0x55, tempdir+"/dumped.eds")
		assert.Nil(t, err)
	})

}
