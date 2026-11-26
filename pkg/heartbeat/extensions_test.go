package heartbeat

import (
	"testing"

	"github.com/samsamfire/gocanopen/v2/pkg/od"
	"github.com/stretchr/testify/assert"
)

func TestWriteEntry1016NilStream(t *testing.T) {
	assert.NotPanics(t, func() {
		_, err := writeEntry1016(nil, []byte{0, 0, 0, 0})
		assert.Equal(t, od.ErrDevIncompat, err)
	})
}
