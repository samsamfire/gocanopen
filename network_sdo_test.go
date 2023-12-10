package canopen

import (
	"math"
	"testing"
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
	"REAL32 value": math.Float64frombits(0x55555555),
	"REAL64 value": math.Float64frombits(0x55555555),
}

func TestRead(t *testing.T) {
	network := createNetwork()
	defer network.Disconnect()
	for indexName, key := range SDO_UNSIGNED_READ_MAP {
		val, _ := network.Read(NODE_ID_TEST, indexName, "")
		if val != key {
			t.Errorf("error or incorrect value %v (%v expected)", val, key)
		}
	}
	for indexName, key := range SDO_INTEGER_READ_MAP {
		val, _ := network.Read(NODE_ID_TEST, indexName, "")
		if val != key {
			t.Errorf("error or incorrect value %v (%v expected)", val, key)
		}
	}
	for indexName, key := range SDO_FLOAT_READ_MAP {
		val, _ := network.Read(NODE_ID_TEST, indexName, "")
		if val != key {
			t.Errorf("error or incorrect value %v (%v expected)", val, key)
		}
	}

}

func TestReadUint(t *testing.T) {
	network := createNetwork()
	defer network.Disconnect()
	for indexName, key := range SDO_UNSIGNED_READ_MAP {
		val, _ := network.ReadUint(NODE_ID_TEST, indexName, "")
		if val != key {
			t.Errorf("error or incorrect value %v (%v expected)", val, key)
		}
	}
	_, err := network.ReadUint(NODE_ID_TEST, "INTEGER8 value", "")
	if err != ODR_TYPE_MISMATCH {
		t.Errorf("should have type mismatch, instead %v", err)
	}

}

func TestReadInt(t *testing.T) {
	network := createNetwork()
	defer network.Disconnect()
	for indexName, key := range SDO_INTEGER_READ_MAP {
		val, _ := network.ReadInt(NODE_ID_TEST, indexName, "")
		if val != key {
			t.Errorf("error or incorrect value %v (%v expected)", val, key)
		}
	}
	_, err := network.ReadInt(NODE_ID_TEST, "UNSIGNED8 value", "")
	if err != ODR_TYPE_MISMATCH {
		t.Errorf("should have type mismatch, instead %v", err)
	}
}

func TestReadFloat(t *testing.T) {
	network := createNetwork()
	defer network.Disconnect()
	for indexName, key := range SDO_FLOAT_READ_MAP {
		val, _ := network.ReadFloat(NODE_ID_TEST, indexName, "")
		if val != key {
			t.Errorf("error or incorrect value %v (%v expected)", val, key)
		}
	}
	_, err := network.ReadFloat(NODE_ID_TEST, "UNSIGNED8 value", "")
	if err != ODR_TYPE_MISMATCH {
		t.Errorf("should have type mismatch, instead %v", err)
	}
}

func TestReadString(t *testing.T) {
	network := createNetwork()
	defer network.Disconnect()
	val, err := network.ReadString(NODE_ID_TEST, "VISIBLE STRING value", "")
	if err != nil || val != "AStringCannotBeLongerThanTheDefaultValue" {
		t.Fatal(err)
	}
}

func TestWrite(t *testing.T) {
	network := createNetwork()
	defer network.Disconnect()
	err := network.Write(NODE_ID_TEST, "REAL32 value", "", float32(1500.1))
	if err != nil {
		t.Fatal(err)
	}
	val, err := network.ReadFloat(NODE_ID_TEST, "REAL32 value", "")
	if err != nil || val-1500.1 > 0.01 {
		t.Errorf("value is %v (expected %v)", val, 1500.1)
	}
}
