package canopen

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

const NODE_GW_TEST_LOCAL_ID = uint8(0x66)

func createClient() (*HTTPGatewayClient, func()) {
	gw := createGateway()
	ts := httptest.NewServer(gw.serveMux)
	client := &HTTPGatewayClient{ts.URL, "1.0", 0, 1}
	return client, func() {
		defer gw.network.Disconnect()
	}
}

func TestInvalidURIs(t *testing.T) {
	client, close := createClient()
	defer close()
	_, err := client.get("/")
	assert.EqualValues(t, ErrGwSyntaxError, err)
	_, err = client.get("/10/start//")
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
		resp, err := client.get(fmt.Sprintf("/10/%s", command))
		assert.Nil(t, err)
		assert.NotNil(t, resp)
	}
	for _, command := range commands {
		resp, err := client.get(fmt.Sprintf("/all/%s", command))
		assert.Nil(t, err)
		assert.NotNil(t, resp)
	}
	for _, command := range commands {
		resp, err := client.get(fmt.Sprintf("/none/%s", command))
		assert.Nil(t, err)
		assert.NotNil(t, resp)
	}
	for _, command := range commands {
		resp, err := client.get(fmt.Sprintf("/default/%s", command))
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
