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

var (
	ErrRequestNotSupported         = &HTTPGatewayError{Code: 100, Message: "Request not supported"}
	ErrSyntaxError                 = &HTTPGatewayError{Code: 101, Message: "Syntax error"}
	ErrRequestNotProcessed         = &HTTPGatewayError{Code: 102, Message: "Request not processed due to internal state"}
	ErrTimeout                     = &HTTPGatewayError{Code: 103, Message: "Time-out (where applicable)"}
	ErrNoDefaultNetSet             = &HTTPGatewayError{Code: 104, Message: "No default net set"}
	ErrNoDefaultNodeSet            = &HTTPGatewayError{Code: 105, Message: "No default node set"}
	ErrUnsupportedNet              = &HTTPGatewayError{Code: 106, Message: "Unsupported net"}
	ErrUnsupportedNode             = &HTTPGatewayError{Code: 107, Message: "Unsupported node"}
	ErrCommandCancellationFailed   = &HTTPGatewayError{Code: 108, Message: "Command cancellation failed or ignored"}
	ErrEmergencyConsumerNotEnabled = &HTTPGatewayError{Code: 109, Message: "Emergency consumer not enabled"}
	ErrWrongNMTState               = &HTTPGatewayError{Code: 204, Message: "Wrong NMT state"}
	ErrWrongPassword               = &HTTPGatewayError{Code: 300, Message: "Wrong password (User management)"}
	ErrSuperUsersExceeded          = &HTTPGatewayError{Code: 301, Message: "Number of super users exceeded (User management)"}
	ErrNodeAccessDenied            = &HTTPGatewayError{Code: 302, Message: "Node access denied (User management)"}
	ErrNoSessionAvailable          = &HTTPGatewayError{Code: 303, Message: "No session available (User management)"}
	ErrPDOAlreadyUsed              = &HTTPGatewayError{Code: 400, Message: "PDO already used"}
	ErrPDOLengthExceeded           = &HTTPGatewayError{Code: 401, Message: "PDO length exceeded"}
	ErrLSSImplementationError      = &HTTPGatewayError{Code: 501, Message: "LSS implementation-/manufacturer-specific error"}
	ErrLSSNodeIDNotSupported       = &HTTPGatewayError{Code: 502, Message: "LSS node-ID not supported"}
	ErrLSSBitRateNotSupported      = &HTTPGatewayError{Code: 503, Message: "LSS bit-rate not supported"}
	ErrLSSParameterStoringFailed   = &HTTPGatewayError{Code: 504, Message: "LSS parameter storing failed"}
	ErrLSSCommandFailed            = &HTTPGatewayError{Code: 505, Message: "LSS command failed because of media error"}
	ErrRunningOutOfMemory          = &HTTPGatewayError{Code: 600, Message: "Running out of memory"}
	ErrCANInterfaceNotAvailable    = &HTTPGatewayError{Code: 601, Message: "CAN interface currently not available"}
	ErrSizeLowerThanSDOBufferSize  = &HTTPGatewayError{Code: 602, Message: "Size to be set lower than minimum SDO buffer size"}
	ErrManufacturerSpecificError   = &HTTPGatewayError{Code: 900, Message: "Manufacturer-specific error"}
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
	Code         int
	SdoAbortCode int
	Message      string
}

func (e *HTTPGatewayError) Error() string {
	return fmt.Sprintf("error %d: %s", e.Code, e.Message)
}

func (e *HTTPGatewayError) JSON() []byte {
	jData, err := json.Marshal(map[string]int{"ERROR": e.Code})
	if err != nil {
		return []byte(`{"ERROR:":103}`) // Not processed because of an error
	}
	if e.SdoAbortCode != 0 {
		sdoAbortErrorFormated := fmt.Sprintf(`{"ERROR:":{"sdo-abort-code":"%s"}}`, fmt.Sprintf("%x", e.SdoAbortCode))
		return []byte(sdoAbortErrorFormated)
	}
	return jData
}

type HTTPGateway struct {
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

// Parse value given datatype
func parseValue(value []byte, dataType string) {

}

// Gets SDO command as string and processes it
// Returns index & subindex to be written/read
func parseSdoCommand(command []string) (index uint64, subindex uint64, err *HTTPGatewayError) {
	if len(command) != 3 {
		return 0, 0, ErrSyntaxError
	}
	indexStr := command[1]
	subIndexStr := command[2]
	// Unclear if this is "supported" not really specified in 309-5
	if indexStr == "all" {
		// Read all OD ?
		return 0, 0, ErrRequestNotSupported
	}
	index, e := strconv.ParseUint(indexStr, 0, 64)
	if e != nil {
		return 0, 0, ErrSyntaxError
	}
	subIndex, e := strconv.ParseUint(subIndexStr, 0, 64)
	if e != nil {
		return 0, 0, ErrSyntaxError
	}
	return index, subIndex, nil
}

func parseNetworkParam(defaultNetwork uint16, network string) (net uint64, gwError *HTTPGatewayError) {
	switch network {
	case "default", "none":
		return uint64(defaultNetwork), nil
	case "all":
		return 0, ErrUnsupportedNet

	}
	// This automatically treats 0x,0X,... correctly
	// which is allowed in the spec
	net, err := strconv.ParseUint(network, 0, 64)
	if err != nil {
		return 0, ErrSyntaxError
	}
	if net == 0 || net > 0xFFFF {
		return 0, ErrSyntaxError
	}
	return net, nil
}

func parseNodeParam(defaultNode uint8, nodeStr string) (node uint64, gwError *HTTPGatewayError) {
	switch nodeStr {
	case "default", "none":
		return uint64(defaultNode), nil
	case "all":
		return 0, ErrUnsupportedNode
	}
	// This automatically treats 0x,0X,... correctly
	// which is allowed in the spec
	node, err := strconv.ParseUint(nodeStr, 0, 64)
	if err != nil {
		return 0, ErrSyntaxError
	}
	if node == 0 || (node > 127 && node != 255) {
		return 0, ErrSyntaxError
	}
	return node, nil
}

// Handle gateway http request
func (gateway *HTTPGateway) handleRequest(w http.ResponseWriter, r *http.Request) {
	match := regURI.FindStringSubmatch(r.URL.Path)
	log.Debugf("[HTTP] new gateway request : %v", r.URL.Path)
	// hole match + 5 submatches
	if len(match) != 6 {
		w.Write(ErrSyntaxError.JSON())
		return
	}
	apiVersion := match[1]
	if apiVersion != API_VERSION {
		w.Write(ErrRequestNotSupported.JSON())
		return
	}
	sequence, err := strconv.Atoi(match[2])
	if err != nil || sequence > MAX_SEQUENCE_NB {
		w.Write(ErrSyntaxError.JSON())
		return
	}
	netStr := match[3]
	net, gwError := parseNetworkParam(gateway.defaultNetworkId, netStr)
	if gwError != nil {
		w.Write(gwError.JSON())
		return
	}
	nodeStr := match[4]
	node, gwError := parseNodeParam(gateway.defaultNodeId, nodeStr)
	if gwError != nil {
		w.Write(gwError.JSON())
		return
	}
	// Get JSON body
	var parameters json.RawMessage
	err = json.NewDecoder(r.Body).Decode(&parameters)
	if err != nil && err != io.EOF {
		log.Warnf("[HTTP] failed to unmarshal request body : %v", err)
		w.Write(ErrSyntaxError.JSON())
	}
	// Create request object
	request := &HTTPGatewayRequest{nodeId: uint8(node), networkId: uint16(net), command: match[5], sequence: uint32(sequence), parameters: parameters}
	gwErr := gateway.processRequest(w, request)
	if gwErr != nil {
		w.Write(gwErr.JSON())
		return
	}
}

// process HTPPGatewayRequest
func (gateway *HTTPGateway) processRequest(w http.ResponseWriter, request *HTTPGatewayRequest) *HTTPGatewayError {
	log.Debugf("[HTTP][x%x] processing request net %v | node %v | command %v | body %s",
		request.sequence,
		request.networkId,
		request.nodeId,
		request.command,
		request.parameters,
	)
	// NMT commands
	network := gateway.network
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
		return ErrRequestNotSupported
	}
	// Try SDO or PDO command patterns
	matchSDO := regSDO.FindStringSubmatch(request.command)
	matchPDO := regPDO.FindStringSubmatch(request.command)
	if len(matchSDO) < 2 && len(matchPDO) < 2 {
		return ErrSyntaxError
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
				log.Errorf("[HTTP] sdo error %v", err)
				abortCode, ok := err.(SDOAbortCode)
				if ok {
					return &HTTPGatewayError{SdoAbortCode: int(abortCode)}
				}
				return &HTTPGatewayError{SdoAbortCode: int(SDO_ABORT_GENERAL)}
			}
			_, err = fmt.Fprintf(w, `{"data":"0x%s"}`, hex.EncodeToString(gateway.sdoUploadBuffer[:n]))
			if err != nil {
				return ErrRequestNotProcessed
			}
			return nil
		case "w", "write":
			var sdoWrite SDOWrite
			err := json.Unmarshal(request.parameters, &sdoWrite)
			if err != nil {
				return ErrSyntaxError
			}
			datatypeValue, ok := HTTP_DATATYPE_MAP[sdoWrite.Datatype]
			if !ok {
				return ErrRequestNotSupported
			}
			encodedValue, err := encode(sdoWrite.Value, datatypeValue, 0)
			if err != nil {
				return ErrSyntaxError
			}
			err = network.WriteRaw(request.nodeId, uint16(index), uint8(subindex), encodedValue)
			if err != nil {
				log.Errorf("[HTTP] sdo error %v", err)
				abortCode, ok := err.(SDOAbortCode)
				if ok {
					return &HTTPGatewayError{SdoAbortCode: int(abortCode)}
				}
				return &HTTPGatewayError{SdoAbortCode: int(SDO_ABORT_GENERAL)}
			}
			return nil

		default:
			return ErrSyntaxError
		}
	}
	if len(matchPDO) <= 2 {
		// TODO
	}

	log.Errorf("[HTTP] request did not match any of the known commands, probably a syntax error")
	return ErrSyntaxError

}

// Create a new gateway
func NewGateway(defaultNetworkId uint16, defaultNodeId uint8, sdoUploadBufferSize int, network *Network) *HTTPGateway {
	gateway := &HTTPGateway{}
	gateway.defaultNetworkId = defaultNetworkId
	gateway.defaultNodeId = defaultNodeId
	gateway.sdoUploadBuffer = make([]byte, sdoUploadBufferSize)
	gateway.network = network
	gateway.serveMux = http.NewServeMux()
	gateway.serveMux.HandleFunc("/", gateway.handleRequest)
	return gateway
}

func (gateway *HTTPGateway) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, gateway.serveMux)
}
