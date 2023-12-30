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
func (od *ObjectDictionary) addVariable(variable *Variable) {
	od.addEntry(&Entry{Index: variable.Index, Name: variable.Name, Object: variable, Extension: nil, subEntriesNameMap: map[string]uint8{}})
}

// Add a record to OD
func (od *ObjectDictionary) AddRecord(index uint16, name string, record []Record) {
	od.addEntry(&Entry{Index: index, Name: name, Object: record, Extension: nil, subEntriesNameMap: map[string]uint8{}})
}

// Add an array to OD
func (od *ObjectDictionary) AddArray(index uint16, name string, array Array) {
	od.addEntry(&Entry{Index: index, Name: name, Object: array, Extension: nil, subEntriesNameMap: map[string]uint8{}})
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
	encoded, err := encode(value, datatype, 0)
	encodedCopy := make([]byte, len(encoded))
	copy(encodedCopy, encoded)
	if err != nil {
		return nil, err
	}
	variable := &Variable{
		Index:        index,
		SubIndex:     subindex,
		Name:         name,
		data:         encoded,
		defaultValue: encodedCopy,
		Attribute:    attribute,
		DataType:     datatype,
	}
	od.addVariable(variable)
	return variable, nil
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

	variables := make([]Variable, 0)
	variables = append(variables, Variable{
		data:         []byte{0x5},
		Name:         "Highest sub-index supported",
		DataType:     UNSIGNED8,
		Attribute:    ATTRIBUTE_SDO_R,
		defaultValue: []byte{0x5},
		Index:        BASE_RPDO_COMMUNICATION_INDEX + indexOffset,
		SubIndex:     0,
	})
	variables = append(variables, Variable{
		data:         []byte{0, 0, 0, 0},
		Name:         fmt.Sprintf("COB-ID used by %s", pdoType),
		DataType:     UNSIGNED32,
		Attribute:    ATTRIBUTE_SDO_RW,
		defaultValue: []byte{0, 0, 0, 0},
		Index:        BASE_RPDO_COMMUNICATION_INDEX + indexOffset,
		SubIndex:     1,
	})
	variables = append(variables, Variable{
		data:         []byte{0},
		Name:         "Transmission type",
		DataType:     UNSIGNED8,
		Attribute:    ATTRIBUTE_SDO_RW,
		defaultValue: []byte{0},
		Index:        BASE_RPDO_COMMUNICATION_INDEX + indexOffset,
		SubIndex:     2,
	})
	variables = append(variables, Variable{
		data:         []byte{0, 0},
		Name:         "Event timer",
		DataType:     UNSIGNED16,
		Attribute:    ATTRIBUTE_SDO_RW,
		defaultValue: []byte{0, 0},
		Index:        BASE_RPDO_COMMUNICATION_INDEX + indexOffset,
		SubIndex:     5,
	})
	od.AddArray(
		BASE_RPDO_COMMUNICATION_INDEX+indexOffset,
		fmt.Sprintf("%s communication parameter", pdoType),
		Array{Variables: variables},
	)

	variables = make([]Variable, 0)
	variables = append(variables, Variable{
		data:         []byte{0},
		Name:         "Number of mapped application objects in PDO",
		DataType:     UNSIGNED8,
		Attribute:    ATTRIBUTE_SDO_RW,
		defaultValue: []byte{0},
		Index:        BASE_RPDO_MAPPING_INDEX + indexOffset,
		SubIndex:     0,
	})
	for i := uint8(1); i <= MAX_MAPPED_ENTRIES; i++ {
		variables = append(variables, Variable{
			data:         []byte{0, 0, 0, 0},
			Name:         fmt.Sprintf("Application object %d", i),
			DataType:     UNSIGNED32,
			Attribute:    ATTRIBUTE_SDO_RW,
			defaultValue: []byte{0, 0, 0, 0},
			Index:        BASE_RPDO_MAPPING_INDEX + indexOffset,
			SubIndex:     i,
		})
	}
	od.AddArray(
		BASE_RPDO_MAPPING_INDEX+indexOffset,
		fmt.Sprintf("%s mapping parameter", pdoType),
		Array{Variables: variables},
	)
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

// OD object of type "VAR" object used for holding any sub object
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
	Index           uint16
	SubIndex        uint8
}

// OD object of type "ARRAY"
type Array struct {
	Variables []Variable
}

// OD object of type "RECORD"
type Record struct {
	Variable Variable // An array is also a record type
	Subindex uint8
}
