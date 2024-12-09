package http

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/samsamfire/gocanopen/pkg/gateway"
	"github.com/samsamfire/gocanopen/pkg/nmt"
)

// Wrapper around [http.ResponseWriter] but keeps track of any writes already done
// This allows us to perform default behaviour if handler has not already sent a response
type doneWriter struct {
	http.ResponseWriter
	done bool
}

// Handle a [GatewayRequest] according to CiA 309-5
type GatewayRequestHandler func(w doneWriter, req *GatewayRequest) error

func (w *doneWriter) WriteHeader(status int) {
	w.done = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *doneWriter) Write(b []byte) (int, error) {
	w.done = true
	return w.ResponseWriter.Write(b)
}

// Create a new sanitized api request object from raw http request
// This function also checks that values are within bounds etc.
func (g *GatewayServer) newRequestFromRaw(r *http.Request) (*GatewayRequest, error) {
	// Global expression match
	match := regURI.FindStringSubmatch(r.URL.Path)
	if len(match) != 6 {
		g.logger.Error("request does not match a known API pattern")
		return nil, ErrGwSyntaxError
	}
	// Check differents components of API route : api, sequence number, network and node
	apiVersion := match[1]
	if apiVersion != API_VERSION {
		g.logger.Error("api version is not supported", "version", apiVersion)
		return nil, ErrGwRequestNotSupported
	}
	sequence, err := strconv.Atoi(match[2])
	if err != nil || sequence > MAX_SEQUENCE_NB {
		g.logger.Error("error processing sequence number", "sequence", match[2])
		return nil, ErrGwSyntaxError
	}
	netStr := match[3]
	netInt, err := parseNodeOrNetworkParam(netStr)
	if err != nil || netInt == 0 || netInt > 0xFFFF {
		g.logger.Error("error processing network param", "param", netStr)
		return nil, ErrGwUnsupportedNet
	}
	nodeStr := match[4]
	nodeInt, err := parseNodeOrNetworkParam(nodeStr)
	if err != nil || nodeInt == 0 || nodeInt > 127 {
		g.logger.Error("error processing node param", "param", nodeStr)
	}

	// Unmarshall request body
	var parameters json.RawMessage
	err = json.NewDecoder(r.Body).Decode(&parameters)
	if err != nil && err != io.EOF {
		g.logger.Warn("failed to unmarshal request body", "err", err)
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

// Default handler of any HTTP gateway request
// This parses a typical request and forwards it to the correct handler
func (g *GatewayServer) handleRequest(w http.ResponseWriter, raw *http.Request) {
	g.logger.Debug("handle incoming request", "endpoint", raw.URL)
	req, err := g.newRequestFromRaw(raw)
	if err != nil {
		w.Write(NewResponseError(0, err))
		return
	}
	// An api command (URI) is in the form /command/sub-command/... etc...
	// and can have variable parameters such as indexes as well as a body.
	// We first check inside a map that the full command is present inside of a handler map.
	// If full command is not found we then check again
	// but with truncated command up to the first "/".
	// e.g. '/reset/node' exists and is handled straight away
	// '/read/0x2000/0x0' does not exist in map, so we then check 'read' which does exist
	var route GatewayRequestHandler
	route, ok := g.routes[req.command]
	if !ok {
		indexFirstSep := strings.Index(req.command, "/")
		var firstCommand string
		if indexFirstSep != -1 {
			firstCommand = req.command[:indexFirstSep]
		} else {
			firstCommand = req.command
		}
		route, ok = g.routes[firstCommand]
		if !ok {
			g.logger.Debug("no handler found", "command", req.command, "firstCommand", firstCommand)
			w.Write(NewResponseError(int(req.sequence), ErrGwRequestNotSupported))
			return
		}
	}
	// Process the actual command
	dw := doneWriter{ResponseWriter: w, done: false}
	err = route(dw, req)
	if err != nil {
		w.Write(NewResponseError(int(req.sequence), err))
		return
	}
	if !dw.done {
		// No response specific command has been given, reply with default success
		dw.Write(NewResponseSuccess(int(req.sequence)))
		return
	}
}

// Create a handler for processing NMT request
func createNmtHandler(bg *gateway.BaseGateway, command nmt.Command) GatewayRequestHandler {
	return func(w doneWriter, req *GatewayRequest) error {
		switch req.nodeId {
		case TOKEN_DEFAULT, TOKEN_NONE:
			return bg.NMTCommand(bg.DefaultNodeId(), command)
		case TOKEN_ALL:
			return bg.NMTCommand(0, command)
		default:
			return bg.NMTCommand(uint8(req.nodeId), command)
		}
	}
}

// Can be used for specifying some routes that can be implemented in CiA 309
// But are not in this gateway
func handlerNotSupported(w doneWriter, req *GatewayRequest) error {
	return ErrGwRequestNotSupported
}

// Handle a read
// This includes different type of handlers : SDO, PDO, ...
func (g *GatewayServer) handlerRead(w doneWriter, req *GatewayRequest) error {
	matchSDO := regSDO.FindStringSubmatch(req.command)
	if len(matchSDO) >= 2 {
		return g.handlerSDORead(w, req, matchSDO)
	}
	matchPDO := regPDO.FindStringSubmatch(req.command)
	if len(matchPDO) >= 2 {
		return handlerNotSupported(w, req)
	}
	return ErrGwSyntaxError
}

func (g *GatewayServer) handlerSDORead(w doneWriter, req *GatewayRequest, commands []string) error {
	index, subindex, err := parseSdoCommand(commands[1:])
	if err != nil {
		g.logger.Error("unable to parse SDO command", "err", err)
		return err
	}

	n, err := g.ReadSDO(uint8(req.nodeId), uint16(index), uint8(subindex))
	if err != nil {
		w.Write(NewResponseError(int(req.sequence), err))
		return nil
	}
	buf := g.Buffer()[:n]
	slices.Reverse(buf)
	resp := SDOReadResponse{
		GatewayResponseBase: NewResponseBase(int(req.sequence), "OK"),
		Data:                "0x" + hex.EncodeToString(buf),
		Length:              n,
	}
	respRaw, err := json.Marshal(resp)
	if err != nil {
		return ErrGwRequestNotProcessed
	}
	w.Write(respRaw)
	return nil
}

// Handle a write
// This includes different type of handlers : SDO, PDO, ...
func (g *GatewayServer) handleWrite(w doneWriter, req *GatewayRequest) error {
	matchSDO := regSDO.FindStringSubmatch(req.command)
	if len(matchSDO) >= 2 {
		return g.handlerSDOWrite(w, req, matchSDO)
	}
	matchPDO := regPDO.FindStringSubmatch(req.command)
	if len(matchPDO) >= 2 {
		return handlerNotSupported(w, req)
	}
	return ErrGwSyntaxError
}

func (g *GatewayServer) handlerSDOWrite(w doneWriter, req *GatewayRequest, commands []string) error {
	index, subindex, err := parseSdoCommand(commands[1:])
	if err != nil {
		g.logger.Error("unable to parse SDO command", "err", err)
		return err
	}

	var sdoWrite SDOWriteRequest
	err = json.Unmarshal(req.parameters, &sdoWrite)
	if err != nil {
		return ErrGwSyntaxError
	}
	datatype, ok := DATATYPE_MAP[sdoWrite.Datatype]
	if !ok {
		g.logger.Error("requested datatype is wrong or unsupported", "dataType", sdoWrite.Datatype)
		return ErrGwRequestNotSupported
	}
	err = g.WriteSDO(uint8(req.nodeId), uint16(index), uint8(subindex), sdoWrite.Value, datatype)
	if err != nil {
		w.Write(NewResponseError(int(req.sequence), err))
		return nil
	}
	return nil
}

// Update SDO client timeout
func (g *GatewayServer) handleSDOTimeout(w doneWriter, req *GatewayRequest) error {

	var sdoTimeout SDOSetTimeoutRequest
	err := json.Unmarshal(req.parameters, &sdoTimeout)
	if err != nil {
		return ErrGwSyntaxError
	}
	sdoTimeoutInt, err := strconv.ParseUint(sdoTimeout.Value, 0, 64)
	if err != nil || sdoTimeoutInt > 0xFFFF {
		return ErrGwSyntaxError
	}
	return g.SetSDOTimeout(uint32(sdoTimeoutInt))
}

func (g *GatewayServer) handleGetVersion(w doneWriter, req *GatewayRequest) error {
	version, err := g.GetVersion()
	if err != nil {
		return ErrGwRequestNotProcessed
	}
	resp := VersionInfo{
		GatewayResponseBase: NewResponseBase(int(req.sequence), "OK"),
		GatewayVersion:      &version,
	}
	respRaw, err := json.Marshal(resp)
	if err != nil {
		return ErrGwRequestNotProcessed
	}
	w.Write(respRaw)
	return nil
}

func (g *GatewayServer) handleSetDefaultNetwork(w doneWriter, req *GatewayRequest) error {
	var defaultNetwork SetDefaultNetOrNode
	err := json.Unmarshal(req.parameters, &defaultNetwork)
	if err != nil {
		return ErrGwSyntaxError
	}
	networkId, err := strconv.ParseUint(defaultNetwork.Value, 0, 64)
	if err != nil || networkId > 0xFFFF || networkId == 0 {
		return ErrGwSyntaxError
	}
	g.SetDefaultNetworkId(uint16(networkId))
	respRaw := NewResponseSuccess(int(req.sequence))
	w.Write(respRaw)
	return nil
}

func (g *GatewayServer) handleSetDefaultNode(w doneWriter, req *GatewayRequest) error {
	var defaultNode SetDefaultNetOrNode
	err := json.Unmarshal(req.parameters, &defaultNode)
	if err != nil {
		return ErrGwSyntaxError
	}
	nodeId, err := strconv.ParseUint(defaultNode.Value, 0, 64)
	if err != nil || nodeId > 0xFF || nodeId == 0 {
		return ErrGwSyntaxError
	}
	g.SetDefaultNodeId(uint8(nodeId))
	respRaw := NewResponseSuccess(int(req.sequence))
	w.Write(respRaw)
	return nil
}
