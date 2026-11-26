package od

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExportDefaultEds(t *testing.T) {
	odict := Default()
	tempdir := t.TempDir()
	t.Run("export default EDS", func(t *testing.T) {
		err := ExportEDS(odict, false, tempdir+"exported.eds")
		assert.Nil(t, err)
		odictNew, err := Parse(tempdir+"exported.eds", 0x10)
		assert.Nil(t, err)
		// Check equality bewteen entries
		for index, entry := range odict.entriesByIndexValue {
			assert.Equal(t, entry.Name, odictNew.entriesByIndexValue[index].Name)
			switch o := entry.object.(type) {
			case *Variable:
				other := odictNew.entriesByIndexValue[index].object.(*Variable)
				assert.Equal(t, o.value, other.value)
			}
		}
	})
	t.Run("add variable and export EDS", func(t *testing.T) {
		_, err := odict.AddVariableType(0x1023, "some entry", UNSIGNED32, AttributeSdoRw, "0")
		assert.Nil(t, err)
		err = ExportEDS(odict, false, tempdir+"exported_with_var.eds")
		assert.Nil(t, err)
		odictNew, err := Parse(tempdir+"exported_with_var.eds", 0x10)
		assert.Nil(t, err)
		entry := odictNew.Index(0x1022)
		assert.NotNil(t, entry)
	})
}

func TestExportRoundTrip(t *testing.T) {
	eds := `
[2000]
ParameterName=Read only mappable
DataType=0x0002
AccessType=ro
PDOMapping=1
DefaultValue=-5
LowLimit=-10
HighLimit=10

[2001]
ParameterName=Record with holes
ObjectType=0x9
SubNumber=2

[2001sub0]
ParameterName=Highest
DataType=0x0005
AccessType=ro
DefaultValue=5

[2001sub5]
ParameterName=Only one
DataType=0x0003
AccessType=wo
DefaultValue=-300

[2002]
ParameterName=Single sub record
ObjectType=0x9
SubNumber=1

[2002sub0]
ParameterName=Highest
DataType=0x0005
DefaultValue=0

[1800]
ParameterName=Signed in communication area
DataType=0x0004
DefaultValue=-1

[2003]
ParameterName=Real
DataType=0x0008
DefaultValue=1.5

[2004]
ParameterName=Octet string
DataType=0x000A
PDOMapping=0
DefaultValue=0102AB
`
	odict, err := Parse([]byte(eds), 0)
	assert.Nil(t, err)
	filename := t.TempDir() + "/exported.eds"
	assert.Nil(t, ExportEDS(odict, false, filename))
	exported, err := Parse(filename, 0)
	assert.Nil(t, err)
	if err != nil {
		return
	}
	assert.Equal(t, dumpVariables(odict), dumpVariables(exported))
}

func dumpVariables(odict *ObjectDictionary) map[string]Variable {
	out := map[string]Variable{}
	for index, entry := range odict.entriesByIndexValue {
		var vars []*Variable
		switch o := entry.object.(type) {
		case *Variable:
			vars = []*Variable{o}
		case *VariableList:
			vars = o.Variables
		}
		for _, v := range vars {
			out[entry.Name+"/"+v.Name+"/"+string(rune(index))] = Variable{
				Name: v.Name, SubIndex: v.SubIndex, DataType: v.DataType, Attribute: v.Attribute,
				value: v.value, lowLimit: v.lowLimit, highLimit: v.highLimit,
			}
		}
	}
	return out
}
