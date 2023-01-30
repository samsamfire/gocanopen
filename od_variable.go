package canopen

import (
	"encoding/binary"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/ini.v1"
)

// Create variable from section entry
func buildVariable(
	section *ini.Section,
	name string,
	nodeId uint8,
	index uint16,
	subindex uint8,
) (*Variable, error) {

	// Prepare with known values
	variable := &Variable{
		Name: name,
	}

	// Get AccessType
	accessType, err := section.GetKey("AccessType")
	if err != nil {
		return nil, fmt.Errorf("Failed to get AccessType for %x : %x", index, subindex)
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
	//Determine variable attribute
	variable.Attribute = calculateAttribute(accessType.String(), pdoMapping)

	// TODO maybe add support for datatype particularities (>1B)
	dataType, err := strconv.ParseInt(section.Key("DataType").Value(), 0, 8)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse DataType for %x : %x, because %v", index, subindex, err)
	}

	variable.DataType = byte(dataType)

	// All the parameters aftewards are optional elements that can be used in EDS
	if highLimit, err := section.GetKey("HighLimit"); err == nil {
		variable.HighLimit, err = highLimit.Int()
		if err != nil {
			return nil, fmt.Errorf("Failed to parse HighLimit for %x : %x, because %v", index, subindex, err)
		}
	}

	if lowLimit, err := section.GetKey("LowLimit"); err == nil {
		variable.LowLimit, err = lowLimit.Int()
		if err != nil {
			return nil, fmt.Errorf("Failed to parse LowLimit for %x : %x, because %v", index, subindex, err)
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
		variable.DefaultValue, err = encode(defaultValueStr, variable.DataType, nodeId)
		if err != nil {
			return nil, fmt.Errorf("Failed to parse DefaultValue for %x : %x, because %v", index, subindex, err)
		}
		// Also update Data with default value
		variable.Data = variable.DefaultValue
	}

	return variable, nil
}

// Encode value from EDS into bytes respecting canopen datatype
func encode(variable string, datatype uint8, nodeId uint8) ([]byte, error) {

	var data []byte

	if variable == "" {
		// Treat empty string as a 0 value
		variable = "0x0"
	}

	if datatype == BOOLEAN || datatype == UNSIGNED8 || datatype == INTEGER8 {
		parsed, err := strconv.ParseUint(variable, 0, 8)
		if err != nil {
			return nil, err
		}
		return []byte{byte(uint8(parsed + uint64(nodeId)))}, nil
	}

	switch datatype {

	case UNSIGNED16:
		parsed, err := strconv.ParseUint(variable, 0, 16)
		if err != nil {
			return nil, err
		}
		data = make([]byte, 2)
		binary.LittleEndian.PutUint16(data, uint16(parsed+uint64(nodeId)))
	case UNSIGNED32:
		parsed, err := strconv.ParseUint(variable, 0, 32)
		if err != nil {
			return nil, err
		}
		data = make([]byte, 4)
		binary.LittleEndian.PutUint32(data, uint32(parsed+uint64(nodeId)))

	case INTEGER16:
		parsed, err := strconv.ParseUint(variable, 0, 16)
		if err != nil {
			return nil, err
		}
		data = make([]byte, 2)
		binary.LittleEndian.PutUint16(data, uint16(parsed+uint64(nodeId)))
	case INTEGER32:
		parsed, err := strconv.ParseUint(variable, 0, 32)
		if err != nil {
			return nil, err
		}
		data = make([]byte, 4)
		binary.LittleEndian.PutUint32(data, uint32(parsed+uint64(nodeId)))

	case VISIBLE_STRING:
		return []byte(variable), nil

	default:
		return nil, ODR_TYPE_MISMATCH

	}
	return data, nil
}

// Calculate the attribute in function of the of attribute type and pdo mapping for EDS entry
func calculateAttribute(access_type string, pdo_mapping bool) ODA {
	var attribute ODA
	switch access_type {
	case "rw":
		attribute = ODA_SDO_RW
	case "ro":
		attribute = ODA_SDO_R
	case "wo":
		attribute = ODA_SDO_W
	case "const":
		attribute = 0
	default:
		attribute = ODA_SDO_RW
	}
	if pdo_mapping {
		attribute |= ODA_TRPDO
	}
	return attribute
}
