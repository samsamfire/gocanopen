package canopen

import (
	"reflect"
	"testing"

	"gopkg.in/ini.v1"
)

func Test_buildVariable(t *testing.T) {
	type args struct {
		section  *ini.Section
		name     string
		nodeId   uint8
		index    uint16
		subindex uint8
	}
	tests := []struct {
		name    string
		args    args
		want    *Variable
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildVariable(tt.args.section, tt.args.name, tt.args.nodeId, tt.args.index, tt.args.subindex)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildVariable() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildVariable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_encode(t *testing.T) {

	// Test unnsigned8
	data, err := encode("0x10", UNSIGNED8, 0)

	if err != nil {
		t.Errorf("Error %s", err)
	}
	if !reflect.DeepEqual([]byte{0x10}, data) {
		t.Errorf("%d", data)
	}
	// Test unsigned16
	data, _ = encode("0x10", UNSIGNED16, 0)
	if !reflect.DeepEqual([]byte{0x10, 0x00}, data) {
		t.Errorf("%d", data)
	}

	// Test unsigned32
	data, _ = encode("0x10", UNSIGNED32, 0)
	if !reflect.DeepEqual([]byte{0x10, 0x00, 0x00, 0x00}, data) {
		t.Errorf("%d", data)
	}

	// Test signed8
	data, _ = encode("0x20", INTEGER8, 0)
	if !reflect.DeepEqual([]byte{0x20}, data) {
		t.Errorf("%d", data)
	}
	// Test signed16
	data, _ = encode("0x20", INTEGER16, 0)
	if !reflect.DeepEqual([]byte{0x20, 0x00}, data) {
		t.Errorf("%d", data)
	}
	// Test signed32
	data, _ = encode("0x20", INTEGER32, 0)
	if !reflect.DeepEqual([]byte{0x20, 0x00, 0x00, 0x00}, data) {
		t.Errorf("%d", data)
	}

	// Test bool
	data, _ = encode("0x1", BOOLEAN, 0)
	if !reflect.DeepEqual([]byte{0x1}, data) {
		t.Errorf("%d", data)
	}

	// Test encoding a value that is wrong
	_, err = encode("90000", UNSIGNED8, 0)
	if err == nil {
		t.Error("Error should'nt be nil")
	}

}
