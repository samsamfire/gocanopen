package network

import (
	"testing"
	"time"

	"github.com/samsamfire/gocanopen/pkg/config"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/samsamfire/gocanopen/pkg/sdo"
	"github.com/stretchr/testify/assert"
)

func TestSyncConfigurator(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	conf := network.Configurator(NodeIdTest)

	// Test Sync update producer cob id & possible errors
	err := conf.ProducerEnableSYNC()
	assert.Nil(t, err)
	err = conf.WriteCanIdSYNC(0x81)
	assert.Nil(t, err)
	err = conf.ProducerDisableSYNC()
	assert.Nil(t, err)
	err = conf.WriteCanIdSYNC(0x81)
	assert.Nil(t, err)

	// Test Sync update counter overflow & possible errors
	err = conf.WriteCommunicationPeriod(100_100)
	assert.Nil(t, err)
	commPeriod, _ := conf.ReadCommunicationPeriod()
	assert.EqualValues(t, 100_100, commPeriod)
	err = conf.WriteCounterOverflow(100)
	assert.Equal(t, sdo.AbortDataDeviceState, err)
	err = conf.WriteCommunicationPeriod(0)
	assert.Nil(t, err)
	err = conf.WriteCounterOverflow(250)
	assert.Equal(t, sdo.AbortInvalidValue, err)
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

var TEST_MAPPING = []config.PDOMappingParameter{
	{Index: 0x2001, Subindex: 0x0, LengthBits: 8},
	{Index: 0x2002, Subindex: 0x0, LengthBits: 8},
	{Index: 0x2003, Subindex: 0x0, LengthBits: 16},
	{Index: 0x2004, Subindex: 0x0, LengthBits: 32},
}

// Test everything common to RPDO & TPDO
func TestPDOConfiguratorCommon(t *testing.T) {
	pdos := []uint16{1, 257}
	network := CreateNetworkTest()
	defer network.Disconnect()

	conf := network.Configurator(NodeIdTest)

	t.Run("pdo configurations", func(t *testing.T) {
		for _, pdoNb := range pdos {
			err := conf.WriteEventTimer(pdoNb, 1111)
			assert.Nil(t, err)
			eventTimer, _ := conf.ReadEventTimer(pdoNb)
			assert.EqualValues(t, 1111, eventTimer)
			err = conf.WriteTransmissionType(pdoNb, 11)
			assert.Nil(t, err)
			transType, _ := conf.ReadTransmissionType(pdoNb)
			assert.EqualValues(t, 11, transType)
			err = conf.DisablePDO(pdoNb)
			assert.Nil(t, err)
			enabled, _ := conf.ReadEnabledPDO(pdoNb)
			assert.Equal(t, false, enabled)
			err = conf.ClearMappings(pdoNb)
			assert.Nil(t, err)
			err = conf.WriteMappings(pdoNb, TEST_MAPPING)
			assert.Nil(t, err)
			mappingsFdbk, _ := conf.ReadMappings(pdoNb)
			assert.Equal(t, TEST_MAPPING, mappingsFdbk)
			err = conf.WriteCanIdPDO(pdoNb, 0x211)
			assert.Nil(t, err)
			cobId, _ := conf.ReadCobIdPDO(pdoNb)
			assert.EqualValues(t, 0x211, cobId&0x7FF)
			config := config.PDOConfigurationParameter{
				CanId:            0x255,
				TransmissionType: 22,
				EventTimer:       1400,
				InhibitTime:      1200,
				Mappings:         TEST_MAPPING,
			}
			err = conf.WriteConfigurationPDO(pdoNb, config)
			assert.Nil(t, err)
			readConfig, _ := conf.ReadConfigurationPDO(pdoNb)
			assert.EqualValues(t, config.CanId, readConfig.CanId)
			assert.EqualValues(t, config.TransmissionType, readConfig.TransmissionType)
			assert.EqualValues(t, config.EventTimer, readConfig.EventTimer)
			assert.EqualValues(t, config.Mappings, readConfig.Mappings)

			err = conf.EnablePDO(pdoNb)
			assert.Nil(t, err)
		}
	})

	t.Run("read all pdo configurations", func(t *testing.T) {
		rpdos, tpdos, err := conf.ReadConfigurationAllPDO()
		assert.NotNil(t, rpdos)
		assert.NotNil(t, tpdos)
		assert.Nil(t, err)
		assert.Len(t, rpdos, 4)
		assert.Len(t, tpdos, 4)
	})
}

func TestPDOConfiguratorNotCommon(t *testing.T) {

	network := CreateNetworkTest()
	defer network.Disconnect()
	conf := network.Configurator(NodeIdTest)
	err := conf.WriteInhibitTime(1, 2222)
	assert.Nil(t, err)
	err = conf.WriteInhibitTime(257, 2222)
	assert.Nil(t, err)
	inhibitTime, err := conf.ReadInhibitTime(257)
	assert.Nil(t, err)
	assert.EqualValues(t, 2222, inhibitTime)
}

var receivedErrorCodes []uint16

func emCallback(ident uint16, errorCode uint16, errorRegister byte, errorBit byte, infoCode uint32) {
	receivedErrorCodes = append(receivedErrorCodes, errorCode)
}

func TestHBConfigurator(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	node, _ := network.Local(NodeIdTest)
	node.EMCY.SetCallback(emCallback)
	config := network.Configurator(NodeIdTest)
	err := config.WriteMonitoredNode(1, 0x25, 100)
	assert.Nil(t, err)
	// Test duplicate entry
	err = config.WriteMonitoredNode(3, 0x25, 100)
	assert.Equal(t, err, sdo.AbortParamIncompat)
	_, err = network.CreateLocalNode(0x25, od.Default())
	assert.Nil(t, err)
	max, _ := config.ReadMaxMonitorableNodes()
	// Test that we receive at least one emergency
	assert.EqualValues(t, 8, max)
	time.Sleep(1 * time.Second)
	assert.GreaterOrEqual(t, len(receivedErrorCodes), 1)
	monitoredNodes, err := config.ReadMonitoredNodes()
	assert.Nil(t, err)
	assert.Len(t, monitoredNodes, 8)
	// Test hearbeat update / read
	val, _ := config.ReadHeartbeatPeriod()
	assert.EqualValues(t, 1000, val)
	err = config.WriteHeartbeatPeriod(900)
	assert.Nil(t, err)
	val, _ = config.ReadHeartbeatPeriod()
	assert.EqualValues(t, val, 900)
}

func TestTimeConfigurator(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	conf := network.Configurator(NodeIdTest)
	node, _ := network.Local(NodeIdTest)
	err := conf.ProducerEnableTIME()
	assert.Nil(t, err)
	assert.Equal(t, true, node.TIME.Producer())
	err = conf.ProducerDisableTIME()
	assert.Nil(t, err)
	assert.Equal(t, false, node.TIME.Producer())
	err = conf.ProducerEnableTIME()
	assert.Nil(t, err)
	assert.Equal(t, true, node.TIME.Producer())
	err = conf.ConsumerDisableTIME()
	assert.Nil(t, err)
	assert.Equal(t, false, node.TIME.Consumer())
	err = conf.ConsumerEnableTIME()
	assert.Nil(t, err)
	assert.Equal(t, true, node.TIME.Consumer())
	err = conf.ConsumerDisableTIME()
	assert.Nil(t, err)
	assert.Equal(t, false, node.TIME.Consumer())
}

func TestGeneralObjects(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	conf := network.Configurator(NodeIdTest)
	name, err := conf.ReadManufacturerDeviceName()
	assert.Nil(t, err)
	assert.Equal(t, "DUT", name)
	name, err = conf.ReadManufacturerHardwareVersion()
	assert.Nil(t, err)
	assert.Equal(t, "v400", name)
	name, err = conf.ReadManufacturerSoftwareVersion()
	assert.Nil(t, err)
	assert.Equal(t, "v1.1.2r", name)
	identity, err := conf.ReadIdentity()
	assert.Nil(t, err)
	assert.EqualValues(t, 0, identity.VendorId)
	manufInfo := conf.ReadManufacturerInformation()
	assert.Equal(t, config.ManufacturerInformation{
		ManufacturerDeviceName:      "DUT",
		ManufacturerHardwareVersion: "v400",
		ManufacturerSoftwareVersion: "v1.1.2r",
	}, manufInfo)
}
