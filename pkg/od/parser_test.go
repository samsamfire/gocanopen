package od

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseDefault(t *testing.T) {

	od := Default()
	assert.NotNil(t, od)
}

func BenchmarkParser(b *testing.B) {
	b.Run("od default parse", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			_, err := Parse(rawDefaultOd, 0x10)
			assert.Nil(b, err)
		}
	})

	b.Run("od default parse v2", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			_, err := ParseV2(rawDefaultOd, 0x10)
			assert.Nil(b, err)
		}
	})

}
