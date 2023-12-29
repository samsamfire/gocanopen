package canopen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
		err = conf.ClearMappings(pdoNb)
		assert.Nil(t, err)
		mappings := []PDOMapping{
			{Index: 0x2001, Subindex: 0x0, LengthBits: 8},
			{Index: 0x2002, Subindex: 0x0, LengthBits: 8},
			{Index: 0x2003, Subindex: 0x0, LengthBits: 16},
			{Index: 0x2004, Subindex: 0x0, LengthBits: 32},
		}
		err = conf.UpdateMappings(pdoNb, mappings)
		assert.Nil(t, err)
		mappingsFdbk, _ := conf.ReadMappings(pdoNb)
		assert.Equal(t, mappings, mappingsFdbk)
		err = conf.WriteCanId(pdoNb, 0x211)
		assert.Nil(t, err)
		cobId, _ := conf.ReadCobId(pdoNb)
		assert.EqualValues(t, 0x211, cobId&0x7FF)
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
