package http

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetError(t *testing.T) {
	req := GatewayResponseBase{Sequence: "1", Response: "ERROR:100"}
	err := req.GetError()
	assert.Equal(t, NewGatewayError(100), err)
	req = GatewayResponseBase{Sequence: "1", Response: "OK"}
	err = req.GetError()
	assert.Equal(t, nil, err)
}
