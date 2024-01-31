package http

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	canopen "github.com/samsamfire/gocanopen"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

const NODE_ID_TEST = uint8(0x66)

func init() {
	log.SetLevel(log.DebugLevel)
}

func createNetworkEmpty() *canopen.Network {
	bus := canopen.NewVirtualCanBus("localhost:18888")
	bus.SetReceiveOwn(true)
	network := canopen.NewNetwork(bus)
	e := network.Connect()
	if e != nil {
		panic(e)
	}
	return &network
}

func createNetwork() *canopen.Network {
	network := createNetworkEmpty()
	_, err := network.CreateLocalNode(NODE_ID_TEST, "../../testdata/base.eds")
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

func createClient() (*GatewayClient, func()) {
	gw := createGateway()
	ts := httptest.NewServer(gw.serveMux)
	client := NewGatewayClient(ts.URL, API_VERSION, 1)
	return client, func() {
		defer gw.Disconnect()
	}
}

func TestInvalidURIs(t *testing.T) {
	client, close := createClient()
	defer close()
	_, err := client.do(http.MethodGet, "/", nil)
	assert.EqualValues(t, ErrGwSyntaxError, err)
	_, err = client.do(http.MethodGet, "/10/start//", nil)
	assert.EqualValues(t, ErrGwSyntaxError, err)
}

func TestNMTCommand(t *testing.T) {
	client, close := createClient()
	defer close()
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
		resp, err := client.do(http.MethodPut, fmt.Sprintf("/10/%s", command), nil)
		assert.Nil(t, err)
		assert.NotNil(t, resp)
	}
	for _, command := range commands {
		resp, err := client.do(http.MethodPut, fmt.Sprintf("/all/%s", command), nil)
		assert.Nil(t, err)
		assert.NotNil(t, resp)
	}
	for _, command := range commands {
		resp, err := client.do(http.MethodPut, fmt.Sprintf("/none/%s", command), nil)
		assert.Nil(t, err)
		assert.NotNil(t, resp)
	}
	for _, command := range commands {
		resp, err := client.do(http.MethodPut, fmt.Sprintf("/default/%s", command), nil)
		assert.Nil(t, err)
		assert.NotNil(t, resp)
	}
}

func TestSDOAccessCommands(t *testing.T) {
	client, close := createClient()
	defer close()
	for i := uint16(0x2001); i <= 0x2009; i++ {
		_, _, err := client.Read(NODE_ID_TEST, i, 0)
		assert.Nil(t, err)
	}
}
