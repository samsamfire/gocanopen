package canopen

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"
)

const API_VERSION = "1.0"
const MAX_SEQUENCE_NB = 2<<31 - 1
const URI_PATTERN = `/cia309-5/(\d+\.\d+)/(\d{1,10})/(0x[0-9a-f]{1,4}|\d{1,10}|default|none|all)/(0x[0-9a-f]{1,2}|\d{1,3}|default|none|all)/(.*)`
const SDO_COMMAND_URI_PATTERN = `(r|read|w|write)/(all|0x[0-9a-f]{1,4}|\d{1,5})/?(0x[0-9a-f]{1,2}|\d{1,3})?`
const PDO_COMMAND_URI_PATTERN = `(r|read|w|write)/(p|pdo)/(0x[0-9a-f]{1,3}|\d{1,4})`

var regURI = regexp.MustCompile(URI_PATTERN)
var regSDO = regexp.MustCompile(SDO_COMMAND_URI_PATTERN)
var regPDO = regexp.MustCompile(PDO_COMMAND_URI_PATTERN)

var ERROR_GATEWAY_DESCRIPTION_MAP = map[int]string{
	100: "Request not supported",
	101: "Syntax error",
	102: "Request not processed due to internal state",
	103: "Time-out (where applicable)",
	104: "No default net set",
	105: "No default node set",
	106: "Unsupported net",
	107: "Unsupported node",
	108: "Command cancellation failed or ignored",
	109: "Emergency consumer not enabled",
	204: "Wrong NMT state",
	300: "Wrong password (User management)",
	301: "Number of super users exceeded (User management)",
	302: "Node access denied (User management)",
	303: "No session available (User management)",
	400: "PDO already used",
	401: "PDO length exceeded",
	501: "LSS implementation-/manufacturer-specific error",
	502: "LSS node-ID not supported",
	503: "LSS bit-rate not supported",
	504: "LSS parameter storing failed",
	505: "LSS command failed because of media error",
	600: "Running out of memory",
	601: "CAN interface currently not available",
	602: "Size to be set lower than minimum SDO buffer size",
	900: "Manufacturer-specific error",
}

var (
	ErrGwRequestNotSupported         = &HTTPGatewayError{Code: 100}
	ErrGwSyntaxError                 = &HTTPGatewayError{Code: 101}
	ErrGwRequestNotProcessed         = &HTTPGatewayError{Code: 102}
	ErrGwTimeout                     = &HTTPGatewayError{Code: 103}
	ErrGwNoDefaultNetSet             = &HTTPGatewayError{Code: 104}
	ErrGwNoDefaultNodeSet            = &HTTPGatewayError{Code: 105}
	ErrGwUnsupportedNet              = &HTTPGatewayError{Code: 106}
	ErrGwUnsupportedNode             = &HTTPGatewayError{Code: 107}
	ErrGwCommandCancellationFailed   = &HTTPGatewayError{Code: 108}
	ErrGwEmergencyConsumerNotEnabled = &HTTPGatewayError{Code: 109}
	ErrGwWrongNMTState               = &HTTPGatewayError{Code: 204}
	ErrGwWrongPassword               = &HTTPGatewayError{Code: 300}
	ErrGwSuperUsersExceeded          = &HTTPGatewayError{Code: 301}
	ErrGwNodeAccessDenied            = &HTTPGatewayError{Code: 302}
	ErrGwNoSessionAvailable          = &HTTPGatewayError{Code: 303}
	ErrGwPDOAlreadyUsed              = &HTTPGatewayError{Code: 400}
	ErrGwPDOLengthExceeded           = &HTTPGatewayError{Code: 401}
	ErrGwLSSImplementationError      = &HTTPGatewayError{Code: 501}
	ErrGwLSSNodeIDNotSupported       = &HTTPGatewayError{Code: 502}
	ErrGwLSSBitRateNotSupported      = &HTTPGatewayError{Code: 503}
	ErrGwLSSParameterStoringFailed   = &HTTPGatewayError{Code: 504}
	ErrGwLSSCommandFailed            = &HTTPGatewayError{Code: 505}
	ErrGwRunningOutOfMemory          = &HTTPGatewayError{Code: 600}
	ErrGwCANInterfaceNotAvailable    = &HTTPGatewayError{Code: 601}
	ErrGwSizeLowerThanSDOBufferSize  = &HTTPGatewayError{Code: 602}
	ErrGwManufacturerSpecificError   = &HTTPGatewayError{Code: 900}
)

var HTTP_DATATYPE_MAP = map[string]uint8{
	"b":   BOOLEAN,
	"u8":  UNSIGNED8,
	"u16": UNSIGNED16,
	"u32": UNSIGNED32,
	"u64": UNSIGNED64,
	"i8":  INTEGER8,
	"i16": INTEGER16,
	"i32": INTEGER32,
	"i64": INTEGER64,
	"r32": REAL32,
	"r64": REAL64,
	"vs":  VISIBLE_STRING,
}

type HTTPGatewayError struct {
	Code int // Can be either an sdo abort code or a gateway error code
}

func (e *HTTPGatewayError) Error() string {
	if e.Code <= 999 {
		return fmt.Sprintf("ERROR:%d", e.Code)
	}
	// Return as a hex value (sdo aborts)
	return fmt.Sprintf("ERROR:0x%x", e.Code)
}

type HTTPGatewayServer struct {
	base             *BaseGateway
	network          *Network
	serveMux         *http.ServeMux
	defaultNetworkId uint16
	defaultNodeId    uint8
	sdoBuffer        []byte
	routes           map[string]HTTPRequestHandler
}

type HTTPGatewayRequest struct {
	nodeId     int    // node id concerned, some special negative values are used for "all", "default" & "none"
	networkId  int    // networkId to be used, some special negative values are used for "all", "default" & "none"
	command    string // command can be composed of different parts
	sequence   uint32 // sequence number
	parameters json.RawMessage
}

// Default handler of any HTTP gateway request
// This parses a typical request and forwards it to the correct handler
func (gateway *HTTPGatewayServer) handleRequest(w http.ResponseWriter, raw *http.Request) {
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
	var route HTTPRequestHandler
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

// Create a new gateway
func NewGateway(defaultNetworkId uint16, defaultNodeId uint8, sdoUploadBufferSize int, network *Network) *HTTPGatewayServer {
	gw := &HTTPGatewayServer{}
	gw.defaultNetworkId = defaultNetworkId
	gw.defaultNodeId = defaultNodeId
	gw.sdoBuffer = make([]byte, sdoUploadBufferSize)
	gw.network = network
	gw.serveMux = http.NewServeMux()
	gw.serveMux.HandleFunc("/", gw.handleRequest)
	base := &BaseGateway{network: network}
	gw.base = base
	gw.routes = make(map[string]HTTPRequestHandler)

	// Add all handlers

	// CiA 309-5 | 4.1
	gw.addRoute("r", gw.handlerRead)
	gw.addRoute("read", gw.handlerRead)
	gw.addRoute("w", gw.handleWrite)
	gw.addRoute("write", gw.handleWrite)
	gw.addRoute("set/sdo-timeout", gw.handleSDOTimeout)

	// CiA 309-5 | 4.3
	gw.addRoute("start", createNmtHandler(base, NMT_ENTER_OPERATIONAL))
	gw.addRoute("stop", createNmtHandler(base, NMT_ENTER_STOPPED))
	gw.addRoute("preop", createNmtHandler(base, NMT_ENTER_PRE_OPERATIONAL))
	gw.addRoute("preoperational", createNmtHandler(base, NMT_ENTER_PRE_OPERATIONAL))
	gw.addRoute("reset/node", createNmtHandler(base, NMT_RESET_NODE))
	gw.addRoute("reset/comm", createNmtHandler(base, NMT_RESET_COMMUNICATION))
	gw.addRoute("reset/communication", createNmtHandler(base, NMT_RESET_COMMUNICATION))
	gw.addRoute("enable/guarding", handlerNotSupported)
	gw.addRoute("disable/guarding", handlerNotSupported)
	gw.addRoute("enable/heartbeat", handlerNotSupported)
	gw.addRoute("disable/heartbeat", handlerNotSupported)

	return gw
}

func (gateway *HTTPGatewayServer) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, gateway.serveMux)
}

// Add a route to the server for handling a specific command
func (g *HTTPGatewayServer) addRoute(command string, handler HTTPRequestHandler) {
	g.routes[command] = handler
}
