package od

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
)

var _logger = slog.Default()

// ObjectDictionary is used for storing all entries of a CANopen node
// according to CiA 301. This is the internal representation of an EDS file
type ObjectDictionary struct {
	logger              *slog.Logger
	rawOd               []byte
	entriesByIndexValue map[uint16]*Entry
	entriesByIndexName  map[string]*Entry
}

// Create a new reader object for reading
// raw OD file.
func (od *ObjectDictionary) NewReaderSeeker() io.ReadSeeker {
	return bytes.NewReader(od.rawOd)
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
	f := NewFileObject(filePath, od.logger, writeMode, readMode)
	entry, _ := od.AddVariableType(index, indexName, DOMAIN, AttributeSdoRw, "") // Cannot error
	entry.logger.Info("adding extension file i/o", "path", filePath)
	entry.AddExtension(f, ReadEntryFileObject, WriteEntryFileObject)
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

// Creates new OD object streamer at the specified index and subindex
func (od *ObjectDictionary) Streamer(index uint16, subindex uint8, origin bool) (*Streamer, error) {
	entry := od.Index(index)
	streamer, err := NewStreamer(entry, subindex, origin)
	return &streamer, err
}

// Entries returns map of indexes and entries
func (od *ObjectDictionary) Entries() map[uint16]*Entry {
	return od.entriesByIndexValue
}
