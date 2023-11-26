package canopen

import (
	"encoding/binary"
	"fmt"

	log "github.com/sirupsen/logrus"
	"gopkg.in/ini.v1"
)

// An entry can be any object type : variable, array, record or domain
// It is a specific index of the object dictionary and can contain multiple sub indexes
type Entry struct {
	Index             uint16
	Name              string
	Object            any
	Extension         *Extension
	subEntriesNameMap map[string]uint8
}

// Get variable from given sub index
// Subindex can either be a string or an int, or uint8
func (entry *Entry) SubIndex(subIndex any) (v *Variable, e error) {
	if entry == nil {
		return nil, ODR_IDX_NOT_EXIST
	}
	switch object := entry.Object.(type) {
	case Variable:
		if subIndex != 0 && subIndex != "" {
			return nil, ODR_SUB_NOT_EXIST
		}
		return &object, nil
	case Array:
		subEntriesCount := len(object.Variables)
		switch sub := subIndex.(type) {
		case string:
			subIndexInt, ok := entry.subEntriesNameMap[sub]
			if ok {
				return &object.Variables[subIndexInt], nil
			}
			return nil, ODR_SUB_NOT_EXIST
		case int:
			if uint8(sub) >= uint8(subEntriesCount) {
				return nil, ODR_SUB_NOT_EXIST
			}
			return &object.Variables[uint8(sub)], nil
		case uint8:
			if sub >= uint8(subEntriesCount) {
				return nil, ODR_SUB_NOT_EXIST
			}
			return &object.Variables[sub], nil
		default:
			return nil, ODR_DEV_INCOMPAT
		}
	case []Record:
		records := object
		var record *Record
		switch sub := subIndex.(type) {
		case string:
			subIndexInt, ok := entry.subEntriesNameMap[sub]
			if ok {
				for i := range records {
					if records[i].Subindex == subIndexInt {
						record = &records[i]
						return &record.Variable, nil
					}
				}
			}
			return nil, ODR_SUB_NOT_EXIST
		case int:
			for i := range records {
				if records[i].Subindex == uint8(sub) {
					record = &records[i]
					return &record.Variable, nil
				}
			}
			return nil, ODR_SUB_NOT_EXIST
		case uint8:
			for i := range records {
				if records[i].Subindex == sub {
					record = &records[i]
					return &record.Variable, nil
				}
			}
			return nil, ODR_SUB_NOT_EXIST
		default:
			return nil, ODR_DEV_INCOMPAT

		}
	default:
		// This is not normal
		return nil, ODR_DEV_INCOMPAT
	}

}

// Add a member to Entry, this is only possible for Array or Record objects
func (entry *Entry) AddMember(section *ini.Section, name string, nodeId uint8, subIndex uint8) error {

	switch object := entry.Object.(type) {
	case Variable:
		return fmt.Errorf("cannot add a member to variable type")
	case Array:
		variable, err := buildVariable(section, name, nodeId, entry.Index, subIndex)
		if err != nil {
			return err
		}
		object.Variables[subIndex] = *variable
		entry.Object = object
		entry.subEntriesNameMap[name] = subIndex
		return nil

	case []Record:
		variable, err := buildVariable(section, name, nodeId, entry.Index, subIndex)
		if err != nil {
			return err
		}
		entry.Object = append(object, Record{Subindex: subIndex, Variable: *variable})
		entry.subEntriesNameMap[name] = subIndex
		return nil

	default:
		return fmt.Errorf("add member not supported for %T", object)
	}
}

// Add an extension to entry and return created extension
// object can be any custom object
func (entry *Entry) AddExtension(object any, read StreamReader, write StreamWriter) *Extension {
	extension := &Extension{Object: object, Read: read, Write: write}
	entry.Extension = extension
	return extension
}

// Get number of sub entries. Depends on type
func (entry *Entry) SubCount() int {

	switch object := entry.Object.(type) {
	case Variable:
		return 1
	case Array:
		return len(object.Variables)

	case []Record:
		return len(object)

	default:
		// This is not normal
		log.Errorf("The entry %v has an invalid type", entry)
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
func (entry *Entry) GetPtr(subIndex uint8, length uint16) (*[]byte, error) {
	streamer, err := NewStreamer(entry, subIndex, true)
	if err != nil {
		return nil, err
	}
	if int(streamer.stream.DataLength) != int(length) && length != 0 {
		return nil, ODR_TYPE_MISMATCH
	}
	return &streamer.stream.Data, nil
}

// Read Uint8 inside object dictionary
func (entry *Entry) Uint8(subIndex uint8, data *uint8) error {
	b := make([]byte, 1)
	err := entry.readSubExactly(subIndex, b, true)
	if err != nil {
		return err
	}
	*data = b[0]
	return nil
}

// Read Uint16 inside object dictionary
func (entry *Entry) Uint16(subIndex uint8, data *uint16) error {
	b := make([]byte, 2)
	err := entry.readSubExactly(subIndex, b, true)
	if err != nil {
		return err
	}
	*data = binary.LittleEndian.Uint16(b)
	return nil
}

// Read Uint32 inside object dictionary
func (entry *Entry) Uint32(subIndex uint8, data *uint32) error {
	b := make([]byte, 4)
	err := entry.readSubExactly(subIndex, b, true)
	if err != nil {
		return err
	}
	*data = binary.LittleEndian.Uint32(b)
	return nil
}

// Read Uint64 inside object dictionary
func (entry *Entry) Uint64(subIndex uint8, data *uint64) error {
	b := make([]byte, 8)
	err := entry.readSubExactly(subIndex, b, true)
	if err != nil {
		return err
	}
	*data = binary.LittleEndian.Uint64(b)
	return nil
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
