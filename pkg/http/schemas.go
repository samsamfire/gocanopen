package http

import "encoding/json"

// HTTP response from the server
type GatewayResponse struct {
	Sequence string `json:"sequence,omitempty"`
	Data     string `json:"data,omitempty"`
	Length   string `json:"length,omitempty"`
	Response string `json:"response,omitempty"`
}

// HTTP request to the server
type GatewayRequest struct {
	nodeId     int    // node id concerned, some special negative values are used for "all", "default" & "none"
	networkId  int    // networkId to be used, some special negative values are used for "all", "default" & "none"
	command    string // command can be composed of different parts
	sequence   uint32 // sequence number
	parameters json.RawMessage
}

type SDOTimeoutRequest struct {
	Value string `json:"value"`
}

type SDOWriteRequest struct {
	Value    string `json:"value"`
	Datatype string `json:"datatype"`
}

type SDOReadResponse struct {
	Sequence string `json:"sequence"`
	Response string `json:"response"`
	Data     string `json:"data"`
	Length   int    `json:"length,omitempty"`
}
