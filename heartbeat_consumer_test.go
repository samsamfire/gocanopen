package canopen

import (
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

var receivedErrorCodes []uint16

func emCallback(ident uint16, errorCode uint16, errorRegister byte, errorBit byte, infoCode uint32) {
	log.Debug("received emergency")
	receivedErrorCodes = append(receivedErrorCodes, errorCode)
}

func TestHBConfigurator(t *testing.T) {
	network := createNetwork()
	node := network.Nodes[NODE_ID_TEST].(*LocalNode)
	node.EM.SetCallback(emCallback)
	defer network.Disconnect()
	config := network.Configurator(NODE_ID_TEST)
	config.HB.WriteMonitoredNode(1, 0x25, 100)
	//Test duplicate entry
	config.HB.WriteMonitoredNode(2, 0x25, 100)
	err := config.HB.WriteMonitoredNode(3, 0x25, 100)
	assert.Equal(t, err, SDO_ABORT_PRAM_INCOMPAT)
	network.CreateNode(0x25, "testdata/base.eds")
	max, _ := config.HB.ReadMaxMonitorable()
	// Test that we receive at least one emergency
	assert.EqualValues(t, 8, max)
	time.Sleep(1 * time.Second)
	assert.GreaterOrEqual(t, len(receivedErrorCodes), 1)
}
