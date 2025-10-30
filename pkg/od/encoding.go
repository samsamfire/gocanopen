package od

import (
	"encoding/binary"
	"math"
	"strconv"
)

// EncodeFromString value from EDS into bytes respecting canopen datatype
func EncodeFromString(value string, datatype uint8, offset uint8) ([]byte, error) {

	var data []byte
	var err error
	var parsedInt int64
	var parsedUint uint64

	if value == "" {
		// Treat empty string as a 0 value
		value = "0"
	}

	switch datatype {
	case BOOLEAN, UNSIGNED8:
		parsedUint, err = strconv.ParseUint(value, 0, 8)
		data = []byte{byte(uint8(parsedUint + uint64(offset)))}

	case INTEGER8:
		parsedInt, err = strconv.ParseInt(value, 0, 8)
		data = []byte{byte(parsedInt + int64(offset))}

	case UNSIGNED16:
		parsedUint, err = strconv.ParseUint(value, 0, 16)
		data = make([]byte, 2)
		binary.LittleEndian.PutUint16(data, uint16(parsedUint+uint64(offset)))

	case INTEGER16:
		parsedInt, err = strconv.ParseInt(value, 0, 16)
		data = make([]byte, 2)
		binary.LittleEndian.PutUint16(data, uint16(parsedInt+int64(offset)))

	case UNSIGNED32:
		parsedUint, err = strconv.ParseUint(value, 0, 32)
		data = make([]byte, 4)
		binary.LittleEndian.PutUint32(data, uint32(parsedUint+uint64(offset)))

	case INTEGER32:
		parsedInt, err = strconv.ParseInt(value, 0, 32)
		data = make([]byte, 4)
		binary.LittleEndian.PutUint32(data, uint32(parsedInt+int64(offset)))

	case REAL32:
		var parsedFloat float64
		parsedFloat, err = strconv.ParseFloat(value, 32)
		data = make([]byte, 4)
		binary.LittleEndian.PutUint32(data, math.Float32bits(float32(parsedFloat)))

	case UNSIGNED64:
		parsedUint, err = strconv.ParseUint(value, 0, 64)
		data = make([]byte, 8)
		binary.LittleEndian.PutUint64(data, parsedUint+uint64(offset))

	case INTEGER64:
		parsedInt, err = strconv.ParseInt(value, 0, 64)
		data = make([]byte, 8)
		binary.LittleEndian.PutUint64(data, uint64(parsedInt+int64(offset)))

	case REAL64:
		var parsedFloat float64
		parsedFloat, err = strconv.ParseFloat(value, 64)
		data = make([]byte, 8)
		binary.LittleEndian.PutUint64(data, math.Float64bits(parsedFloat))

	case VISIBLE_STRING, OCTET_STRING:
		return []byte(value), nil

	case DOMAIN:
		return []byte{}, nil

	default:
		return nil, ErrTypeMismatch

	}
	return data, err
}

// Encode from generic type
func EncodeFromTypeExact(data any) ([]byte, error) {
	var encoded []byte
	switch val := data.(type) {
	case uint8:
		encoded = []byte{val}
	case int8:
		encoded = []byte{byte(val)}
	case uint16:
		encoded = make([]byte, 2)
		binary.LittleEndian.PutUint16(encoded, val)
	case int16:
		encoded = make([]byte, 2)
		binary.LittleEndian.PutUint16(encoded, uint16(val))
	case uint32:
		encoded = make([]byte, 4)
		binary.LittleEndian.PutUint32(encoded, val)
	case int32:
		encoded = make([]byte, 4)
		binary.LittleEndian.PutUint32(encoded, uint32(val))
	case uint64:
		encoded = make([]byte, 8)
		binary.LittleEndian.PutUint64(encoded, val)
	case int64:
		encoded = make([]byte, 8)
		binary.LittleEndian.PutUint64(encoded, uint64(val))
	case string:
		encoded = []byte(val)
	case float32:
		encoded = make([]byte, 4)
		binary.LittleEndian.PutUint32(encoded, math.Float32bits(val))
	case float64:
		encoded = make([]byte, 8)
		binary.LittleEndian.PutUint64(encoded, math.Float64bits(val))
	case []byte:
		encoded = val
	default:
		return nil, ErrTypeMismatch
	}
	return encoded, nil
}

func EncodeFromTypeExactToBuffer(data any, dataType uint8, buf []byte) error {

	switch val := data.(type) {
	case bool:
		if dataType != BOOLEAN {
			return ErrTypeMismatch
		}
		if val {
			buf[0] = 1
		} else {
			buf[0] = 0
		}
	case uint8:
		if dataType != UNSIGNED8 {
			return ErrTypeMismatch
		}
		buf[0] = val
	case int8:
		if dataType != INTEGER8 {
			return ErrTypeMismatch
		}
		buf[0] = byte(val)
	case uint16:
		if dataType != UNSIGNED16 {
			return ErrTypeMismatch
		}
		binary.LittleEndian.PutUint16(buf, val)
	case int16:
		if dataType != INTEGER16 {
			return ErrTypeMismatch
		}
		binary.LittleEndian.PutUint16(buf, uint16(val))
	case uint32:
		if dataType != UNSIGNED32 {
			return ErrTypeMismatch
		}
		binary.LittleEndian.PutUint32(buf, val)
	case int32:
		if dataType != INTEGER32 {
			return ErrTypeMismatch
		}
		binary.LittleEndian.PutUint32(buf, uint32(val))
	case uint64:
		if dataType != UNSIGNED64 {
			return ErrTypeMismatch
		}
		binary.LittleEndian.PutUint64(buf, val)
	case int64:
		if dataType != INTEGER64 {
			return ErrTypeMismatch
		}
		binary.LittleEndian.PutUint64(buf, uint64(val))
	case float32:
		if dataType != REAL32 {
			return ErrTypeMismatch
		}
		binary.LittleEndian.PutUint32(buf, math.Float32bits(val))
	case float64:
		if dataType != REAL64 {
			return ErrTypeMismatch
		}
		binary.LittleEndian.PutUint64(buf, math.Float64bits(val))
	case string:
		if dataType != VISIBLE_STRING {
			return ErrTypeMismatch
		}
		if len(val) > len(buf) {
			return ErrDataLong
		}
		clear(buf)
		copy(buf, []byte(val))
	case []byte:
		if len(val) > len(buf) {
			return ErrDataLong
		}
		clear(buf)
		copy(buf, val)
	default:
		return ErrTypeMismatch
	}
	return nil
}

func EncodeFromType(data any) ([]byte, error) {
	var encoded []byte
	switch val := data.(type) {
	case uint8:
		encoded = []byte{val}
	case int8:
		encoded = []byte{byte(val)}
	case uint16:
		encoded = make([]byte, 2)
		binary.LittleEndian.PutUint16(encoded, val)
	case int16:
		encoded = make([]byte, 2)
		binary.LittleEndian.PutUint16(encoded, uint16(val))
	case uint32:
		encoded = make([]byte, 4)
		binary.LittleEndian.PutUint32(encoded, val)
	case int32:
		encoded = make([]byte, 4)
		binary.LittleEndian.PutUint32(encoded, uint32(val))
	case uint64:
		encoded = make([]byte, 8)
		binary.LittleEndian.PutUint64(encoded, val)
	case int64:
		encoded = make([]byte, 8)
		binary.LittleEndian.PutUint64(encoded, uint64(val))
	case string:
		encoded = []byte(val)
	case float32:
		encoded = make([]byte, 4)
		binary.LittleEndian.PutUint32(encoded, math.Float32bits(val))
	case float64:
		encoded = make([]byte, 8)
		binary.LittleEndian.PutUint64(encoded, math.Float64bits(val))
	case []byte:
		encoded = val
	default:
		return nil, ErrTypeMismatch
	}
	return encoded, nil
}

// Helper function for checking consistency between size and datatype
func CheckSize(length int, dataType uint8) error {
	switch dataType {
	case BOOLEAN, UNSIGNED8, INTEGER8:
		if length < 1 {
			return ErrDataShort
		} else if length > 1 {
			return ErrDataLong
		}
	case UNSIGNED16, INTEGER16:
		if length < 2 {
			return ErrDataShort
		} else if length > 2 {
			return ErrDataLong
		}

	case UNSIGNED32, INTEGER32, REAL32:
		if length < 4 {
			return ErrDataShort
		} else if length > 4 {
			return ErrDataLong
		}
	case UNSIGNED64, INTEGER64, REAL64:
		if length < 8 {
			return ErrDataShort
		} else if length > 8 {
			return ErrDataLong
		}
	// All other datatypes, no size check
	default:
		return nil
	}
	return nil

}

// // Helper function to check that concrete type of data and dataType
// // are consistent
// func CheckDatatype(data any, dataType byte) error {
// 	switch data.(type) {
// 	case uint8:
// 		if dataType != UNSIGNED8 {
// 			return ErrTypeMismatch
// 		}
// 	case uint16:
// 		if dataType != UNSIGNED16 {
// 			return ErrTypeMismatch
// 		}
// 	case uint32:
// 		if dataType != UNSIGNED32 {
// 			return ErrTypeMismatch
// 		}
// 	case uint64:
// 		if dataType != UNSIGNED64 {
// 			return ErrTypeMismatch
// 		}
// 	case int8:
// 		if dataType != INTEGER8 {
// 			return ErrTypeMismatch
// 		}
// 	case int16:
// 		if dataType != INTEGER16 {
// 			return ErrTypeMismatch
// 		}
// 	case int32:
// 		if dataType != INTEGER32 {
// 			return ErrTypeMismatch
// 		}
// 	case int64:
// 		if dataType != INTEGER64 {
// 			return ErrTypeMismatch
// 		}
// 	case float32:
// 		if dataType != REAL32 {
// 			return ErrTypeMismatch
// 		}
// 	case float64:
// 		if dataType != REAL64 {
// 			return ErrTypeMismatch
// 		}
// 	case string:
// 		if dataType != UNICODE_STRING {
// 			return ErrTypeMismatch
// 		}
// 	case []byte:
// 		if dataType != OCTET_STRING {
// 			return ErrTypeMismatch
// 		}
// 	default:
// 		return ErrTypeMismatch
// 	}
// 	return nil
// }

// Decode byte array given the CANopen data type
// Function will return either string, int64, uint64, or float64
func DecodeToType(data []byte, dataType uint8) (v any, e error) {
	e = CheckSize(len(data), dataType)
	if e != nil {
		return nil, e
	}
	// Cast to correct type
	switch dataType {
	case BOOLEAN, UNSIGNED8:
		return uint64(data[0]), nil
	case INTEGER8:
		return int64(data[0]), nil
	case UNSIGNED16:
		return uint64(binary.LittleEndian.Uint16(data)), nil
	case INTEGER16:
		return int64(int16(binary.LittleEndian.Uint16(data))), nil
	case UNSIGNED32:
		return uint64(binary.LittleEndian.Uint32(data)), nil
	case INTEGER32:
		return int64(int32(binary.LittleEndian.Uint32(data))), nil
	case UNSIGNED64:
		return uint64(binary.LittleEndian.Uint64(data)), nil
	case INTEGER64:
		return int64(binary.LittleEndian.Uint64(data)), nil
	case REAL32:
		parsed := binary.LittleEndian.Uint32(data)
		return float64(math.Float32frombits(parsed)), nil
	case REAL64:
		parsed := binary.LittleEndian.Uint64(data)
		return math.Float64frombits(parsed), nil
	case VISIBLE_STRING, OCTET_STRING:
		return string(data), nil
	case DOMAIN:
		return int64(0), nil
	default:
		return nil, ErrTypeMismatch
	}
}

// Decode byte array given the CANopen data type
// Function will return the exact type (uint8,uint16,...,int8,...)
func DecodeToTypeExact(data []byte, dataType uint8) (v any, e error) {
	e = CheckSize(len(data), dataType)
	if e != nil {
		return nil, e
	}
	// Cast to correct type
	switch dataType {
	case BOOLEAN, UNSIGNED8:
		return data[0], nil
	case INTEGER8:
		return int8(data[0]), nil
	case UNSIGNED16:
		return binary.LittleEndian.Uint16(data), nil
	case INTEGER16:
		return int16(binary.LittleEndian.Uint16(data)), nil
	case UNSIGNED32:
		return binary.LittleEndian.Uint32(data), nil
	case INTEGER32:
		return int32(binary.LittleEndian.Uint32(data)), nil
	case UNSIGNED64:
		return binary.LittleEndian.Uint64(data), nil
	case INTEGER64:
		return int64(binary.LittleEndian.Uint64(data)), nil
	case REAL32:
		parsed := binary.LittleEndian.Uint32(data)
		return math.Float32frombits(parsed), nil
	case REAL64:
		parsed := binary.LittleEndian.Uint64(data)
		return math.Float64frombits(parsed), nil
	case VISIBLE_STRING, OCTET_STRING:
		return string(data), nil
	case DOMAIN:
		return int64(0), nil
	default:
		return nil, ErrTypeMismatch
	}
}

// Decode byte array given the CANopen data type
// Function will return either string, int64, uint64, or float64
func DecodeToString(data []byte, dataType uint8, base int) (v string, e error) {
	e = CheckSize(len(data), dataType)
	if e != nil {
		return "", e
	}
	// Cast to correct type
	switch dataType {
	case BOOLEAN, UNSIGNED8:
		return strconv.FormatUint(uint64(data[0]), base), nil
	case INTEGER8:
		return strconv.FormatInt(int64(data[0]), base), nil
	case UNSIGNED16:
		return strconv.FormatUint(uint64(binary.LittleEndian.Uint16(data)), base), nil
	case INTEGER16:
		return strconv.FormatInt(int64(int16(binary.LittleEndian.Uint16(data))), base), nil
	case UNSIGNED32:
		return strconv.FormatUint(uint64(binary.LittleEndian.Uint32(data)), base), nil
	case INTEGER32:
		return strconv.FormatInt(int64(int32(binary.LittleEndian.Uint32(data))), base), nil
	case UNSIGNED64:
		return strconv.FormatUint(uint64(binary.LittleEndian.Uint64(data)), base), nil
	case INTEGER64:
		return strconv.FormatInt(int64(binary.LittleEndian.Uint64(data)), base), nil
	case REAL32:
		parsed := binary.LittleEndian.Uint32(data)
		return strconv.FormatFloat(float64(math.Float32frombits(parsed)), 'f', -1, 64), nil
	case REAL64:
		parsed := binary.LittleEndian.Uint64(data)
		return strconv.FormatFloat(math.Float64frombits(parsed), 'f', -1, 64), nil
	case VISIBLE_STRING, OCTET_STRING:
		return string(data), nil
	case DOMAIN:
		return "0", nil
	default:
		return "", ErrTypeMismatch
	}
}

// Decode the attribute in function of the of attribute type and pdo mapping for EDS entry
func EncodeAttribute(accessType string, pdoMapping bool, dataType uint8) uint8 {

	var attribute uint8

	switch accessType {
	case "rw":
		attribute = AttributeSdoRw
	case "ro", "const":
		attribute = AttributeSdoR
	case "wo":
		attribute = AttributeSdoW
	default:
		attribute = AttributeSdoRw
	}
	if pdoMapping {
		attribute |= AttributeTrpdo
	}
	if dataType == VISIBLE_STRING || dataType == OCTET_STRING {
		attribute |= AttributeStr
	}
	return attribute
}

// Encode attribute
func DecodeAttribute(attribute uint8) string {
	switch {
	case attribute&AttributeSdoRw > 0:
		return "rw"
	case attribute&AttributeSdoR > 0:
		return "ro"
	case attribute&AttributeSdoW > 0:
		return "wo"
	default:
		return "rw"
	}
}
