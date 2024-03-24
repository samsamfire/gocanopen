package od

import (
	"encoding/binary"
	"fmt"
	"reflect"
	"runtime"
	"strings"

	log "github.com/sirupsen/logrus"
	"gopkg.in/ini.v1"
)

// An Entry object is the main building block of an [ObjectDictionary].
// it holds an OD entry, i.e. an OD object at a specific index.
// An entry can be one of the following object types, defined by CiA 301
//   - VAR [Variable]
//   - DOMAIN [Variable]
//   - ARRAY [VariableList]
//   - RECORD [VariableList]
//
// If the Object is an ARRAY or a RECORD it can hold also multiple sub entries.
// sub entries are always of type VAR, for simplicity.
type Entry struct {
	// The OD index e.g. x1006
	Index uint16
	// The OD name inside of EDS
	Name string
	// The OD object type, as cited above.
	ObjectType uint8
	// Either a [Variable] or a [VariableList] object
	object            any
	extension         *extension
	subEntriesNameMap map[string]uint8
}

// Subindex returns the [Variable] at a given subindex.
// subindex can be a string, int, or uint8.
// When using a string it will try to find the subindex according to the OD naming.
func (entry *Entry) SubIndex(subIndex any) (v *Variable, e error) {
	if entry == nil {
		return nil, ODR_IDX_NOT_EXIST
	}
	switch object := entry.object.(type) {
	case *Variable:
		if subIndex != 0 && subIndex != "" {
			return nil, ODR_SUB_NOT_EXIST
		}
		return object, nil
	case *VariableList:
		var convertedSubIndex uint8
		var ok bool
		switch sub := subIndex.(type) {
		case string:
			convertedSubIndex, ok = entry.subEntriesNameMap[sub]
			if !ok {
				return nil, ODR_SUB_NOT_EXIST
			}
		case int:
			if sub >= 256 {
				return nil, ODR_DEV_INCOMPAT
			}
			convertedSubIndex = uint8(sub)
		case uint8:
			convertedSubIndex = sub
		default:
			return nil, ODR_DEV_INCOMPAT

		}
		return object.GetSubObject(convertedSubIndex)
	default:
		// This is not normal
		return nil, ODR_DEV_INCOMPAT
	}

}

// Add a member to Entry, this is only possible for Record/Array objects
func (entry *Entry) addSectionMember(section *ini.Section, name string, nodeId uint8, subIndex uint8) error {
	record, ok := entry.object.(*VariableList)
	if !ok {
		return fmt.Errorf("cannot add member to type : %T", record)
	}
	variable, err := NewVariableFromSection(section, name, nodeId, entry.Index, subIndex)
	if err != nil {
		return err
	}
	switch entry.ObjectType {
	case OBJ_ARR:
		record.Variables[subIndex] = variable
		entry.subEntriesNameMap[name] = subIndex
	case OBJ_RECORD:
		record.Variables = append(record.Variables, variable)
		entry.subEntriesNameMap[name] = subIndex
	default:
		return fmt.Errorf("add member not supported for ObjectType : %v", entry.ObjectType)
	}
	return nil
}

// Add an extension to an OD entry
// This allows an OD entry to perform custom behaviour on read or on write.
// Some extensions are already defined in this package for defined CiA entries
// e.g. objects x1005, x1006, etc.
// Implementation of the default StreamReader & StreamWriter for a regular OD entry
// can be found here [ReadEntryDefault] & [WriteEntryDefault].
func (entry *Entry) AddExtension(object any, read StreamReader, write StreamWriter) {
	log.Debugf("[OD][EXTENSION][x%x] added OD extension : %v, %v",
		entry.Index,
		getFunctionName(read),
		getFunctionName(write),
	)
	extension := &extension{object: object, read: read, write: write}
	entry.extension = extension
}

// SubCount returns the number of sub entries inside entry.
// If entry is of VAR type it will return 1
func (entry *Entry) SubCount() int {

	switch object := entry.object.(type) {
	case *Variable:
		return 1
	case *VariableList:
		return len(object.Variables)
	default:
		// This is not normal
		log.Errorf("The entry %v has an invalid type %T", entry, entry)
		return 1
	}
}

func (entry *Entry) Extension() *extension {
	return entry.extension
}

func (entry *Entry) FlagPDOByte(subIndex byte) *uint8 {
	return &entry.extension.flagsPDO[subIndex>>3]
}

// GetRawData returns the raw byte slice stored inside of OD
func (entry *Entry) GetRawData(subIndex uint8, length uint16) ([]byte, error) {
	streamer, err := NewStreamer(entry, subIndex, true)
	if err != nil {
		return nil, err
	}
	if int(streamer.stream.DataLength) != int(length) && length != 0 {
		return nil, ODR_TYPE_MISMATCH
	}
	return streamer.stream.Data, nil
}

// Uint8 reads data inside of OD as if it were and UNSIGNED8.
// It returns an error if length is incorrect or read failed.
func (entry *Entry) Uint8(subIndex uint8) (uint8, error) {
	b := make([]byte, 1)
	err := entry.readSubExactly(subIndex, b, true)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

// Uint16 reads data inside of OD as if it were and UNSIGNED16.
// It returns an error if length is incorrect or read failed.
func (entry *Entry) Uint16(subIndex uint8) (uint16, error) {
	b := make([]byte, 2)
	err := entry.readSubExactly(subIndex, b, true)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(b), nil
}

// Uint32 reads data inside of OD as if it were and UNSIGNED32.
// It returns an error if length is incorrect or read failed.
func (entry *Entry) Uint32(subIndex uint8) (uint32, error) {
	b := make([]byte, 4)
	err := entry.readSubExactly(subIndex, b, true)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b), nil
}

// Uint64 reads data inside of OD as if it were and UNSIGNED64.
// It returns an error if length is incorrect or read failed.
func (entry *Entry) Uint64(subIndex uint8) (uint64, error) {
	b := make([]byte, 8)
	err := entry.readSubExactly(subIndex, b, true)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(b), nil
}

// PutUint8 writes an UNSIGNED8 to OD entry.
// origin can be set to true in order to bypass any existing extension.
func (entry *Entry) PutUint8(subIndex uint8, value uint8, origin bool) error {
	b := []byte{value}
	err := entry.writeSubExactly(subIndex, b, origin)
	if err != nil {
		return err
	}
	return nil
}

// PutUint16 writes an UNSIGNED16 to OD entry.
// origin can be set to true in order to bypass any existing extension.
func (entry *Entry) PutUint16(subIndex uint8, data uint16, origin bool) error {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, data)
	err := entry.writeSubExactly(subIndex, b, origin)
	if err != nil {
		return err
	}
	return nil
}

// PutUint32 writes an UNSIGNED32 to OD entry.
// origin can be set to true in order to bypass any existing extension.
func (entry *Entry) PutUint32(subIndex uint8, data uint32, origin bool) error {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, data)
	err := entry.writeSubExactly(subIndex, b, origin)
	if err != nil {
		return err
	}
	return nil
}

// PutUint64 writes an UNSIGNED64 to OD entry.
// origin can be set to true in order to bypass any existing extension.
func (entry *Entry) PutUint64(subIndex uint8, data uint64, origin bool) error {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, data)
	err := entry.writeSubExactly(subIndex, b, origin)
	if err != nil {
		return err
	}
	return nil
}

// Read exactly len(b) bytes from OD at (index,subIndex)
// Origin parameter controls extension usage if exists
func (entry *Entry) readSubExactly(subIndex uint8, b []byte, origin bool) error {
	streamer, err := NewStreamer(entry, subIndex, origin)
	if err != nil {
		return err
	}
	if int(streamer.stream.DataLength) != len(b) {
		return ODR_TYPE_MISMATCH
	}
	_, err = streamer.Read(b)
	return err
}

// Write exactly len(b) bytes to OD at (index,subIndex)
// Origin parameter controls extension usage if exists
func (entry *Entry) writeSubExactly(subIndex uint8, b []byte, origin bool) error {
	streamer, err := NewStreamer(entry, subIndex, origin)
	if err != nil {
		return err
	}
	if int(streamer.stream.DataLength) != len(b) {
		return ODR_TYPE_MISMATCH
	}
	_, err = streamer.Write(b)
	return err

}

// Returns last part of function name
func getFunctionName(i interface{}) string {
	fullName := runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
	fullNameSplitted := strings.Split(fullName, ".")
	return fullNameSplitted[len(fullNameSplitted)-1]
}
