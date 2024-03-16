package http

import "fmt"

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
	ErrGwRequestNotSupported         = &GatewayError{Code: 100}
	ErrGwSyntaxError                 = &GatewayError{Code: 101}
	ErrGwRequestNotProcessed         = &GatewayError{Code: 102}
	ErrGwTimeout                     = &GatewayError{Code: 103}
	ErrGwNoDefaultNetSet             = &GatewayError{Code: 104}
	ErrGwNoDefaultNodeSet            = &GatewayError{Code: 105}
	ErrGwUnsupportedNet              = &GatewayError{Code: 106}
	ErrGwUnsupportedNode             = &GatewayError{Code: 107}
	ErrGwCommandCancellationFailed   = &GatewayError{Code: 108}
	ErrGwEmergencyConsumerNotEnabled = &GatewayError{Code: 109}
	ErrGwWrongNMTState               = &GatewayError{Code: 204}
	ErrGwWrongPassword               = &GatewayError{Code: 300}
	ErrGwSuperUsersExceeded          = &GatewayError{Code: 301}
	ErrGwNodeAccessDenied            = &GatewayError{Code: 302}
	ErrGwNoSessionAvailable          = &GatewayError{Code: 303}
	ErrGwPDOAlreadyUsed              = &GatewayError{Code: 400}
	ErrGwPDOLengthExceeded           = &GatewayError{Code: 401}
	ErrGwLSSImplementationError      = &GatewayError{Code: 501}
	ErrGwLSSNodeIDNotSupported       = &GatewayError{Code: 502}
	ErrGwLSSBitRateNotSupported      = &GatewayError{Code: 503}
	ErrGwLSSParameterStoringFailed   = &GatewayError{Code: 504}
	ErrGwLSSCommandFailed            = &GatewayError{Code: 505}
	ErrGwRunningOutOfMemory          = &GatewayError{Code: 600}
	ErrGwCANInterfaceNotAvailable    = &GatewayError{Code: 601}
	ErrGwSizeLowerThanSDOBufferSize  = &GatewayError{Code: 602}
	ErrGwManufacturerSpecificError   = &GatewayError{Code: 900}
)

type GatewayError struct {
	Code int // Can be either an sdo abort code or a gateway error code
}

func NewGatewayError(code int) error {
	return &GatewayError{Code: code}
}

func (e *GatewayError) Error() string {
	if e.Code <= 999 {
		return fmt.Sprintf("ERROR:%d", e.Code)
	}
	// Return as a hex value (sdo aborts)
	return fmt.Sprintf("ERROR:0x%x", e.Code)
}
