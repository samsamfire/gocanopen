package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/samsamfire/gocanopen/pkg/gateway"
	log "github.com/sirupsen/logrus"
)

type GatewayClient struct {
	http.Client
	baseURL           string
	apiVersion        string
	currentSequenceNb int
	networkId         int
}

func NewGatewayClient(baseURL string, apiVersion string, networkId int) *GatewayClient {
	return &GatewayClient{
		Client:     http.Client{},
		baseURL:    baseURL,
		networkId:  networkId,
		apiVersion: apiVersion,
	}
}

// HTTP request to CiA endpoint
// Does error checking : http related errors, json decode errors
// or actual gateway errors
func (client *GatewayClient) Do(method string, uri string, body io.Reader, response GatewayResponse) error {
	client.currentSequenceNb += 1
	baseUri := client.baseURL + "/cia309-5" + fmt.Sprintf("/%s/%d/%d", client.apiVersion, client.currentSequenceNb, client.networkId)
	req, err := http.NewRequest(method, baseUri+uri, body)
	if err != nil {
		log.Errorf("[HTTP][CLIENT] http error : %v", err)
		return err
	}
	// HTTP request
	httpResp, err := client.Client.Do(req)
	if err != nil {
		log.Errorf("[HTTP][CLIENT] http error : %v", err)
		return err
	}
	// Decode JSON "generic" response
	err = json.NewDecoder(httpResp.Body).Decode(response)
	if err != nil {
		log.Errorf("[HTTP][CLIENT] error decoding json response : %v", err)
		return err
	}
	// Check for gateway errors
	err = response.GetError()
	if err != nil {
		return err
	}
	// Check for sequence nb mismatch
	sequence := response.GetSequenceNb()
	if client.currentSequenceNb != sequence {
		log.Errorf("[HTTP][CLIENT][SEQ:%v] sequence number does not match expected value (%v)", sequence, client.currentSequenceNb)
		return fmt.Errorf("error in sequence number")
	}
	return nil
}

// ReadRaw via SDO
func (client *GatewayClient) ReadRaw(nodeId uint8, index uint16, subIndex uint8) (data string, length int, err error) {
	resp := new(SDOReadResponse)
	err = client.Do(http.MethodGet, fmt.Sprintf("/%d/r/%d/%d", nodeId, index, subIndex), nil, resp)
	if err != nil {
		return
	}
	return resp.Data, resp.Length, nil
}

// WriteRaw via SDO
func (client *GatewayClient) WriteRaw(nodeId uint8, index uint16, subIndex uint8, value string, datatype string) error {
	req := new(SDOWriteRequest)
	resp := new(GatewayResponseBase)
	req.Value = value
	req.Datatype = datatype
	encodedReq, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return client.Do(http.MethodPut, fmt.Sprintf("/%d/w/%d/%d", nodeId, index, subIndex), bytes.NewBuffer(encodedReq), resp)
}

// Update SDO client timeout
func (client *GatewayClient) SetSDOTimeout(timeoutMs uint16) error {
	req := new(SDOSetTimeoutRequest)
	req.Value = "0x" + strconv.FormatInt(int64(timeoutMs), 16)
	encodedReq, err := json.Marshal(req)
	if err != nil {
		return err
	}
	resp := new(GatewayResponseBase)
	return client.Do(http.MethodPut, "/all/set/sdo-timeout", bytes.NewBuffer(encodedReq), resp)
}

// Read gateway version
func (client *GatewayClient) GetVersion() (*gateway.GatewayVersion, error) {
	versionInfo := new(VersionInfo)
	err := client.Do(http.MethodGet, "/none/info/version", nil, versionInfo)
	return versionInfo.GatewayVersion, err
}
