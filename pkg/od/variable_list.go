package od

// VariableList is the data representation for
// storing a "RECORD" or "ARRAY" object type
type VariableList struct {
	objectType uint8 // either "RECORD" or "ARRAY"
	Variables  []*Variable
}

// GetSubObject returns the [Variable] corresponding to a given
// subindex.
func (rec *VariableList) GetSubObject(subindex uint8) (*Variable, error) {
	if rec.objectType == ObjectTypeARRAY {
		subEntriesCount := len(rec.Variables)
		if subindex >= uint8(subEntriesCount) {
			return nil, ErrSubNotExist
		}
		return rec.Variables[subindex], nil
	}
	for i, variable := range rec.Variables {
		if variable.SubIndex == subindex {
			return rec.Variables[i], nil
		}
	}
	return nil, ErrSubNotExist
}

// AddSubObject adds a [Variable] to the VariableList
// If the VariableList is an ARRAY then the subindex should be
// identical to the actual placement inside of the array.
// Otherwise it can be any valid subindex value, and the VariableList
// will grow accordingly
func (rec *VariableList) AddSubObject(
	subindex uint8,
	name string,
	datatype uint8,
	attribute uint8,
	value string,
) (*Variable, error) {
	encoded, err := EncodeFromString(value, datatype, 0)
	encodedCopy := make([]byte, len(encoded))
	copy(encodedCopy, encoded)
	if err != nil {
		return nil, err
	}
	if rec.objectType == ObjectTypeARRAY {
		if int(subindex) >= len(rec.Variables) {
			_logger.Error("trying to add a sub-object to array but ouf of bounds",
				"subindex", subindex,
				"length", len(rec.Variables),
			)
			return nil, ErrSubNotExist
		}
		variable, err := NewVariable(subindex, name, datatype, attribute, value)
		if err != nil {
			return nil, err
		}
		rec.Variables[subindex] = variable
		return rec.Variables[subindex], nil
	}
	variable, err := NewVariable(subindex, name, datatype, attribute, value)
	if err != nil {
		return nil, err
	}
	rec.Variables = append(rec.Variables, variable)
	return rec.Variables[len(rec.Variables)-1], nil
}

func NewRecord() *VariableList {
	return &VariableList{objectType: ObjectTypeRECORD, Variables: make([]*Variable, 0)}
}

func NewArray(length uint8) *VariableList {
	return &VariableList{objectType: ObjectTypeARRAY, Variables: make([]*Variable, length)}
}
