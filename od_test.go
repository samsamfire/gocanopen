package canopen

import (
	"testing"
)

func TestParseEDS(t *testing.T) {

	od, err := ParseEDS("base.eds")
	od.Print()

	if err != nil {
		t.Errorf("Error %s", err)
	}
}
