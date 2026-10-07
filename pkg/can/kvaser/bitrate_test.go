package kvaser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanlibBitrate(t *testing.T) {
	cases := map[int]int32{
		1_000_000: Bitrate1M,
		500_000:   Bitrate500k,
		250_000:   Bitrate250k,
		125_000:   Bitrate125k,
	}
	for hz, want := range cases {
		got, err := canlibBitrate(hz)
		assert.NoError(t, err)
		assert.Equal(t, want, got, "bitrate %v", hz)
	}

	_, err := canlibBitrate(123_456)
	assert.Error(t, err)
}
