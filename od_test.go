package canopen

import (
	"testing"
)

func TestParseEDS(t *testing.T) {

	_, err := ParseEDS("testdata/base.eds", 0x10)

	if err != nil {
		t.Errorf("Error %s", err)
	}
}
