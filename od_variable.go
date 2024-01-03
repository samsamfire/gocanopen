package canopen

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
	return uint32(len(variable.data))
}

// Return default value as byte slice
func (variable *Variable) DefaultValue() []byte {
	return variable.defaultValue
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
		return nil, fmt.Errorf("failed to get AccessType for %x : %x", index, subindex)
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
		return nil, fmt.Errorf("failed to parse DataType for %x : %x, because %v", index, subindex, err)
	}
	variable.DataType = byte(dataType)

	//Determine variable attribute
	variable.Attribute = calculateAttribute(accessType.String(), pdoMapping, variable.DataType)

	// All the parameters aftewards are optional elements that can be used in EDS
	if highLimit, err := section.GetKey("HighLimit"); err == nil {
		variable.HighLimit, err = highLimit.Int()
		if err != nil {
			return nil, fmt.Errorf("failed to parse HighLimit for %x : %x, because %v", index, subindex, err)
		}
	}

	if lowLimit, err := section.GetKey("LowLimit"); err == nil {
		variable.LowLimit, err = lowLimit.Int()
		if err != nil {
			return nil, fmt.Errorf("failed to parse LowLimit for %x : %x, because %v", index, subindex, err)
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
		variable.defaultValue, err = encode(defaultValueStr, variable.DataType, nodeId)
		if err != nil {
			return nil, fmt.Errorf("failed to parse DefaultValue for %x : %x, because %v", index, subindex, err)
		}
		variable.data = make([]byte, len(variable.defaultValue))
		copy(variable.data, variable.defaultValue)
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
	encoded, err := encode(value, datatype, 0)
	encodedCopy := make([]byte, len(encoded))
	copy(encodedCopy, encoded)
	if err != nil {
		return nil, err
	}
	variable := &Variable{
		SubIndex:     subindex,
		Name:         name,
		data:         encoded,
		defaultValue: encodedCopy,
		Attribute:    attribute,
		DataType:     datatype,
	}
	return variable, nil
}

// Encode value from EDS into bytes respecting canopen datatype
// nodeId is used as an offset
func encode(variable string, datatype uint8, nodeId uint8) ([]byte, error) {

	var data []byte
	var err error
	var parsed uint64

	if variable == "" {
		// Treat empty string as a 0 value
		variable = "0x0"
	}

	switch datatype {
	case BOOLEAN, UNSIGNED8, INTEGER8:
		parsed, err = strconv.ParseUint(variable, 0, 8)
		data = []byte{byte(uint8(parsed + uint64(nodeId)))}

	case UNSIGNED16, INTEGER16:
		parsed, err = strconv.ParseUint(variable, 0, 16)
		data = make([]byte, 2)
		binary.LittleEndian.PutUint16(data, uint16(parsed+uint64(nodeId)))

	case UNSIGNED32, INTEGER32, REAL32:
		parsed, err = strconv.ParseUint(variable, 0, 32)
		data = make([]byte, 4)
		binary.LittleEndian.PutUint32(data, uint32(parsed+uint64(nodeId)))

	case UNSIGNED64, INTEGER64, REAL64:
		parsed, err = strconv.ParseUint(variable, 0, 64)
		data = make([]byte, 8)
		binary.LittleEndian.PutUint64(data, parsed+uint64(nodeId))

	case VISIBLE_STRING:
		return []byte(variable), nil

	case DOMAIN:
		return []byte{}, nil

	default:
		return nil, ODR_TYPE_MISMATCH

	}
	if err != nil {
		return nil, err
	}

	return data, nil
}

// Helper function for checking consistency between size and datatype
func checkSize(length int, dataType uint8) error {
	switch dataType {
	case BOOLEAN, UNSIGNED8, INTEGER8:
		if length < 1 {
			return ODR_DATA_SHORT
		} else if length > 1 {
			return ODR_DATA_LONG
		}
	case UNSIGNED16, INTEGER16:
		if length < 2 {
			return ODR_DATA_SHORT
		} else if length > 2 {
			return ODR_DATA_LONG
		}

	case UNSIGNED32, INTEGER32, REAL32:
		if length < 4 {
			return ODR_DATA_SHORT
		} else if length > 4 {
			return ODR_DATA_LONG
		}
	case UNSIGNED64, INTEGER64, REAL64:
		if length < 8 {
			return ODR_DATA_SHORT
		} else if length > 8 {
			return ODR_DATA_LONG
		}
	// All other datatypes, no size check
	default:
		return nil
	}
	return nil

}

// Decode byte array given the CANopen data type
// Function will return either string, int64, uint64, or float64
func decode(data []byte, dataType uint8) (v any, e error) {
	e = checkSize(len(data), dataType)
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
	case VISIBLE_STRING:
		return string(data), nil
	default:
		return nil, ODR_TYPE_MISMATCH
	}
}

// Calculate the attribute in function of the of attribute type and pdo mapping for EDS entry
func calculateAttribute(accessType string, pdoMapping bool, dataType uint8) uint8 {

	var attribute uint8

	switch accessType {
	case "rw":
		attribute = ATTRIBUTE_SDO_RW
	case "ro", "const":
		attribute = ATTRIBUTE_SDO_R
	case "wo":
		attribute = ATTRIBUTE_SDO_W
	default:
		attribute = ATTRIBUTE_SDO_RW
	}
	if pdoMapping {
		attribute |= ATTRIBUTE_TRPDO
	}
	if dataType == VISIBLE_STRING || dataType == OCTET_STRING {
		attribute |= ATTRIBUTE_STR
	}
	return attribute
}
