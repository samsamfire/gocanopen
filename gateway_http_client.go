package canopen

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

type HTTPGatewayClient struct {
	BaseURL           string
	ApiVersion        string
	CurrentSequenceNb int
	NetworkId         int
}

func NewHTTPGatewayClient(baseURL string, apiVersion string, networkId int) *HTTPGatewayClient {
	return &HTTPGatewayClient{BaseURL: baseURL, NetworkId: networkId, ApiVersion: apiVersion}
}

type HTTPGatewayResponse struct {
	Sequence int    `json:"sequence,omitempty"`
	Data     string `json:"data,omitempty"`
	Length   int    `json:"length,omitempty"`
	Response string `json:"response,omitempty"`
}

// Get the actual json response, return error if bad http or json decode error
func (client *HTTPGatewayClient) get(uri string) (resp *HTTPGatewayResponse, err error) {
	client.CurrentSequenceNb += 1
	newUri := client.BaseURL + "/cia309-5" + fmt.Sprintf("/%s/%d/%d", client.ApiVersion, client.CurrentSequenceNb, client.NetworkId)
	httpResponse, err := http.Get(newUri + uri)
	if err != nil {
		log.Errorf("[HTTP][CLIENT] http request errored : %v", err)
		return
	}
	// Get JSON response
	jsonRsp := new(HTTPGatewayResponse)
	err = json.NewDecoder(httpResponse.Body).Decode(jsonRsp)
	if err != nil {
		log.Errorf("[HTTP][CLIENT] error decoding json response : %v", err)
		return
	}
	// Check if any gateway errors
	if strings.HasPrefix(jsonRsp.Response, "ERROR:") {
		responseSplitted := strings.Split(jsonRsp.Response, ":")
		if len(responseSplitted) != 2 {
			log.Errorf("[HTTP][CLIENT] error decoding error field ('ERROR:xxxx') inside json response : %v", err)
			err = fmt.Errorf("error decoding error field ('ERROR:' : %v, %T)", jsonRsp.Response, jsonRsp.Response)
			return
		}
		var errorCode uint64
		errorCode, err = strconv.ParseUint(responseSplitted[1], 0, 64)
		if err != nil {
			return
		}
		err = &HTTPGatewayError{Code: int(errorCode)}
		log.Warnf("[HTTP][CLIENT][SEQ:%v] command resulted in error from server : %v", jsonRsp.Sequence, err)
		return
	}
	// Check if sequence number is correct
	if client.CurrentSequenceNb != jsonRsp.Sequence {
		log.Warnf("[HTTP][CLIENT][SEQ:%v] sequence number does not match expected value (%v)", jsonRsp.Sequence, client.CurrentSequenceNb)
	}
	return jsonRsp, nil
}

// Read via SDO
func (client *HTTPGatewayClient) Read(nodeId uint8, index uint16, subIndex uint8) (data string, length int, err error) {
	resp, err := client.get(fmt.Sprintf("/%d/r/%d/%d", nodeId, index, subIndex))
	if err != nil {
		return
	}
	return resp.Data, 0, nil
}

// Write via SDO
// func (client *HTTPGatewayClient) Write(nodeId uint8, index uint16, subIndex uint8, data string) error {
// 	resp, err := client.get(fmt.Sprintf("/%d/w/%d/%d", nodeId, index, subIndex))
// 	if err != nil {
// 		return
// 	}
// 	return resp.Data, 0, nil
// }
