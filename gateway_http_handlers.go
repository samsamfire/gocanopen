package canopen

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"

	log "github.com/sirupsen/logrus"
)

// A ResponseWriter but keeps track of any writes already done
// This is useful for custom processing in each handler
// But adding default behaviour for errors / success

type doneWriter struct {
	http.ResponseWriter
	done bool
}

func (w *doneWriter) WriteHeader(status int) {
	w.done = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *doneWriter) Write(b []byte) (int, error) {
	w.done = true
	return w.ResponseWriter.Write(b)
}

// Gets an HTTP request and handles it according to CiA 309-5
type HTTPRequestHandler func(w doneWriter, req *HTTPGatewayRequest) error

func createNmtHandler(bg *BaseGateway, command NMTCommand) HTTPRequestHandler {
	return func(w doneWriter, req *HTTPGatewayRequest) error {
		switch req.nodeId {
		case TOKEN_DEFAULT, TOKEN_NONE:
			return bg.NMTCommand(bg.defaultNodeId, command)
		case TOKEN_ALL:
			return bg.NMTCommand(0, command)
		default:
			return bg.NMTCommand(uint8(req.nodeId), command)
		}
	}
}

// Can be used for specifying some routes that can be implemented in CiA 309
// But are not in this gateway
func handlerNotSupported(w doneWriter, req *HTTPGatewayRequest) error {
	return ErrGwRequestNotSupported
}

// Handle a read
// This includes different type of handlers : SDO, PDO, ...
func (gw *HTTPGatewayServer) handlerRead(w doneWriter, req *HTTPGatewayRequest) error {
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

func (gw *HTTPGatewayServer) handlerSDORead(w doneWriter, req *HTTPGatewayRequest, commands []string) error {
	index, subindex, err := parseSdoCommand(commands[1:])
	if err != nil {
		log.Errorf("[HTTP][SERVER] unable to parse SDO command : %v", err)
		return err
	}
	net := gw.base.network
	buffer := gw.sdoBuffer

	n, err := net.ReadRaw(uint8(req.nodeId), uint16(index), uint8(subindex), buffer)
	if err != nil {
		w.Write(NewResponseError(int(req.sequence), err))
		return nil
	}
	sdoResp := httpSDOReadResponse{
		Sequence: int(req.sequence),
		Response: "OK",
		Data:     "0x" + hex.EncodeToString(buffer[:n]),
		Length:   n,
	}
	sdoResRaw, err := json.Marshal(sdoResp)
	if err != nil {
		return ErrGwRequestNotProcessed
	}
	w.Write(sdoResRaw)
	return nil
}

// Handle a write
// This includes different type of handlers : SDO, PDO, ...
func (gw *HTTPGatewayServer) handleWrite(w doneWriter, req *HTTPGatewayRequest) error {
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

func (gw *HTTPGatewayServer) handlerSDOWrite(w doneWriter, req *HTTPGatewayRequest, commands []string) error {
	index, subindex, err := parseSdoCommand(commands[1:])
	if err != nil {
		log.Errorf("[HTTP][SERVER] unable to parse SDO command : %v", err)
		return err
	}
	net := gw.base.network

	var sdoWrite httpSDOWriteRequest
	err = json.Unmarshal(req.parameters, &sdoWrite)
	if err != nil {
		return ErrGwSyntaxError
	}
	datatype, ok := HTTP_DATATYPE_MAP[sdoWrite.Datatype]
	if !ok {
		log.Errorf("[HTTP][SERVER] requested datatype is either wrong or unsupported : %v", sdoWrite.Datatype)
		return ErrGwRequestNotSupported
	}
	encodedValue, err := encode(sdoWrite.Value, datatype, 0)
	if err != nil {
		return ErrGwSyntaxError
	}
	err = net.WriteRaw(uint8(req.nodeId), uint16(index), uint8(subindex), encodedValue)
	if err != nil {
		w.Write(NewResponseError(int(req.sequence), err))
		return nil
	}
	return nil
}

// Update SDO client timeout
func (gw *HTTPGatewayServer) handleSDOTimeout(w doneWriter, req *HTTPGatewayRequest) error {

	var sdoTimeout httpSDOTimeoutRequest
	err := json.Unmarshal(req.parameters, &sdoTimeout)
	if err != nil {
		return ErrGwSyntaxError
	}
	sdoTimeoutInt, err := strconv.ParseUint(sdoTimeout.Value, 0, 64)
	if err != nil || sdoTimeoutInt > 0xFFFF {
		return ErrGwSyntaxError
	}
	return gw.base.SetSDOTimeout(uint32(sdoTimeoutInt))
}
