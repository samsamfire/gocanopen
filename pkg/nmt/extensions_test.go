package nmt

import (
	"testing"

	"github.com/samsamfire/gocanopen/v2/pkg/od"
	"github.com/stretchr/testify/assert"
)

func TestWriteEntry1017NilStream(t *testing.T) {
	assert.NotPanics(t, func() {
		_, err := writeEntry1017(nil, []byte{0, 0})
		assert.Equal(t, od.ErrDevIncompat, err)
	})
}
