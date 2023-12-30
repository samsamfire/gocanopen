package canopen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSyncConfigurator(t *testing.T) {
	network := createNetwork()
	defer network.Disconnect()
	conf := NewSYNCConfigurator(NODE_ID_TEST, network.sdoClient)

	// Test Sync update producer cob id & possible errors
	err := conf.ProducerEnable()
	assert.Nil(t, err)
	err = conf.WriteCanId(0x81)
	assert.Nil(t, err)
	err = conf.ProducerDisable()
	assert.Nil(t, err)
	err = conf.WriteCanId(0x81)
	assert.Nil(t, err)

	// Test Sync update counter overflow & possible errors
	err = conf.WriteCommunicationPeriod(100_100)
	assert.Nil(t, err)
	commPeriod, _ := conf.ReadCommunicationPeriod()
	assert.EqualValues(t, 100_100, commPeriod)
	err = conf.WriteCounterOverflow(100)
	assert.Equal(t, SDO_ABORT_DATA_DEV_STATE, err)
	conf.WriteCommunicationPeriod(0)
	err = conf.WriteCounterOverflow(250)
	assert.Equal(t, SDO_ABORT_INVALID_VALUE, err)
	err = conf.WriteCounterOverflow(10)
	assert.Nil(t, err)
	counterOverflow, err := conf.ReadCounterOverflow()
	assert.Nil(t, err, err)
	assert.EqualValues(t, 10, counterOverflow)
	err = conf.WriteWindowLengthPdos(110)
	assert.Nil(t, err)
	windowPdos, _ := conf.ReadWindowLengthPdos()
	assert.EqualValues(t, 110, windowPdos)
}
