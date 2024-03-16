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

func TestRead(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	for indexName, key := range SDO_UNSIGNED_READ_MAP {
		val, _ := network.Read(NODE_ID_TEST, indexName, "")
		assert.Equal(t, key, val)
	}
	for indexName, key := range SDO_INTEGER_READ_MAP {
		val, _ := network.Read(NODE_ID_TEST, indexName, "")
		assert.Equal(t, key, val)
	}
	for indexName, key := range SDO_FLOAT_READ_MAP {
		val, _ := network.Read(NODE_ID_TEST, indexName, "")
		assert.Equal(t, key, val)
	}
}

func TestReadUint(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	for indexName, key := range SDO_UNSIGNED_READ_MAP {
		val, _ := network.ReadUint(NODE_ID_TEST, indexName, "")
		assert.Equal(t, key, val)
	}
	_, err := network.ReadUint(NODE_ID_TEST, "INTEGER8 value", "")
	assert.Equal(t, od.ODR_TYPE_MISMATCH, err)
}

func TestReadInt(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	for indexName, key := range SDO_INTEGER_READ_MAP {
		val, _ := network.ReadInt(NODE_ID_TEST, indexName, "")
		assert.Equal(t, key, val)
	}
	_, err := network.ReadInt(NODE_ID_TEST, "UNSIGNED8 value", "")
	assert.Equal(t, od.ODR_TYPE_MISMATCH, err)
}

func TestReadFloat(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	for indexName, key := range SDO_FLOAT_READ_MAP {
		val, _ := network.ReadFloat(NODE_ID_TEST, indexName, "")
		assert.InDelta(t, key, val, 0.01)
	}
	_, err := network.ReadFloat(NODE_ID_TEST, "UNSIGNED8 value", "")
	assert.Equal(t, od.ODR_TYPE_MISMATCH, err)
}

func TestReadString(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	val, err := network.ReadString(NODE_ID_TEST, "VISIBLE STRING value", "")
	assert.Equal(t, "AStringCannotBeLongerThanTheDefaultValue", val)
	assert.Equal(t, nil, err, err)
}

func TestWrite(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	err := network.Write(NODE_ID_TEST, "REAL32 value", "", float32(1500.1))
	assert.Nil(t, err)
	val, _ := network.ReadFloat(NODE_ID_TEST, "REAL32 value", "")
	assert.InDelta(t, 1500.1, val, 0.01)
}
