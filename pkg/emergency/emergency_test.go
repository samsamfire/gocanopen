package emergency

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Report / reset are only logged when the error state actually changes,
// as no emergency message is sent otherwise
func TestErrorReportLoggedOnChangeOnly(t *testing.T) {
	buf := &bytes.Buffer{}
	emcy := NewEMCYForLogging(slog.New(slog.NewTextHandler(buf, nil)))

	count := func(msg string) int {
		return strings.Count(buf.String(), "msg=\""+msg+"\"")
	}

	emcy.ErrorReset(EmHeartbeatConsumer, 0)
	assert.Equal(t, 0, count("reset emergency"))

	for range 3 {
		emcy.ErrorReport(EmHeartbeatConsumer, ErrHeartbeat, 0)
	}
	assert.Equal(t, 1, count("report emergency"))
	assert.True(t, emcy.IsError(EmHeartbeatConsumer))

	for range 3 {
		emcy.ErrorReset(EmHeartbeatConsumer, 0)
	}
	assert.Equal(t, 1, count("reset emergency"))
	assert.False(t, emcy.IsError(EmHeartbeatConsumer))
}
