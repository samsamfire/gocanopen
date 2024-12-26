package od

import (
	"encoding/binary"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/ini.v1"
)

// Return number of bytes
func (variable *Variable) DataLength() uint32 {
	return uint32(len(variable.value))
}

// Return default value as byte slice
func (variable *Variable) DefaultValue() []byte {
	return variable.valueDefault
}

// Create variable from section entry
func NewVariableFromSection(
	section *ini.Section,
	name string,
	nodeId uint8,
	index uint16,
	subindex uint8,
) (*Variable, error) {

	variable := &Variable{
		Name:     name,
		SubIndex: subindex,
	}

	// Get AccessType
	accessType, err := section.GetKey("AccessType")
	if err != nil {
		return nil, fmt.Errorf("failed to get 'AccessType' for %x : %x", index, subindex)
	}

	// Get PDOMapping to know if pdo mappable
	var pdoMapping bool
	if pM, err := section.GetKey("PDOMapping"); err == nil {
		pdoMapping, err = pM.Bool()
		if err != nil {
			return nil, err
		}
	} else {
		pdoMapping = true
	}

	// TODO maybe add support for datatype particularities (>1B)
	dataType, err := strconv.ParseInt(section.Key("DataType").Value(), 0, 8)
	if err != nil {
		return nil, fmt.Errorf("failed to parse 'DataType' for %x : %x, because %v", index, subindex, err)
	}
	variable.DataType = byte(dataType)
	variable.Attribute = EncodeAttribute(accessType.String(), pdoMapping, variable.DataType)

	if highLimit, err := section.GetKey("HighLimit"); err == nil {
		variable.highLimit, err = EncodeFromString(highLimit.Value(), variable.DataType, 0)
		if err != nil {
			_logger.Warn("error parsing HighLimit",
				"index", fmt.Sprintf("x%x", index),
				"subindex", fmt.Sprintf("x%x", subindex),
				"error", err,
			)
		}
	}

	if lowLimit, err := section.GetKey("LowLimit"); err == nil {
		variable.lowLimit, err = EncodeFromString(lowLimit.Value(), variable.DataType, 0)
		if err != nil {
			_logger.Warn("error parsing LowLimit",
				"index", fmt.Sprintf("x%x", index),
				"subindex", fmt.Sprintf("x%x", subindex),
				"error", err,
			)
		}
	}

	if defaultValue, err := section.GetKey("DefaultValue"); err == nil {
		defaultValueStr := defaultValue.Value()
		// If $NODEID is in default value then remove it, and add it afterwards
		if strings.Contains(defaultValueStr, "$NODEID") {
			re := regexp.MustCompile(`\+?\$NODEID\+?`)
			defaultValueStr = re.ReplaceAllString(defaultValueStr, "")
		} else {
			nodeId = 0
		}
		variable.valueDefault, err = EncodeFromString(defaultValueStr, variable.DataType, nodeId)
		if err != nil {
			return nil, fmt.Errorf("failed to parse 'DefaultValue' for x%x|x%x, because %v (datatype :x%x)", index, subindex, err, variable.DataType)
		}
		variable.value = make([]byte, len(variable.valueDefault))
		copy(variable.value, variable.valueDefault)
	}

	return variable, nil
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

// EncodeFromString value from EDS into bytes respecting canopen datatype
func EncodeFromString(value string, datatype uint8, offset uint8) ([]byte, error) {

	var data []byte
	var err error
	var parsedInt int64
	var parsedUint uint64

	if value == "" {
		// Treat empty string as a 0 value
		value = "0"
	}

	switch datatype {
	case BOOLEAN, UNSIGNED8:
		parsedUint, err = strconv.ParseUint(value, 0, 8)
		data = []byte{byte(uint8(parsedUint + uint64(offset)))}

	case INTEGER8:
		parsedInt, err = strconv.ParseInt(value, 0, 8)
		data = []byte{byte(parsedInt + int64(offset))}

	case UNSIGNED16:
		parsedUint, err = strconv.ParseUint(value, 0, 16)
		data = make([]byte, 2)
		binary.LittleEndian.PutUint16(data, uint16(parsedUint+uint64(offset)))

	case INTEGER16:
		parsedInt, err = strconv.ParseInt(value, 0, 16)
		data = make([]byte, 2)
		binary.LittleEndian.PutUint16(data, uint16(parsedInt+int64(offset)))

	case UNSIGNED32:
		parsedUint, err = strconv.ParseUint(value, 0, 32)
		data = make([]byte, 4)
		binary.LittleEndian.PutUint32(data, uint32(parsedUint+uint64(offset)))

	case INTEGER32:
		parsedInt, err = strconv.ParseInt(value, 0, 32)
		data = make([]byte, 4)
		binary.LittleEndian.PutUint32(data, uint32(parsedInt+int64(offset)))

	case REAL32:
		var parsedFloat float64
		parsedFloat, err = strconv.ParseFloat(value, 32)
		data = make([]byte, 4)
		binary.LittleEndian.PutUint32(data, math.Float32bits(float32(parsedFloat)))

	case UNSIGNED64:
		parsedUint, err = strconv.ParseUint(value, 0, 64)
		data = make([]byte, 8)
		binary.LittleEndian.PutUint64(data, parsedUint+uint64(offset))

	case INTEGER64:
		parsedInt, err = strconv.ParseInt(value, 0, 64)
		data = make([]byte, 8)
		binary.LittleEndian.PutUint64(data, uint64(parsedInt+int64(offset)))

	case REAL64:
		var parsedFloat float64
		parsedFloat, err = strconv.ParseFloat(value, 64)
		data = make([]byte, 8)
		binary.LittleEndian.PutUint64(data, math.Float64bits(parsedFloat))

	case VISIBLE_STRING, OCTET_STRING:
		return []byte(value), nil

	case DOMAIN:
		return []byte{}, nil

	default:
		return nil, ErrTypeMismatch

	}
	return data, err
}

// Encode from generic type
func EncodeFromGeneric(data any) ([]byte, error) {
	var encoded []byte
	switch val := data.(type) {
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
	case []byte:
		encoded = val
	default:
		return nil, ErrTypeMismatch
	}
	return encoded, nil
}

// Helper function for checking consistency between size and datatype
func CheckSize(length int, dataType uint8) error {
	switch dataType {
	case BOOLEAN, UNSIGNED8, INTEGER8:
		if length < 1 {
			return ErrDataShort
		} else if length > 1 {
			return ErrDataLong
		}
	case UNSIGNED16, INTEGER16:
		if length < 2 {
			return ErrDataShort
		} else if length > 2 {
			return ErrDataLong
		}

	case UNSIGNED32, INTEGER32, REAL32:
		if length < 4 {
			return ErrDataShort
		} else if length > 4 {
			return ErrDataLong
		}
	case UNSIGNED64, INTEGER64, REAL64:
		if length < 8 {
			return ErrDataShort
		} else if length > 8 {
			return ErrDataLong
		}
	// All other datatypes, no size check
	default:
		return nil
	}
	return nil

}

// Decode byte array given the CANopen data type
// Function will return either string, int64, uint64, or float64
func DecodeToType(data []byte, dataType uint8) (v any, e error) {
	e = CheckSize(len(data), dataType)
	if e != nil {
		return nil, e
	}
	// Cast to correct type
	switch dataType {
	case BOOLEAN, UNSIGNED8:
		return uint64(data[0]), nil
	case INTEGER8:
		return int64(data[0]), nil
	case UNSIGNED16:
		return uint64(binary.LittleEndian.Uint16(data)), nil
	case INTEGER16:
		return int64(int16(binary.LittleEndian.Uint16(data))), nil
	case UNSIGNED32:
		return uint64(binary.LittleEndian.Uint32(data)), nil
	case INTEGER32:
		return int64(int32(binary.LittleEndian.Uint32(data))), nil
	case UNSIGNED64:
		return uint64(binary.LittleEndian.Uint64(data)), nil
	case INTEGER64:
		return int64(binary.LittleEndian.Uint64(data)), nil
	case REAL32:
		parsed := binary.LittleEndian.Uint32(data)
		return float64(math.Float32frombits(parsed)), nil
	case REAL64:
		parsed := binary.LittleEndian.Uint64(data)
		return math.Float64frombits(parsed), nil
	case VISIBLE_STRING, OCTET_STRING:
		return string(data), nil
	case DOMAIN:
		return int64(0), nil
	default:
		return nil, ErrTypeMismatch
	}
}

// Decode byte array given the CANopen data type
// Function will return the exact type (uint8,uint16,...,int8,...)
func DecodeToTypeExact(data []byte, dataType uint8) (v any, e error) {
	e = CheckSize(len(data), dataType)
	if e != nil {
		return nil, e
	}
	// Cast to correct type
	switch dataType {
	case BOOLEAN, UNSIGNED8:
		return data[0], nil
	case INTEGER8:
		return int8(data[0]), nil
	case UNSIGNED16:
		return binary.LittleEndian.Uint16(data), nil
	case INTEGER16:
		return int16(binary.LittleEndian.Uint16(data)), nil
	case UNSIGNED32:
		return binary.LittleEndian.Uint32(data), nil
	case INTEGER32:
		return int32(binary.LittleEndian.Uint32(data)), nil
	case UNSIGNED64:
		return binary.LittleEndian.Uint64(data), nil
	case INTEGER64:
		return int64(binary.LittleEndian.Uint64(data)), nil
	case REAL32:
		parsed := binary.LittleEndian.Uint32(data)
		return math.Float32frombits(parsed), nil
	case REAL64:
		parsed := binary.LittleEndian.Uint64(data)
		return math.Float64frombits(parsed), nil
	case VISIBLE_STRING, OCTET_STRING:
		return string(data), nil
	case DOMAIN:
		return int64(0), nil
	default:
		return nil, ErrTypeMismatch
	}
}

// Decode byte array given the CANopen data type
// Function will return either string, int64, uint64, or float64
func DecodeToString(data []byte, dataType uint8, base int) (v string, e error) {
	e = CheckSize(len(data), dataType)
	if e != nil {
		return "", e
	}
	// Cast to correct type
	switch dataType {
	case BOOLEAN, UNSIGNED8:
		return strconv.FormatUint(uint64(data[0]), base), nil
	case INTEGER8:
		return strconv.FormatInt(int64(data[0]), base), nil
	case UNSIGNED16:
		return strconv.FormatUint(uint64(binary.LittleEndian.Uint16(data)), base), nil
	case INTEGER16:
		return strconv.FormatInt(int64(int16(binary.LittleEndian.Uint16(data))), base), nil
	case UNSIGNED32:
		return strconv.FormatUint(uint64(binary.LittleEndian.Uint32(data)), base), nil
	case INTEGER32:
		return strconv.FormatInt(int64(int32(binary.LittleEndian.Uint32(data))), base), nil
	case UNSIGNED64:
		return strconv.FormatUint(uint64(binary.LittleEndian.Uint64(data)), base), nil
	case INTEGER64:
		return strconv.FormatInt(int64(binary.LittleEndian.Uint64(data)), base), nil
	case REAL32:
		parsed := binary.LittleEndian.Uint32(data)
		return strconv.FormatFloat(float64(math.Float32frombits(parsed)), 'f', -1, 64), nil
	case REAL64:
		parsed := binary.LittleEndian.Uint64(data)
		return strconv.FormatFloat(math.Float64frombits(parsed), 'f', -1, 64), nil
	case VISIBLE_STRING, OCTET_STRING:
		return string(data), nil
	case DOMAIN:
		return "0", nil
	default:
		return "", ErrTypeMismatch
	}
}

// Decode the attribute in function of the of attribute type and pdo mapping for EDS entry
func EncodeAttribute(accessType string, pdoMapping bool, dataType uint8) uint8 {

	var attribute uint8

	switch accessType {
	case "rw":
		attribute = AttributeSdoRw
	case "ro", "const":
		attribute = AttributeSdoR
	case "wo":
		attribute = AttributeSdoW
	default:
		attribute = AttributeSdoRw
	}
	if pdoMapping {
		attribute |= AttributeTrpdo
	}
	if dataType == VISIBLE_STRING || dataType == OCTET_STRING {
		attribute |= AttributeStr
	}
	return attribute
}

// Encode attribute
func DecodeAttribute(attribute uint8) string {
	switch {
	case attribute&AttributeSdoRw > 0:
		return "rw"
	case attribute&AttributeSdoR > 0:
		return "ro"
	case attribute&AttributeSdoW > 0:
		return "wo"
	default:
		return "rw"
	}
}
