package od

import (
	"archive/zip"
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unsafe"
)

// Parser is the signature of an EDS parser, see [Parse]
type Parser func(file any, nodeId uint8) (*ObjectDictionary, error)

// Parse parses an EDS or DCF file (CiA 306 INI format) into an [ObjectDictionary].
//
// file can be either :
//   - a string : path to the file
//   - a []byte : the file content (it is copied, the caller keeps ownership)
//   - an [io.Reader] (e.g. *os.File) : read until EOF
//
// nodeId is used to resolve "$NODEID" expressions inside of values
// e.g. DefaultValue=$NODEID+0x180
//
// Sections may appear in any order, keys and the "sub" keyword
// are case insensitive, and ';' / '#' comments are ignored.
func Parse(file any, nodeId uint8) (*ObjectDictionary, error) {
	var data []byte
	var err error
	switch f := file.(type) {
	case string:
		data, err = os.ReadFile(f)
	case []byte:
		// The OD keeps a reference on the raw file (e.g. for 0x1021 EDS storage)
		// and parsed strings point into it, so it must not be mutated by the caller.
		data = bytes.Clone(f)
	case io.Reader:
		data, err = io.ReadAll(f)
	default:
		return nil, fmt.Errorf("[OD] unsupported EDS source type %T", file)
	}
	if err != nil {
		return nil, err
	}
	return parse(data, nodeId)
}

// ParseV2 is kept for backward compatibility, it calls [Parse].
//
// Deprecated: the v1 (ini based) and v2 parsers have been merged into
// a single implementation, use [Parse].
func ParseV2(file any, nodeId uint8) (*ObjectDictionary, error) {
	return Parse(file, nodeId)
}

// [EDSFormatHandler] takes a formatType, nodeId and a reader
// to handle an EDS file stored as a proprietary format (zip, etc)
type EDSFormatHandler func(nodeId uint8, formatType uint8, reader io.Reader) (*ObjectDictionary, error)

// Default EDS format handler used by this library
// This can be used as a template to add other format handlers
func DefaultEDSFormatHandler(nodeId uint8, formatType uint8, reader io.Reader) (*ObjectDictionary, error) {

	switch formatType {

	case FormatEDSAscii:
		raw, err := io.ReadAll(reader)
		if err != nil {
			return nil, err
		}
		return parse(raw, nodeId)

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
		defer func() { _ = r.Close() }()
		uncompressed, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		return parse(uncompressed, nodeId)

	default:
		return nil, ErrEdsFormat
	}
}

func NewOD() *ObjectDictionary {
	return newODWithCapacity(0)
}

func newODWithCapacity(capacity int) *ObjectDictionary {
	return &ObjectDictionary{
		logger:              _logger.With("service", "[OD]"),
		entriesByIndexValue: make(map[uint16]*Entry, capacity),
		entriesByIndexName:  make(map[string]*Entry, capacity),
	}
}

type sectionKind uint8

const (
	sectionOther sectionKind = iota // [FileInfo], [DeviceInfo], [1018Name], ...
	sectionIndex                    // [1018]
	sectionSub                      // [1018sub1]
)

// Raw key values of an index or sub-index section.
// Strings point into the parser buffer, nothing is copied.
type edsSection struct {
	kind          sectionKind
	line          int
	header        string
	index         uint16
	subIndex      uint8
	parameterName string
	objectType    string
	subNumber     string
	compactSubObj string
	dataType      string
	accessType    string
	defaultValue  string
	pdoMapping    string
	lowLimit      string
	highLimit     string
}

// A sub-index section whose parent index was not yet defined
type pendingSub struct {
	section  edsSection
	variable *Variable
}

type parser struct {
	od      *ObjectDictionary
	nodeId  uint8
	entries []Entry    // slab, avoids one allocation per entry
	vars    []Variable // slab, avoids one allocation per variable
	arena   []byte     // slab for variable values
	pending []pendingSub
}

// parse takes ownership of data : it is stored as the raw OD
// and all parsed strings (names) reference it.
func parse(data []byte, nodeId uint8) (*ObjectDictionary, error) {
	// Every object section is at least one Entry or Variable, use this
	// to size everything upfront. Single byte bytes.Count is vectorized.
	nbSections := bytes.Count(data, []byte{'['}) + 1

	p := &parser{
		od:     newODWithCapacity(nbSections / 2),
		nodeId: nodeId,
		vars:   make([]Variable, nbSections),
		arena:  make([]byte, 0, nbSections*16),
	}
	p.od.rawOd = data

	// Skip UTF-8 BOM
	data = bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF"))

	cur := edsSection{}
	lineNb := 0

	for len(data) > 0 {
		var line []byte
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			line, data = data[:i], data[i+1:]
		} else {
			line, data = data, nil
		}
		lineNb++

		line = trimSpaces(line)
		if len(line) == 0 {
			continue
		}

		switch line[0] {
		case ';', '#':
			continue
		case '[':
			if err := p.finishSection(&cur); err != nil {
				return nil, err
			}
			end := bytes.IndexByte(line, ']')
			if end < 0 {
				return nil, fmt.Errorf("[OD] line %d: unterminated section header %q", lineNb, line)
			}
			cur = edsSection{line: lineNb}
			name := trimSpaces(line[1:end])
			cur.header = b2s(name)
			cur.kind, cur.index, cur.subIndex = classifySection(name)
			continue
		}

		// Keys of non object sections are not needed
		if cur.kind == sectionOther {
			continue
		}
		// Keys are short, a simple loop is faster than bytes.IndexByte
		eq := 0
		for eq < len(line) && line[eq] != '=' {
			eq++
		}
		if eq == len(line) {
			continue
		}
		// line is already trimmed
		key, value := line[:eq], line[eq+1:]
		for len(key) > 0 && (key[len(key)-1] == ' ' || key[len(key)-1] == '\t') {
			key = key[:len(key)-1]
		}
		for len(value) > 0 && (value[0] == ' ' || value[0] == '\t') {
			value = value[1:]
		}
		cur.set(key, b2s(value))
	}

	if err := p.finishSection(&cur); err != nil {
		return nil, err
	}

	// Attach sub-indexes that were defined before their parent index
	for i := range p.pending {
		ps := &p.pending[i]
		entry := p.od.entriesByIndexValue[ps.section.index]
		if entry == nil {
			return nil, ps.section.errorf("no parent section [%04X]", ps.section.index)
		}
		if err := attachSub(entry, &ps.section, ps.variable); err != nil {
			return nil, err
		}
	}

	return p.od, nil
}

// Store the value of a known key
func (s *edsSection) set(key []byte, value string) {
	// Fast path for the usual spelling, no allocation for string(key) here
	switch string(key) {
	case "ParameterName":
		s.parameterName = value
	case "ObjectType":
		s.objectType = value
	case "DataType":
		s.dataType = value
	case "AccessType":
		s.accessType = value
	case "DefaultValue":
		s.defaultValue = value
	case "PDOMapping":
		s.pdoMapping = value
	case "SubNumber":
		s.subNumber = value
	case "LowLimit":
		s.lowLimit = value
	case "HighLimit":
		s.highLimit = value
	case "CompactSubObj":
		s.compactSubObj = value
	default:
		s.setFold(key, value)
	}
}

// Keys are case insensitive
func (s *edsSection) setFold(key []byte, value string) {
	switch len(key) {
	case 8:
		if equalFold(key, "DataType") {
			s.dataType = value
		} else if equalFold(key, "LowLimit") {
			s.lowLimit = value
		}
	case 9:
		if equalFold(key, "SubNumber") {
			s.subNumber = value
		} else if equalFold(key, "HighLimit") {
			s.highLimit = value
		}
	case 10:
		if equalFold(key, "ObjectType") {
			s.objectType = value
		} else if equalFold(key, "AccessType") {
			s.accessType = value
		} else if equalFold(key, "PDOMapping") {
			s.pdoMapping = value
		}
	case 12:
		if equalFold(key, "DefaultValue") {
			s.defaultValue = value
		}
	case 13:
		if equalFold(key, "ParameterName") {
			s.parameterName = value
		} else if equalFold(key, "CompactSubObj") {
			s.compactSubObj = value
		}
	}
}

func (s *edsSection) errorf(format string, args ...any) error {
	return fmt.Errorf("[OD] line %d [%s]: %s", s.line, s.header, fmt.Sprintf(format, args...))
}

// Classify a section name i.e. "1018" or "1018sub1" or anything else
func classifySection(name []byte) (sectionKind, uint16, uint8) {
	if len(name) < 4 {
		return sectionOther, 0, 0
	}
	index, ok := parseHex(name[:4])
	if !ok {
		return sectionOther, 0, 0
	}
	if len(name) == 4 {
		return sectionIndex, uint16(index), 0
	}
	// Sub-index is at most 0xFF, i.e. 1 or 2 hex digits
	if len(name) < 8 || len(name) > 9 || !equalFold(name[4:7], "sub") {
		return sectionOther, 0, 0
	}
	subIndex, ok := parseHex(name[7:])
	if !ok {
		return sectionOther, 0, 0
	}
	return sectionSub, uint16(index), uint8(subIndex)
}

func (p *parser) finishSection(s *edsSection) error {
	switch s.kind {
	case sectionIndex:
		return p.finishIndex(s)
	case sectionSub:
		return p.finishSub(s)
	}
	return nil
}

func (p *parser) finishIndex(s *edsSection) error {
	if _, exists := p.od.entriesByIndexValue[s.index]; exists {
		return s.errorf("duplicate section")
	}

	objectType := ObjectTypeVAR // CiA 306 default
	if s.objectType != "" {
		ot, err := parseUintField(s.objectType, 8)
		if err != nil {
			return s.errorf("invalid ObjectType %q", s.objectType)
		}
		objectType = uint8(ot)
	}

	entry := p.newEntry()
	entry.logger = p.od.logger
	entry.Index = s.index
	entry.Name = s.parameterName
	entry.ObjectType = objectType

	switch objectType {
	case ObjectTypeVAR, ObjectTypeDOMAIN:
		variable, err := p.newVariable(s, 0)
		if err != nil {
			return err
		}
		entry.object = variable

	case ObjectTypeARRAY:
		if compact, _ := parseUintField(s.compactSubObj, 8); compact != 0 {
			return s.errorf("CompactSubObj is not supported")
		}
		// Array objects do not allow holes in sub-index numbers
		subNumber, err := parseUintField(s.subNumber, 8)
		if err != nil {
			return s.errorf("invalid or missing SubNumber %q", s.subNumber)
		}
		entry.object = &VariableList{
			objectType:        ObjectTypeARRAY,
			Variables:         make([]*Variable, subNumber),
			subEntriesNameMap: make(map[string]uint8, subNumber),
		}

	case ObjectTypeRECORD:
		// Record objects allow holes in mapping, sub-objects are appended
		// SubNumber is only used as a capacity hint
		subNumber, _ := parseUintField(s.subNumber, 8)
		entry.object = &VariableList{
			objectType:        ObjectTypeRECORD,
			Variables:         make([]*Variable, 0, subNumber),
			subEntriesNameMap: make(map[string]uint8, subNumber),
		}

	default:
		return s.errorf("unknown ObjectType 0x%x", objectType)
	}

	p.od.entriesByIndexValue[entry.Index] = entry
	p.od.entriesByIndexName[entry.Name] = entry
	return nil
}

func (p *parser) finishSub(s *edsSection) error {
	variable, err := p.newVariable(s, s.subIndex)
	if err != nil {
		return err
	}
	entry := p.od.entriesByIndexValue[s.index]
	if entry == nil {
		// Parent not defined yet, attach at the end
		p.pending = append(p.pending, pendingSub{section: *s, variable: variable})
		return nil
	}
	return attachSub(entry, s, variable)
}

func attachSub(entry *Entry, s *edsSection, variable *Variable) error {
	list, ok := entry.object.(*VariableList)
	if !ok {
		return s.errorf("parent [%04X] is not an ARRAY or a RECORD", s.index)
	}
	switch list.objectType {
	case ObjectTypeARRAY:
		if int(s.subIndex) >= len(list.Variables) {
			return s.errorf("sub-index 0x%x out of range, SubNumber is %d", s.subIndex, len(list.Variables))
		}
		if list.Variables[s.subIndex] != nil {
			return s.errorf("duplicate section")
		}
		list.Variables[s.subIndex] = variable
	default:
		for _, v := range list.Variables {
			if v.SubIndex == s.subIndex {
				return s.errorf("duplicate section")
			}
		}
		list.Variables = append(list.Variables, variable)
	}
	list.subEntriesNameMap[variable.Name] = s.subIndex
	return nil
}

func (p *parser) newEntry() *Entry {
	if len(p.entries) == 0 {
		p.entries = make([]Entry, 64)
	}
	entry := &p.entries[0]
	p.entries = p.entries[1:]
	return entry
}

// Allocate n bytes from arena, capacity is clamped so that
// an append can never overwrite a neighbouring value.
func (p *parser) alloc(n int) []byte {
	if n > cap(p.arena)-len(p.arena) {
		p.arena = make([]byte, 0, max(n, 4096))
	}
	start := len(p.arena)
	p.arena = p.arena[:start+n]
	return p.arena[start : start+n : start+n]
}

// Create a variable from the section VAR definition
func (p *parser) newVariable(s *edsSection, subIndex uint8) (*Variable, error) {
	if s.dataType == "" {
		return nil, s.errorf("missing DataType")
	}
	dt, err := parseUintField(s.dataType, 8)
	if err != nil {
		return nil, s.errorf("invalid DataType %q", s.dataType)
	}
	dataType := uint8(dt)

	// CiA 306 defaults to not mappable, but files exported by older versions
	// of this library have no PDOMapping key at all and rely on the
	// historical od.Parse behaviour : missing means mappable.
	pdoMapping := true
	if s.pdoMapping != "" {
		pdoMapping, err = parseBool(s.pdoMapping)
		if err != nil {
			return nil, s.errorf("invalid PDOMapping %q", s.pdoMapping)
		}
	}

	if len(p.vars) == 0 {
		p.vars = make([]Variable, 64)
	}
	variable := &p.vars[0]
	p.vars = p.vars[1:]

	variable.Name = s.parameterName
	variable.SubIndex = subIndex
	variable.DataType = dataType
	variable.Attribute = EncodeAttribute(s.accessType, pdoMapping, dataType)

	value, offset := cleanValue(s.defaultValue, dataType, p.nodeId)
	if dataType == OCTET_STRING {
		// Octet strings are written as hexadecimal e.g. "0102AB" or "01 02 AB"
		if octets, ok := decodeOctetString(value); ok {
			buf := p.alloc(2 * len(octets))
			variable.value = buf[:len(octets):len(octets)]
			variable.valueDefault = buf[len(octets):]
			copy(variable.value, octets)
			copy(variable.valueDefault, octets)
			return variable, nil
		}
	}
	size, err := encodedSize(value, dataType)
	if err != nil {
		return nil, s.errorf("unsupported DataType 0x%x", dataType)
	}
	// value & default value share a single allocation
	buf := p.alloc(2 * size)
	variable.value = buf[:size:size]
	variable.valueDefault = buf[size:]
	if err := encodeInto(variable.value, value, dataType, offset); err != nil {
		return nil, s.errorf("invalid DefaultValue %q for DataType 0x%x : %v", s.defaultValue, dataType, err)
	}
	copy(variable.valueDefault, variable.value)

	// Limits are informative, a bad one is not fatal
	if s.lowLimit != "" {
		variable.lowLimit = p.encodeLimit(s, s.lowLimit, dataType, "LowLimit")
	}
	if s.highLimit != "" {
		variable.highLimit = p.encodeLimit(s, s.highLimit, dataType, "HighLimit")
	}
	return variable, nil
}

func (p *parser) encodeLimit(s *edsSection, limit string, dataType uint8, key string) []byte {
	value, offset := cleanValue(limit, dataType, p.nodeId)
	size, err := encodedSize(value, dataType)
	if err == nil {
		buf := p.alloc(size)
		if err = encodeInto(buf, value, dataType, offset); err == nil {
			return buf
		}
	}
	p.od.logger.Warn("error parsing "+key,
		"index", fmt.Sprintf("x%x", s.index),
		"subindex", fmt.Sprintf("x%x", s.subIndex),
		"error", err,
	)
	return nil
}

// cleanValue prepares a raw EDS value for encoding : resolves $NODEID,
// removes trailing comments of numeric values and quotes of strings.
// It returns the value to encode and the offset to add to it.
func cleanValue(value string, dataType uint8, nodeId uint8) (string, uint8) {
	switch dataType {
	case VISIBLE_STRING, OCTET_STRING, UNICODE_STRING, DOMAIN:
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = value[1 : len(value)-1]
		}
		return value, 0
	}
	value = cutComment(value, false)
	if strings.IndexByte(value, '$') < 0 {
		return value, 0
	}
	stripped, found := stripNodeId(value)
	if !found {
		return value, 0
	}
	return stripped, nodeId
}

// Decode an hexadecimal octet string, spaces between bytes are allowed.
// Returns false if value is empty or not valid hexadecimal, in which
// case the raw characters are used.
func decodeOctetString(value string) ([]byte, bool) {
	if strings.ContainsAny(value, " \t") {
		value = strings.Map(func(r rune) rune {
			if r == ' ' || r == '\t' {
				return -1
			}
			return r
		}, value)
	}
	if value == "" {
		return nil, false
	}
	octets, err := hex.DecodeString(value)
	return octets, err == nil
}

// Remove "$NODEID" and its "+" operator from an expression,
// e.g. "$NODEID+0x180" or "0x180 + $NODEID" both give "0x180"
func stripNodeId(value string) (string, bool) {
	const token = "$NODEID"
	i := indexFold(value, token)
	if i < 0 {
		return value, false
	}
	before := strings.TrimRight(value[:i], " \t")
	after := strings.TrimLeft(value[i+len(token):], " \t")
	if strings.HasSuffix(before, "+") {
		before = strings.TrimRight(before[:len(before)-1], " \t")
	} else if strings.HasPrefix(after, "+") {
		after = strings.TrimLeft(after[1:], " \t")
	}
	return before + after, true
}

func indexFold(s string, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i] == substr[0] && strings.EqualFold(s[i:i+len(substr)], substr) {
			return i
		}
	}
	return -1
}

// Parse a numeric field like ObjectType, DataType or SubNumber
// with CiA 306 notation (decimal, 0x hex or leading 0 octal)
func parseUintField(value string, bitSize int) (uint64, error) {
	return strconv.ParseUint(cutComment(value, true), 0, bitSize)
}

func parseBool(value string) (bool, error) {
	switch value = cutComment(value, true); value {
	case "1", "0":
		return value == "1", nil
	}
	switch strings.ToLower(value) {
	case "0x1", "t", "true", "y", "yes", "on":
		return true, nil
	case "0x0", "f", "false", "n", "no", "off":
		return false, nil
	}
	return false, strconv.ErrSyntax
}

// Remove a trailing ';' or '#' comment. If atSpace is set, the
// value also ends at the first space (single token values).
// Values are short, a simple loop is faster than strings.IndexAny
func cutComment(value string, atSpace bool) string {
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case ';', '#':
			return strings.TrimRight(value[:i], " \t")
		case ' ', '\t':
			if atSpace {
				return value[:i]
			}
		}
	}
	return value
}

// Remove ' ', '\t' and '\r' characters at beginning and end of line
func trimSpaces(b []byte) []byte {
	if len(b) == 0 || (b[0] > ' ' && b[len(b)-1] > ' ') {
		return b
	}
	start, end := 0, len(b)
	for start < end && (b[start] == ' ' || b[start] == '\t' || b[start] == '\r') {
		start++
	}
	for end > start && (b[end-1] == ' ' || b[end-1] == '\t' || b[end-1] == '\r') {
		end--
	}
	return b[start:end]
}

// ASCII case insensitive comparison, s must be lowercase or mixed case ASCII
func equalFold(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}
	for i := 0; i < len(b); i++ {
		c1, c2 := b[i], s[i]
		if c1 == c2 {
			continue
		}
		if c1|0x20 != c2|0x20 || c1|0x20 < 'a' || c1|0x20 > 'z' {
			return false
		}
	}
	return true
}

// Parse up to 4 hexadecimal ascii digits
func parseHex(b []byte) (uint32, bool) {
	if len(b) == 0 || len(b) > 4 {
		return 0, false
	}
	var num uint32
	for _, c := range b {
		var digit byte
		switch {
		case c >= '0' && c <= '9':
			digit = c - '0'
		case c >= 'A' && c <= 'F':
			digit = c - 'A' + 10
		case c >= 'a' && c <= 'f':
			digit = c - 'a' + 10
		default:
			return 0, false
		}
		num = num<<4 | uint32(digit)
	}
	return num, true
}

// Zero copy conversion, b must never be modified afterwards.
// The parser only uses it on its own buffer, which is immutable.
func b2s(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}
