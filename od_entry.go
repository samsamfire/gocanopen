package canopen

import (
	"encoding/binary"
	"fmt"

	log "github.com/sirupsen/logrus"
	"gopkg.in/ini.v1"
)

// An entry holds an OD entry, i.e. an OD object at a specific index
// An entry can be one of the following object types : variable, array, record or domain
// Depending on the object type it may contain some some objects
type Entry struct {
	Index             uint16
	Name              string
	ObjectType        uint8
	Object            any
	Extension         *Extension
	subEntriesNameMap map[string]uint8
}

// Get variable for a given sub index
// Subindex can be a string,int, or uint8
// When using a string it will try to find the subindex according to the OD naming
func (entry *Entry) SubIndex(subIndex any) (v *Variable, e error) {
	if entry == nil {
		return nil, ODR_IDX_NOT_EXIST
	}
	switch object := entry.Object.(type) {
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
func (entry *Entry) AddMember(section *ini.Section, name string, nodeId uint8, subIndex uint8) error {
	record, ok := entry.Object.(*VariableList)
	if !ok {
		return fmt.Errorf("cannot add member to type : %T", record)
	}
	variable, err := NewVariableFromSection(section, name, nodeId, entry.Index, subIndex)
	if err != nil {
		return err
	}
	switch entry.ObjectType {
	case OBJ_ARR:
		record.Variables[subIndex] = *variable
		entry.subEntriesNameMap[name] = subIndex
	case OBJ_RECORD:
		record.Variables = append(record.Variables, *variable)
		entry.subEntriesNameMap[name] = subIndex
	default:
		return fmt.Errorf("add member not supported for ObjectType : %v", entry.ObjectType)
	}
	return nil
}

// Add an extension to entry and return created extension
// object can be any custom object
func (entry *Entry) AddExtension(object any, read StreamReader, write StreamWriter) *Extension {
	log.Debugf("[OD][EXTENSION][x%x] added an extension : %v, %v",
		entry.Index,
		getFunctionName(read),
		getFunctionName(write),
	)
	extension := &Extension{Object: object, Read: read, Write: write}
	entry.Extension = extension
	return extension
}

// Get number of sub entries. Depends on type
func (entry *Entry) SubCount() int {

	switch object := entry.Object.(type) {
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

// Getptr inside OD, similar to read
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

// Read Uint8 inside object dictionary
func (entry *Entry) Uint8(subIndex uint8) (uint8, error) {
	b := make([]byte, 1)
	err := entry.readSubExactly(subIndex, b, true)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

// Read Uint16 inside object dictionary
func (entry *Entry) Uint16(subIndex uint8) (uint16, error) {
	b := make([]byte, 2)
	err := entry.readSubExactly(subIndex, b, true)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(b), nil
}

// Read Uint32 inside object dictionary
func (entry *Entry) Uint32(subIndex uint8) (uint32, error) {
	b := make([]byte, 4)
	err := entry.readSubExactly(subIndex, b, true)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b), nil
}

// Read Uint64 inside object dictionary
func (entry *Entry) Uint64(subIndex uint8) (uint64, error) {
	b := make([]byte, 8)
	err := entry.readSubExactly(subIndex, b, true)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(b), nil
}

// Set Uint8, 16 , 32 , 64
func (entry *Entry) PutUint8(subIndex uint8, data uint8, origin bool) error {
	b := []byte{data}
	err := entry.writeSubExactly(subIndex, b, origin)
	if err != nil {
		return err
	}
	return nil
}

func (entry *Entry) PutUint16(subIndex uint8, data uint16, origin bool) error {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, data)
	err := entry.writeSubExactly(subIndex, b, origin)
	if err != nil {
		return err
	}
	return nil
}

func (entry *Entry) PutUint32(subIndex uint8, data uint32, origin bool) error {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, data)
	err := entry.writeSubExactly(subIndex, b, origin)
	if err != nil {
		return err
	}
	return nil
}

func (entry *Entry) PutUint64(subIndex uint8, data uint64, origin bool) error {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, data)
	err := entry.writeSubExactly(subIndex, b, origin)
	if err != nil {
		return err
	}
	return nil
}
