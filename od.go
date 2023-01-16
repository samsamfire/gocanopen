package canopen

import (
	"fmt"
	"regexp"
	"strconv"

	"gopkg.in/ini.v1"
)

/*
	TODOS:

- What to do for ParmaterValue
- Properly handle $NODEID stuff
*/

const (
	BOOLEAN        uint8 = 0x01
	INTEGER8       uint8 = 0x02
	INTEGER16      uint8 = 0x03
	INTEGER32      uint8 = 0x04
	UNSIGNED8      uint8 = 0x05
	UNSIGNED16     uint8 = 0x06
	UNSIGNED32     uint8 = 0x07
	REAL32         uint8 = 0x08
	VISIBLE_STRING uint8 = 0x09
	OCTET_STRING   uint8 = 0x0A
	UNICODE_STRING uint8 = 0x0B
	DOMAIN         uint8 = 0x0F
	REAL64         uint8 = 0x11
	INTEGER64      uint8 = 0x15
	UNSIGNED64     uint8 = 0x1B
)

// Calculate the corresponding OD attribute
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

/*Parse an EDS and file and create an ObjectDictionary*/
func ParseEDS(filePath string, nodeId uint8) (*ObjectDictionary, error) {

	od := NewOD()

	// Open the EDS file
	edsFile, err := ini.Load(filePath)
	if err != nil {
		//
	}

	// Get all the sections in the file
	sections := edsFile.Sections()

	// Get the index sections
	matchIdxRegExp := regexp.MustCompile(`^[0-9A-Fa-f]{4}$`)
	matchSubidxRegExp := regexp.MustCompile(`^([0-9A-Fa-f]{4})sub([0-9A-Fa-f]+)$`)

	// Iterate over all the sections
	for _, section := range sections {
		sectionName := section.Name()

		// Match indexes : This adds new entries to the dictionary
		if matchIdxRegExp.MatchString(sectionName) {
			// Add a new entry inside object dictionary
			idx, err := strconv.ParseUint(section.Name(), 16, 16)
			if err != nil {
				return nil, err
			}
			index := uint16(idx)

			name := section.Key("ParameterName").String()

			objType, err := strconv.ParseUint(section.Key("ObjectType").Value(), 0, 8)
			objectType := uint8(objType)

			//If no object type, default to 7 (CiA spec)
			if err != nil {
				objectType = 7
			}

			//objectType determines what type of entry we should add to dictionary : Variable, Array or Record
			switch objectType {
			case OBJ_VAR:
				// Build en entry for Variable type
				variable, err := buildVariable(section, name, nodeId, index, 0)
				if err != nil {
					return nil, err
				}
				od.AddEntry(Entry{Index: index, Object: *variable, Extension: nil})

			case OBJ_DOMAIN:
				// Build en entry for Domain type
				variable, err := buildVariable(section, name, nodeId, index, 0)
				if err != nil {
					return nil, err
				}
				od.AddEntry(Entry{Index: index, Object: *variable, Extension: nil})

			case OBJ_ARR:
				// Get number of elements inside array
				subNumber, err := strconv.ParseUint(section.Key("SubNumber").Value(), 0, 8)
				if err != nil {
					return nil, err
				}
				array := Array{Variables: make([]Variable, subNumber)}
				od.AddEntry(Entry{Index: uint16(index), Name: name, Object: array, Extension: nil})

			case OBJ_RECORD:
				record := []Record{}
				od.AddEntry(Entry{Index: index, Name: name, Object: record, Extension: nil})

			default:
				continue
			}

		}

		// Match subindexes, add the subindex values to Record or Array objects
		if matchSubidxRegExp.MatchString(sectionName) {

			//Index part are the first 4 letters (A subindex entry looks like 5000Sub1)
			idx, err := strconv.ParseUint(sectionName[0:4], 16, 16)
			if err != nil {
				return nil, err
			}
			index := uint16(idx)
			// Subindex part is from the 7th letter onwards
			sidx, err := strconv.ParseUint(sectionName[7:], 16, 8)
			if err != nil {
				return nil, err
			}

			subIndex := uint8(sidx)
			name := section.Key("ParameterName").String()

			entry := od.Find(index)
			if entry == nil {
				return nil, fmt.Errorf("index with id %d not found", index)
			}
			// Add new subindex entry member
			err = entry.AddMember(section, name, nodeId, subIndex)
			if err != nil {
				return nil, err
			}

		}
	}

	return &od, nil

}
