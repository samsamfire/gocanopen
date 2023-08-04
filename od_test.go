package canopen

import (
	"testing"
)

func TestParseEDS(t *testing.T) {

	od, err := ParseEDS("testdata/base.eds", 0x10)
	od.Print()

	if err != nil {
		t.Errorf("Error %s", err)
	}
}
