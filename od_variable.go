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

	variable := &Variable{
		Name: name,
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
		variable.DefaultValue, err = encode(defaultValueStr, variable.DataType, nodeId)
		if err != nil {
			return nil, fmt.Errorf("failed to parse DefaultValue for %x : %x, because %v", index, subindex, err)
		}
		// Also update Data with default value
		variable.Data = variable.DefaultValue
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

	case UNSIGNED32, INTEGER32:
		parsed, err = strconv.ParseUint(variable, 0, 32)
		data = make([]byte, 4)
		binary.LittleEndian.PutUint32(data, uint32(parsed+uint64(nodeId)))

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

// Calculate the attribute in function of the of attribute type and pdo mapping for EDS entry
func calculateAttribute(accessType string, pdoMapping bool, dataType uint8) ODA {

	var attribute ODA

	switch accessType {
	case "rw":
		attribute = ODA_SDO_RW
	case "ro", "const":
		attribute = ODA_SDO_R
	case "wo":
		attribute = ODA_SDO_W
	default:
		attribute = ODA_SDO_RW
	}
	if pdoMapping {
		attribute |= ODA_TRPDO
	}
	if dataType == VISIBLE_STRING || dataType == OCTET_STRING {
		attribute |= ODA_STR
	}
	return attribute
}
