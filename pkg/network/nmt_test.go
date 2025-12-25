package network

import (
	"testing"
	"time"

	"github.com/samsamfire/gocanopen/pkg/nmt"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/samsamfire/gocanopen/pkg/sdo"
	"github.com/stretchr/testify/assert"
)

func TestNmt(t *testing.T) {
	net1 := CreateNetworkEmptyTest()
	defer net1.Disconnect()
	net2 := CreateNetworkEmptyTest()
	defer net2.Disconnect()

	t.Run("simple boot up", func(t *testing.T) {

		local, err := net1.CreateLocalNode(NodeIdTest, od.Default())
		assert.Nil(t, err)
		defer net1.RemoveNode(NodeIdTest)
		assert.Eventually(t, func() bool {
			state := local.NMT.GetInternalState()
			return state == nmt.StateOperational
		}, 1*time.Second, 20*time.Millisecond)
	})

	t.Run("operational to pre-operational", func(t *testing.T) {
		local, err := net1.CreateLocalNode(NodeIdTest, od.Default())
		assert.Nil(t, err)
		defer net1.RemoveNode(NodeIdTest)

		err = net2.Command(NodeIdTest, nmt.CommandEnterPreOperational)
		assert.Nil(t, err)
		assert.Eventually(t, func() bool {
			state := local.NMT.GetInternalState()
			return state == nmt.StatePreOperational
		}, 1*time.Second, 20*time.Millisecond)
	})

	t.Run("operational to stopped", func(t *testing.T) {
		local, err := net1.CreateLocalNode(NodeIdTest, od.Default())
		assert.Nil(t, err)
		defer net1.RemoveNode(NodeIdTest)

		err = net2.Command(NodeIdTest, nmt.CommandEnterStopped)
		assert.Nil(t, err)
		assert.Eventually(t, func() bool {
			state := local.NMT.GetInternalState()
			return state == nmt.StateStopped
		}, 1*time.Second, 20*time.Millisecond)
	})

	t.Run("no sdo in stopped", func(t *testing.T) {
		local, err := net1.CreateLocalNode(NodeIdTest, od.Default())
		assert.Nil(t, err)
		defer net1.RemoveNode(NodeIdTest)

		// We are able to read version
		remote, _ := net2.AddRemoteNode(NodeIdTest, od.Default())
		_, err = remote.Configurator().ReadManufacturerSoftwareVersion()
		assert.Nil(t, err)

		err = net2.Command(NodeIdTest, nmt.CommandEnterStopped)
		assert.Nil(t, err)
		assert.Eventually(t, func() bool {
			state := local.NMT.GetInternalState()
			return state == nmt.StateStopped
		}, 1*time.Second, 20*time.Millisecond)

		// SDO read should timeout
		_, err = remote.Configurator().ReadManufacturerSoftwareVersion()
		assert.Equal(t, sdo.AbortTimeout, err)
	})

	t.Run("reset when stopped", func(t *testing.T) {
		local, err := net1.CreateLocalNode(NodeIdTest, od.Default())
		assert.Nil(t, err)
		defer net1.RemoveNode(NodeIdTest)

		err = net2.Command(NodeIdTest, nmt.CommandEnterStopped)
		assert.Nil(t, err)
		assert.Eventually(t, func() bool {
			state := local.NMT.GetInternalState()
			return state == nmt.StateStopped
		}, 1*time.Second, 20*time.Millisecond)

		// Reset node, should result in transitioning back to operational
		err = net2.Command(NodeIdTest, nmt.CommandResetNode)
		assert.Nil(t, err)
		assert.Eventually(t, func() bool {
			state := local.NMT.GetInternalState()
			return state == nmt.StateOperational
		}, 1*time.Second, 20*time.Millisecond)
	})

	t.Run("reset comm when stopped", func(t *testing.T) {
		local, err := net1.CreateLocalNode(NodeIdTest, od.Default())
		assert.Nil(t, err)
		defer net1.RemoveNode(NodeIdTest)

		err = net2.Command(NodeIdTest, nmt.CommandEnterStopped)
		assert.Nil(t, err)
		assert.Eventually(t, func() bool {
			state := local.NMT.GetInternalState()
			return state == nmt.StateStopped
		}, 1*time.Second, 20*time.Millisecond)

		// Reset comm, should not influence the nmt state
		err = net2.Command(NodeIdTest, nmt.CommandResetCommunication)
		assert.Nil(t, err)
		time.Sleep(1 * time.Second)
		assert.Equal(t, nmt.StateStopped, local.NMT.GetInternalState())
	})

	t.Run("boot up on reset", func(t *testing.T) {
		local, err := net1.CreateLocalNode(NodeIdTest, od.Default())
		assert.Nil(t, err)
		defer net1.RemoveNode(NodeIdTest)

		consumer, err := net2.CreateLocalNode(NodeIdTest+1, od.Default())
		assert.Nil(t, err)
		defer net2.RemoveNode(NodeIdTest + 1)
		configConsumer := consumer.Configurator()
		err = configConsumer.WriteMonitoredNode(1, NodeIdTest, 1000)
		assert.Nil(t, err)

		eventHandler := EventHandler{}
		consumer.HBConsumer.OnEvent(eventHandler.OnEvent)

		err = net2.Command(NodeIdTest, nmt.CommandResetNode)
		assert.Nil(t, err)

		assert.Eventually(t, func() bool {
			return eventHandler.NbEventBoot() == 1 && local.NMT.GetInternalState() == nmt.StateOperational
		}, 2*time.Second, 20*time.Millisecond)

	})

}
