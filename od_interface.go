package canopen

import (
	"fmt"

	log "github.com/sirupsen/logrus"
)

// Object dictionary contains all node data
type ObjectDictionary struct {
	filePath            string
	entriesByIndexValue map[uint16]*Entry
	entriesByIndexName  map[string]*Entry
}

// Add an entry to OD, existing entry will be replaced
func (od *ObjectDictionary) addEntry(entry *Entry) {
	_, entryIndexValueExists := od.entriesByIndexValue[entry.Index]
	if entryIndexValueExists {
		log.Warnf("[OD] overwritting entry index x%x", entry.Index)
	}
	od.entriesByIndexValue[entry.Index] = entry
	od.entriesByIndexName[entry.Name] = entry
}

// Add a variable type entry to OD with given variable, existing entry will be
func (od *ObjectDictionary) addVariable(index uint16, variable *Variable) {
	od.addEntry(&Entry{Index: index, Name: variable.Name, Object: variable, ObjectType: OBJ_VAR, Extension: nil, subEntriesNameMap: map[string]uint8{}})
}

// Creates and adds a Variable to OD
func (od *ObjectDictionary) AddVariableType(
	index uint16,
	subindex uint8,
	name string,
	datatype uint8,
	attribute uint8,
	value string,
) (*Variable, error) {
	variable, err := NewVariable(subindex, name, datatype, attribute, value)
	if err != nil {
		return nil, err
	}
	od.addVariable(index, variable)
	return variable, nil
}

// Adds a record/variable to OD
func (od *ObjectDictionary) AddVariableList(index uint16, name string, varList *VariableList) {
	od.addEntry(&Entry{Index: index, Name: name, Object: varList, ObjectType: varList.objectType, Extension: nil, subEntriesNameMap: map[string]uint8{}})
}

// Add file like object entry to OD
func (od *ObjectDictionary) AddFile(index uint16, indexName string, filePath string, readMode int, writeMode int) error {
	log.Infof("[OD] adding file object entry : %v at x%x", filePath, index)
	fileObject := &FileObject{FilePath: filePath, ReadMode: readMode, WriteMode: writeMode}
	od.AddVariableType(index, 0, indexName, DOMAIN, ATTRIBUTE_SDO_RW, "") // Cannot error
	entry := od.Index(index)
	entry.AddExtension(fileObject, ReadEntryFileObject, WriteEntryFileObject)
	return nil
}

func (od *ObjectDictionary) addPDO(pdoNb uint16, isRPDO bool) error {
	// TODO check that no empty spaces in PDO numbering before the given number
	indexOffset := pdoNb - 1
	pdoType := "RPDO"
	if !isRPDO {
		indexOffset += 0x400
		pdoType = "TPDO"
	}

	pdoComm := NewRecord()
	pdoComm.AddSubObject(0, "Highest sub-index supported", UNSIGNED8, ATTRIBUTE_SDO_R, "0x5")
	pdoComm.AddSubObject(1, fmt.Sprintf("COB-ID used by %s", pdoType), UNSIGNED32, ATTRIBUTE_SDO_RW, "0x0")
	pdoComm.AddSubObject(2, "Transmission type", UNSIGNED8, ATTRIBUTE_SDO_RW, "0x0")
	pdoComm.AddSubObject(5, "Event timer", UNSIGNED16, ATTRIBUTE_SDO_RW, "0x0")

	od.AddVariableList(BASE_RPDO_COMMUNICATION_INDEX+indexOffset, fmt.Sprintf("%s communication parameter", pdoType), pdoComm)

	pdoMap := NewRecord()
	pdoMap.AddSubObject(0, "Number of mapped application objects in PDO", UNSIGNED8, ATTRIBUTE_SDO_RW, "0x0")
	for i := uint8(1); i <= MAX_MAPPED_ENTRIES; i++ {
		pdoMap.AddSubObject(i, fmt.Sprintf("Application object %d", i), UNSIGNED32, ATTRIBUTE_SDO_RW, "0x0")
	}
	od.AddVariableList(BASE_RPDO_MAPPING_INDEX+indexOffset, fmt.Sprintf("%s mapping parameter", pdoType), pdoMap)

	log.Infof("[OD] Added new PDO object to OD : %s%v", pdoType, pdoNb)
	return nil
}

// Add an RPDO entry to OD with defaults
func (od *ObjectDictionary) AddRPDO(rpdoNb uint16) error {
	if rpdoNb < 1 || rpdoNb > 512 {
		return ODR_DEV_INCOMPAT
	}
	return od.addPDO(rpdoNb, true)
}

// Add a TPDO entry to OD with defaults
func (od *ObjectDictionary) AddTPDO(tpdoNb uint16) error {
	if tpdoNb < 1 || tpdoNb > 512 {
		return ODR_DEV_INCOMPAT
	}
	return od.addPDO(tpdoNb, false)
}

// Add a SYNC object with defaults
// This will add SYNC with 0x1005,0x1006,0x1007 & 0x1019
func (od *ObjectDictionary) AddSYNC() {
	od.AddVariableType(0x1005, 0, "COB-ID SYNC message", UNSIGNED32, ATTRIBUTE_SDO_RW, "0x80000080") // Disabled with standard cob-id
	od.AddVariableType(0x1006, 0, "Communication cycle period", UNSIGNED32, ATTRIBUTE_SDO_RW, "0x0")
	od.AddVariableType(0x1007, 0, "Synchronous window length", UNSIGNED32, ATTRIBUTE_SDO_RW, "0x0")
	od.AddVariableType(0x1019, 0, "Synchronous counter overflow value", UNSIGNED8, ATTRIBUTE_SDO_RW, "0x0")
	log.Infof("[OD] Added new SYNC object to OD")
}

// Get an entry corresponding to a given index
// Index can either be a string, int or uint16
// This method does not return an error for chaining
func (od *ObjectDictionary) Index(index any) *Entry {
	var entry *Entry
	switch ind := index.(type) {
	case string:
		entry = od.entriesByIndexName[ind]
	case int:
		entry = od.entriesByIndexValue[uint16(ind)]
	case uint:
		entry = od.entriesByIndexValue[uint16(ind)]
	case uint16:
		entry = od.entriesByIndexValue[ind]
	default:
		return nil
	}
	return entry
}

type FileInfo struct {
	FileName         string
	FileVersion      string
	FileRevision     string
	LastEDS          string
	EDSVersion       string
	Description      string
	CreationTime     string
	CreationDate     string
	CreatedBy        string
	ModificationTime string
	ModificationDate string
	ModifiedBy       string
}

// OD object used to store a "VAR" or "DOMAIN" object type
type Variable struct {
	data            []byte
	Name            string
	DataType        byte
	Attribute       uint8 // Attribute contains the access type and pdo mapping info
	ParameterValue  string
	defaultValue    []byte
	StorageLocation string
	LowLimit        int
	HighLimit       int
	SubIndex        uint8
}

// OD object used to store a "RECORD" or "ARRAY" object type
type VariableList struct {
	objectType uint8 // either "RECORD" or "ARRAY"
	Variables  []Variable
}

func (rec *VariableList) GetSubObject(subindex uint8) (*Variable, error) {
	if rec.objectType == OBJ_ARR {
		subEntriesCount := len(rec.Variables)
		if subindex >= uint8(subEntriesCount) {
			return nil, ODR_SUB_NOT_EXIST
		}
		return &rec.Variables[subindex], nil
	}
	for i, variable := range rec.Variables {
		if variable.SubIndex == subindex {
			return &rec.Variables[i], nil
		}
	}
	return nil, ODR_SUB_NOT_EXIST
}

func (rec *VariableList) AddSubObject(
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
	if rec.objectType == OBJ_ARR {
		if int(subindex) >= len(rec.Variables) {
			log.Error("[OD] trying to add a sub object to array but out of bounds")
			return nil, ODR_SUB_NOT_EXIST
		}
		variable, err := NewVariable(subindex, name, datatype, attribute, value)
		if err != nil {
			return nil, err
		}
		rec.Variables[subindex] = *variable
		return &rec.Variables[subindex], nil
	}
	variable, err := NewVariable(subindex, name, datatype, attribute, value)
	if err != nil {
		return nil, err
	}
	rec.Variables = append(rec.Variables, *variable)
	return &rec.Variables[len(rec.Variables)-1], nil
}

func NewRecord() *VariableList {
	return &VariableList{objectType: OBJ_RECORD, Variables: make([]Variable, 0)}
}

func NewArray(length uint8) *VariableList {
	return &VariableList{objectType: OBJ_ARR, Variables: make([]Variable, length)}
}
