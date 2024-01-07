package canopen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
	confs := []*PDOConfigurator{
		NewRPDOConfigurator(NODE_ID_TEST, network.sdoClient),
		NewTPDOConfigurator(NODE_ID_TEST, network.sdoClient),
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
	rpdoConf := NewRPDOConfigurator(NODE_ID_TEST, network.sdoClient)
	tpdoConf := NewTPDOConfigurator(NODE_ID_TEST, network.sdoClient)
	err := rpdoConf.WriteInhibitTime(pdoNb, 2222)
	assert.Equal(t, SDO_ABORT_SUB_UNKNOWN, err, err)
	err = tpdoConf.WriteInhibitTime(pdoNb, 2222)
	assert.Nil(t, err)
	inhibitTime, err := tpdoConf.ReadInhibitTime(pdoNb)
	assert.Nil(t, err)
	assert.EqualValues(t, 2222, inhibitTime)
}
