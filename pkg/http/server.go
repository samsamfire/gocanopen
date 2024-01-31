package http

import (
	"net/http"
	"regexp"

	canopen "github.com/samsamfire/gocanopen"
)

const API_VERSION = "1.0"
const MAX_SEQUENCE_NB = 2<<31 - 1
const URI_PATTERN = `/cia309-5/(\d+\.\d+)/(\d{1,10})/(0x[0-9a-f]{1,4}|\d{1,10}|default|none|all)/(0x[0-9a-f]{1,2}|\d{1,3}|default|none|all)/(.*)`
const SDO_COMMAND_URI_PATTERN = `(r|read|w|write)/(all|0x[0-9a-f]{1,4}|\d{1,5})/?(0x[0-9a-f]{1,2}|\d{1,3})?`
const PDO_COMMAND_URI_PATTERN = `(r|read|w|write)/(p|pdo)/(0x[0-9a-f]{1,3}|\d{1,4})`

var regURI = regexp.MustCompile(URI_PATTERN)
var regSDO = regexp.MustCompile(SDO_COMMAND_URI_PATTERN)
var regPDO = regexp.MustCompile(PDO_COMMAND_URI_PATTERN)

var DATATYPE_MAP = map[string]uint8{
	"b":   canopen.BOOLEAN,
	"u8":  canopen.UNSIGNED8,
	"u16": canopen.UNSIGNED16,
	"u32": canopen.UNSIGNED32,
	"u64": canopen.UNSIGNED64,
	"i8":  canopen.INTEGER8,
	"i16": canopen.INTEGER16,
	"i32": canopen.INTEGER32,
	"i64": canopen.INTEGER64,
	"r32": canopen.REAL32,
	"r64": canopen.REAL64,
	"vs":  canopen.VISIBLE_STRING,
}

type GatewayServer struct {
	*canopen.BaseGateway
	serveMux *http.ServeMux
	routes   map[string]GatewayRequestHandler
}

// Create a new gateway
func NewGatewayServer(network *canopen.Network, defaultNetworkId uint16, defaultNodeId uint8, sdoUploadBufferSize int) *GatewayServer {
	base := canopen.NewBaseGateway(network, defaultNetworkId, defaultNodeId, sdoUploadBufferSize)
	gw := &GatewayServer{BaseGateway: base}
	gw.serveMux = http.NewServeMux()
	gw.serveMux.HandleFunc("/", gw.handleRequest) // This base route handles all the requests
	gw.routes = make(map[string]GatewayRequestHandler)

	// CiA 309-5 | 4.1
	gw.addRoute("r", gw.handlerRead)
	gw.addRoute("read", gw.handlerRead)
	gw.addRoute("w", gw.handleWrite)
	gw.addRoute("write", gw.handleWrite)
	gw.addRoute("set/sdo-timeout", gw.handleSDOTimeout)

	// CiA 309-5 | 4.3
	gw.addRoute("start", createNmtHandler(base, canopen.NMT_ENTER_OPERATIONAL))
	gw.addRoute("stop", createNmtHandler(base, canopen.NMT_ENTER_STOPPED))
	gw.addRoute("preop", createNmtHandler(base, canopen.NMT_ENTER_PRE_OPERATIONAL))
	gw.addRoute("preoperational", createNmtHandler(base, canopen.NMT_ENTER_PRE_OPERATIONAL))
	gw.addRoute("reset/node", createNmtHandler(base, canopen.NMT_RESET_NODE))
	gw.addRoute("reset/comm", createNmtHandler(base, canopen.NMT_RESET_COMMUNICATION))
	gw.addRoute("reset/communication", createNmtHandler(base, canopen.NMT_RESET_COMMUNICATION))
	gw.addRoute("enable/guarding", handlerNotSupported)
	gw.addRoute("disable/guarding", handlerNotSupported)
	gw.addRoute("enable/heartbeat", handlerNotSupported)
	gw.addRoute("disable/heartbeat", handlerNotSupported)

	// CiA 309-5 | 4.6
	//gw.addRoute("info/version")

	return gw
}

// Process server, blocking
func (gateway *GatewayServer) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, gateway.serveMux)
}

// Add a route to the server for handling a specific command
func (g *GatewayServer) addRoute(command string, handler GatewayRequestHandler) {
	g.routes[command] = handler
}
