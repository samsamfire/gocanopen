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

	// Internal time keeps advancing after being set
	time.Sleep(100 * time.Millisecond)
	assert.GreaterOrEqual(t, timeInstance.InternalTime().Sub(nowPlus1Day), 100*time.Millisecond)
}

func TestConvertTimeIsUTC(t *testing.T) {
	// Encoding must not depend on the local timezone
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip("Europe/Paris timezone not available")
	}
	saved := time.Local
	time.Local = paris
	defer func() { time.Local = saved }()

	// 2000-01-01 12:00 UTC : 5844 days after 1984-01-01, 43 200 000 ms after midnight
	in := time.Date(2000, time.January, 1, 12, 0, 0, 0, time.UTC)
	expected := [8]byte{0x00, 0x2E, 0x93, 0x02, 0xD4, 0x16}
	assert.Equal(t, expected, convertTimeToByte(in))
	assert.Equal(t, expected, convertTimeToByte(in.In(paris)))
	assert.True(t, in.Equal(convertByteToTime(expected)))
}
