package od

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"slices"
	"strings"
)

// Export OD inside of an EDS file
// OD can be exported with default values (initial values)
// Or with current values (with new PDO mapping for example)
// The created file is not 100% compliant with CiA but will
// work for this library.
func ExportEDS(odict *ObjectDictionary, defaultValues bool, filename string) error {
	if defaultValues {
		return os.WriteFile(filename, odict.rawOd, 0o644)
	}

	// Export indexes from lowest to highest
	indexes := make([]uint16, 0, len(odict.entriesByIndexValue))
	for index := range odict.entriesByIndexValue {
		indexes = append(indexes, index)
	}
	slices.Sort(indexes)

	buf := &bytes.Buffer{}
	for _, index := range indexes {
		entry := odict.entriesByIndexValue[index]

		switch object := entry.object.(type) {
		case *Variable:
			// Single objects (VAR,DOMAIN,...)
			fmt.Fprintf(buf, "[%04X]\n", index)
			if err := writeVariable(buf, index, object, entry.ObjectType); err != nil {
				return fmt.Errorf("[OD] error populating section index at %x : %v", index, err)
			}

		case *VariableList:
			// Multi objects (RECORD,ARRAY,...)
			// Header section e.g.
			// [1A03]
			// ParameterName=TPDO mapping parameter
			// ObjectType=0x9
			// SubNumber=0x9
			subNumber := 0
			for _, variable := range object.Variables {
				if variable != nil {
					subNumber++
				}
			}
			fmt.Fprintf(buf, "[%04X]\nParameterName=%s\nObjectType=0x%X\nSubNumber=0x%X\n\n",
				index, entry.Name, object.objectType, subNumber)

			for _, variable := range object.Variables {
				if variable == nil {
					continue
				}
				fmt.Fprintf(buf, "[%04Xsub%X]\n", index, variable.SubIndex)
				if err := writeVariable(buf, index, variable, ObjectTypeVAR); err != nil {
					return fmt.Errorf("[OD] error populating section index at %x|%x : %v", index, variable.SubIndex, err)
				}
			}

		default:
			return fmt.Errorf("[OD] unexpected object type %T at %x", object, index)
		}
	}
	return os.WriteFile(filename, buf.Bytes(), 0o644)
}

// Write a section with relevant information for a variable type
func writeVariable(buf *bytes.Buffer, index uint16, variable *Variable, objectType uint8) error {
	variable.mu.Lock()
	defer variable.mu.Unlock()

	value, err := encodeValueString(index, variable.value, variable.DataType)
	if err != nil {
		return err
	}
	pdoMapping := 0
	if variable.Attribute&AttributeTrpdo != 0 {
		pdoMapping = 1
	}
	fmt.Fprintf(buf, "ParameterName=%s\nObjectType=0x%X\nDataType=0x%04X\nAccessType=%s\nPDOMapping=%d\nDefaultValue=%s\n",
		variable.Name, objectType, variable.DataType, DecodeAttribute(variable.Attribute), pdoMapping, value)

	if variable.lowLimit != nil {
		if limit, err := encodeValueString(index, variable.lowLimit, variable.DataType); err == nil {
			fmt.Fprintf(buf, "LowLimit=%s\n", limit)
		}
	}
	if variable.highLimit != nil {
		if limit, err := encodeValueString(index, variable.highLimit, variable.DataType); err == nil {
			fmt.Fprintf(buf, "HighLimit=%s\n", limit)
		}
	}
	buf.WriteByte('\n')
	return nil
}

func encodeValueString(index uint16, data []byte, dataType uint8) (string, error) {
	switch dataType {
	case BOOLEAN, UNSIGNED8, UNSIGNED16, UNSIGNED32, UNSIGNED64:
		if index >= AreaCommunicationProfileStart && index <= AreaCommunicationProfileEnd {
			// Write values as hex strings, facilitates reading
			decoded, err := DecodeToString(data, dataType, 16)
			return "0x" + decoded, err
		}
	case OCTET_STRING:
		return strings.ToUpper(hex.EncodeToString(data)), nil
	case DOMAIN:
		return "", nil
	}
	return DecodeToString(data, dataType, 10)
}
