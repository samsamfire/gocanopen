package od

import (
	"archive/zip"
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"

	"gopkg.in/ini.v1"
)

//go:embed base.eds

var f embed.FS
var rawDefaultOd []byte

// Get index & subindex matching
var matchIdxRegExp = regexp.MustCompile(`^[0-9A-Fa-f]{4}$`)
var matchSubidxRegExp = regexp.MustCompile(`^([0-9A-Fa-f]{4})sub([0-9A-Fa-f]+)$`)

// Return embeded default object dictionary
func Default() *ObjectDictionary {
	defaultOd, err := Parse(rawDefaultOd, 0)
	if err != nil {
		panic(err)
	}
	return defaultOd
}

// trimSpaces trims spaces efficiently without new allocations
func trimSpaces(b []byte) []byte {
	start, end := 0, len(b)

	// Trim left space
	for start < end && (b[start] == ' ' || b[start] == '\t') {
		start++
	}
	// Trim right space
	for end > start && (b[end-1] == ' ' || b[end-1] == '\t') {
		end--
	}
	return b[start:end]
}

// Parse an EDS file
// file can be either a path or an *os.File or []byte
// Other file types could be supported in the future
func ParseV2(file any, nodeId uint8) (*ObjectDictionary, error) {

	filename := file.(string)
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var section string

	od := NewOD()

	entry := &Entry{}
	vList := &VariableList{}
	var subindex uint8

	isEntry := false
	isSubEntry := false

	var defaultValue string
	var parameterName string
	var objectType string
	var pdoMapping string
	var lowLimit string
	var highLimit string
	var subNumber string
	var accessType string
	var dataType string

	//  Scan all .ini lines
	for scanner.Scan() {

		line := trimSpaces(scanner.Bytes()) // Read as []byte to reduce allocations

		// Skip empty lines and comments
		if len(line) == 0 || line[0] == ';' || line[0] == '#' {
			continue
		}

		// Handle section headers: [section]
		if line[0] == '[' && line[len(line)-1] == ']' {

			// New section, this means we have finished building
			// Previous one, so take all the values and update the section
			if parameterName != "" && isEntry {
				entry.Name = parameterName
				vList, err = PopulateEntry(
					entry,
					nodeId,
					parameterName,
					defaultValue,
					objectType,
					pdoMapping,
					lowLimit,
					highLimit,
					accessType,
					dataType,
					subNumber,
				)
				if err != nil {
					return nil, fmt.Errorf("failed to create new entry %v", err)
				}
			} else if parameterName != "" && isSubEntry {
				err = PopulateSubEntry(
					entry,
					vList,
					nodeId,
					parameterName,
					defaultValue,
					objectType,
					pdoMapping,
					lowLimit,
					highLimit,
					accessType,
					dataType,
					subindex,
				)
				if err != nil {
					return nil, fmt.Errorf("failed to create sub entry %v", err)
				}
			}

			// Match indexes and not sub indexes
			section = string(line[1 : len(line)-1])

			isEntry = false
			isSubEntry = false

			if matchIdxRegExp.MatchString(section) {

				// Add a new entry inside object dictionary
				idx, err := strconv.ParseUint(section, 16, 16)
				if err != nil {
					return nil, err
				}
				isEntry = true
				entry = &Entry{}
				entry.Index = uint16(idx)
				entry.subEntriesNameMap = map[string]uint8{}
				entry.logger = od.logger.With("index", idx)
				od.entriesByIndexValue[uint16(idx)] = entry

			} else if matchSubidxRegExp.MatchString(section) {
				// Do we need to do smthg ?
				// TODO we could get entry to double check if ever something is out of order
				isSubEntry = true
				// Subindex part is from the 7th letter onwards
				sidx, err := strconv.ParseUint(section[7:], 16, 8)
				if err != nil {
					return nil, err
				}
				subindex = uint8(sidx)
			}

			// Reset all values
			defaultValue = ""
			parameterName = ""
			objectType = ""
			pdoMapping = ""
			lowLimit = ""
			highLimit = ""
			subNumber = ""
			accessType = ""
			dataType = ""

			continue
		}

		// We are in a section so we need to populate the given entry
		// Parse key-value pairs: key = value
		// We will create variables for storing intermediate values
		// Once we are at the end of the section

		if equalsIdx := bytes.IndexByte(line, '='); equalsIdx != -1 {
			key := string(trimSpaces(line[:equalsIdx]))
			value := string(trimSpaces(line[equalsIdx+1:]))

			// We will get the different elements of the entry
			switch key {
			case "ParameterName":
				parameterName = value
			case "ObjectType":
				objectType = value
			case "SubNumber":
				subNumber = value
			case "AccessType":
				accessType = value
			case "DataType":
				dataType = value
			case "LowLimit":
				lowLimit = value
			case "HighLimit":
				highLimit = value
			case "DefaultValue":
				defaultValue = value
			case "PDOMapping":
				pdoMapping = value

			}
		}
	}

	return od, nil
}

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
	reader := bytes.NewReader(buf.Bytes())
	od.Reader = reader
	od.iniFile = edsFile

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

func init() {
	rawDefaultOd, _ = f.ReadFile("base.eds")
}
