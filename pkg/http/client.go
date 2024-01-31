package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

type GatewayClient struct {
	client            *http.Client
	baseURL           string
	apiVersion        string
	currentSequenceNb int
	networkId         int
}

func NewGatewayClient(baseURL string, apiVersion string, networkId int) *GatewayClient {
	return &GatewayClient{
		client:     &http.Client{},
		baseURL:    baseURL,
		networkId:  networkId,
		apiVersion: apiVersion,
	}
}

// Extract error if any inside of reponse
func (resp *GatewayResponse) GetError() error {
	// Check if any gateway errors
	if !strings.HasPrefix(resp.Response, "ERROR:") {
		return nil
	}
	responseSplitted := strings.Split(resp.Response, ":")
	if len(responseSplitted) != 2 {
		return fmt.Errorf("error decoding error field ('ERROR:' : %v)", resp.Response)
	}
	var errorCode uint64
	errorCode, err := strconv.ParseUint(responseSplitted[1], 0, 64)
	if err != nil {
		return fmt.Errorf("error decoding error field ('ERROR:' : %v)", err)
	}
	return NewGatewayError(int(errorCode))
}

// HTTP request to CiA endpoint
// Does high level error checking : http related errors, json decode errors
// or wrong sequence number
func (client *GatewayClient) do(method string, uri string, body io.Reader) (resp *GatewayResponse, err error) {
	client.currentSequenceNb += 1
	baseUri := client.baseURL + "/cia309-5" + fmt.Sprintf("/%s/%d/%d", client.apiVersion, client.currentSequenceNb, client.networkId)
	req, err := http.NewRequest(method, baseUri+uri, body)
	if err != nil {
		log.Errorf("[HTTP][CLIENT] http error : %v", err)
		return nil, err
	}
	// HTTP request
	httpResp, err := client.client.Do(req)
	if err != nil {
		log.Errorf("[HTTP][CLIENT] http error : %v", err)
		return nil, err
	}
	// Decode JSON "generic" response
	jsonRsp := new(GatewayResponse)
	err = json.NewDecoder(httpResp.Body).Decode(jsonRsp)
	if err != nil {
		log.Errorf("[HTTP][CLIENT] error decoding json response : %v", err)
		return nil, err
	}
	// Check if sequence number is correct
	sequence, err := strconv.Atoi(jsonRsp.Sequence)
	if client.currentSequenceNb != sequence || err != nil {
		log.Errorf("[HTTP][CLIENT][SEQ:%v] sequence number does not match expected value (%v)", jsonRsp.Sequence, client.currentSequenceNb)
		return nil, fmt.Errorf("error in sequence number")
	}
	return jsonRsp, nil
}

// Read via SDO
func (client *GatewayClient) Read(nodeId uint8, index uint16, subIndex uint8) (data string, length int, err error) {
	resp, err := client.do(http.MethodGet, fmt.Sprintf("/%d/r/%d/%d", nodeId, index, subIndex), nil)
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
