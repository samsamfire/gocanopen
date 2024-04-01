package network

import (
	"math"
	"testing"

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
	"REAL32 value": float64(math.Float32frombits(uint32(0x55555555))),
	"REAL64 value": math.Float64frombits(0x55555555),
}

func TestReadLocal(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	local, err := network.Local(NODE_ID_TEST)
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
			assert.Equal(t, key, val)
		}

	})
	t.Run("Read Uint", func(t *testing.T) {
		for indexName, key := range SDO_UNSIGNED_READ_MAP {
			val, _ := local.ReadUint(indexName, "")
			assert.Equal(t, key, val)
		}
		_, err := local.ReadUint("INTEGER8 value", "")
		assert.Equal(t, od.ODR_TYPE_MISMATCH, err)
	})

	t.Run("Read Uint", func(t *testing.T) {
		for indexName, key := range SDO_UNSIGNED_READ_MAP {
			val, _ := local.ReadUint(indexName, "")
			assert.Equal(t, key, val)
		}
		_, err := local.ReadUint("INTEGER8 value", "")
		assert.Equal(t, od.ODR_TYPE_MISMATCH, err)
	})

	t.Run("Read Int", func(t *testing.T) {
		for indexName, key := range SDO_INTEGER_READ_MAP {
			val, _ := local.ReadInt(indexName, "")
			assert.Equal(t, key, val)
		}
		_, err := local.ReadInt("UNSIGNED8 value", "")
		assert.Equal(t, od.ODR_TYPE_MISMATCH, err)
	})

	t.Run("Read Float", func(t *testing.T) {
		for indexName, key := range SDO_FLOAT_READ_MAP {
			val, _ := local.ReadFloat(indexName, "")
			assert.InDelta(t, key, val, 0.01)
		}
		_, err := local.ReadFloat("UNSIGNED8 value", "")
		assert.Equal(t, od.ODR_TYPE_MISMATCH, err)
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
	remote, err := network2.AddRemoteNode(NODE_ID_TEST, od.Default())
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
			assert.Equal(t, key, val)
		}

	})
	t.Run("Read Uint", func(t *testing.T) {
		for indexName, key := range SDO_UNSIGNED_READ_MAP {
			val, _ := remote.ReadUint(indexName, "")
			assert.Equal(t, key, val)
		}
		_, err := remote.ReadUint("INTEGER8 value", "")
		assert.Equal(t, od.ODR_TYPE_MISMATCH, err)
	})

	t.Run("Read Uint", func(t *testing.T) {
		for indexName, key := range SDO_UNSIGNED_READ_MAP {
			val, _ := remote.ReadUint(indexName, "")
			assert.Equal(t, key, val)
		}
		_, err := remote.ReadUint("INTEGER8 value", "")
		assert.Equal(t, od.ODR_TYPE_MISMATCH, err)
	})

	t.Run("Read Int", func(t *testing.T) {
		for indexName, key := range SDO_INTEGER_READ_MAP {
			val, _ := remote.ReadInt(indexName, "")
			assert.Equal(t, key, val)
		}
		_, err := remote.ReadInt("UNSIGNED8 value", "")
		assert.Equal(t, od.ODR_TYPE_MISMATCH, err)
	})

	t.Run("Read Float", func(t *testing.T) {
		for indexName, key := range SDO_FLOAT_READ_MAP {
			val, _ := remote.ReadFloat(indexName, "")
			assert.InDelta(t, key, val, 0.01)
		}
		_, err := remote.ReadFloat("UNSIGNED8 value", "")
		assert.Equal(t, od.ODR_TYPE_MISMATCH, err)
	})

	t.Run("Read String", func(t *testing.T) {
		val, err := remote.ReadString("VISIBLE STRING value", "")
		assert.Equal(t, "AStringCannotBeLongerThanTheDefaultValue", val)
		assert.Equal(t, nil, err, err)
	})

	t.Run("Write any", func(t *testing.T) {
		network2 := CreateNetworkEmptyTest()
		defer network2.Disconnect()
		remote, err := network2.AddRemoteNode(NODE_ID_TEST, od.Default())
		assert.Nil(t, err)
		err = remote.Write("REAL32 value", "", float32(1500.1))
		assert.Nil(t, err)
		val, _ := remote.ReadFloat("REAL32 value", "")
		assert.InDelta(t, 1500.1, val, 0.01)
	})

}
