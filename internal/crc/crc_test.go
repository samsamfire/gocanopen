package crc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCcittSingle(t *testing.T) {
	crc := CRC16(0)
	crc.Single(10)
	assert.EqualValues(t, 0xA14A, crc)
}
