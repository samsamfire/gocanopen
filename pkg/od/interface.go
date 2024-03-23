package od

import (
	"fmt"
	"io"

	log "github.com/sirupsen/logrus"
)

// ObjectDictionary is used for storing all entries of a CANopen node
// according to CiA 301. This is the internal representation of an EDS file
type ObjectDictionary struct {
	Reader              io.ReadSeeker
	entriesByIndexValue map[uint16]*Entry
	entriesByIndexName  map[string]*Entry
}

// Add an entry to OD, any existing entry will be replaced
func (od *ObjectDictionary) addEntry(entry *Entry) {
	_, entryIndexValueExists := od.entriesByIndexValue[entry.Index]
	if entryIndexValueExists {
		log.Warnf("[OD] overwritting entry index x%x", entry.Index)
	}
	od.entriesByIndexValue[entry.Index] = entry
	od.entriesByIndexName[entry.Name] = entry
	log.Debugf("[OD] adding %v | %v at %x", OBJ_NAME_MAP[entry.ObjectType], entry.Name, entry.Index)
}

// Add a variable type entry to OD with given variable, existing entry will be
func (od *ObjectDictionary) addVariable(index uint16, variable *Variable) *Entry {
	entry := &Entry{
		Index:             index,
		Name:              variable.Name,
		object:            variable,
		ObjectType:        OBJ_VAR,
		extension:         nil,
		subEntriesNameMap: map[string]uint8{}}
	od.addEntry(entry)
	return entry
}

// AddVariableType adds an entry of type VAR to OD
// the value should be given as a string with hex representation
// e.g. 0x22 or 0x55555
// If the variable already exists, it will be overwritten
func (od *ObjectDictionary) AddVariableType(
	index uint16,
	name string,
	datatype uint8,
	attribute uint8,
	value string,
) (*Entry, error) {
	variable, err := NewVariable(0, name, datatype, attribute, value)
	if err != nil {
		return nil, err
	}
	entry := od.addVariable(index, variable)
	return entry, nil
}

// AddVariableList adds an entry of type ARRAY or RECORD depending on [VariableList]
func (od *ObjectDictionary) AddVariableList(index uint16, name string, varList *VariableList) *Entry {
	entry := &Entry{
		Index:             index,
		Name:              name,
		object:            varList,
		ObjectType:        varList.objectType,
		extension:         nil,
		subEntriesNameMap: map[string]uint8{}}

	od.addEntry(entry)
	return entry
}

// AddFile adds a file like object, of type DOMAIN to OD
// readMode and writeMode should be given to determine what type of access to the file is allowed
// e.g. os.O_RDONLY if only reading is allowed
func (od *ObjectDictionary) AddFile(index uint16, indexName string, filePath string, readMode int, writeMode int) {
	log.Infof("[OD] adding file object entry : %v at x%x", filePath, index)
	fileObject := &FileObject{FilePath: filePath, ReadMode: readMode, WriteMode: writeMode}
	entry, _ := od.AddVariableType(index, indexName, DOMAIN, ATTRIBUTE_SDO_RW, "") // Cannot error
	entry.AddExtension(fileObject, ReadEntryFileObject, WriteEntryFileObject)
}

// AddReader adds an io.Reader object, of type DOMAIN to OD
func (od *ObjectDictionary) AddReader(index uint16, indexName string, reader io.Reader) {
	log.Infof("[OD] adding a reader entry : %v at x%x", indexName, index)
	entry, _ := od.AddVariableType(index, indexName, DOMAIN, ATTRIBUTE_SDO_R, "") // Cannot error
	entry.AddExtension(reader, ReadEntryReader, WriteEntryDisabled)
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
	pdoComm.AddSubObject(3, "Inhibit time", UNSIGNED16, ATTRIBUTE_SDO_RW, "0x0")
	pdoComm.AddSubObject(4, "Reserved", UNSIGNED16, ATTRIBUTE_SDO_RW, "0x0")
	pdoComm.AddSubObject(5, "Event timer", UNSIGNED16, ATTRIBUTE_SDO_RW, "0x0")

	od.AddVariableList(BASE_RPDO_COMMUNICATION_INDEX+indexOffset, fmt.Sprintf("%s communication parameter", pdoType), pdoComm)

	pdoMap := NewRecord()
	pdoMap.AddSubObject(0, "Number of mapped application objects in PDO", UNSIGNED8, ATTRIBUTE_SDO_RW, "0x0")
	for i := uint8(1); i <= PDO_MAX_MAPPED_ENTRIES; i++ {
		pdoMap.AddSubObject(i, fmt.Sprintf("Application object %d", i), UNSIGNED32, ATTRIBUTE_SDO_RW, "0x0")
	}
	od.AddVariableList(BASE_RPDO_MAPPING_INDEX+indexOffset, fmt.Sprintf("%s mapping parameter", pdoType), pdoMap)

	log.Infof("[OD] Added new PDO object to OD : %s%v", pdoType, pdoNb)
	return nil
}

// AddRPDO adds an RPDO entry to the OD.
// This means that an RPDO Communication & Mapping parameter
// entries are created with the given rpdoNb.
// This however does not create the corresponding CANopen objects
func (od *ObjectDictionary) AddRPDO(rpdoNb uint16) error {
	if rpdoNb < 1 || rpdoNb > 512 {
		return ODR_DEV_INCOMPAT
	}
	return od.addPDO(rpdoNb, true)
}

// AddTPDO adds a TPDO entry to the OD.
// This means that a TPDO Communication & Mapping parameter
// entries are created with the given tpdoNb.
// This however does not create the corresponding CANopen objects
func (od *ObjectDictionary) AddTPDO(tpdoNb uint16) error {
	if tpdoNb < 1 || tpdoNb > 512 {
		return ODR_DEV_INCOMPAT
	}
	return od.addPDO(tpdoNb, false)
}

// AddSYNC adds a SYNC entry to the OD.
// This adds objects 0x1005, 0x1006, 0x1007 & 0x1019 to the OD.
// By default, SYNC is added with producer disabled and can id of 0x80
func (od *ObjectDictionary) AddSYNC() {
	od.AddVariableType(0x1005, "COB-ID SYNC message", UNSIGNED32, ATTRIBUTE_SDO_RW, "0x80000080") // Disabled with standard cob-id
	od.AddVariableType(0x1006, "Communication cycle period", UNSIGNED32, ATTRIBUTE_SDO_RW, "0x0")
	od.AddVariableType(0x1007, "Synchronous window length", UNSIGNED32, ATTRIBUTE_SDO_RW, "0x0")
	od.AddVariableType(0x1019, "Synchronous counter overflow value", UNSIGNED8, ATTRIBUTE_SDO_RW, "0x0")
	log.Infof("[OD] Added new SYNC object to OD")
}

// Index returns an OD entry at the specified index.
// index can either be a string, int or uint16.
// This method does not return an error (for chaining with Subindex()) but instead returns
// nil if no corresponding [Entry] is found.
func (od *ObjectDictionary) Index(index any) *Entry {
	switch ind := index.(type) {
	case string:
		return od.entriesByIndexName[ind]
	case int:
		return od.entriesByIndexValue[uint16(ind)]
	case uint:
		return od.entriesByIndexValue[uint16(ind)]
	case uint16:
		return od.entriesByIndexValue[ind]
	default:
		return nil
	}
}

// type FileInfo struct {
// 	FileName         string
// 	FileVersion      string
// 	FileRevision     string
// 	LastEDS          string
// 	EDSVersion       string
// 	Description      string
// 	CreationTime     string
// 	CreationDate     string
// 	CreatedBy        string
// 	ModificationTime string
// 	ModificationDate string
// 	ModifiedBy       string
// }

// Variable is the main data representation for a value stored inside of OD
// It is used to store a "VAR" or "DOMAIN" object type as well as
// any sub entry of a "RECORD" or "ARRAY" object type
type Variable struct {
	valueDefault []byte
	value        []byte
	// Name of this variable
	Name string
	// The CiA 301 data type of this variable
	DataType byte
	// Attribute contains the access type as well as the mapping
	// information. e.g. ATTRIBUTE_SDO_RW | ATTRIBUTE_RPDO
	Attribute uint8
	// StorageLocation has information on which medium is the data
	// stored. Currently this is unused, everything is stored in RAM
	StorageLocation string
	// The minimum value for this variable
	LowLimit int
	// The maximum value for this variable
	HighLimit int
	// The subindex for this variable if part of an ARRAY or RECORD
	SubIndex uint8
}

// VariableList is the data representation for
// storing a "RECORD" or "ARRAY" object type
type VariableList struct {
	objectType uint8 // either "RECORD" or "ARRAY"
	Variables  []Variable
}

// GetSubObject returns the [Variable] corresponding to
// a given subindex if not found, it errors with
// ODR_SUB_NOT_EXIST
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

// AddSubObject adds a [Variable] to the VariableList
// If the VariableList is an ARRAY then the subindex should be
// identical to the actual placement inside of the array.
// Otherwise it can be any valid subindex value, and the VariableList
// will grow accordingly
func (rec *VariableList) AddSubObject(
	subindex uint8,
	name string,
	datatype uint8,
	attribute uint8,
	value string,
) (*Variable, error) {
	encoded, err := Encode(value, datatype, 0)
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
