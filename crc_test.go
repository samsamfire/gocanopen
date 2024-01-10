package canopen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCcittSingle(t *testing.T) {
	crc := crc16(0)
	crc.ccittSingle(10)
	assert.EqualValues(t, 0xA14A, crc)
}
