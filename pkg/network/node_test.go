package network

import (
	"fmt"
	"testing"
	"time"

	"github.com/samsamfire/gocanopen/pkg/node"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/samsamfire/gocanopen/pkg/pdo"
	"github.com/stretchr/testify/assert"
)

var SDO_BOOL_MAP = map[string]bool{
	"BOOLEAN value": true,
}

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

var SDO_STRING_READ_MAP = map[string]string{
	"VISIBLE STRING value": "AStringCannotBeLongerThanTheDefaultValue",
}

func TestRemoteNode(t *testing.T) {
	t.Run("add remote node", func(t *testing.T) {
		network := CreateNetworkTest()
		networkRemote := CreateNetworkEmptyTest()
		defer network.Disconnect()
		defer networkRemote.Disconnect()
		node, err := networkRemote.AddRemoteNode(NodeIdTest, od.Default())
		assert.Nil(t, err)
		assert.NotNil(t, node)
		err = node.StartPDOs(true)
		assert.Nil(t, err, err)
	})

	t.Run("wrong mapping in OD", func(t *testing.T) {
		odict := od.Default()
		odict.AddRPDO(1)
		pdoMap := od.NewRecord()
		pdoMap.AddSubObject(0, "Number of mapped application objects in PDO", od.UNSIGNED8, od.AttributeSdoRw, "0x1")
		for i := range od.MaxMappedEntriesPdo {
			pdoMap.AddSubObject(i+1, fmt.Sprintf("Application object %d", i+1), od.UNSIGNED32, od.AttributeSdoRw, "0x21000010")
		}
		odict.AddVariableList(od.EntryRPDOMappingStart, "RPDO mapping parameter", pdoMap)

		network := CreateNetworkTest()
		networkRemote := CreateNetworkEmptyTest()
		defer network.Disconnect()
		defer networkRemote.Disconnect()
		node, err := networkRemote.AddRemoteNode(NodeIdTest, odict)
		assert.Nil(t, err)
		assert.NotNil(t, node)
		err = node.StartPDOs(true)
		assert.Nil(t, err, err)

	})

	t.Run("rpdo updates correctly", func(t *testing.T) {
		network := CreateNetworkTest()
		networkRemote := CreateNetworkEmptyTest()
		defer network.Disconnect()
		defer networkRemote.Disconnect()
		remoteNode, err := networkRemote.AddRemoteNode(NodeIdTest, od.Default())
		configurator := network.Configurator(NodeIdTest)
		configurator.EnablePDO(1 + pdo.MaxRpdoNumber)
		assert.Nil(t, err)
		assert.NotNil(t, remoteNode)
		err = network.WriteRaw(NodeIdTest, 0x2002, 0, []byte{10}, false)
		assert.Nil(t, err)
		time.Sleep(500 * time.Millisecond)
		read := make([]byte, 1)
		remoteNode.SDOClient.ReadRaw(0, 0x2002, 0x0, read)
		// assert.Equal(t, node.NodeRunning, remoteNode.GetState())
		assert.Equal(t, []byte{0x33}, read)
	})

	t.Run("rpdo updates correctly using remote OD", func(t *testing.T) {
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
		err = configurator.EnablePDO(1 + pdo.MaxRpdoNumber)
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
	})
}

func TestCreateLocalNode(t *testing.T) {
	network := CreateNetworkTest()
	networkRemote := CreateNetworkEmptyTest()
	defer network.Disconnect()
	defer networkRemote.Disconnect()
	_, err := networkRemote.CreateLocalNode(0x90, od.Default())
	assert.Equal(t, ErrIdRange, err)
}

func TestNodeReadAnyExact(t *testing.T) {
	network := CreateNetworkTest()
	networkRemote := CreateNetworkEmptyTest()
	defer network.Disconnect()
	defer networkRemote.Disconnect()

	loc, err := network.Local(NodeIdTest)
	assert.Nil(t, err)
	remote, err := networkRemote.AddRemoteNode(NodeIdTest, od.Default())
	assert.Nil(t, err)

	for _, n := range []node.Node{loc, remote} {
		t.Run(fmt.Sprintf("Read Any Exact %T bool", n), func(t *testing.T) {
			val, err := n.ReadAnyExact("BOOLEAN value", "")
			assert.Equal(t, SDO_BOOL_MAP["BOOLEAN value"], val)
			assert.Nil(t, err)
		})
		t.Run(fmt.Sprintf("Read Any Exact %T uint8", n), func(t *testing.T) {
			val, err := n.ReadAnyExact("UNSIGNED8 value", "")
			assert.Equal(t, uint8(SDO_UNSIGNED_READ_MAP["UNSIGNED8 value"]), val)
			assert.Nil(t, err)
		})
		t.Run(fmt.Sprintf("Read Any Exact %T uint16", n), func(t *testing.T) {
			val, err := n.ReadAnyExact("UNSIGNED16 value", "")
			assert.Equal(t, uint16(SDO_UNSIGNED_READ_MAP["UNSIGNED16 value"]), val)
			assert.Nil(t, err)
		})
		t.Run(fmt.Sprintf("Read Any Exact %T uint32", n), func(t *testing.T) {
			val, err := n.ReadAnyExact("UNSIGNED32 value", "")
			assert.Equal(t, uint32(SDO_UNSIGNED_READ_MAP["UNSIGNED32 value"]), val)
			assert.Nil(t, err)
		})
		t.Run(fmt.Sprintf("Read Any Exact %T uint64", n), func(t *testing.T) {
			val, err := n.ReadAnyExact("UNSIGNED64 value", "")
			assert.Equal(t, uint64(SDO_UNSIGNED_READ_MAP["UNSIGNED64 value"]), val)
			assert.Nil(t, err)
		})

		t.Run(fmt.Sprintf("Read Any Exact %T int8", n), func(t *testing.T) {
			val, err := n.ReadAnyExact("INTEGER8 value", "")
			assert.Equal(t, int8(SDO_INTEGER_READ_MAP["INTEGER8 value"]), val)
			assert.Nil(t, err)
		})
		t.Run(fmt.Sprintf("Read Any Exact %T int16", n), func(t *testing.T) {
			val, err := n.ReadAnyExact("INTEGER16 value", "")
			assert.Equal(t, int16(SDO_INTEGER_READ_MAP["INTEGER16 value"]), val)
			assert.Nil(t, err)
		})
		t.Run(fmt.Sprintf("Read Any Exact %T int32", n), func(t *testing.T) {
			val, err := n.ReadAnyExact("INTEGER32 value", "")
			assert.Equal(t, int32(SDO_INTEGER_READ_MAP["INTEGER32 value"]), val)
			assert.Nil(t, err)
		})
		t.Run(fmt.Sprintf("Read Any Exact %T int64", n), func(t *testing.T) {
			val, err := n.ReadAnyExact("INTEGER64 value", "")
			assert.Equal(t, int64(SDO_INTEGER_READ_MAP["INTEGER64 value"]), val)
			assert.Nil(t, err)
		})
		t.Run(fmt.Sprintf("Read Any Exact %T float32", n), func(t *testing.T) {
			val, err := n.ReadAnyExact("REAL32 value", "")
			assert.Equal(t, float32(SDO_FLOAT_READ_MAP["REAL32 value"]), val)
			assert.Nil(t, err)
		})
		t.Run(fmt.Sprintf("Read Any Exact %T float64", n), func(t *testing.T) {
			val, err := n.ReadAnyExact("REAL64 value", "")
			assert.Equal(t, float64(SDO_FLOAT_READ_MAP["REAL64 value"]), val)
			assert.Nil(t, err)
		})
		t.Run(fmt.Sprintf("Read Any Exact %T string", n), func(t *testing.T) {
			val, err := n.ReadAnyExact("VISIBLE STRING value", "")
			assert.Equal(t, SDO_STRING_READ_MAP["VISIBLE STRING value"], val)
			assert.Nil(t, err)
		})
	}
}

func TestNodeReadX(t *testing.T) {
	network := CreateNetworkTest()
	networkRemote := CreateNetworkEmptyTest()
	defer network.Disconnect()
	defer networkRemote.Disconnect()

	loc, err := network.Local(NodeIdTest)
	assert.Nil(t, err)
	remote, err := networkRemote.AddRemoteNode(NodeIdTest, od.Default())
	assert.Nil(t, err)

	for _, n := range []node.Node{loc, remote} {

		t.Run(fmt.Sprintf("Read Bool %T valid entry", n), func(t *testing.T) {
			for indexName, value := range SDO_BOOL_MAP {
				val, err := n.ReadBool(indexName, "")
				assert.Equal(t, value, val)
				assert.Nil(t, err)
			}
		})

		t.Run(fmt.Sprintf("Read Bool %T invalid entry", n), func(t *testing.T) {
			_, err := n.ReadBool("UNSIGNED8 value", "")
			assert.Equal(t, od.ErrTypeMismatch, err)
		})

		t.Run(fmt.Sprintf("Read Any %T valid entries", n), func(t *testing.T) {
			for indexName, value := range SDO_UNSIGNED_READ_MAP {
				val, err := n.ReadAny(indexName, "")
				assert.Equal(t, value, val)
				assert.Nil(t, err)
			}
			for indexName, value := range SDO_INTEGER_READ_MAP {
				val, err := n.ReadAny(indexName, "")
				assert.Equal(t, value, val)
				assert.Nil(t, err)
			}
			for indexName, value := range SDO_FLOAT_READ_MAP {
				val, err := n.ReadAny(indexName, "")
				assert.InDelta(t, value, val, 1e-5)
				assert.Nil(t, err)
			}
		})

		t.Run(fmt.Sprintf("Read Uint %T valid entries", n), func(t *testing.T) {
			for indexName, value := range SDO_UNSIGNED_READ_MAP {
				val, err := n.ReadUint(indexName, "")
				assert.Equal(t, value, val)
				assert.Nil(t, err)
			}
		})

		t.Run(fmt.Sprintf("Read Uint %T invalid entry", n), func(t *testing.T) {
			_, err := n.ReadUint("INTEGER8 value", "")
			assert.Equal(t, od.ErrTypeMismatch, err)
		})

		t.Run(fmt.Sprintf("Read Int %T valid entries", n), func(t *testing.T) {
			for indexName, value := range SDO_INTEGER_READ_MAP {
				val, _ := n.ReadInt(indexName, "")
				assert.Equal(t, value, val)
			}
		})

		t.Run(fmt.Sprintf("Read Int %T invalid entry", n), func(t *testing.T) {
			_, err := n.ReadInt("UNSIGNED8 value", "")
			assert.Equal(t, od.ErrTypeMismatch, err)
		})

		t.Run(fmt.Sprintf("Read Float %T valid entries", n), func(t *testing.T) {
			for indexName, value := range SDO_FLOAT_READ_MAP {
				val, _ := n.ReadFloat(indexName, "")
				assert.InDelta(t, value, val, 0.01)
			}
		})

		t.Run(fmt.Sprintf("Read Float %T invalid entry", n), func(t *testing.T) {
			_, err := n.ReadFloat("UNSIGNED8 value", "")
			assert.Equal(t, od.ErrTypeMismatch, err)
		})

		t.Run(fmt.Sprintf("Read String %T valid entry", n), func(t *testing.T) {
			val, err := n.ReadString("VISIBLE STRING value", "")
			assert.Equal(t, "AStringCannotBeLongerThanTheDefaultValue", val)
			assert.Equal(t, nil, err, err)
		})
	}
}

func TestWriteExactThenReadWithType(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	local, _ := network.Local(NodeIdTest)
	t.Run("bool", func(t *testing.T) {
		err := local.WriteAnyExact("BOOLEAN value", 0, true)
		assert.Nil(t, err)
		v, err := local.ReadBool("BOOLEAN value", 0)
		assert.Nil(t, err)
		assert.Equal(t, true, v)
	})
	t.Run("uint8", func(t *testing.T) {
		err := local.WriteAnyExact("UNSIGNED8 value", 0, uint8(55))
		assert.Nil(t, err)
		v, err := local.ReadUint8("UNSIGNED8 value", 0)
		assert.Nil(t, err)
		assert.Equal(t, uint8(55), v)
	})
	t.Run("uint16", func(t *testing.T) {
		err := local.WriteAnyExact("UNSIGNED16 value", 0, uint16(1234))
		assert.Nil(t, err)
		v, err := local.ReadUint16("UNSIGNED16 value", 0)
		assert.Nil(t, err)
		assert.Equal(t, uint16(1234), v)
	})
	t.Run("uint32", func(t *testing.T) {
		err := local.WriteAnyExact("UNSIGNED32 value", 0, uint32(567899))
		assert.Nil(t, err)
		v, err := local.ReadUint32("UNSIGNED32 value", 0)
		assert.Nil(t, err)
		assert.Equal(t, uint32(567899), v)
	})
	t.Run("uint64", func(t *testing.T) {
		err := local.WriteAnyExact("UNSIGNED64 value", 0, uint64(1234321))
		assert.Nil(t, err)
		v, err := local.ReadUint64("UNSIGNED64 value", 0)
		assert.Nil(t, err)
		assert.Equal(t, uint64(1234321), v)
	})
	t.Run("int8", func(t *testing.T) {
		err := local.WriteAnyExact("INTEGER8 value", 0, int8(11))
		assert.Nil(t, err)
		v, err := local.ReadInt8("INTEGER8 value", 0)
		assert.Nil(t, err)
		assert.Equal(t, int8(11), v)
	})
	t.Run("int16", func(t *testing.T) {
		err := local.WriteAnyExact("INTEGER16 value", 0, int16(11231))
		assert.Nil(t, err)
		v, err := local.ReadInt16("INTEGER16 value", 0)
		assert.Nil(t, err)
		assert.Equal(t, int16(11231), v)
	})
	t.Run("int32", func(t *testing.T) {
		err := local.WriteAnyExact("INTEGER32 value", 0, int32(98789))
		assert.Nil(t, err)
		v, err := local.ReadInt32("INTEGER32 value", 0)
		assert.Nil(t, err)
		assert.Equal(t, int32(98789), v)
	})
	t.Run("int64", func(t *testing.T) {
		err := local.WriteAnyExact("INTEGER64 value", 0, int64(-5999))
		assert.Nil(t, err)
		v, err := local.ReadInt64("INTEGER64 value", 0)
		assert.Nil(t, err)
		assert.Equal(t, int64(-5999), v)
	})
	t.Run("float32", func(t *testing.T) {
		err := local.WriteAnyExact("REAL32 value", 0, float32(0.6))
		assert.Nil(t, err)
		v, err := local.ReadFloat32("REAL32 value", 0)
		assert.Nil(t, err)
		assert.Equal(t, float32(0.6), v)
	})
	t.Run("float64", func(t *testing.T) {
		err := local.WriteAnyExact("REAL64 value", 0, float64(1996.1))
		assert.Nil(t, err)
		v, err := local.ReadFloat64("REAL64 value", 0)
		assert.Nil(t, err)
		assert.Equal(t, float64(1996.1), v)
	})
	t.Run("string", func(t *testing.T) {
		err := local.WriteAnyExact("VISIBLE STRING value", 0, "hi there")
		assert.Nil(t, err)
		v, err := local.ReadString("VISIBLE STRING value", 0)
		assert.Nil(t, err)
		assert.Equal(t, "hi there", v)
	})
}

func TestTimeSynchronization(t *testing.T) {
	const slaveId = 0x66
	network := CreateNetworkTest()
	defer network.Disconnect()

	// Set master node as time producer with interval 100ms
	masterNode, _ := network.Local(NodeIdTest)
	masterNode.TIME.SetProducerInterval(100 * time.Millisecond)
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
		assert.InDelta(t, 0, timeDiff.Milliseconds(), 50)
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
