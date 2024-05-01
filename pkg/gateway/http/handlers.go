package http

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/samsamfire/gocanopen/pkg/gateway"
	"github.com/samsamfire/gocanopen/pkg/nmt"
	log "github.com/sirupsen/logrus"
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

// Default handler of any HTTP gateway request
// This parses a typical request and forwards it to the correct handler
func (gateway *GatewayServer) handleRequest(w http.ResponseWriter, raw *http.Request) {
	log.Debugf("[HTTP][SERVER] new request : %v", raw.URL)
	req, err := NewGatewayRequestFromRaw(raw)
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
	route, ok := gateway.routes[req.command]
	if !ok {
		indexFirstSep := strings.Index(req.command, "/")
		var firstCommand string
		if indexFirstSep != -1 {
			firstCommand = req.command[:indexFirstSep]
		} else {
			firstCommand = req.command
		}
		route, ok = gateway.routes[firstCommand]
		if !ok {
			log.Debugf("[HTTP][SERVER] no handler found for : '%v' or '%v'", req.command, firstCommand)
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
func (gw *GatewayServer) handlerRead(w doneWriter, req *GatewayRequest) error {
	matchSDO := regSDO.FindStringSubmatch(req.command)
	if len(matchSDO) >= 2 {
		return gw.handlerSDORead(w, req, matchSDO)
	}
	matchPDO := regPDO.FindStringSubmatch(req.command)
	if len(matchPDO) >= 2 {
		return handlerNotSupported(w, req)
	}
	return ErrGwSyntaxError
}

func (gw *GatewayServer) handlerSDORead(w doneWriter, req *GatewayRequest, commands []string) error {
	index, subindex, err := parseSdoCommand(commands[1:])
	if err != nil {
		log.Errorf("[HTTP][SERVER] unable to parse SDO command : %v", err)
		return err
	}

	n, err := gw.ReadSDO(uint8(req.nodeId), uint16(index), uint8(subindex))
	if err != nil {
		w.Write(NewResponseError(int(req.sequence), err))
		return nil
	}
	buf := gw.Buffer()[:n]
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
func (gw *GatewayServer) handleWrite(w doneWriter, req *GatewayRequest) error {
	matchSDO := regSDO.FindStringSubmatch(req.command)
	if len(matchSDO) >= 2 {
		return gw.handlerSDOWrite(w, req, matchSDO)
	}
	matchPDO := regPDO.FindStringSubmatch(req.command)
	if len(matchPDO) >= 2 {
		return handlerNotSupported(w, req)
	}
	return ErrGwSyntaxError
}

func (gw *GatewayServer) handlerSDOWrite(w doneWriter, req *GatewayRequest, commands []string) error {
	index, subindex, err := parseSdoCommand(commands[1:])
	if err != nil {
		log.Errorf("[HTTP][SERVER] unable to parse SDO command : %v", err)
		return err
	}

	var sdoWrite SDOWriteRequest
	err = json.Unmarshal(req.parameters, &sdoWrite)
	if err != nil {
		return ErrGwSyntaxError
	}
	datatype, ok := DATATYPE_MAP[sdoWrite.Datatype]
	if !ok {
		log.Errorf("[HTTP][SERVER] requested datatype is either wrong or unsupported : %v", sdoWrite.Datatype)
		return ErrGwRequestNotSupported
	}
	err = gw.WriteSDO(uint8(req.nodeId), uint16(index), uint8(subindex), sdoWrite.Value, datatype)
	if err != nil {
		w.Write(NewResponseError(int(req.sequence), err))
		return nil
	}
	return nil
}

// Update SDO client timeout
func (gw *GatewayServer) handleSDOTimeout(w doneWriter, req *GatewayRequest) error {

	var sdoTimeout SDOSetTimeoutRequest
	err := json.Unmarshal(req.parameters, &sdoTimeout)
	if err != nil {
		return ErrGwSyntaxError
	}
	sdoTimeoutInt, err := strconv.ParseUint(sdoTimeout.Value, 0, 64)
	if err != nil || sdoTimeoutInt > 0xFFFF {
		return ErrGwSyntaxError
	}
	return gw.SetSDOTimeout(uint32(sdoTimeoutInt))
}

func (gw *GatewayServer) handleGetVersion(w doneWriter, req *GatewayRequest) error {
	version, err := gw.GetVersion()
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

func (gw *GatewayServer) handleSetDefaultNetwork(w doneWriter, req *GatewayRequest) error {
	var defaultNetwork SetDefaultNetOrNode
	err := json.Unmarshal(req.parameters, &defaultNetwork)
	if err != nil {
		return ErrGwSyntaxError
	}
	networkId, err := strconv.ParseUint(defaultNetwork.Value, 0, 64)
	if err != nil || networkId > 0xFFFF || networkId == 0 {
		return ErrGwSyntaxError
	}
	gw.SetDefaultNetworkId(uint16(networkId))
	respRaw := NewResponseSuccess(int(req.sequence))
	w.Write(respRaw)
	return nil
}

func (gw *GatewayServer) handleSetDefaultNode(w doneWriter, req *GatewayRequest) error {
	var defaultNode SetDefaultNetOrNode
	err := json.Unmarshal(req.parameters, &defaultNode)
	if err != nil {
		return ErrGwSyntaxError
	}
	nodeId, err := strconv.ParseUint(defaultNode.Value, 0, 64)
	if err != nil || nodeId > 0xFF || nodeId == 0 {
		return ErrGwSyntaxError
	}
	gw.SetDefaultNodeId(uint8(nodeId))
	respRaw := NewResponseSuccess(int(req.sequence))
	w.Write(respRaw)
	return nil
}
