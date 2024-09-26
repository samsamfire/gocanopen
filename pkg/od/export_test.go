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
