package od

import (
	"fmt"
	"io"
	"log/slog"
	"sync"

	"gopkg.in/ini.v1"
)

var _logger = slog.Default()

// ObjectDictionary is used for storing all entries of a CANopen node
// according to CiA 301. This is the internal representation of an EDS file
type ObjectDictionary struct {
	Reader              io.ReadSeeker
	logger              *slog.Logger
	iniFile             *ini.File
	entriesByIndexValue map[uint16]*Entry
	entriesByIndexName  map[string]*Entry
}

// Add an entry to OD, any existing entry will be replaced
func (od *ObjectDictionary) addEntry(entry *Entry) {
	_, entryIndexValueExists := od.entriesByIndexValue[entry.Index]
	if entryIndexValueExists {
		entry.logger.Warn("overwritting entry")
	}
	od.entriesByIndexValue[entry.Index] = entry
	od.entriesByIndexName[entry.Name] = entry
	entry.logger.Debug("adding entry", "objectType", OBJ_NAME_MAP[entry.ObjectType])
}

// Add a variable type entry to OD with given variable, existing entry will be
func (od *ObjectDictionary) addVariable(index uint16, variable *Variable) *Entry {
	entry := NewEntry(od.logger, index, variable.Name, variable, ObjectTypeVAR)
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
	entry := NewEntry(od.logger, index, name, varList, varList.objectType)
	od.addEntry(entry)
	return entry
}

// AddFile adds a file like object, of type DOMAIN to OD
// readMode and writeMode should be given to determine what type of access to the file is allowed
// e.g. os.O_RDONLY if only reading is allowed
func (od *ObjectDictionary) AddFile(index uint16, indexName string, filePath string, readMode int, writeMode int) {
	fileObject := &FileObject{FilePath: filePath, ReadMode: readMode, WriteMode: writeMode}
	entry, _ := od.AddVariableType(index, indexName, DOMAIN, AttributeSdoRw, "") // Cannot error
	entry.logger.Info("adding extension file i/o", "path", filePath)
	entry.AddExtension(fileObject, ReadEntryFileObject, WriteEntryFileObject)
}

// AddReader adds an io.Reader object, of type DOMAIN to OD
func (od *ObjectDictionary) AddReader(index uint16, indexName string, reader io.Reader) {
	entry, _ := od.AddVariableType(index, indexName, DOMAIN, AttributeSdoR, "") // Cannot error
	entry.logger.Info("adding extension reader")
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
	pdoComm.AddSubObject(0, "Highest sub-index supported", UNSIGNED8, AttributeSdoR, "0x5")
	pdoComm.AddSubObject(1, fmt.Sprintf("COB-ID used by %s", pdoType), UNSIGNED32, AttributeSdoRw, "0x0")
	pdoComm.AddSubObject(2, "Transmission type", UNSIGNED8, AttributeSdoRw, "0x0")
	pdoComm.AddSubObject(3, "Inhibit time", UNSIGNED16, AttributeSdoRw, "0x0")
	pdoComm.AddSubObject(4, "Reserved", UNSIGNED8, AttributeSdoRw, "0x0")
	pdoComm.AddSubObject(5, "Event timer", UNSIGNED16, AttributeSdoRw, "0x0")

	od.AddVariableList(EntryRPDOCommunicationStart+indexOffset, fmt.Sprintf("%s communication parameter", pdoType), pdoComm)

	pdoMap := NewRecord()
	pdoMap.AddSubObject(0, "Number of mapped application objects in PDO", UNSIGNED8, AttributeSdoRw, "0x0")
	for i := range MaxMappedEntriesPdo {
		pdoMap.AddSubObject(i+1, fmt.Sprintf("Application object %d", i+1), UNSIGNED32, AttributeSdoRw, "0x0")
	}
	od.AddVariableList(EntryRPDOMappingStart+indexOffset, fmt.Sprintf("%s mapping parameter", pdoType), pdoMap)
	od.logger.Info("added new PDO oject to OD", "type", pdoType, "nb", pdoNb)
	return nil
}

// AddRPDO adds an RPDO entry to the OD.
// This means that an RPDO Communication & Mapping parameter
// entries are created with the given rpdoNb.
// This however does not create the corresponding CANopen objects
func (od *ObjectDictionary) AddRPDO(rpdoNb uint16) error {
	if rpdoNb < 1 || rpdoNb > 512 {
		return ErrDevIncompat
	}
	return od.addPDO(rpdoNb, true)
}

// AddTPDO adds a TPDO entry to the OD.
// This means that a TPDO Communication & Mapping parameter
// entries are created with the given tpdoNb.
// This however does not create the corresponding CANopen objects
func (od *ObjectDictionary) AddTPDO(tpdoNb uint16) error {
	if tpdoNb < 1 || tpdoNb > 512 {
		return ErrDevIncompat
	}
	return od.addPDO(tpdoNb, false)
}

// AddSYNC adds a SYNC entry to the OD.
// This adds objects 0x1005, 0x1006, 0x1007 & 0x1019 to the OD.
// By default, SYNC is added with producer disabled and can id of 0x80
func (od *ObjectDictionary) AddSYNC() {
	od.AddVariableType(0x1005, "COB-ID SYNC message", UNSIGNED32, AttributeSdoRw, "0x80000080") // Disabled with standard cob-id
	od.AddVariableType(0x1006, "Communication cycle period", UNSIGNED32, AttributeSdoRw, "0x0")
	od.AddVariableType(0x1007, "Synchronous window length", UNSIGNED32, AttributeSdoRw, "0x0")
	od.AddVariableType(0x1019, "Synchronous counter overflow value", UNSIGNED8, AttributeSdoRw, "0x0")
	od.logger.Info("added new SYNC object to OD")
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

// Entries returns map of indexes and entries
func (od *ObjectDictionary) Entries() map[uint16]*Entry {
	return od.entriesByIndexValue
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
	mu           sync.RWMutex
	valueDefault []byte
	value        []byte
	// Name of this variable
	Name string
	// The CiA 301 data type of this variable
	DataType byte
	// Attribute contains the access type as well as the mapping
	// information. e.g. AttributeSdoRw | AttributeRpdo
	Attribute uint8
	// StorageLocation has information on which medium is the data
	// stored. Currently this is unused, everything is stored in RAM
	StorageLocation string
	// The minimum value for this variable
	lowLimit []byte
	// The maximum value for this variable
	highLimit []byte
	// The subindex for this variable if part of an ARRAY or RECORD
	SubIndex uint8
}

// VariableList is the data representation for
// storing a "RECORD" or "ARRAY" object type
type VariableList struct {
	objectType uint8 // either "RECORD" or "ARRAY"
	Variables  []*Variable
}

// GetSubObject returns the [Variable] corresponding to
// a given subindex if not found, it errors with
// ODR_SUB_NOT_EXIST
func (rec *VariableList) GetSubObject(subindex uint8) (*Variable, error) {
	if rec.objectType == ObjectTypeARRAY {
		subEntriesCount := len(rec.Variables)
		if subindex >= uint8(subEntriesCount) {
			return nil, ErrSubNotExist
		}
		return rec.Variables[subindex], nil
	}
	for i, variable := range rec.Variables {
		if variable.SubIndex == subindex {
			return rec.Variables[i], nil
		}
	}
	return nil, ErrSubNotExist
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
	encoded, err := EncodeFromString(value, datatype, 0)
	encodedCopy := make([]byte, len(encoded))
	copy(encodedCopy, encoded)
	if err != nil {
		return nil, err
	}
	if rec.objectType == ObjectTypeARRAY {
		if int(subindex) >= len(rec.Variables) {
			_logger.Error("trying to add a sub-object to array but ouf of bounds",
				"subindex", subindex,
				"length", len(rec.Variables),
			)
			return nil, ErrSubNotExist
		}
		variable, err := NewVariable(subindex, name, datatype, attribute, value)
		if err != nil {
			return nil, err
		}
		rec.Variables[subindex] = variable
		return rec.Variables[subindex], nil
	}
	variable, err := NewVariable(subindex, name, datatype, attribute, value)
	if err != nil {
		return nil, err
	}
	rec.Variables = append(rec.Variables, variable)
	return rec.Variables[len(rec.Variables)-1], nil
}

func NewRecord() *VariableList {
	return &VariableList{objectType: ObjectTypeRECORD, Variables: make([]*Variable, 0)}
}

func NewArray(length uint8) *VariableList {
	return &VariableList{objectType: ObjectTypeARRAY, Variables: make([]*Variable, length)}
}
