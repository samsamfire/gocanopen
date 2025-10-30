package node

// Read entry via direct local OD access
// - index should be the same as accepted by [od.ObjectDictionary.Index]
// - subindex should be the same as accepted by [od.Entry.SubIndex]
// returns value as actual OD "base" datatype
// i.e. one of : uint64, int64, float64, string, []byte
func (node *BaseNode) ReadAny(index any, subindex any) (any, error) {

	// We need index,subindex & datatype to be able to decode data.
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return nil, err
	}
	return odVar.Any()
}

// Read entry via direct local OD access
// - index should be the same as accepted by [od.ObjectDictionary.Index]
// - subindex should be the same as accepted by [od.Entry.SubIndex]
// returns the exact OD datatype :
// i.e. one of : uint8, ..., uint64, int8, ..., int64,
// float32, float64, string, []byte
func (node *BaseNode) ReadAnyExact(index any, subindex any) (any, error) {

	// We need index,subindex & datatype to be able to decode data.
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return nil, err
	}
	return odVar.AnyExact()
}

// Read entry via direct local OD access
// - index should be the same as accepted by [od.ObjectDictionary.Index]
// - subindex should be the same as accepted by [od.Entry.SubIndex]
// returns a copy of the OD value as raw []byte
func (node *BaseNode) ReadBytes(index any, subindex any) ([]byte, error) {
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return nil, err
	}
	return odVar.Bytes(), nil
}

// Read entry via direct local OD access
// - index should be the same as accepted by [od.ObjectDictionary.Index]
// - subindex should be the same as accepted by [od.Entry.SubIndex]
// returns as bool
func (node *BaseNode) ReadBool(index any, subindex any) (bool, error) {
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return false, err
	}
	return odVar.Bool()
}

// Read entry via direct local OD access
// - index should be the same as accepted by [od.ObjectDictionary.Index]
// - subindex should be the same as accepted by [od.Entry.SubIndex]
// returns uint8, uint16, uint32, uint64 value as uint64
func (node *BaseNode) ReadUint(index any, subindex any) (value uint64, e error) {
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return 0, err
	}
	return odVar.Uint()
}

// Read entry via direct local OD access
// - index should be the same as accepted by [od.ObjectDictionary.Index]
// - subindex should be the same as accepted by [od.Entry.SubIndex]
// returns int8, int16, int32, int64 value as int64
func (node *BaseNode) ReadInt(index any, subindex any) (value int64, e error) {
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return 0, err
	}
	return odVar.Int()
}

// Read entry via direct local OD access
// - index should be the same as accepted by [od.ObjectDictionary.Index]
// - subindex should be the same as accepted by [od.Entry.SubIndex]
// returns float32, float64 value as float64
func (node *BaseNode) ReadFloat(index any, subindex any) (value float64, e error) {
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return 0, err
	}
	return odVar.Float()
}

// Read entry via direct local OD access
// - index should be the same as accepted by [od.ObjectDictionary.Index]
// - subindex should be the same as accepted by [od.Entry.SubIndex]
// returns value as string
func (node *BaseNode) ReadString(index any, subindex any) (value string, e error) {
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return "", err
	}
	return odVar.String()
}

// Read entry via direct local OD access
// - index should be the same as accepted by [od.ObjectDictionary.Index]
// - subindex should be the same as accepted by [od.Entry.SubIndex]
// returns value as uint8
func (node *BaseNode) ReadUint8(index any, subindex any) (value uint8, e error) {
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return 0, err
	}
	return odVar.Uint8()
}

// Read entry via direct local OD access
// - index should be the same as accepted by [od.ObjectDictionary.Index]
// - subindex should be the same as accepted by [od.Entry.SubIndex]
// returns value as uint16
func (node *BaseNode) ReadUint16(index any, subindex any) (value uint16, e error) {
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return 0, err
	}
	return odVar.Uint16()
}

// Read entry via direct local OD access
// - index should be the same as accepted by [od.ObjectDictionary.Index]
// - subindex should be the same as accepted by [od.Entry.SubIndex]
// returns value as uint32
func (node *BaseNode) ReadUint32(index any, subindex any) (value uint32, e error) {
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return 0, err
	}
	return odVar.Uint32()
}

// Read entry via direct local OD access
// - index should be the same as accepted by [od.ObjectDictionary.Index]
// - subindex should be the same as accepted by [od.Entry.SubIndex]
// returns value as uint64
func (node *BaseNode) ReadUint64(index any, subindex any) (value uint64, e error) {
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return 0, err
	}
	return odVar.Uint64()
}

// Read entry via direct local OD access
// - index should be the same as accepted by [od.ObjectDictionary.Index]
// - subindex should be the same as accepted by [od.Entry.SubIndex]
// returns value as int8
func (node *BaseNode) ReadInt8(index any, subindex any) (value int8, e error) {
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return 0, err
	}
	return odVar.Int8()
}

// Read entry via direct local OD access
// - index should be the same as accepted by [od.ObjectDictionary.Index]
// - subindex should be the same as accepted by [od.Entry.SubIndex]
// returns value as int16
func (node *BaseNode) ReadInt16(index any, subindex any) (value int16, e error) {
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return 0, err
	}
	return odVar.Int16()
}

// Read entry via direct local OD access
// - index should be the same as accepted by [od.ObjectDictionary.Index]
// - subindex should be the same as accepted by [od.Entry.SubIndex]
// returns value as int32
func (node *BaseNode) ReadInt32(index any, subindex any) (value int32, e error) {
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return 0, err
	}
	return odVar.Int32()
}

// Read entry via direct local OD access
// - index should be the same as accepted by [od.ObjectDictionary.Index]
// - subindex should be the same as accepted by [od.Entry.SubIndex]
// returns value as int64
func (node *BaseNode) ReadInt64(index any, subindex any) (value int64, e error) {
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return 0, err
	}
	return odVar.Int64()
}

// Read entry via direct local OD access
// - index should be the same as accepted by [od.ObjectDictionary.Index]
// - subindex should be the same as accepted by [od.Entry.SubIndex]
// returns value as float32
func (node *BaseNode) ReadFloat32(index any, subindex any) (value float32, e error) {
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return 0, err
	}
	return odVar.Float32()
}

// Read entry via direct local OD access
// - index should be the same as accepted by [od.ObjectDictionary.Index]
// - subindex should be the same as accepted by [od.Entry.SubIndex]
// returns value as float64
func (node *BaseNode) ReadFloat64(index any, subindex any) (value float64, e error) {
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return 0, err
	}
	return odVar.Float64()
}

// Write entry via direct local OD access
// - index should be the same as accepted by [od.ObjectDictionary.Index]
// - subindex should be the same as accepted by [od.Entry.SubIndex]
// write any datatype i.e. one of : uint8, ..., uint64, int8, ..., int64,
// float32, float64, string, []byte
func (node *BaseNode) WriteAnyExact(index any, subindex any, value any) error {
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return err
	}
	return odVar.PutAnyExact(value)
}

// Write entry via direct local OD access
// - index should be the same as accepted by [od.ObjectDictionary.Index]
// - subindex should be the same as accepted by [od.Entry.SubIndex]
// write data as raw bytes, only length will be checked, no assumtions
// are made.
func (node *BaseNode) WriteBytes(index any, subindex any, value []byte) error {
	entry := node.od.Index(index)
	odVar, err := entry.SubIndex(subindex)
	if err != nil {
		return err
	}
	return odVar.PutBytes(value)
}
