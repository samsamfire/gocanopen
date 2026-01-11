package time

import (
	"log/slog"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSetInternalTime(t *testing.T) {
	now := time.Now()
	// Check that reading and setting time is precise
	now = now.Round(1 * time.Millisecond)
	timeInstance := &TIME{logger: slog.Default()}
	timeInstance.SetInternalTime(now)
	internalTime := timeInstance.InternalTime()
	timeDiff := internalTime.Sub(now)
	assert.LessOrEqual(t, math.Abs(float64(timeDiff.Milliseconds())), 2.0)
	nowPlus1Day := now.Add(24 * time.Hour)
	timeInstance.SetInternalTime(nowPlus1Day)
	timeDiff = timeInstance.InternalTime().Sub(nowPlus1Day)
	assert.LessOrEqual(t, math.Abs(float64(timeDiff.Milliseconds())), 2.0)
}
