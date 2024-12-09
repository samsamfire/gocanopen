package http

import (
	"log/slog"
	"net/http"
	"regexp"

	"github.com/samsamfire/gocanopen/pkg/gateway"
	"github.com/samsamfire/gocanopen/pkg/network"
	"github.com/samsamfire/gocanopen/pkg/nmt"
	"github.com/samsamfire/gocanopen/pkg/od"
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
	"b":   od.BOOLEAN,
	"u8":  od.UNSIGNED8,
	"u16": od.UNSIGNED16,
	"u32": od.UNSIGNED32,
	"u64": od.UNSIGNED64,
	"i8":  od.INTEGER8,
	"i16": od.INTEGER16,
	"i32": od.INTEGER32,
	"i64": od.INTEGER64,
	"r32": od.REAL32,
	"r64": od.REAL64,
	"vs":  od.VISIBLE_STRING,
}

type GatewayServer struct {
	*gateway.BaseGateway
	logger   *slog.Logger
	serveMux *http.ServeMux
	routes   map[string]GatewayRequestHandler
}

// Create a new gateway
func NewGatewayServer(network *network.Network, logger *slog.Logger, defaultNetworkId uint16, defaultNodeId uint8, sdoUploadBufferSize int) *GatewayServer {

	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("service", "[HTTP]")
	base := gateway.NewBaseGateway(network, logger, defaultNetworkId, defaultNodeId, sdoUploadBufferSize)
	g := &GatewayServer{BaseGateway: base, logger: logger}
	g.serveMux = http.NewServeMux()
	g.serveMux.HandleFunc("/", g.handleRequest) // This base route handles all the requests
	g.routes = make(map[string]GatewayRequestHandler)

	g.logger.Info("initializing http gateway (CiA 309-5) endpoints")
	// CiA 309-5 | 4.1
	g.addRoute("r", g.handlerRead)
	g.addRoute("read", g.handlerRead)
	g.addRoute("w", g.handleWrite)
	g.addRoute("write", g.handleWrite)
	g.addRoute("set/sdo-timeout", g.handleSDOTimeout)

	// CiA 309-5 | 4.3
	g.addRoute("start", createNmtHandler(base, nmt.CommandEnterOperational))
	g.addRoute("stop", createNmtHandler(base, nmt.CommandEnterStopped))
	g.addRoute("preop", createNmtHandler(base, nmt.CommandEnterPreOperational))
	g.addRoute("preoperational", createNmtHandler(base, nmt.CommandEnterPreOperational))
	g.addRoute("reset/node", createNmtHandler(base, nmt.CommandResetNode))
	g.addRoute("reset/comm", createNmtHandler(base, nmt.CommandResetCommunication))
	g.addRoute("reset/communication", createNmtHandler(base, nmt.CommandResetCommunication))
	g.addRoute("enable/guarding", handlerNotSupported)
	g.addRoute("disable/guarding", handlerNotSupported)
	g.addRoute("enable/heartbeat", handlerNotSupported)
	g.addRoute("disable/heartbeat", handlerNotSupported)

	// CiA 309-5 | 4.6
	g.addRoute("set/network", g.handleSetDefaultNetwork)
	g.addRoute("set/node", g.handleSetDefaultNode)
	g.addRoute("info/version", g.handleGetVersion)

	g.logger.Info("finished initializing")

	return g
}

// Process server, blocking
func (g *GatewayServer) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, g.serveMux)
}

// Add a route to the server for handling a specific command
func (g *GatewayServer) addRoute(command string, handler GatewayRequestHandler) {
	g.logger.Debug("registering route", "command", command)
	g.routes[command] = handler
}
