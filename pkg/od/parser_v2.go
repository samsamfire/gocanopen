package od

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// v2 of OD parser, this implementation is ~10x faster
// than the previous one but has some caveats :
//
//   - it expects OD definitions to be "in order" i.e.
//     for example this is not possible :
//     [1000]
//     ...
//     [1000sub0]
//     ...
//     [1001sub0]
//     ...
//     [1000sub1]
//     ...
//     [1001]
//
// The remaining bottlenecks are the following :
//
//   - regexp are pretty slow, not sure if would could do much better
//   - bytes to string conversions for values create a lot of unnecessary allocation.
//     As values are mostly stored in bytes anyway, we could remove this step.
//   - file i/o ==> not much to do here
func ParseV2(file any, nodeId uint8) (*ObjectDictionary, error) {

	var err error
	bu := &bytes.Buffer{}

	switch fType := file.(type) {
	case string:
		f, err := os.Open(fType)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		bu = &bytes.Buffer{}
		io.Copy(bu, f)

	case []byte:
		bu = bytes.NewBuffer(fType)
	default:
		return nil, fmt.Errorf("unsupported type")
	}

	od := NewOD()

	var section string
	entry := &Entry{}
	vList := &VariableList{}
	isEntry := false
	isSubEntry := false
	subindex := uint8(0)

	var defaultValue string
	var parameterName string
	var objectType string
	var pdoMapping string
	var lowLimit string
	var highLimit string
	var subNumber string
	var accessType string
	var dataType string

	scanner := bufio.NewScanner(bu)

	for scanner.Scan() {

		// New line detected
		lineRaw := scanner.Bytes()

		// Skip if less than 2 chars
		if len(lineRaw) < 2 {
			continue
		}

		line := trimSpaces(lineRaw)

		// Skip empty lines and comments
		if len(line) == 0 || line[0] == ';' || line[0] == '#' {
			continue
		}

		// Handle section headers: [section]
		if line[0] == '[' && line[len(line)-1] == ']' {
			// A section should be of length 4 at least
			if len(line) < 4 {
				continue
			}

			// New section, this means we have finished building
			// Previous one, so take all the values and update the section
			if parameterName != "" && isEntry {
				entry.Name = parameterName
				od.entriesByIndexName[parameterName] = entry
				vList, err = populateEntry(
					entry,
					nodeId,
					parameterName,
					defaultValue,
					objectType,
					pdoMapping,
					lowLimit,
					highLimit,
					accessType,
					dataType,
					subNumber,
				)

				if err != nil {
					return nil, fmt.Errorf("failed to create new entry %v", err)
				}

			} else if parameterName != "" && isSubEntry {
				err = populateSubEntry(
					entry,
					vList,
					nodeId,
					parameterName,
					defaultValue,
					objectType,
					pdoMapping,
					lowLimit,
					highLimit,
					accessType,
					dataType,
					subindex,
				)

				if err != nil {
					return nil, fmt.Errorf("failed to create sub entry %v", err)
				}
			}

			isEntry = false
			isSubEntry = false
			sectionBytes := line[1 : len(line)-1]

			// Check if a sub entry or the actual entry
			// A subentry should be more than 4 bytes long
			subSection := sectionBytes[4:]
			if len(subSection) < 4 && matchIdxRegExp.Match(sectionBytes) {
				section = string(sectionBytes)

				// Add a new entry inside object dictionary
				idx, err := strconv.ParseUint(section, 16, 16)
				if err != nil {
					return nil, err
				}
				isEntry = true
				entry = &Entry{}
				entry.Index = uint16(idx)
				entry.subEntriesNameMap = map[string]uint8{}
				entry.logger = od.logger
				od.entriesByIndexValue[uint16(idx)] = entry

			} else if matchSubidxRegExp.Match(sectionBytes) {
				section = string(sectionBytes)
				// TODO we could get entry to double check if ever something is out of order
				isSubEntry = true
				// Subindex part is from the 7th letter onwards
				sidx, err := strconv.ParseUint(section[7:], 16, 8)
				if err != nil {
					return nil, err
				}
				subindex = uint8(sidx)
			}

			// Reset all values
			defaultValue = ""
			parameterName = ""
			objectType = ""
			pdoMapping = ""
			lowLimit = ""
			highLimit = ""
			subNumber = ""
			accessType = ""
			dataType = ""

			continue
		}

		// We are in a section so we need to populate the given entry
		// Parse key-value pairs: key = value
		// We will create variables for storing intermediate values
		// Once we are at the end of the section

		if equalsIdx := bytes.IndexByte(line, '='); equalsIdx != -1 {
			key := string(trimSpaces(line[:equalsIdx]))
			value := string(trimSpaces(line[equalsIdx+1:]))

			// We will get the different elements of the entry
			switch key {
			case "ParameterName":
				parameterName = value
			case "ObjectType":
				objectType = value
			case "SubNumber":
				subNumber = value
			case "AccessType":
				accessType = value
			case "DataType":
				dataType = value
			case "LowLimit":
				lowLimit = value
			case "HighLimit":
				highLimit = value
			case "DefaultValue":
				defaultValue = value
			case "PDOMapping":
				pdoMapping = value

			}
		}
	}
	return od, nil
}

func populateEntry(
	entry *Entry,
	nodeId uint8,
	parameterName string,
	defaultValue string,
	objectType string,
	pdoMapping string,
	lowLimit string,
	highLimit string,
	accessType string,
	dataType string,
	subNumber string,
) (*VariableList, error) {

	oType := uint8(0)
	// Determine object type
	// If no object type, default to 7 (CiA spec)
	if objectType == "" {
		oType = 7
	} else {
		oTypeUint, err := strconv.ParseUint(objectType, 0, 8)
		if err != nil {
			return nil, fmt.Errorf("failed to parse object type %v", err)
		}
		oType = uint8(oTypeUint)
	}
	entry.ObjectType = oType

	// Add necessary stuff depending on oType
	switch oType {

	case ObjectTypeVAR, ObjectTypeDOMAIN:
		variable := &Variable{}
		if dataType == "" {
			return nil, fmt.Errorf("need data type")
		}
		dataTypeUint, err := strconv.ParseUint(dataType, 0, 8)
		if err != nil {
			return nil, fmt.Errorf("failed to parse object type %v", err)
		}

		// Get Attribute
		dType := uint8(dataTypeUint)
		attribute := EncodeAttribute(accessType, pdoMapping == "1", dType)

		variable.Name = parameterName
		variable.DataType = dType
		variable.Attribute = attribute
		variable.SubIndex = 0

		if strings.Contains(defaultValue, "$NODEID") {
			re := regexp.MustCompile(`\+?\$NODEID\+?`)
			defaultValue = re.ReplaceAllString(defaultValue, "")
		} else {
			nodeId = 0
		}
		variable.valueDefault, err = EncodeFromString(defaultValue, variable.DataType, nodeId)
		if err != nil {
			return nil, fmt.Errorf("failed to parse 'DefaultValue' for x%x|x%x, because %v (datatype :x%x)", "", 0, err, variable.DataType)
		}
		variable.value = make([]byte, len(variable.valueDefault))
		copy(variable.value, variable.valueDefault)
		entry.object = variable
		return nil, nil

	case ObjectTypeARRAY:
		// Array objects do not allow holes in subindex numbers
		// So pre-init slice up to subnumber
		sub, err := strconv.ParseUint(subNumber, 0, 8)
		if err != nil {
			return nil, fmt.Errorf("failed to parse subnumber %v", err)
		}
		vList := NewArray(uint8(sub))
		entry.object = vList
		return vList, nil

	case ObjectTypeRECORD:
		// Record objects allow holes in mapping
		// Sub-objects will be added with "append"
		vList := NewRecord()
		entry.object = vList
		return vList, nil

	default:
		return nil, fmt.Errorf("unknown object type %v", oType)
	}
}

func populateSubEntry(
	entry *Entry,
	vlist *VariableList,
	nodeId uint8,
	parameterName string,
	defaultValue string,
	objectType string,
	pdoMapping string,
	lowLimit string,
	highLimit string,
	accessType string,
	dataType string,
	subIndex uint8,
) error {
	if dataType == "" {
		return fmt.Errorf("need data type")
	}
	dataTypeUint, err := strconv.ParseUint(dataType, 0, 8)
	if err != nil {
		return fmt.Errorf("failed to parse object type %v", err)
	}

	// Get Attribute
	dType := uint8(dataTypeUint)
	attribute := EncodeAttribute(accessType, pdoMapping == "1", dType)

	variable := &Variable{
		Name:      parameterName,
		DataType:  byte(dataTypeUint),
		Attribute: attribute,
		SubIndex:  subIndex,
	}
	if strings.Contains(defaultValue, "$NODEID") {
		re := regexp.MustCompile(`\+?\$NODEID\+?`)
		defaultValue = re.ReplaceAllString(defaultValue, "")
	} else {
		nodeId = 0
	}
	variable.valueDefault, err = EncodeFromString(defaultValue, variable.DataType, nodeId)
	if err != nil {
		return fmt.Errorf("failed to parse 'DefaultValue' %v %v %v", err, defaultValue, variable.DataType)
	}
	variable.value = make([]byte, len(variable.valueDefault))
	copy(variable.value, variable.valueDefault)

	switch entry.ObjectType {
	case ObjectTypeARRAY:
		vlist.Variables[subIndex] = variable
		entry.subEntriesNameMap[parameterName] = subIndex
	case ObjectTypeRECORD:
		vlist.Variables = append(vlist.Variables, variable)
		entry.subEntriesNameMap[parameterName] = subIndex
	default:
		return fmt.Errorf("add member not supported for ObjectType : %v", entry.ObjectType)
	}

	return nil
}

// Remove '\t' and ' ' characters at beginning
// and beginning of line
func trimSpaces(b []byte) []byte {
	start, end := 0, len(b)

	for start < end && (b[start] == ' ' || b[start] == '\t') {
		start++
	}
	for end > start && (b[end-1] == ' ' || b[end-1] == '\t') {
		end--
	}
	return b[start:end]
}
