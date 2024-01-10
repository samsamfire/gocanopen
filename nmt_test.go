package canopen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNMT(t *testing.T) {

	network := createNetwork()
	defer network.Disconnect()
	config := network.Configurator(NODE_ID_TEST)
	val, _ := config.NMT.ReadHeartbeatPeriod()
	assert.EqualValues(t, 1000, val)
	config.NMT.WriteHeartbeatPeriod(900)
	val, _ = config.NMT.ReadHeartbeatPeriod()
	assert.EqualValues(t, val, 900)
}
