package node

import (
	"io"

	"github.com/samsamfire/gocanopen/pkg/od"
)

// import (
// 	"io"

// 	"github.com/samsamfire/gocanopen/pkg/od"
// )

// // Read an entry using a base sdo client
// // index and subindex can either be strings or integers
// // this method requires the corresponding node OD to be loaded
// // returned value can be either string, uint64, int64 or float64
func (node *BaseNode) ReadAny(index any, subindex any) (any, error) {

	// We need index,subindex & datatype to be able to decode data.
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return nil, err
	}
	r, err := node.SDOClient.NewRawReader(
		node.GetID(),
		entry.Index,
		odVar.SubIndex,
		false,
		0,
	) // size not specified
	if err != nil {
		return 0, err
	}
	// Perform the actual read. This can be long
	n, err := r.Read(node.rxBuffer)
	if err != nil && err != io.EOF {
		return n, err
	}
	// Decode data to ~type
	return od.DecodeToType(node.rxBuffer[:n], odVar.DataType)
}

// Read an entry using a base sdo client
// index and subindex can either be strings or integers
// this method requires the corresponding node OD to be loaded
// returned value corresponds to the exact datatype
// (uint8,uint16,...,int8,int16,...,float32,float64,...)
func (node *BaseNode) ReadAnyExact(index any, subindex any) (any, error) {

	// We need index,subindex & datatype to be able to decode data.
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return nil, err
	}
	r, err := node.SDOClient.NewRawReader(
		node.GetID(),
		entry.Index,
		odVar.SubIndex,
		false,
		0,
	) // size not specified
	if err != nil {
		return 0, err
	}
	// Perform the actual read. This can be long
	n, err := r.Read(node.rxBuffer)
	if err != nil && err != io.EOF {
		return n, err
	}
	// Decode data to ~type
	return od.DecodeToTypeExact(node.rxBuffer[:n], odVar.DataType)
}

// [Deprecated] use ReadAny instead
func (node *BaseNode) Read(index any, subindex any) (value any, e error) {
	return node.ReadAny(index, subindex)
}

// Same as [ReadAny] but enforces the returned type as uint64
func (node *RemoteNode) ReadUint(index any, subindex any) (value uint64, e error) {
	v, err := node.ReadAny(index, subindex)
	if err != nil {
		return 0, err
	}
	value, ok := v.(uint64)
	if !ok {
		return 0, od.ErrTypeMismatch
	}
	return value, nil
}

// Same as [ReadAny] but enforces the returned type as uint32
func (node *RemoteNode) ReadUint32(index any, subindex any) (value uint32, e error) {
	v, err := node.ReadAnyExact(index, subindex)
	if err != nil {
		return 0, err
	}
	value, ok := v.(uint32)
	if !ok {
		return 0, od.ErrTypeMismatch
	}
	return value, nil
}

// Same as [ReadAny] but enforces the returned type as uint16
func (node *RemoteNode) ReadUint16(index any, subindex any) (value uint16, e error) {
	v, err := node.ReadAnyExact(index, subindex)
	if err != nil {
		return 0, err
	}
	value, ok := v.(uint16)
	if !ok {
		return 0, od.ErrTypeMismatch
	}
	return value, nil
}

// Same as [ReadAny] but enforces the returned type as uint8
func (node *RemoteNode) ReadUint8(index any, subindex any) (value uint8, e error) {
	v, err := node.ReadAnyExact(index, subindex)
	if err != nil {
		return 0, err
	}
	value, ok := v.(uint8)
	if !ok {
		return 0, od.ErrTypeMismatch
	}
	return value, nil
}

// Same as [ReadAny] but enforces the returned type as int64
func (node *RemoteNode) ReadInt(index any, subindex any) (value int64, e error) {
	v, err := node.ReadAny(index, subindex)
	if err != nil {
		return 0, err
	}
	value, ok := v.(int64)
	if !ok {
		return 0, od.ErrTypeMismatch
	}
	return value, nil
}

// Same as [ReadAny] but enforces the returned type as int32
func (node *RemoteNode) ReadInt32(index any, subindex any) (value int32, e error) {
	v, err := node.ReadAnyExact(index, subindex)
	if err != nil {
		return 0, err
	}
	value, ok := v.(int32)
	if !ok {
		return 0, od.ErrTypeMismatch
	}
	return value, nil
}

// Same as [ReadAny] but enforces the returned type as int16
func (node *RemoteNode) ReadInt16(index any, subindex any) (value int16, e error) {
	v, err := node.ReadAnyExact(index, subindex)
	if err != nil {
		return 0, err
	}
	value, ok := v.(int16)
	if !ok {
		return 0, od.ErrTypeMismatch
	}
	return value, nil
}

// Same as [ReadAny] but enforces the returned type as int8
func (node *RemoteNode) ReadInt8(index any, subindex any) (value int8, e error) {
	v, err := node.ReadAnyExact(index, subindex)
	if err != nil {
		return 0, err
	}
	value, ok := v.(int8)
	if !ok {
		return 0, od.ErrTypeMismatch
	}
	return value, nil
}

// Same as [ReadAny] but enforces the returned type as float64
func (node *RemoteNode) ReadFloat(index any, subindex any) (value float64, e error) {
	v, err := node.ReadAny(index, subindex)
	if err != nil {
		return 0, err
	}
	value, ok := v.(float64)
	if !ok {
		return 0, od.ErrTypeMismatch
	}
	return value, nil
}

// Same as [ReadAny] but enforces the returned type as float32
func (node *RemoteNode) ReadFloat32(index any, subindex any) (value float32, e error) {
	v, err := node.ReadAnyExact(index, subindex)
	if err != nil {
		return 0, err
	}
	value, ok := v.(float32)
	if !ok {
		return 0, od.ErrTypeMismatch
	}
	return value, nil
}

// Same as [ReadAny] but enforces the returned type as string
func (node *RemoteNode) ReadString(index any, subindex any) (value string, e error) {
	v, err := node.ReadAny(index, subindex)
	if err != nil {
		return "", err
	}
	value, ok := v.(string)
	if !ok {
		return "", od.ErrTypeMismatch
	}
	return value, nil
}

// // Read an entry from a remote node
// // this method does not require corresponding OD to be loaded
// // value will be read as a raw byte slice
// // does not support block transfer
func (node *BaseNode) ReadRaw(index uint16, subIndex uint8, data []byte) (int, error) {
	return node.SDOClient.ReadRaw(node.id, index, subIndex, data)
}

// Write an entry to a remote node
// index and subindex can either be strings or integers
// this method requires the corresponding node OD to be loaded
// value should correspond to the expected datatype
func (node *BaseNode) WriteAny(index any, subindex any, value any) error {
	// Find corresponding Variable inside OD
	// This will be used to determine information on the expected value
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return err
	}
	return node.SDOClient.WriteRaw(node.id, entry.Index, odVar.SubIndex, value, false)
}

// [Deprecated] use WriteAny instead
func (node *BaseNode) Write(index any, subindex any, value any) error {
	return node.WriteAny(index, subindex, value)
}

// Write an entry to a remote node
// this method does not require corresponding OD to be loaded
// value will be written as a raw byte slice
// does not support block transfer
func (node *BaseNode) WriteRaw(index uint16, subIndex uint8, data []byte) error {
	return node.SDOClient.WriteRaw(node.id, index, subIndex, data, false)
}
