package network

import (
	"testing"
	"time"

	"github.com/samsamfire/gocanopen/v2/pkg/can/virtual"
	"github.com/samsamfire/gocanopen/v2/pkg/heartbeat"
	"github.com/samsamfire/gocanopen/v2/pkg/nmt"
	"github.com/samsamfire/gocanopen/v2/pkg/node"
	"github.com/samsamfire/gocanopen/v2/pkg/od"
	"github.com/samsamfire/gocanopen/v2/pkg/sdo"
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

	t.Run("boot up with heartbeat producer disabled", func(t *testing.T) {
		// A null producer heartbeat time disables the heartbeat producer, it
		// should not prevent the node from booting
		odict := od.Default()
		err := odict.Index(0x1017).PutUint16(0, 0, true)
		assert.Nil(t, err)

		hbId := uint32(heartbeat.ServiceId) + uint32(NodeIdTest)
		collector := &FrameCollector{}
		_, err = net2.Subscribe(hbId, 0x7FF, false, collector)
		assert.Nil(t, err)

		local, err := net1.CreateLocalNode(NodeIdTest, odict)
		assert.Nil(t, err)
		defer net1.RemoveNode(NodeIdTest)

		// The node should boot, and the boot-up message is mandatory even when
		// the heartbeat producer is disabled
		assert.Eventually(t, func() bool {
			return local.NMT.GetInternalState() == nmt.StateOperational
		}, 1*time.Second, 20*time.Millisecond)
		frames := collector.GetFrames(hbId)
		assert.Len(t, frames, 1)
		if len(frames) > 0 {
			assert.Equal(t, uint8(1), frames[0].DLC)
			assert.Equal(t, nmt.StateInitializing, frames[0].Data[0])
		}

		// SDO should be served, i.e. the state change was propagated
		_, err = net2.Configurator(NodeIdTest).ReadManufacturerSoftwareVersion()
		assert.Nil(t, err)

		// No cyclic heartbeat should follow, the producer is disabled
		time.Sleep(500 * time.Millisecond)
		assert.Len(t, collector.GetFrames(hbId), 1)
	})

	t.Run("boot up on reset", func(t *testing.T) {
		local, err := net1.CreateLocalNode(NodeIdTest, od.Default())
		assert.Nil(t, err)
		defer net1.RemoveNode(NodeIdTest)

		consumer, err := net2.CreateLocalNode(NodeIdTest+1, od.Default())
		assert.Nil(t, err)
		defer net2.RemoveNode(NodeIdTest + 1)
		configConsumer := consumer.Configurator()

		err = configConsumer.WriteMonitoredNode(1, NodeIdTest, 1000*time.Millisecond)
		assert.Nil(t, err)

		time.Sleep(200 * time.Millisecond)

		eventHandler := EventHandler{}
		consumer.HBConsumer.OnEvent(eventHandler.OnEvent)

		err = net1.Command(NodeIdTest, nmt.CommandResetNode)
		assert.Nil(t, err)

		assert.Eventually(t, func() bool {
			return eventHandler.NbEventBoot() == 1 && local.NMT.GetInternalState() == nmt.StateOperational
		}, 2*time.Second, 20*time.Millisecond)

	})

}

// A state change callback should be able to use the NMT object
func TestNmtCallbackUsesNmt(t *testing.T) {
	network := CreateNetworkTest()
	defer network.Disconnect()
	local, err := network.Local(NodeIdTest)
	assert.Nil(t, err)

	states := make(chan uint8, 10)
	local.NMT.AddStateChangeCallback(func(state uint8) {
		states <- local.NMT.GetInternalState()
	})
	go local.NMT.SendInternalCommand(uint8(nmt.CommandEnterPreOperational))
	select {
	case state := <-states:
		assert.Equal(t, nmt.StatePreOperational, state)
	case <-time.After(1 * time.Second):
		t.Fatal("state change callback blocked")
	}
}

// A command sent by a node should also apply to itself when concerned
func TestNmtSendCommand(t *testing.T) {
	for _, receiveOwn := range []bool{false, true} {
		canBus, err := NewBus("virtual", "localhost:18888", 0)
		assert.Nil(t, err)
		canBus.(*virtual.Bus).SetReceiveOwn(receiveOwn)
		network := NewNetwork(canBus)
		assert.Nil(t, network.Connect())
		network2 := CreateNetworkEmptyTest()
		local, err := network.CreateLocalNode(NodeIdTest, od.Default())
		assert.Nil(t, err)
		other, err := network2.CreateLocalNode(NodeIdTest+1, od.Default())
		assert.Nil(t, err)
		for _, node := range []*node.LocalNode{local, other} {
			assert.Eventually(t, func() bool {
				return node.NMT.GetInternalState() == nmt.StateOperational
			}, 1*time.Second, 10*time.Millisecond)
		}

		done := make(chan error, 1)
		go func() { done <- local.NMT.SendCommand(nmt.CommandEnterPreOperational, 0) }()
		select {
		case err := <-done:
			assert.Nil(t, err)
		case <-time.After(1 * time.Second):
			t.Fatalf("SendCommand blocked, receiveOwn %v", receiveOwn)
		}
		for _, node := range []*node.LocalNode{local, other} {
			assert.Eventually(t, func() bool {
				return node.NMT.GetInternalState() == nmt.StatePreOperational
			}, 1*time.Second, 10*time.Millisecond, "receiveOwn %v", receiveOwn)
		}
		network2.Disconnect()
		network.Disconnect()
	}
}
