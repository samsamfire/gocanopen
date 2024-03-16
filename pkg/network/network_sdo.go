package network

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/samsamfire/gocanopen/pkg/od"
)

// Helper function for reading a remote node entry as bytes
func (network *Network) readBytes(nodeId uint8, index any, subindex any) ([]byte, uint8, error) {
	od, err := network.GetOD(nodeId)
	if err != nil {
		return nil, 0, err
	}
	// Find corresponding Variable inside OD
	// This will be used to determine information on the expected value
	entry := od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return nil, 0, err
	}
	data := make([]byte, odVar.DataLength())
	nbRead, err := network.ReadRaw(nodeId, entry.Index, odVar.SubIndex, data)
	if err != nil {
		return nil, 0, err
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
	return od.Decode(data, dataType)
}

// Same as Read but enforces the returned type as uint64
func (network *Network) ReadUint(nodeId uint8, index any, subindex any) (value uint64, e error) {
	data, dataType, err := network.readBytes(nodeId, index, subindex)
	if err != nil {
		return 0, err
	}
	e = od.CheckSize(len(data), dataType)
	if e != nil {
		return 0, e
	}
	// Cast to correct type
	switch dataType {
	case od.BOOLEAN, od.UNSIGNED8:
		return uint64(data[0]), nil
	case od.UNSIGNED16:
		return uint64(binary.LittleEndian.Uint16(data)), nil
	case od.UNSIGNED32:
		return uint64(binary.LittleEndian.Uint32(data)), nil
	case od.UNSIGNED64:
		return uint64(binary.LittleEndian.Uint64(data)), nil
	default:
		return 0, od.ODR_TYPE_MISMATCH
	}
}

// Same as Read but enforces the returned type as int64
func (network *Network) ReadInt(nodeId uint8, index any, subindex any) (value int64, e error) {
	data, dataType, err := network.readBytes(nodeId, index, subindex)
	if err != nil {
		return 0, err
	}
	e = od.CheckSize(len(data), dataType)
	if e != nil {
		return 0, e
	}
	// Cast to correct type
	switch dataType {
	case od.BOOLEAN, od.INTEGER8:
		return int64(data[0]), nil
	case od.INTEGER16:
		return int64(int16(binary.LittleEndian.Uint16(data))), nil
	case od.INTEGER32:
		return int64(int32(binary.LittleEndian.Uint32(data))), nil
	case od.INTEGER64:
		return int64(binary.LittleEndian.Uint64(data)), nil
	default:
		return 0, od.ODR_TYPE_MISMATCH
	}
}

// Same as Read but enforces the returned type as float
func (network *Network) ReadFloat(nodeId uint8, index any, subindex any) (value float64, e error) {
	data, dataType, err := network.readBytes(nodeId, index, subindex)
	if err != nil {
		return 0, err
	}
	e = od.CheckSize(len(data), dataType)
	if e != nil {
		return 0, e
	}
	// Cast to correct type
	switch dataType {
	case od.REAL32:
		parsed := binary.LittleEndian.Uint32(data)
		return float64(math.Float32frombits(parsed)), nil
	case od.REAL64:
		parsed := binary.LittleEndian.Uint64(data)
		return math.Float64frombits(parsed), nil
	default:
		return 0, od.ODR_TYPE_MISMATCH
	}
}

// Same as Read but enforces the returned type as string
func (network *Network) ReadString(nodeId uint8, index any, subindex any) (value string, e error) {
	data, dataType, err := network.readBytes(nodeId, index, subindex)
	if err != nil {
		return "", err
	}
	e = od.CheckSize(len(data), dataType)
	if e != nil {
		return "", e
	}
	// Cast to correct type
	switch dataType {
	case od.OCTET_STRING, od.VISIBLE_STRING, od.UNICODE_STRING:
		return string(data), nil
	default:
		return "", od.ODR_TYPE_MISMATCH
	}
}

// EDSFormatHandler takes a formatType and a reader
// to handle an EDS file stored as a proprietary format (zip, etc)
type EDSFormatHandler func(formatType uint8, reader io.Reader) (*od.ObjectDictionary, error)

// Read object dictionary using object 1021 (EDS storage) of a remote node
// Optional callback can be provided to perform manufacturer specific parsing
// in case a custom foramt is used
func (network *Network) ReadEDS(nodeId uint8, edsFormatHandler EDSFormatHandler) (*od.ObjectDictionary, error) {
	rawEds, err := network.ReadAll(nodeId, 0x1021, 0)
	if err != nil {
		return nil, err
	}
	edsFormat := []byte{0}
	_, err = network.ReadRaw(nodeId, 0x1022, 0, edsFormat)
	switch edsFormatHandler {
	case nil:
		// No callback & format is not specified or
		// Storage format is 0
		// Use default ASCII format
		if err != nil || edsFormat[0] == 0 {
			od, err := od.Parse(rawEds, nodeId)
			if err != nil {
				return nil, err
			}
			return od, nil
		} else {
			return nil, fmt.Errorf("supply a handler for the format : %v", edsFormat[0])
		}
	default:
		odReader := bytes.NewBuffer(rawEds)
		od, err := edsFormatHandler(edsFormat[0], odReader)
		if err != nil {
			return nil, err
		}
		return od, nil
	}
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
	entry := od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return err
	}

	err = network.WriteRaw(nodeId, entry.Index, odVar.SubIndex, value, false)
	if err != nil {
		return err
	}
	return nil
}
