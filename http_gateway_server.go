// CiA 309-5 implementation
package canopen

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
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
	network          *Network
	sdoTimeout       uint16
	serveMux         *http.ServeMux
	defaultNetworkId uint16
	defaultNodeId    uint8
	sdoUploadBuffer  []byte
}

type HTTPGatewayRequest struct {
	nodeId     uint8  // node id concerned
	networkId  uint16 // networkId to be used
	command    string // command can be composed of different parts
	sequence   uint32 // sequence number
	parameters json.RawMessage
}

type SDOWrite struct {
	Value    string `json:"value"`
	Datatype string `json:"datatype"`
}

type GWSDOReadResponse struct {
	Sequence string `json:"sequence"`
	Response string `json:"response"`
	Data     string `json:"data"`
	Length   string `json:"length,omitempty"`
}

// Gets SDO command as string and processes it
// Returns index & subindex to be written/read
func parseSdoCommand(command []string) (index uint64, subindex uint64, err *HTTPGatewayError) {
	if len(command) != 3 {
		return 0, 0, ErrGwSyntaxError
	}
	indexStr := command[1]
	subIndexStr := command[2]
	// Unclear if this is "supported" not really specified in 309-5
	if indexStr == "all" {
		// Read all OD ?
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
	return index, subIndex, nil
}

func parseNetworkParam(defaultNetwork uint16, network string) (net uint64, gwError *HTTPGatewayError) {
	switch network {
	case "default", "none":
		return uint64(defaultNetwork), nil
	case "all":
		return 0, ErrGwUnsupportedNet

	}
	// This automatically treats 0x,0X,... correctly
	// which is allowed in the spec
	net, err := strconv.ParseUint(network, 0, 64)
	if err != nil {
		return 0, ErrGwSyntaxError
	}
	if net == 0 || net > 0xFFFF {
		return 0, ErrGwSyntaxError
	}
	return net, nil
}

func parseNodeParam(defaultNode uint8, nodeStr string) (node uint64, gwError *HTTPGatewayError) {
	switch nodeStr {
	case "default", "none":
		return uint64(defaultNode), nil
	case "all":
		return 0, ErrGwUnsupportedNode
	}
	// This automatically treats 0x,0X,... correctly
	// which is allowed in the spec
	node, err := strconv.ParseUint(nodeStr, 0, 64)
	if err != nil {
		return 0, ErrGwSyntaxError
	}
	if node == 0 || (node > 127 && node != 255) {
		return 0, ErrGwSyntaxError
	}
	return node, nil
}

// Handle gateway http request
func (gateway *HTTPGatewayServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	match := regURI.FindStringSubmatch(r.URL.Path)
	log.Debugf("[HTTP][SERVER] new gateway request : %v", r.URL.Path)
	sequence, gatewayError := func() (seq int, e error) {
		// hole match + 5 submatches
		if len(match) != 6 {
			log.Errorf("[HTTP][SERVER] request does not match a command")
			return 0, ErrGwSyntaxError
		}
		apiVersion := match[1]
		if apiVersion != API_VERSION {
			log.Errorf("[HTTP][SERVER] api version is not supported")
			return 0, ErrGwRequestNotSupported
		}
		sequence, err := strconv.Atoi(match[2])
		if err != nil || sequence > MAX_SEQUENCE_NB {
			log.Errorf("[HTTP][SERVER] error processing sequence number")
			return 0, ErrGwSyntaxError
		}
		netStr := match[3]
		net, gwError := parseNetworkParam(gateway.defaultNetworkId, netStr)
		if gwError != nil {
			log.Errorf("[HTTP][SERVER] error processing network param")
			return sequence, gwError
		}
		nodeStr := match[4]
		node, gwError := parseNodeParam(gateway.defaultNodeId, nodeStr)
		if gwError != nil {
			log.Errorf("[HTTP][SERVER] error processing node param")
			return sequence, gwError
		}
		// Get JSON body
		var parameters json.RawMessage
		err = json.NewDecoder(r.Body).Decode(&parameters)
		if err != nil && err != io.EOF {
			log.Warnf("[HTTP][SERVER] failed to unmarshal request body : %v", err)
			return sequence, ErrGwSyntaxError
		}

		request := &HTTPGatewayRequest{nodeId: uint8(node), networkId: uint16(net), command: match[5], sequence: uint32(sequence), parameters: parameters}
		gwErr := gateway.processCANopenRequest(w, request)
		// CANopen request can fail for multiple reasons (sdo aborts, etc)
		if gwErr != nil {
			log.Warnf("[HTTP][SERVER] failed to process canopen request : %v", gwErr)
			return sequence, gwErr
		}
		return sequence, nil
	}()
	if gatewayError != nil {
		jData, _ := json.Marshal(map[string]string{"sequence": strconv.Itoa(sequence), "response": gatewayError.Error()})
		w.Write(jData)
	}
}

// Process the actual CANopen actions
func (gateway *HTTPGatewayServer) processCANopenRequest(w http.ResponseWriter, request *HTTPGatewayRequest) *HTTPGatewayError {
	log.Debugf("[HTTP][SERVER][SEQ:%d] processing request net %v | node %v | command %v | body %s",
		request.sequence,
		request.networkId,
		request.nodeId,
		request.command,
		request.parameters,
	)
	network := gateway.network
	// NMT commands
	switch request.command {
	case "start":
		network.Command(request.nodeId, NMT_ENTER_OPERATIONAL)
	case "stop":
		network.Command(request.nodeId, NMT_ENTER_STOPPED)
	case "preop", "preoperational":
		network.Command(request.nodeId, NMT_ENTER_PRE_OPERATIONAL)
	case "reset/node":
		network.Command(request.nodeId, NMT_RESET_NODE)
	case "reset/comm", "reset/communication":
		network.Command(request.nodeId, NMT_RESET_COMMUNICATION)

	}
	if !strings.HasPrefix(request.command, "r/") &&
		!strings.HasPrefix(request.command, "read") &&
		!strings.HasPrefix(request.command, "w/") &&
		!strings.HasPrefix(request.command, "write") {
		return ErrGwRequestNotSupported
	}
	// Try SDO or PDO command patterns
	matchSDO := regSDO.FindStringSubmatch(request.command)
	matchPDO := regPDO.FindStringSubmatch(request.command)
	if len(matchSDO) < 2 && len(matchPDO) < 2 {
		return ErrGwSyntaxError
	}
	if len(matchSDO) >= 2 {
		// SDO pattern
		readOrWrite := matchSDO[1]
		index, subindex, err := parseSdoCommand(matchSDO[1:]) // First string is hole match
		if err != nil {
			return err
		}
		switch readOrWrite {
		case "r", "read":
			n, err := network.ReadRaw(request.nodeId, uint16(index), uint8(subindex), gateway.sdoUploadBuffer)
			if err != nil {
				abortCode, ok := err.(SDOAbortCode)
				if !ok {
					abortCode = SDO_ABORT_GENERAL
				}
				return &HTTPGatewayError{Code: int(abortCode)}
			}
			resp := GWSDOReadResponse{Sequence: strconv.Itoa(int(request.sequence)), Response: "OK", Data: "0x" + hex.EncodeToString(gateway.sdoUploadBuffer[:n])}
			respRaw, err := json.Marshal(resp)
			if err != nil {
				return ErrGwRequestNotProcessed
			}
			_, err = w.Write(respRaw)
			if err != nil {
				return ErrGwRequestNotProcessed
			}
			return nil
		case "w", "write":
			var sdoWrite SDOWrite
			err := json.Unmarshal(request.parameters, &sdoWrite)
			if err != nil {
				return ErrGwSyntaxError
			}
			datatypeValue, ok := HTTP_DATATYPE_MAP[sdoWrite.Datatype]
			if !ok {
				return ErrGwRequestNotSupported
			}
			encodedValue, err := encode(sdoWrite.Value, datatypeValue, 0)
			if err != nil {
				return ErrGwSyntaxError
			}
			err = network.WriteRaw(request.nodeId, uint16(index), uint8(subindex), encodedValue)
			if err != nil {
				abortCode, ok := err.(SDOAbortCode)
				if !ok {
					abortCode = SDO_ABORT_GENERAL
				}
				return &HTTPGatewayError{Code: int(abortCode)}
			}
			return nil

		default:
			return ErrGwSyntaxError
		}
	}
	if len(matchPDO) <= 2 {
		// TODO
	}

	log.Errorf("[HTTP][SERVER] request did not match any of the known commands, probably a syntax error")
	return ErrGwSyntaxError

}

// Create a new gateway
func NewGateway(defaultNetworkId uint16, defaultNodeId uint8, sdoUploadBufferSize int, network *Network) *HTTPGatewayServer {
	gateway := &HTTPGatewayServer{}
	gateway.defaultNetworkId = defaultNetworkId
	gateway.defaultNodeId = defaultNodeId
	gateway.sdoUploadBuffer = make([]byte, sdoUploadBufferSize)
	gateway.network = network
	gateway.serveMux = http.NewServeMux()
	gateway.serveMux.HandleFunc("/", gateway.handleRequest)
	return gateway
}

func (gateway *HTTPGatewayServer) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, gateway.serveMux)
}
