package canopen

import (
	"testing"
)

func TestParseEDS(t *testing.T) {

	od, err := ParseEDS("base.eds", 0x10)
	od.Print()

	if err != nil {
		t.Errorf("Error %s", err)
	}
}

func TestPrintEDS(t *testing.T) {

	od, err := ParseEDS("base.eds", 0x10)
	od.Print()

	if err != nil {
		t.Errorf("Error %s", err)
	}
}
