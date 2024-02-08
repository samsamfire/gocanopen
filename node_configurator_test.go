package canopen

import (
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestNmtConfigurator(t *testing.T) {
	network := createNetwork()
	defer network.Disconnect()
	config := network.Configurator(NODE_ID_TEST).NMT
	val, _ := config.ReadHeartbeatPeriod()
	assert.EqualValues(t, 1000, val)
	config.WriteHeartbeatPeriod(900)
	val, _ = config.ReadHeartbeatPeriod()
	assert.EqualValues(t, val, 900)
}

func TestSyncConfigurator(t *testing.T) {
	network := createNetwork()
	defer network.Disconnect()
	conf := network.Configurator(NODE_ID_TEST).SYNC

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

var TEST_MAPPING = []PDOMapping{
	{Index: 0x2001, Subindex: 0x0, LengthBits: 8},
	{Index: 0x2002, Subindex: 0x0, LengthBits: 8},
	{Index: 0x2003, Subindex: 0x0, LengthBits: 16},
	{Index: 0x2004, Subindex: 0x0, LengthBits: 32},
}

// Test everything common to RPDO & TPDO
func TestPDOConfiguratorCommon(t *testing.T) {
	pdoNb := uint16(1)
	network := createNetwork()
	defer network.Disconnect()
	confs := []pdoConfigurator{
		network.Configurator(NODE_ID_TEST).RPDO,
		network.Configurator(NODE_ID_TEST).TPDO,
	}
	for _, conf := range confs {
		err := conf.WriteEventTimer(pdoNb, 1111)
		assert.Nil(t, err)
		eventTimer, _ := conf.ReadEventTimer(pdoNb)
		assert.EqualValues(t, 1111, eventTimer)
		err = conf.WriteTransmissionType(pdoNb, 11)
		assert.Nil(t, err)
		transType, _ := conf.ReadTransmissionType(pdoNb)
		assert.EqualValues(t, 11, transType)
		err = conf.Disable(pdoNb)
		assert.Nil(t, err)
		enabled, _ := conf.ReadEnabled(pdoNb)
		assert.Equal(t, false, enabled)
		err = conf.ClearMappings(pdoNb)
		assert.Nil(t, err)
		err = conf.WriteMappings(pdoNb, TEST_MAPPING)
		assert.Nil(t, err)
		mappingsFdbk, _ := conf.ReadMappings(pdoNb)
		assert.Equal(t, TEST_MAPPING, mappingsFdbk)
		err = conf.WriteCanId(pdoNb, 0x211)
		assert.Nil(t, err)
		cobId, _ := conf.ReadCobId(pdoNb)
		assert.EqualValues(t, 0x211, cobId&0x7FF)
		config := PDOConfiguration{
			CanId:            0x255,
			TransmissionType: 22,
			EventTimer:       1400,
			InhibitTime:      1200,
			Mappings:         TEST_MAPPING,
		}
		err = conf.WriteConfiguration(pdoNb, config)
		assert.Nil(t, err)
		readConfig, _ := conf.ReadConfiguration(pdoNb)
		assert.EqualValues(t, config.CanId, readConfig.CanId)
		assert.EqualValues(t, config.TransmissionType, readConfig.TransmissionType)
		assert.EqualValues(t, config.EventTimer, readConfig.EventTimer)
		assert.EqualValues(t, config.Mappings, readConfig.Mappings)

		err = conf.Enable(pdoNb)
		assert.Nil(t, err)
	}
}

func TestPDOConfiguratorNotCommon(t *testing.T) {
	pdoNb := uint16(1)
	network := createNetwork()
	defer network.Disconnect()
	rpdoConf := network.Configurator(NODE_ID_TEST).RPDO
	tpdoConf := network.Configurator(NODE_ID_TEST).TPDO
	err := rpdoConf.WriteInhibitTime(pdoNb, 2222)
	assert.Equal(t, SDO_ABORT_SUB_UNKNOWN, err, err)
	err = tpdoConf.WriteInhibitTime(pdoNb, 2222)
	assert.Nil(t, err)
	inhibitTime, err := tpdoConf.ReadInhibitTime(pdoNb)
	assert.Nil(t, err)
	assert.EqualValues(t, 2222, inhibitTime)
}

var receivedErrorCodes []uint16

func emCallback(ident uint16, errorCode uint16, errorRegister byte, errorBit byte, infoCode uint32) {
	log.Debug("received emergency")
	receivedErrorCodes = append(receivedErrorCodes, errorCode)
}

func TestHBConfigurator(t *testing.T) {
	network := createNetwork()
	defer network.Disconnect()
	node := network.nodes[NODE_ID_TEST].(*LocalNode)
	node.EMCY.SetCallback(emCallback)
	config := network.Configurator(NODE_ID_TEST).HB
	config.WriteMonitoredNode(1, 0x25, 100)
	//Test duplicate entry
	config.WriteMonitoredNode(2, 0x25, 100)
	err := config.WriteMonitoredNode(3, 0x25, 100)
	assert.Equal(t, err, SDO_ABORT_PRAM_INCOMPAT)
	network.CreateLocalNode(0x25, "testdata/base.eds")
	max, _ := config.ReadMaxMonitorable()
	// Test that we receive at least one emergency
	assert.EqualValues(t, 8, max)
	time.Sleep(1 * time.Second)
	assert.GreaterOrEqual(t, len(receivedErrorCodes), 1)
	monitoredNodes, err := config.ReadMonitoredNodes()
	assert.Nil(t, err)
	assert.Len(t, monitoredNodes, 8)
}

func TestTimeConfigurator(t *testing.T) {
	network := createNetwork()
	defer network.Disconnect()
	conf := network.Configurator(NODE_ID_TEST).TIME
	err := conf.ProducerEnable()
	assert.Nil(t, err)
	node := network.nodes[NODE_ID_TEST].(*LocalNode)
	assert.Equal(t, true, node.TIME.isProducer)
	err = conf.ProducerDisable()
	assert.Nil(t, err)
	assert.Equal(t, false, node.TIME.isProducer)
	err = conf.ConsumerDisable()
	assert.Nil(t, err)
	assert.Equal(t, false, node.TIME.isConsumer)
	err = conf.ConsumerEnable()
	assert.Nil(t, err)
	assert.Equal(t, true, node.TIME.isConsumer)
}