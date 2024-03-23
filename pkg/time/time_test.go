package time

import (
	"testing"
	"time"

	// import assert
	"github.com/stretchr/testify/assert"
)

var test = time.Time{}

func TestSetInternalTime(t *testing.T) {

	timeInstance := &TIME{}
	timeInstance.SetInternalTime(time.Now())
	expectedDays := uint16(time.Since(timestampOrigin).Hours() / 24)
	assert.Equal(t, timeInstance.days, expectedDays)
}
