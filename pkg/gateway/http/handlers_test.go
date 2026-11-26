package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Gateway with custom routes only, no CANopen network needed
func newTestGateway(routes map[string]GatewayRequestHandler) *GatewayServer {
	return &GatewayServer{logger: slog.Default(), routes: routes}
}

func doRequest(t *testing.T, g *GatewayServer, path string) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	g.handleRequest(rec, httptest.NewRequest(http.MethodPut, path, nil))
	// Exactly one JSON object must be returned
	var resp map[string]any
	dec := json.NewDecoder(rec.Body)
	require.NoError(t, dec.Decode(&resp))
	assert.False(t, dec.More(), "response contains more than one body : %s", rec.Body.String())
	return resp
}

func TestHandlerResponseNotDuplicated(t *testing.T) {
	g := newTestGateway(map[string]GatewayRequestHandler{
		"custom": func(w *doneWriter, req *GatewayRequest) error {
			_, err := w.Write(NewResponseSuccess(req.sequence))
			return err
		},
		"default": func(w *doneWriter, req *GatewayRequest) error {
			return nil
		},
	})
	resp := doRequest(t, g, "/cia309-5/1.0/10/1/1/custom")
	assert.Equal(t, "OK", resp["response"])
	// No response written by handler, default success is sent
	resp = doRequest(t, g, "/cia309-5/1.0/10/1/1/default")
	assert.Equal(t, "OK", resp["response"])
}

func TestInvalidNodeIdRejected(t *testing.T) {
	called := false
	g := newTestGateway(map[string]GatewayRequestHandler{
		"start": func(w *doneWriter, req *GatewayRequest) error {
			called = true
			return nil
		},
	})
	for _, node := range []string{"0", "128", "999", "0xff"} {
		resp := doRequest(t, g, "/cia309-5/1.0/10/1/"+node+"/start")
		assert.Equal(t, "ERROR:107", resp["response"], node)
	}
	assert.False(t, called)

	for _, node := range []string{"1", "127", "0x7f", "default", "all", "none"} {
		resp := doRequest(t, g, "/cia309-5/1.0/10/1/"+node+"/start")
		assert.Equal(t, "OK", resp["response"], node)
	}
}

func TestParseSdoCommandBounds(t *testing.T) {
	_, _, err := parseSdoCommand([]string{"r", "0x2000", "0x1FF"})
	assert.Equal(t, ErrGwSyntaxError, err)
	_, _, err = parseSdoCommand([]string{"r", "0x12000", "0x0"})
	assert.Equal(t, ErrGwSyntaxError, err)
	index, subIndex, err := parseSdoCommand([]string{"r", "0x2000", "0xFF"})
	assert.Nil(t, err)
	assert.EqualValues(t, 0x2000, index)
	assert.EqualValues(t, 0xFF, subIndex)
}
