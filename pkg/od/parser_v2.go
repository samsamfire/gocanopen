package od

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// v2 of OD parser, this implementation is ~15x faster
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
// With the current OD architecture, only minor other
// optimizations could be done.
// The remaining bottlenecks are the following :
//
//   - bytes to string conversions for values create a lot of unnecessary allocation.
//     As values are mostly stored in bytes anyway, we could remove this step.
//   - bufio.Scanner() ==> more performant implementation ?
func ParseV2(file any, nodeId uint8) (*ObjectDictionary, error) {

	var err error
	bu := &bytes.Buffer{}

	switch fType := file.(type) {
	case string:
		f, err := os.Open(fType)
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		bu = &bytes.Buffer{}
		io.Copy(bu, f)

	case []byte:
		bu = bytes.NewBuffer(fType)
	default:
		return nil, fmt.Errorf("unsupported type")
	}

	od := NewOD()
	od.rawOd = bu.Bytes()
	entry := &Entry{}
	vList := &VariableList{subEntriesNameMap: make(map[string]uint8)}
	isEntry := false
	isSubEntry := false
	subindex := uint8(0)

	var defaultValue string
	var parameterName string
	var objectType string
	var pdoMapping string
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
			if parameterName != "" {
				if isEntry {
					entry.Name = parameterName
					od.entriesByIndexName[parameterName] = entry
					vList, err = populateEntry(
						entry,
						nodeId,
						parameterName,
						defaultValue,
						objectType,
						pdoMapping,
						accessType,
						dataType,
						subNumber,
					)

					if err != nil {
						return nil, fmt.Errorf("failed to create new entry %v", err)
					}
				} else if isSubEntry {
					err = populateSubEntry(
						entry,
						vList,
						nodeId,
						parameterName,
						defaultValue,
						pdoMapping,
						accessType,
						dataType,
						subindex,
					)

					if err != nil {
						return nil, fmt.Errorf("failed to create sub entry %v", err)
					}
				}
			}

			isEntry = false
			isSubEntry = false
			sectionBytes := line[1 : len(line)-1]

			// Check if a sub entry or the actual entry
			// A subentry should be more than 4 bytes long
			if isValidHex4(sectionBytes) {

				idx, err := hexAsciiToUint(sectionBytes)
				if err != nil {
					return nil, err
				}
				isEntry = true
				entry = &Entry{}
				entry.Index = uint16(idx)
				entry.logger = od.logger
				od.entriesByIndexValue[uint16(idx)] = entry

			} else if isValidSubIndexFormat(sectionBytes) {

				sidx, err := hexAsciiToUint(sectionBytes[7:])
				if err != nil {
					return nil, err
				}
				// TODO we could get entry to double check if ever something is out of order
				isSubEntry = true
				subindex = uint8(sidx)
			}

			// Reset all values
			defaultValue = ""
			parameterName = ""
			objectType = ""
			pdoMapping = ""
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
				parameterName = string(value)
			case "ObjectType":
				objectType = string(value)
			case "SubNumber":
				subNumber = string(value)
			case "AccessType":
				accessType = string(value)
			case "DataType":
				dataType = string(value)
			case "DefaultValue":
				defaultValue = string(value)
			case "PDOMapping":
				pdoMapping = string(value)
			}
		}
	}

	// Last index or subindex part
	// New section, this means we have finished building
	// Previous one, so take all the values and update the section
	if parameterName != "" {
		if isEntry {
			entry.Name = parameterName
			od.entriesByIndexName[parameterName] = entry
			_, err = populateEntry(
				entry,
				nodeId,
				parameterName,
				defaultValue,
				objectType,
				pdoMapping,
				accessType,
				dataType,
				subNumber,
			)

			if err != nil {
				return nil, fmt.Errorf("failed to create new entry %v", err)
			}
		} else if isSubEntry {
			err = populateSubEntry(
				entry,
				vList,
				nodeId,
				parameterName,
				defaultValue,
				pdoMapping,
				accessType,
				dataType,
				subindex,
			)

			if err != nil {
				return nil, fmt.Errorf("failed to create sub entry %v", err)
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
			defaultValue = fastRemoveNodeID(defaultValue)
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
	pdoMapping string,
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
		defaultValue = fastRemoveNodeID(defaultValue)
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
		vlist.subEntriesNameMap[parameterName] = subIndex

	case ObjectTypeRECORD:
		vlist.Variables = append(vlist.Variables, variable)
		vlist.subEntriesNameMap[parameterName] = subIndex
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

func hexAsciiToUint(bytes []byte) (uint64, error) {
	var num uint64

	for _, b := range bytes {
		var digit uint64

		switch {
		case b >= '0' && b <= '9':
			digit = uint64(b - '0') // Convert '0'-'9' to 0-9
		case b >= 'A' && b <= 'F':
			digit = uint64(b - 'A' + 10) // Convert 'A'-'F' to 10-15
		case b >= 'a' && b <= 'f':
			digit = uint64(b - 'a' + 10) // Convert 'a'-'f' to 10-15
		default:
			return 0, fmt.Errorf("invalid hex character: %c", b)
		}

		num = (num << 4) | digit // Left shift by 4 (multiply by 16) and add new digit
	}

	return num, nil
}

// Check if exactly 4 hex digits (no regex)
func isValidHex4(b []byte) bool {
	if len(b) != 4 {
		return false
	}
	for _, c := range b {
		if (c < '0' || c > '9') && (c < 'A' || c > 'F') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// Check if format is "XXXXsubYY" (without regex)
func isValidSubIndexFormat(b []byte) bool {

	// Must be at least "XXXXsubY" (4+3+1 chars)
	if len(b) < 8 {
		return false
	}
	// Check first 4 chars are hex
	if !isValidHex4(b[:4]) {
		return false
	}
	// Check "sub" part (fixed position)
	if string(b[4:7]) != "sub" {
		return false
	}
	// Check remaining are hex
	for _, c := range b[7:] {
		if (c < '0' || c > '9') && (c < 'A' || c > 'F') && (c < 'a' || c > 'f') {
			return false
		}
	}

	return true
}

// Remove "$NODEID" from given string
func fastRemoveNodeID(s string) string {
	b := make([]byte, 0, len(s)) // Preallocate same capacity as input string

	i := 0
	for i < len(s) {
		if s[i] == '$' && len(s) > i+6 && s[i:i+7] == "$NODEID" {
			i += 7 // Skip "$NODEID"
			// Skip optional '+' after "$NODEID"
			if i < len(s) && s[i] == '+' {
				i++
			}
			// Skip optional '+' before "$NODEID"
			if len(b) > 0 && b[len(b)-1] == '+' {
				b = b[:len(b)-1]
			}
			continue
		}
		b = append(b, s[i])
		i++
	}
	return string(b)
}
