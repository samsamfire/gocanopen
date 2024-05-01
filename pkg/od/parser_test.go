package od

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseDefault(t *testing.T) {

	od := Default()
	assert.NotNil(t, od)
}
