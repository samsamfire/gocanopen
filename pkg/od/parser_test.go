package od

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDefault(t *testing.T) {

	od := Default()
	assert.NotNil(t, od)
}

func BenchmarkParser(b *testing.B) {
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		_, err := Parse(rawDefaultOd, 0x10)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestClassifySection(t *testing.T) {
	tests := []struct {
		name     string
		kind     sectionKind
		index    uint16
		subIndex uint8
	}{
		{"1000", sectionIndex, 0x1000, 0},
		{"0A3f", sectionIndex, 0x0A3F, 0},
		{"1A00sub0", sectionSub, 0x1A00, 0},
		{"1a00SUB1f", sectionSub, 0x1A00, 0x1F},
		{"1A00sub100", sectionOther, 0, 0}, // sub-index > 0xFF
		{"1A00sub", sectionOther, 0, 0},
		{"1018Name", sectionOther, 0, 0},
		{"1018Value", sectionOther, 0, 0},
		{"bA3Ei", sectionOther, 0, 0},
		{"FileInfo", sectionOther, 0, 0},
		{"10", sectionOther, 0, 0},
	}
	for _, tc := range tests {
		kind, index, subIndex := classifySection([]byte(tc.name))
		assert.Equal(t, tc.kind, kind, tc.name)
		assert.Equal(t, tc.index, index, tc.name)
		assert.Equal(t, tc.subIndex, subIndex, tc.name)
	}
}

func TestStripNodeId(t *testing.T) {
	tests := map[string]string{
		"$NODEID+0x180":      "0x180",
		"0x180+$NODEID":      "0x180",
		"$NODEID + 0x180":    "0x180",
		"0x180 + $NODEID":    "0x180",
		"$NODEID":            "",
		"$nodeid+0x600":      "0x600",
		"0x80000000+$NODEID": "0x80000000",
	}
	for in, expected := range tests {
		out, found := stripNodeId(in)
		assert.True(t, found, in)
		assert.Equal(t, expected, out, in)
	}
	_, found := stripNodeId("0x180")
	assert.False(t, found)
}

func mustParse(t *testing.T, eds string, nodeId uint8) *ObjectDictionary {
	t.Helper()
	od, err := Parse([]byte(eds), nodeId)
	require.NoError(t, err)
	return od
}

func parseErr(t *testing.T, eds string) error {
	t.Helper()
	_, err := Parse([]byte(eds), 0x10)
	return err
}

func TestParseValues(t *testing.T) {
	eds := `
[1400]
ParameterName=RPDO communication parameter
ObjectType=0x9
SubNumber=0x2

[1400sub0]
ParameterName=Highest sub-index supported
DataType=0x0005
AccessType=ro
DefaultValue=0x01

[1400sub1]
ParameterName=COB-ID used by RPDO
DataType=0x0007
AccessType=rw
DefaultValue=$NODEID+0x200

[2000]
ParameterName=Signed hex
DataType=0x0003
AccessType=rw
DefaultValue=0xFFFF

[2001]
ParameterName=Negative
DataType=0x0002
AccessType=RO
PDOMapping=1
DefaultValue=-5

[2002]
ParameterName=Real from bits
DataType=0x0008
AccessType=rw
DefaultValue=0x3F800000

[2003]
ParameterName=Real
DataType=0x0011
AccessType=rw
DefaultValue=1.5

[2004]
ParameterName=Quoted string
DataType=0x0009
AccessType=const
PDOMapping=0
DefaultValue="hello world"

[2005]
ParameterName=Octal
DataType=0x0005
AccessType=wo
PDOMapping=false
DefaultValue=010

[2006]
ParameterName=Limits
DataType=0x0006
AccessType=rw
LowLimit=0x10
HighLimit=1000
DefaultValue=0x20
`
	od := mustParse(t, eds, 0x10)

	cobId, err := od.Index(0x1400).Uint32(1)
	assert.NoError(t, err)
	assert.EqualValues(t, 0x210, cobId)

	v, _ := od.Index(0x1400).SubIndex("COB-ID used by RPDO")
	assert.NotNil(t, v)
	assert.EqualValues(t, []byte{0x10, 0x02, 0, 0}, v.DefaultValue())

	v, _ = od.Index(0x2000).SubIndex(0)
	i16, err := v.Int16()
	assert.NoError(t, err)
	assert.EqualValues(t, -1, i16)

	v, _ = od.Index("Negative").SubIndex(0)
	i8, err := v.Int8()
	assert.NoError(t, err)
	assert.EqualValues(t, -5, i8)
	assert.Equal(t, AttributeSdoR|AttributeTrpdo, v.Attribute)

	v, _ = od.Index(0x2002).SubIndex(0)
	f32, err := v.Float32()
	assert.NoError(t, err)
	assert.EqualValues(t, 1.0, f32)

	v, _ = od.Index(0x2003).SubIndex(0)
	f64, err := v.Float64()
	assert.NoError(t, err)
	assert.EqualValues(t, 1.5, f64)

	v, _ = od.Index(0x2004).SubIndex(0)
	s, err := v.String()
	assert.NoError(t, err)
	assert.Equal(t, "hello world", s)
	assert.Equal(t, AttributeSdoR|AttributeStr, v.Attribute)

	v, _ = od.Index(0x2005).SubIndex(0)
	u8, _ := v.Uint8()
	assert.EqualValues(t, 8, u8)
	assert.Equal(t, AttributeSdoW, v.Attribute)

	v, _ = od.Index(0x2006).SubIndex(0)
	assert.Equal(t, []byte{0x10, 0}, v.lowLimit)
	assert.Equal(t, binary.LittleEndian.AppendUint16(nil, 1000), v.highLimit)
}

func TestParseSyntax(t *testing.T) {
	// BOM, CRLF, comments, case insensitive keys & "sub", spaces around '=',
	// trailing comment after section header, sections out of order
	eds := "\xEF\xBB\xBF; comment\r\n" +
		"[1018sub1]\r\n" +
		"parametername = Vendor-ID ; with comment\r\n" +
		"DATATYPE=0x0007\r\n" +
		"AccessType = ro\r\n" +
		"DefaultValue = 0x1234 ; inline comment\r\n" +
		"\r\n" +
		"[1018] ; identity\r\n" +
		"# another comment\r\n" +
		"ParameterName=Identity object\r\n" +
		"ObjectType=0x9\r\n" +
		"\tSubNumber=0x2\r\n" +
		"\r\n" +
		"[1018SUB0]\r\n" +
		"ParameterName=Highest sub-index supported\r\n" +
		"DataType=0x0005\r\n" +
		"AccessType=ro\r\n" +
		"DefaultValue=1"

	od := mustParse(t, eds, 0)
	entry := od.Index(0x1018)
	require.NotNil(t, entry)
	assert.Equal(t, "Identity object", entry.Name)
	assert.Equal(t, 2, entry.SubCount())

	vendor, err := entry.Uint32(1)
	assert.NoError(t, err)
	assert.EqualValues(t, 0x1234, vendor)

	highest, err := entry.Uint8(0)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, highest)

	// Value is not stripped of comment for names (they may contain ';')
	v, err := entry.SubIndex(uint8(1))
	assert.NoError(t, err)
	assert.Equal(t, "Vendor-ID ; with comment", v.Name)
}

func TestParseDefaults(t *testing.T) {
	// No ObjectType => VAR, no AccessType => rw, no PDOMapping => mappable
	// (historical behaviour, not CiA 306), no DefaultValue => 0,
	// no ParameterName is allowed
	od := mustParse(t, "[2000]\nDataType=0x0007\n", 0)
	entry := od.Index(0x2000)
	require.NotNil(t, entry)
	assert.Equal(t, ObjectTypeVAR, entry.ObjectType)
	v, err := entry.SubIndex(0)
	require.NoError(t, err)
	assert.Equal(t, AttributeSdoRw|AttributeTrpdo, v.Attribute)
	assert.Equal(t, []byte{0, 0, 0, 0}, v.value)

	// Explicitly not mappable
	od = mustParse(t, "[2000]\nDataType=0x0007\nPDOMapping=0\n", 0)
	v, _ = od.Index(0x2000).SubIndex(0)
	assert.Equal(t, AttributeSdoRw, v.Attribute)

	// DOMAIN object type is kept
	od = mustParse(t, "[2000]\nObjectType=0x2\nDataType=0x000F\n", 0)
	assert.Equal(t, ObjectTypeDOMAIN, od.Index(0x2000).ObjectType)
}

func TestParseArrayHoles(t *testing.T) {
	// SubNumber=3 but only sub0 & sub2 are defined
	eds := `
[2000]
ObjectType=0x8
SubNumber=3
[2000sub0]
DataType=0x5
DefaultValue=2
[2000sub2]
DataType=0x5
DefaultValue=7
`
	od := mustParse(t, eds, 0)
	_, err := od.Index(0x2000).SubIndex(uint8(1))
	assert.Equal(t, ErrSubNotExist, err)
	_, err = od.Streamer(0x2000, 1, true)
	assert.Equal(t, ErrSubNotExist, err)
	value, err := od.Index(0x2000).Uint8(2)
	assert.NoError(t, err)
	assert.EqualValues(t, 7, value)
}

func TestParseErrors(t *testing.T) {
	tests := map[string]string{
		"sub out of array range":  "[2000]\nObjectType=0x8\nSubNumber=1\n[2000sub0]\nDataType=5\n[2000sub5]\nDataType=5\n",
		"orphan sub":              "[2000]\nDataType=5\n[2001sub0]\nDataType=5\n",
		"sub of a VAR":            "[2000]\nDataType=5\n[2000sub0]\nDataType=5\n",
		"duplicate index":         "[2000]\nDataType=5\n[2000]\nDataType=5\n",
		"duplicate record sub":    "[2000]\nObjectType=9\n[2000sub1]\nDataType=5\n[2000sub1]\nDataType=5\n",
		"duplicate array sub":     "[2000]\nObjectType=8\nSubNumber=2\n[2000sub1]\nDataType=5\n[2000sub1]\nDataType=5\n",
		"missing DataType":        "[2000]\nDefaultValue=1\n",
		"unsupported DataType":    "[2000]\nDataType=0x0010\n",
		"invalid DataType":        "[2000]\nDataType=abc\n",
		"unknown ObjectType":      "[2000]\nObjectType=0x5\nDataType=5\n",
		"missing array SubNumber": "[2000]\nObjectType=0x8\n",
		"value out of range":      "[2000]\nDataType=0x5\nDefaultValue=300\n",
		"invalid value":           "[2000]\nDataType=0x7\nDefaultValue=hello\n",
		"invalid PDOMapping":      "[2000]\nDataType=0x7\nPDOMapping=maybe\n",
		"unterminated header":     "[2000\nDataType=0x7\n",
		"compact sub objects":     "[2000]\nObjectType=0x8\nCompactSubObj=3\nDataType=5\n",
	}
	for name, eds := range tests {
		assert.Error(t, parseErr(t, eds), name)
	}
	// Errors give context
	err := parseErr(t, "[FileInfo]\nFileName=x\n\n[2000]\nDataType=0x5\nDefaultValue=300\n")
	assert.ErrorContains(t, err, "line 4 [2000]")
}

func TestParseNoPanic(t *testing.T) {
	// Malformed inputs must never panic
	inputs := []string{
		"", "[", "]", "[]", "=", "=value", "[2000]\n=", "[2000]\n=\n[2000sub0]",
		"[2000sub0]", "[2000]\nObjectType=0x8\nSubNumber=0\n[2000sub0]\nDataType=5",
		"[2000]\nObjectType=0x9\nSubNumber=0xFFFF\n",
		strings.Repeat("[", 100),
	}
	for _, in := range inputs {
		assert.NotPanics(t, func() { _, _ = Parse([]byte(in), 0) }, in)
	}
}

func TestParseLongLine(t *testing.T) {
	// bufio.Scanner would fail on lines > 64KiB
	long := strings.Repeat("a", 100_000)
	od := mustParse(t, "[2000]\nDataType=0x9\nDefaultValue="+long+"\n", 0)
	v, err := od.Index(0x2000).SubIndex(0)
	require.NoError(t, err)
	assert.Len(t, v.value, len(long))
}

func TestParseSources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "base.eds")
	require.NoError(t, os.WriteFile(path, rawDefaultOd, 0o644))
	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	for name, src := range map[string]any{
		"path":   path,
		"bytes":  rawDefaultOd,
		"file":   f,
		"reader": bytes.NewReader(rawDefaultOd),
	} {
		od, err := Parse(src, 0x10)
		require.NoError(t, err, name)
		assert.Len(t, od.Entries(), len(Default().Entries()), name)
	}
	_, err = Parse(1234, 0x10)
	assert.Error(t, err)
	_, err = Parse(filepath.Join(t.TempDir(), "missing.eds"), 0x10)
	assert.Error(t, err)
}

func TestParseDoesNotAliasInput(t *testing.T) {
	raw := []byte("[2000]\nParameterName=name\nDataType=0x7\n")
	od, err := Parse(raw, 0)
	require.NoError(t, err)
	for i := range raw {
		raw[i] = 'x'
	}
	assert.Equal(t, "name", od.Index(0x2000).Name)
	assert.NotNil(t, od.Index("name"))
}

func TestParseIndependentValues(t *testing.T) {
	// Values share an arena, writing one must not affect neighbours
	od := Default()
	before := od.Index(0x1018).Uint32
	vendor, _ := before(1)
	require.NoError(t, od.Index(0x1017).PutUint16(0, 0xFFFF, true))
	require.NoError(t, od.Index(0x1018).PutUint32(2, 0xFFFFFFFF, true))
	after, _ := od.Index(0x1018).Uint32(1)
	assert.Equal(t, vendor, after)
	v, _ := od.Index(0x1017).SubIndex(0)
	assert.NotEqual(t, v.value, v.DefaultValue())
	assert.EqualValues(t, math.MaxUint16, binary.LittleEndian.Uint16(v.value))
}

func TestParseV2IsParse(t *testing.T) {
	// Legacy entry points must keep working and give the same result
	var legacy Parser = ParseV2
	expected, err := Parse(rawDefaultOd, 0x10)
	require.NoError(t, err)
	got, err := legacy(rawDefaultOd, 0x10)
	require.NoError(t, err)
	require.Len(t, got.Entries(), len(expected.Entries()))
	for index, entry := range expected.Entries() {
		other := got.Index(index)
		require.NotNil(t, other, "index x%x", index)
		assert.Equal(t, entry.Name, other.Name)
		assert.Equal(t, entry.ObjectType, other.ObjectType)
		assert.Equal(t, entry.SubCount(), other.SubCount())
	}
}

func TestParseOctetString(t *testing.T) {
	eds := `
[3000]
DataType=0x000A
DefaultValue=0000000000
[3001]
DataType=0x000A
DefaultValue=01 02 AB
[3002]
DataType=0x000A
DefaultValue=not hex
`
	od := mustParse(t, eds, 0)
	v, _ := od.Index(0x3000).SubIndex(0)
	assert.Equal(t, make([]byte, 5), v.value)
	assert.Equal(t, make([]byte, 5), v.DefaultValue())
	v, _ = od.Index(0x3001).SubIndex(0)
	assert.Equal(t, []byte{0x01, 0x02, 0xAB}, v.value)
	// Not valid hexadecimal, raw characters are kept
	v, _ = od.Index(0x3002).SubIndex(0)
	assert.Equal(t, []byte("not hex"), v.value)
}
