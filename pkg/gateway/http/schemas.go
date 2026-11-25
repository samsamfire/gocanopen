package http

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/samsamfire/gocanopen/v2/pkg/gateway"
)

type GatewayResponse interface {
	GetError() error
	GetSequenceNb() uint32
}

// HTTP response base
type GatewayResponseBase struct {
	// Sequence number corresponding to a request
	Sequence string `json:"sequence"`
	// Response, can be "OK", "NEXT", or "ERROR:x"
	Response string `json:"response"`
}

func NewResponseBase(sequence uint32, response string) *GatewayResponseBase {
	return &GatewayResponseBase{
		Sequence: strconv.FormatUint(uint64(sequence), 10),
		Response: response,
	}
}

func NewResponseError(sequence uint32, error error) []byte {
	gwErr, ok := error.(*GatewayError)
	if !ok {
		gwErr = ErrGwRequestNotProcessed // Apparently no "internal error"
	}
	jData, _ := json.Marshal(map[string]string{"sequence": strconv.FormatUint(uint64(sequence), 10), "response": gwErr.Error()})
	return jData
}

func NewResponseSuccess(sequence uint32) []byte {
	jData, _ := json.Marshal(map[string]string{"sequence": strconv.FormatUint(uint64(sequence), 10), "response": "OK"})
	return jData
}

// Extract error if any inside of reponse
func (resp *GatewayResponseBase) GetError() error {
	// Check if any gateway errors
	if !strings.HasPrefix(resp.Response, "ERROR:") {
		return nil
	}
	responseSplitted := strings.Split(resp.Response, ":")
	if len(responseSplitted) != 2 {
		return fmt.Errorf("error decoding error field ('ERROR:' : %v)", resp.Response)
	}
	var errorCode uint64
	errorCode, err := strconv.ParseUint(responseSplitted[1], 0, 64)
	if err != nil {
		return fmt.Errorf("error decoding error field ('ERROR:' : %v)", err)
	}
	return NewGatewayError(int(errorCode))
}

func (resp *GatewayResponseBase) GetSequenceNb() uint32 {
	sequence, _ := strconv.ParseUint(resp.Sequence, 10, 32)
	return uint32(sequence)
}

// HTTP request to the server
type GatewayRequest struct {
	nodeId     int    // node id concerned, some special negative values are used for "all", "default" & "none"
	networkId  int    // networkId to be used, some special negative values are used for "all", "default" & "none"
	command    string // command can be composed of different parts
	sequence   uint32 // sequence number
	parameters json.RawMessage
}
type SDOSetTimeoutRequest struct {
	Value string `json:"value"`
}

type SDOWriteRequest struct {
	Value    string `json:"value"`
	Datatype string `json:"datatype"`
}

type SDOReadResponse struct {
	*GatewayResponseBase
	Data   string `json:"data"`
	Length int    `json:"length,omitempty"`
}

type VersionInfo struct {
	*GatewayResponseBase
	*gateway.GatewayVersion
}

type SetDefaultNetOrNode struct {
	Value string `json:"value"`
}
