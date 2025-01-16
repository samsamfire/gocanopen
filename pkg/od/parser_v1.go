package od

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/ini.v1"
)

type Parser func(file any, nodeId uint8) (*ObjectDictionary, error)

// Parse an EDS file
// file can be either a path or an *os.File or []byte
// Other file types could be supported in the future
func Parse(file any, nodeId uint8) (*ObjectDictionary, error) {
	od := NewOD()
	// Load .ini format
	edsFile, err := ini.Load(file)
	if err != nil {
		return nil, err
	}
	// Automatically export formated .ini inside of internal buffer
	// For reading later on
	// Create a buffer to store the data
	var buf bytes.Buffer

	// Write data from edsFile to the buffer
	// Don't care if fails
	_, _ = edsFile.WriteTo(&buf)
	od.rawOd = buf.Bytes()

	// Get all the sections in the file
	sections := edsFile.Sections()

	// Get index & subindex matching
	matchIdxRegExp := regexp.MustCompile(`^[0-9A-Fa-f]{4}$`)
	matchSubidxRegExp := regexp.MustCompile(`^([0-9A-Fa-f]{4})sub([0-9A-Fa-f]+)$`)

	// Iterate over all the sections
	for _, section := range sections {
		sectionName := section.Name()

		// Match indexes : This adds new entries to the dictionary
		if matchIdxRegExp.MatchString(sectionName) {
			// Add a new entry inside object dictionary
			idx, err := strconv.ParseUint(section.Name(), 16, 16)
			if err != nil {
				return nil, err
			}
			index := uint16(idx)
			name := section.Key("ParameterName").String()
			objType, err := strconv.ParseUint(section.Key("ObjectType").Value(), 0, 8)
			objectType := uint8(objType)

			// If no object type, default to 7 (CiA spec)
			if err != nil {
				objectType = 7
			}

			// objectType determines what type of entry we should add to dictionary : Variable, Array or Record
			switch objectType {
			case ObjectTypeVAR, ObjectTypeDOMAIN:
				variable, err := NewVariableFromSection(section, name, nodeId, index, 0)
				if err != nil {
					return nil, err
				}
				od.addVariable(index, variable)
			case ObjectTypeARRAY:
				// Array objects do not allow holes in subindex numbers
				// So pre-init slice up to subnumber
				subNumber, err := strconv.ParseUint(section.Key("SubNumber").Value(), 0, 8)
				if err != nil {
					return nil, err
				}
				od.AddVariableList(index, name, NewArray(uint8(subNumber)))
			case ObjectTypeRECORD:
				// Record objects allow holes in mapping
				// Sub-objects will be added with "append"
				od.AddVariableList(index, name, NewRecord())
			default:
				return nil, fmt.Errorf("[OD] unknown object type whilst parsing EDS %T", objType)
			}
		}

		// Match subindexes, add the subindex values to Record or Array objects
		if matchSubidxRegExp.MatchString(sectionName) {

			// Index part are the first 4 letters (A subindex entry looks like 5000Sub1)
			idx, err := strconv.ParseUint(sectionName[0:4], 16, 16)
			if err != nil {
				return nil, err
			}
			index := uint16(idx)
			// Subindex part is from the 7th letter onwards
			sidx, err := strconv.ParseUint(sectionName[7:], 16, 8)
			if err != nil {
				return nil, err
			}

			subIndex := uint8(sidx)
			name := section.Key("ParameterName").String()

			entry := od.Index(index)
			if entry == nil {
				return nil, fmt.Errorf("[OD] index with id %d not found", index)
			}
			// Add new subindex entry member
			err = entry.addSectionMember(section, name, nodeId, subIndex)
			if err != nil {
				return nil, err
			}

		}
	}

	return od, nil
}

// [EDSFormatHandler] takes a formatType, nodeId and a reader
// to handle an EDS file stored as a proprietary format (zip, etc)
type EDSFormatHandler func(nodeId uint8, formatType uint8, reader io.Reader) (*ObjectDictionary, error)

// Default EDS format handler used by this library
// This can be used as a template to add other format handlers
func DefaultEDSFormatHandler(nodeId uint8, formatType uint8, reader io.Reader) (*ObjectDictionary, error) {

	switch formatType {

	case FormatEDSAscii:
		return Parse(reader, nodeId)

	case FormatEDSZipped:
		raw, err := io.ReadAll(reader)
		if err != nil {
			return nil, err
		}
		zipped, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			return nil, err
		}
		if len(zipped.File) != 1 {
			return nil, fmt.Errorf("expecting exactly 1 file")
		}
		r, err := zipped.File[0].Open()
		if err != nil {
			return nil, err
		}
		uncompressed, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		return Parse(uncompressed, nodeId)

	default:
		return nil, ErrEdsFormat
	}
}

func NewOD() *ObjectDictionary {
	return &ObjectDictionary{
		logger:              _logger.With("service", "[OD]"),
		entriesByIndexValue: make(map[uint16]*Entry),
		entriesByIndexName:  make(map[string]*Entry),
	}
}

// Create variable from section entry
func NewVariableFromSection(
	section *ini.Section,
	name string,
	nodeId uint8,
	index uint16,
	subindex uint8,
) (*Variable, error) {

	variable := &Variable{
		Name:     name,
		SubIndex: subindex,
	}

	// Get AccessType
	accessType, err := section.GetKey("AccessType")
	if err != nil {
		return nil, fmt.Errorf("failed to get 'AccessType' for %x : %x", index, subindex)
	}

	// Get PDOMapping to know if pdo mappable
	var pdoMapping bool
	if pM, err := section.GetKey("PDOMapping"); err == nil {
		pdoMapping, err = pM.Bool()
		if err != nil {
			return nil, err
		}
	} else {
		pdoMapping = true
	}

	// TODO maybe add support for datatype particularities (>1B)
	dataType, err := strconv.ParseInt(section.Key("DataType").Value(), 0, 8)
	if err != nil {
		return nil, fmt.Errorf("failed to parse 'DataType' for %x : %x, because %v", index, subindex, err)
	}
	variable.DataType = byte(dataType)
	variable.Attribute = EncodeAttribute(accessType.String(), pdoMapping, variable.DataType)

	if highLimit, err := section.GetKey("HighLimit"); err == nil {
		variable.highLimit, err = EncodeFromString(highLimit.Value(), variable.DataType, 0)
		if err != nil {
			_logger.Warn("error parsing HighLimit",
				"index", fmt.Sprintf("x%x", index),
				"subindex", fmt.Sprintf("x%x", subindex),
				"error", err,
			)
		}
	}

	if lowLimit, err := section.GetKey("LowLimit"); err == nil {
		variable.lowLimit, err = EncodeFromString(lowLimit.Value(), variable.DataType, 0)
		if err != nil {
			_logger.Warn("error parsing LowLimit",
				"index", fmt.Sprintf("x%x", index),
				"subindex", fmt.Sprintf("x%x", subindex),
				"error", err,
			)
		}
	}

	if defaultValue, err := section.GetKey("DefaultValue"); err == nil {
		defaultValueStr := defaultValue.Value()
		// If $NODEID is in default value then remove it, and add it afterwards
		if strings.Contains(defaultValueStr, "$NODEID") {
			re := regexp.MustCompile(`\+?\$NODEID\+?`)
			defaultValueStr = re.ReplaceAllString(defaultValueStr, "")
		} else {
			nodeId = 0
		}
		variable.valueDefault, err = EncodeFromString(defaultValueStr, variable.DataType, nodeId)
		if err != nil {
			return nil, fmt.Errorf("failed to parse 'DefaultValue' for x%x|x%x, because %v (datatype :x%x)", index, subindex, err, variable.DataType)
		}
		variable.value = make([]byte, len(variable.valueDefault))
		copy(variable.value, variable.valueDefault)
	}

	return variable, nil
}
