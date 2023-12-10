package canopen

import (
	"encoding/binary"
	"math"
)

// Helper function for reading a remote node entry as bytes
func (network *Network) readBytes(nodeId uint8, index any, subindex any) ([]byte, uint8, error) {
	od, err := network.GetOD(nodeId)
	if err != nil {
		return nil, 0, err
	}
	// Find corresponding Variable inside OD
	// This will be used to determine information on the expected value
	odVar, e := od.Index(index).SubIndex(subindex)
	if e != nil {
		return nil, 0, e
	}
	data := make([]byte, odVar.DataLength())
	nbRead, e := network.sdoClient.ReadRaw(nodeId, odVar.Index, odVar.SubIndex, data)
	if e != nil {
		return nil, 0, e
	}
	return data[:nbRead], odVar.DataType, nil
}

// Read an entry from a remote node
// index and subindex can either be strings or integers
// this method requires the corresponding node OD to be loaded
// Returned value can be either string, uint64, int64 or float64
func (network *Network) Read(nodeId uint8, index any, subindex any) (value any, e error) {
	data, dataType, err := network.readBytes(nodeId, index, subindex)
	if err != nil {
		return nil, err
	}
	return decode(data, dataType)
}

// Same as Read but enforces the returned type as uint64
func (network *Network) ReadUint(nodeId uint8, index any, subindex any) (value uint64, e error) {
	data, dataType, err := network.readBytes(nodeId, index, subindex)
	if err != nil {
		return 0, err
	}
	e = checkSize(data, dataType)
	if e != nil {
		return 0, e
	}
	// Cast to correct type
	switch dataType {
	case BOOLEAN, UNSIGNED8:
		return uint64(data[0]), nil
	case UNSIGNED16:
		return uint64(binary.LittleEndian.Uint16(data)), nil
	case UNSIGNED32:
		return uint64(binary.LittleEndian.Uint32(data)), nil
	case UNSIGNED64:
		return uint64(binary.LittleEndian.Uint64(data)), nil
	default:
		return 0, ODR_TYPE_MISMATCH
	}
}

// Same as Read but enforces the returned type as int64
func (network *Network) ReadInt(nodeId uint8, index any, subindex any) (value int64, e error) {
	data, dataType, err := network.readBytes(nodeId, index, subindex)
	if err != nil {
		return 0, err
	}
	e = checkSize(data, dataType)
	if e != nil {
		return 0, e
	}
	// Cast to correct type
	switch dataType {
	case BOOLEAN, INTEGER8:
		return int64(data[0]), nil
	case INTEGER16:
		return int64(int16(binary.LittleEndian.Uint16(data))), nil
	case INTEGER32:
		return int64(int32(binary.LittleEndian.Uint32(data))), nil
	case INTEGER64:
		return int64(binary.LittleEndian.Uint64(data)), nil
	default:
		return 0, ODR_TYPE_MISMATCH
	}
}

// Same as Read but enforces the returned type as float
func (network *Network) ReadFloat(nodeId uint8, index any, subindex any) (value float64, e error) {
	data, dataType, err := network.readBytes(nodeId, index, subindex)
	if err != nil {
		return 0, err
	}
	e = checkSize(data, dataType)
	if e != nil {
		return 0, e
	}
	// Cast to correct type
	switch dataType {
	case REAL32:
		parsed := binary.LittleEndian.Uint32(data)
		return float64(math.Float32frombits(parsed)), nil
	case REAL64:
		parsed := binary.LittleEndian.Uint64(data)
		return math.Float64frombits(parsed), nil
	default:
		return 0, ODR_TYPE_MISMATCH
	}
}

// Same as Read but enforces the returned type as string
func (network *Network) ReadString(nodeId uint8, index any, subindex any) (value string, e error) {
	data, dataType, err := network.readBytes(nodeId, index, subindex)
	if err != nil {
		return "", err
	}
	e = checkSize(data, dataType)
	if e != nil {
		return "", e
	}
	// Cast to correct type
	switch dataType {
	case OCTET_STRING, VISIBLE_STRING, UNICODE_STRING:
		return string(data), nil
	default:
		return "", ODR_TYPE_MISMATCH
	}
}

// Read an entry from a remote node
// this method does not require corresponding OD to be loaded
// value will be read as a raw byte slice
// does not support block transfer
func (network *Network) ReadRaw(nodeId uint8, index uint16, subIndex uint8, data []byte) (int, error) {
	return network.sdoClient.ReadRaw(nodeId, index, subIndex, data)
}

// Write an entry to a remote node
// index and subindex can either be strings or integers
// this method requires the corresponding node OD to be loaded
// value should correspond to the expected datatype
func (network *Network) Write(nodeId uint8, index any, subindex any, value any) error {
	od, err := network.GetOD(nodeId)
	if err != nil {
		return err
	}
	// Find corresponding Variable inside OD
	// This will be used to determine information on the expected value
	odVar, e := od.Index(index).SubIndex(subindex)
	if e != nil {
		return e
	}
	// TODO : maybe check data type with the current OD ?
	var encoded []byte
	switch val := value.(type) {
	case uint8:
		encoded = []byte{val}
	case int8:
		encoded = []byte{byte(val)}
	case uint16:
		encoded = make([]byte, 2)
		binary.LittleEndian.PutUint16(encoded, val)
	case int16:
		encoded = make([]byte, 2)
		binary.LittleEndian.PutUint16(encoded, uint16(val))
	case uint32:
		encoded = make([]byte, 4)
		binary.LittleEndian.PutUint32(encoded, val)
	case int32:
		encoded = make([]byte, 4)
		binary.LittleEndian.PutUint32(encoded, uint32(val))
	case uint64:
		encoded = make([]byte, 8)
		binary.LittleEndian.PutUint64(encoded, val)
	case int64:
		encoded = make([]byte, 8)
		binary.LittleEndian.PutUint64(encoded, uint64(val))
	case string:
		encoded = []byte(val)
	case float32:
		encoded = make([]byte, 4)
		binary.LittleEndian.PutUint32(encoded, math.Float32bits(val))
	case float64:
		encoded = make([]byte, 8)
		binary.LittleEndian.PutUint64(encoded, math.Float64bits(val))
	default:
		return ODR_TYPE_MISMATCH
	}
	e = network.sdoClient.WriteRaw(nodeId, odVar.Index, odVar.SubIndex, encoded, false)
	if e != nil {
		return e
	}
	return nil
}

// Write an entry to a remote node
// this method does not require corresponding OD to be loaded
// value will be written as a raw byte slice
// does not support block transfer
func (network *Network) WriteRaw(nodeId uint8, index uint16, subIndex uint8, data []byte) error {
	return network.sdoClient.WriteRaw(nodeId, index, subIndex, data, false)
}
