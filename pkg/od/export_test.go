package od

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExportDefaultEds(t *testing.T) {
	odict := Default()
	tempdir := t.TempDir()
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
}
