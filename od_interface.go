package canopen

import (
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

// Add a record to OD
func (od *ObjectDictionary) AddRecord(index uint16, name string, record []Record) {
	od.addEntry(&Entry{Index: index, Name: name, Object: record, Extension: nil, subEntriesNameMap: map[string]uint8{}})
}

// Add an array to OD
func (od *ObjectDictionary) AddArray(index uint16, name string, array Array) {
	od.addEntry(&Entry{Index: index, Name: name, Object: array, Extension: nil, subEntriesNameMap: map[string]uint8{}})
}

// Add a variable to OD
func (od *ObjectDictionary) AddVariable(index uint16, name string, variable Variable) {
	od.addEntry(&Entry{Index: index, Name: name, Object: variable, Extension: nil, subEntriesNameMap: map[string]uint8{}})
}

// Add file like object entry to OD
func (od *ObjectDictionary) AddFile(index uint16, indexName string, filePath string, readMode int, writeMode int) error {
	log.Infof("[OD] adding file object entry : %v at x%x", filePath, index)
	fileObject := &FileObject{FilePath: filePath, ReadMode: readMode, WriteMode: writeMode}
	variable := Variable{
		data:           []byte{},
		Name:           indexName,
		DataType:       DOMAIN,
		Attribute:      ATTRIBUTE_SDO_RW,
		ParameterValue: "",
		DefaultValue:   []byte{},
		Index:          index,
		SubIndex:       0,
	}
	od.AddVariable(index, indexName, variable)
	entry := od.Index(index)
	entry.AddExtension(fileObject, ReadEntryFileObject, WriteEntryFileObject)
	return nil
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
	DefaultValue    []byte
	StorageLocation string
	LowLimit        int
	HighLimit       int
	Index           uint16
	SubIndex        uint8
}

type Array struct {
	Variables []Variable
}
type Record struct {
	Variable Variable
	Subindex uint8
}

func isIDRestricted(canId uint16) bool {
	return canId <= 0x7f ||
		(canId >= 0x101 && canId <= 0x180) ||
		(canId >= 0x581 && canId <= 0x5FF) ||
		(canId >= 0x601 && canId <= 0x67F) ||
		(canId >= 0x6E0 && canId <= 0x6FF) ||
		canId >= 0x701
}

func NewOD() *ObjectDictionary {
	return &ObjectDictionary{
		entriesByIndexValue: make(map[uint16]*Entry),
		entriesByIndexName:  make(map[string]*Entry),
	}
}
