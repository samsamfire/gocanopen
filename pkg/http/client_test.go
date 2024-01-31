package http

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetError(t *testing.T) {
	req := GatewayResponse{Sequence: "1", Data: "", Length: "", Response: "ERROR:100"}
	err := req.GetError()
	assert.Equal(t, NewGatewayError(100), err)
	req = GatewayResponse{Sequence: "1", Data: "", Length: "", Response: "OK"}
	err = req.GetError()
	assert.Equal(t, nil, err)
}
