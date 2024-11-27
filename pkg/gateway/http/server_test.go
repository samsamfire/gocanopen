package http

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/samsamfire/gocanopen/pkg/can/virtual"
	"github.com/samsamfire/gocanopen/pkg/network"
	"github.com/samsamfire/gocanopen/pkg/od"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

const NODE_ID_TEST = uint8(0x66)

func createNetworkEmpty() *network.Network {
	canBus, _ := network.NewBus("virtual", "localhost:18888", 0)
	bus := canBus.(*virtual.Bus)
	bus.SetReceiveOwn(true)
	network := network.NewNetwork(bus)
	e := network.Connect()
	if e != nil {
		panic(e)
	}
	return &network
}

func createNetwork() *network.Network {
	network := createNetworkEmpty()
	_, err := network.CreateLocalNode(NODE_ID_TEST, od.Default())
	if err != nil {
		panic(err)
	}
	return network
}

func createGateway() *GatewayServer {
	network := createNetwork()
	gateway := NewGatewayServer(network, 1, 1, 100)
	return gateway
}

func createClient() (*GatewayClient, *GatewayServer, *httptest.Server) {
	gw := createGateway()
	ts := httptest.NewServer(gw.serveMux)
	client := NewGatewayClient(ts.URL, API_VERSION, 1)
	return client, gw, ts
}

func TestInvalidURIs(t *testing.T) {
	client, gw, ts := createClient()
	defer gw.Disconnect()
	defer ts.Close()
	resp := new(GatewayResponseBase)
	err := client.Do(http.MethodGet, "/", nil, resp)
	assert.EqualValues(t, ErrGwSyntaxError, err)
	err = client.Do(http.MethodGet, "/10/strt//", nil, resp)
	assert.EqualValues(t, ErrGwRequestNotSupported, err)
}

func TestNMTCommand(t *testing.T) {
	client, gw, ts := createClient()
	defer gw.Disconnect()
	defer ts.Close()
	commands := []string{
		"start",
		"stop",
		"preop",
		"preoperational",
		"reset/node",
		"reset/comm",
		"reset/communication",
	}
	for _, command := range commands {
		resp := new(GatewayResponseBase)
		err := client.Do(http.MethodPut, fmt.Sprintf("/10/%s", command), nil, resp)
		assert.Nil(t, err)
		assert.NotNil(t, resp)
	}
	for _, command := range commands {
		resp := new(GatewayResponseBase)
		err := client.Do(http.MethodPut, fmt.Sprintf("/all/%s", command), nil, resp)
		assert.Nil(t, err)
		assert.NotNil(t, resp)
	}
	for _, command := range commands {
		resp := new(GatewayResponseBase)
		err := client.Do(http.MethodPut, fmt.Sprintf("/none/%s", command), nil, resp)
		assert.Nil(t, err)
		assert.NotNil(t, resp)
	}
	for _, command := range commands {
		resp := new(GatewayResponseBase)
		err := client.Do(http.MethodPut, fmt.Sprintf("/default/%s", command), nil, resp)
		assert.Nil(t, err)
		assert.NotNil(t, resp)
	}
}

func TestRead(t *testing.T) {
	client, gw, ts := createClient()
	defer gw.Disconnect()
	defer ts.Close()
	for i := uint16(0x2001); i <= 0x2009; i++ {
		_, _, err := client.ReadRaw(NODE_ID_TEST, i, 0)
		assert.Nil(t, err)
	}
}

func TestWriteRead(t *testing.T) {
	client, gw, ts := createClient()
	defer gw.Disconnect()
	defer ts.Close()
	err := client.WriteRaw(NODE_ID_TEST, 0x2002, 0, "0x10", "i8")
	assert.Nil(t, err)
	err = client.WriteRaw(NODE_ID_TEST, 0x2003, 0, "0x5432", "i16")
	assert.Nil(t, err)
	data, _, _ := client.ReadRaw(NODE_ID_TEST, 0x2003, 0)
	assert.Equal(t, "0x5432", data)
}

func TestSDOTimeout(t *testing.T) {
	client, gw, ts := createClient()
	defer gw.Disconnect()
	defer ts.Close()
	err := client.SetSDOTimeout(1222)
	assert.Nil(t, err)
}

func TestGetVersion(t *testing.T) {
	client, gw, ts := createClient()
	defer gw.Disconnect()
	defer ts.Close()
	version, err := client.GetVersion()
	assert.Nil(t, err)
	assert.Equal(t, "02.01", version.ProtocolVersion)
}
