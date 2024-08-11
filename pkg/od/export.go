package od

import (
	"fmt"
	"sort"
	"strconv"

	"gopkg.in/ini.v1"
)

// Export OD inside of an EDS file
// OD can be exported with default values (initial values)
// Or with current values (with new PDO mapping for example)
// The created file is not 100% compliant with CiA but will
// work for this library.
func ExportEDS(odict *ObjectDictionary, defaultValues bool, filename string) error {
	if defaultValues {
		return odict.iniFile.SaveTo(filename)
	}
	eds := ini.Empty()

	// Sort map keys to export indexes, for lowest to highest
	indexes := make([]int, 0)
	for index := range odict.entriesByIndexValue {
		indexes = append(indexes, int(index))
	}
	sort.Ints(indexes)

	for _, index := range indexes {
		entry := odict.entriesByIndexValue[uint16(index)]

		if entry.SubCount() == 1 {
			// Add entry for single objects (VAR,DOMAIN,...)
			variable, ok := entry.object.(*Variable)
			if !ok {
				return fmt.Errorf("[OD] expecting a variable type at %x", index)
			}
			section, err := eds.NewSection(strconv.FormatUint(uint64(index), 16))
			if err != nil {
				return err
			}
			err = populateSection(section, uint16(index), variable, entry.ObjectType)
			if err != nil {
				return fmt.Errorf("[OD] error populating section index at %x : %v", index, err)
			}
		} else {
			// Add entry for multi objects (RECORD,ARRAY,...)
			variables, ok := entry.object.(*VariableList)
			if !ok {
				return fmt.Errorf("[OD] expecting a variable list type at %x", index)
			}
			// Create header section
			section, err := eds.NewSection(strconv.FormatUint(uint64(index), 16))
			if err != nil {
				return err
			}
			err = populateHeaderSection(section, entry.Name, variables.objectType, uint8(entry.SubCount()))
			if err != nil {
				return err
			}
			// Add all subsections, ordered
			for i, variable := range variables.Variables {
				section, err = eds.NewSection(strconv.FormatUint(uint64(index), 16) + "sub" + strconv.FormatUint(uint64(i), 16))
				if err != nil {
					return err
				}
				err = populateSection(section, uint16(index), variable, entry.ObjectType)
				if err != nil {
					return fmt.Errorf("[OD] error populating section index at %x|%x : %v", index, i, err)
				}
			}
		}
	}
	return eds.SaveTo(filename)
}

// Populate section with relevant information for a variable type
func populateSection(section *ini.Section, index uint16, variable *Variable, objectType uint8) error {
	_, err := section.NewKey("ParameterName", variable.Name)
	if err != nil {
		return err
	}
	_, err = section.NewKey("ObjectType", "0x"+strconv.FormatUint(uint64(objectType), 16))
	if err != nil {
		return err
	}
	_, err = section.NewKey("DataType", "0x"+strconv.FormatUint(uint64(variable.DataType), 16))
	if err != nil {
		return err
	}
	_, err = section.NewKey("AccessType", DecodeAttribute(variable.Attribute))
	if err != nil {
		return err
	}
	var decoded string
	if index >= 0x1000 && index <= 0x1FFF {
		// Write values as hex strings, facilitates reading
		decoded, err = DecodeToString(variable.value, variable.DataType, 16)
		decoded = "0x" + decoded
	} else {
		decoded, err = DecodeToString(variable.value, variable.DataType, 10)
	}
	if err != nil {
		return err
	}
	_, err = section.NewKey("DefaultValue", decoded)
	return err
}

// Populate section with relevant information for beginning of RECORD/ARRAY type.
// Special section for multi sub entries
// e.g.
// [1A03]
// ParameterName=TPDO mapping parameter
// ObjectType=0x9
// SubNumber=0x9
func populateHeaderSection(section *ini.Section, name string, objectType uint8, count uint8) error {
	_, err := section.NewKey("ParameterName", name)
	if err != nil {
		return err
	}
	_, err = section.NewKey("ObjectType", "0x"+strconv.FormatUint(uint64(objectType), 16))
	if err != nil {
		return err
	}
	_, err = section.NewKey("SubNumber", "0x"+strconv.FormatUint(uint64(count), 16))
	if err != nil {
		return err
	}
	return nil
}
