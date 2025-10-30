package od

import (
	"encoding/binary"
	"math"
	"sync"
)

// Variable is the main data representation for a value stored inside of OD
// It is used to store a "VAR" or "DOMAIN" object type as well as
// any sub entry of a "RECORD" or "ARRAY" object type
type Variable struct {
	mu           sync.RWMutex
	valueDefault []byte
	value        []byte
	// Name of this variable
	Name string
	// The CiA 301 data type of this variable
	DataType byte
	// Attribute contains the access type as well as the mapping
	// information. e.g. AttributeSdoRw | AttributeRpdo
	Attribute uint8
	// StorageLocation has information on which medium is the data
	// stored. Currently this is unused, everything is stored in RAM
	StorageLocation string
	// The minimum value for this variable
	lowLimit []byte
	// The maximum value for this variable
	highLimit []byte
	// The subindex for this variable if part of an ARRAY or RECORD
	SubIndex uint8
}

// Create a new variable
func NewVariable(
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
	variable := &Variable{
		SubIndex:     subindex,
		Name:         name,
		value:        encoded,
		valueDefault: encodedCopy,
		Attribute:    attribute,
		DataType:     datatype,
	}
	return variable, nil
}

// Return number of bytes
func (v *Variable) DataLength() uint32 {
	return uint32(len(v.value))
}

// Return default value as byte slice
func (v *Variable) DefaultValue() []byte {
	return v.valueDefault
}

func (v *Variable) Bool() (bool, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.DataType != BOOLEAN {
		return false, ErrTypeMismatch
	}
	return v.value[0] > 0, nil
}

// Return value as uint64 (only for UNSIGNED types)
func (v *Variable) Uint() (uint64, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	switch v.DataType {
	case UNSIGNED8:
		return uint64(v.value[0]), nil
	case UNSIGNED16:
		return uint64(binary.LittleEndian.Uint16(v.value)), nil
	case UNSIGNED32:
		return uint64(binary.LittleEndian.Uint32(v.value)), nil
	case UNSIGNED64:
		return binary.LittleEndian.Uint64(v.value), nil
	default:
		return 0, ErrTypeMismatch
	}
}

func (v *Variable) Uint8() (uint8, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.DataType != UNSIGNED8 {
		return 0, ErrTypeMismatch
	}
	return v.value[0], nil
}

func (v *Variable) Uint16() (uint16, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.DataType != UNSIGNED16 {
		return 0, ErrTypeMismatch
	}
	return binary.LittleEndian.Uint16(v.value), nil
}

func (v *Variable) Uint32() (uint32, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.DataType != UNSIGNED32 {
		return 0, ErrTypeMismatch
	}
	return binary.LittleEndian.Uint32(v.value), nil
}

func (v *Variable) Uint64() (uint64, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.DataType != UNSIGNED64 {
		return 0, ErrTypeMismatch
	}
	return binary.LittleEndian.Uint64(v.value), nil
}

// Return value as int64 (only for SIGNED types)
func (v *Variable) Int() (int64, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	switch v.DataType {
	case INTEGER8:
		return int64(int8(v.value[0])), nil
	case INTEGER16:
		return int64(int16(binary.LittleEndian.Uint16(v.value))), nil
	case INTEGER32:
		return int64(int32(binary.LittleEndian.Uint32(v.value))), nil
	case INTEGER64:
		return int64(binary.LittleEndian.Uint64(v.value)), nil
	default:
		return 0, ErrTypeMismatch
	}
}
func (v *Variable) Int8() (int8, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.DataType != INTEGER8 {
		return 0, ErrTypeMismatch
	}
	return int8(v.value[0]), nil
}

func (v *Variable) Int16() (int16, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.DataType != INTEGER16 {
		return 0, ErrTypeMismatch
	}
	return int16(binary.LittleEndian.Uint16(v.value)), nil
}

func (v *Variable) Int32() (int32, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.DataType != INTEGER32 {
		return 0, ErrTypeMismatch
	}
	return int32(binary.LittleEndian.Uint32(v.value)), nil
}

func (v *Variable) Int64() (int64, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.DataType != INTEGER64 {
		return 0, ErrTypeMismatch
	}
	return int64(binary.LittleEndian.Uint64(v.value)), nil
}

// Return value as float64 (only for REAL types)
func (v *Variable) Float() (float64, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	switch v.DataType {
	case REAL32:
		parsed := binary.LittleEndian.Uint32(v.value)
		return float64(math.Float32frombits(parsed)), nil
	case REAL64:
		parsed := binary.LittleEndian.Uint64(v.value)
		return math.Float64frombits(parsed), nil
	default:
		return 0, ErrTypeMismatch
	}
}

func (v *Variable) Float32() (float32, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.DataType != REAL32 {
		return 0, ErrTypeMismatch
	}
	parsed := binary.LittleEndian.Uint32(v.value)
	return math.Float32frombits(parsed), nil
}

func (v *Variable) Float64() (float64, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.DataType != REAL64 {
		return 0, ErrTypeMismatch
	}
	parsed := binary.LittleEndian.Uint64(v.value)
	return math.Float64frombits(parsed), nil
}

// Return value as string (only for STRING types)
func (v *Variable) String() (string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	switch v.DataType {
	case VISIBLE_STRING, OCTET_STRING:
		// Stop at first null byte
		// TODO
		return string(v.value), nil
	default:
		return "", ErrTypeMismatch
	}
}

// Return value as byte slice
func (v *Variable) Bytes() []byte {
	v.mu.RLock()
	defer v.mu.RUnlock()

	copied := make([]byte, len(v.value))
	copy(copied, v.value)
	return copied
}

// Return value as underlying "base" datatype
func (v *Variable) Any() (any, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return DecodeToType(v.value, v.DataType)
}

// Return value as underlying datatype
func (v *Variable) AnyExact() (any, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return DecodeToTypeExact(v.value, v.DataType)
}

// Update value from bytes.
// This will check that the underlying length is coherent
// but makes no assumption on content
func (v *Variable) PutBytes(value []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.value) != len(value) {
		if len(value) > len(v.value) {
			return ErrDataLong
		}
		return ErrDataShort
	}
	v.value = value
	return nil
}

// This will check that the actual type corresponds
// to the CANopen datatype before updating the value
func (v *Variable) PutAnyExact(value any) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	return EncodeFromTypeExactToBuffer(value, v.DataType, v.value)
}
