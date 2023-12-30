package canopen

import (
	"fmt"
	"regexp"
	"strconv"

	log "github.com/sirupsen/logrus"
	"gopkg.in/ini.v1"
)

// CANopen supported object types
const (
	OBJ_DOMAIN byte = 2
	OBJ_VAR    byte = 7
	OBJ_ARR    byte = 8
	OBJ_RECORD byte = 9
)

var OBJ_NAME_MAP = map[byte]string{
	OBJ_DOMAIN: "DOMAIN  ",
	OBJ_VAR:    "VARIABLE",
	OBJ_ARR:    "ARRAY   ",
	OBJ_RECORD: "RECORD  ",
}

// CANopen supported datatypes
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

// Actual object type used in OD
// Specifically, OBJ_DOMAIN is considered as an ODT_VAR
const (
	ODT_VAR       byte = 0x01
	ODT_ARR       byte = 0x02
	ODT_REC       byte = 0x03
	ODT_TYPE_MASK byte = 0x0F
)

// Create an OD from a given file path on system
func ParseEDSFromFile(filePath string, nodeId uint8) (*ObjectDictionary, error) {
	od, err := parseEDS(filePath, nodeId)
	if err != nil {
		return nil, err
	}
	od.filePath = filePath
	return od, nil
}

// Create an OD from raw bytes
func ParseEDSFromRaw(edsBytes []byte, nodeId uint8) (*ObjectDictionary, error) {
	return parseEDS(edsBytes, nodeId)
}

func parseEDS(filePathOrData any, nodeId uint8) (*ObjectDictionary, error) {
	od := NewOD()
	// Open the EDS file
	edsFile, err := ini.Load(filePathOrData)
	if err != nil {
		return nil, err
	}

	// Get all the sections in the file
	sections := edsFile.Sections()

	// Get index & subindex matching
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
			case OBJ_VAR, OBJ_DOMAIN:
				variable, err := NewVariableFromSection(section, name, nodeId, index, 0)
				if err != nil {
					return nil, err
				}
				od.addVariable(variable)
			case OBJ_ARR:
				// Get number of elements inside array
				subNumber, err := strconv.ParseUint(section.Key("SubNumber").Value(), 0, 8)
				if err != nil {
					return nil, err
				}
				array := Array{Variables: make([]Variable, subNumber)}
				od.AddArray(index, name, array)
			case OBJ_RECORD:
				od.AddRecord(index, name, make([]Record, 0))
			default:
				return nil, fmt.Errorf("[OD] unknown object type whilst parsing EDS %T", objType)
			}

			log.Debugf("[OD] adding %v | %v at %x", OBJ_NAME_MAP[objectType], name, index)

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

			entry := od.Index(index)
			if entry == nil {
				return nil, fmt.Errorf("[OD] index with id %d not found", index)
			}
			// Add new subindex entry member
			err = entry.AddMember(section, name, nodeId, subIndex)
			if err != nil {
				return nil, err
			}

		}
	}

	return od, nil
}

// Print od out
func (od *ObjectDictionary) Print() {
	for k, v := range od.entriesByIndexValue {
		fmt.Printf("Entry %x : %v\n", k, v.Name)
		switch object := v.Object.(type) {
		case Array:
			for subindex, variable := range object.Variables {
				fmt.Printf("\t\tSub Entry %x : %v \n", subindex, variable)
			}

		case []Record:
			for _, subvalue := range object {
				fmt.Printf("\t\tSub Entry %x : %v \n", subvalue.Subindex, subvalue.Variable.Name)
			}
		}

	}
}

func NewOD() *ObjectDictionary {
	return &ObjectDictionary{
		entriesByIndexValue: make(map[uint16]*Entry),
		entriesByIndexName:  make(map[string]*Entry),
	}
}
