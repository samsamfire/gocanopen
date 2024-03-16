package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	log "github.com/sirupsen/logrus"
)

const TOKEN_NONE = -3
const TOKEN_DEFAULT = -2
const TOKEN_ALL = -1

// Gets SDO command as list of strings and processes it
func parseSdoCommand(command []string) (index uint64, subindex uint64, err error) {
	if len(command) != 3 {
		return 0, 0, ErrGwSyntaxError
	}
	indexStr := command[1]
	subIndexStr := command[2]
	// Unclear if this is "supported" not really specified in 309-5
	if indexStr == "all" {
		return 0, 0, ErrGwRequestNotSupported
	}
	index, e := strconv.ParseUint(indexStr, 0, 64)
	if e != nil {
		return 0, 0, ErrGwSyntaxError
	}
	subIndex, e := strconv.ParseUint(subIndexStr, 0, 64)
	if e != nil {
		return 0, 0, ErrGwSyntaxError
	}
	if index > 0xFFFF || subindex > 0xFF {
		return 0, 0, ErrGwSyntaxError
	}
	return index, subIndex, nil
}

// Parse raw network / node string param
func parseNodeOrNetworkParam(param string) (int, error) {
	// Check if any of the string values
	switch param {
	case "default":
		return TOKEN_DEFAULT, nil
	case "none":
		return TOKEN_NONE, nil
	case "all":
		return TOKEN_ALL, nil
	}
	// Else try a specific id
	// This automatically treats 0x,0X,... correctly
	// which is allowed in the spec
	paramUint, err := strconv.ParseUint(param, 0, 64)
	if err != nil {
		return 0, err
	}
	return int(paramUint), nil
}

// Create a new sanitized api request object from raw http request
// This function also checks that values are within bounds etc.
func NewGatewayRequestFromRaw(r *http.Request) (*GatewayRequest, error) {
	// Global expression match
	match := regURI.FindStringSubmatch(r.URL.Path)
	if len(match) != 6 {
		log.Error("[HTTP][SERVER] request does not match a known API pattern")
		return nil, ErrGwSyntaxError
	}
	// Check differents components of API route : api, sequence number, network and node
	apiVersion := match[1]
	if apiVersion != API_VERSION {
		log.Errorf("[HTTP][SERVER] api version %v is not supported", apiVersion)
		return nil, ErrGwRequestNotSupported
	}
	sequence, err := strconv.Atoi(match[2])
	if err != nil || sequence > MAX_SEQUENCE_NB {
		log.Errorf("[HTTP][SERVER] error processing sequence number %v", match[2])
		return nil, ErrGwSyntaxError
	}
	netStr := match[3]
	netInt, err := parseNodeOrNetworkParam(netStr)
	if err != nil || netInt == 0 || netInt > 0xFFFF {
		log.Errorf("[HTTP][SERVER] error processing network param %v", netStr)
		return nil, ErrGwUnsupportedNet
	}
	nodeStr := match[4]
	nodeInt, err := parseNodeOrNetworkParam(nodeStr)
	if err != nil || nodeInt == 0 || nodeInt > 127 {
		log.Errorf("[HTTP][SERVER] error processing node param %v", nodeStr)
	}

	// Unmarshall request body
	var parameters json.RawMessage
	err = json.NewDecoder(r.Body).Decode(&parameters)
	if err != nil && err != io.EOF {
		log.Warnf("[HTTP][SERVER] failed to unmarshal request body : %v", err)
		return nil, ErrGwSyntaxError
	}
	request := &GatewayRequest{
		nodeId:     nodeInt,
		networkId:  netInt,
		command:    match[5], // Contains rest of URL after node
		sequence:   uint32(sequence),
		parameters: parameters,
	}
	return request, nil
}
