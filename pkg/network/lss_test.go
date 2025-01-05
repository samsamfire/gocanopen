package network

import (
	"testing"
	"time"

	"github.com/samsamfire/gocanopen/pkg/lss"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/stretchr/testify/assert"
)

func TestLSSSwitch(t *testing.T) {
	network := CreateNetworkEmptyTest()
	network2 := CreateNetworkEmptyTest()
	defer network.Disconnect()
	defer network2.Disconnect()

	slaveOd := od.Default()
	// vendor, product, revision, sn
	slaveOd.Index(0x1018).PutUint32(1, 0xFF, true)
	slaveOd.Index(0x1018).PutUint32(2, 1234, true)
	slaveOd.Index(0x1018).PutUint32(3, 567, true)
	slaveOd.Index(0x1018).PutUint32(4, 1111, true)

	slave, err := network.CreateLocalNode(NodeIdTest, slaveOd)
	assert.Nil(t, err)
	assert.NotNil(t, slave)
	identity, _ := slave.Configurator().ReadIdentity()
	assert.EqualValues(t, 1111, identity.SerialNumber)

	master := network2.LSS()

	t.Run("switch state global", func(t *testing.T) {
		err := master.SwitchStateGlobal(lss.ModeConfiguration)
		assert.Nil(t, err)

		// Check that slave moves to configuration state
		assert.Eventually(t, func() bool {
			return slave.LSSSlave().GetState() == lss.StateConfiguration
		}, 5*time.Second, 10*time.Millisecond)

		// Check that slave moves to waiting state
		err = master.SwitchStateGlobal(lss.ModeWaiting)
		assert.Nil(t, err)
		assert.Eventually(t, func() bool {
			return slave.LSSSlave().GetState() == lss.StateWaiting
		}, 5*time.Second, 10*time.Millisecond)

	})

	t.Run("switch state selective", func(t *testing.T) {
		err := master.SwitchStateSelective(lss.LSSAddress{Identity: *identity})
		assert.Nil(t, err)
		assert.Equal(t, lss.StateConfiguration, slave.LSSSlave().GetState())

		// Check that slave moves to waiting state
		err = master.SwitchStateGlobal(lss.ModeWaiting)
		assert.Nil(t, err)
		assert.Eventually(t, func() bool {
			return slave.LSSSlave().GetState() == lss.StateWaiting
		}, 5*time.Second, 10*time.Millisecond)
	})
}
