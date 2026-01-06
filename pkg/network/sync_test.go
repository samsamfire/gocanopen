package network

import (
	"testing"
	"time"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/stretchr/testify/assert"
)

func TestSyncProducer(t *testing.T) {
	net := CreateNetworkTest()
	otherNet := CreateNetworkEmptyTest()
	defer net.Disconnect()
	defer otherNet.Disconnect()

	local, err := net.Local(NodeIdTest)
	assert.Nil(t, err)

	c := local.Configurator()
	// Disable first to ensure clean state
	err = c.ProducerDisableSYNC()
	assert.Nil(t, err)
	// Reset period
	err = c.WriteCommunicationPeriod(0)
	assert.Nil(t, err)

	collector := &FrameCollector{}
	// SYNC COB-ID is usually 0x80
	rxCancel, err := otherNet.Subscribe(0x80, 0x7FF, false, collector)
	assert.Nil(t, err)
	defer rxCancel()

	t.Run("enable sync producer", func(t *testing.T) {
		err = c.WriteCommunicationPeriod(100 * time.Millisecond)
		assert.Nil(t, err)

		err = c.ProducerEnableSYNC()
		assert.Nil(t, err)

		time.Sleep(250 * time.Millisecond)
		// Should have received ~3 frames (0ms, 100ms, 200ms)
		count := collector.Count(0x80)
		assert.GreaterOrEqual(t, count, 2)
		assert.LessOrEqual(t, count, 4)
	})

	t.Run("disable sync producer", func(t *testing.T) {
		err = c.ProducerDisableSYNC()
		assert.Nil(t, err)
		collector.Clear()

		time.Sleep(200 * time.Millisecond)
		assert.Equal(t, 0, collector.Count(0x80))
	})
}

func TestSyncCounter(t *testing.T) {
	net := CreateNetworkTest()
	otherNet := CreateNetworkEmptyTest()
	defer net.Disconnect()
	defer otherNet.Disconnect()

	local, err := net.Local(NodeIdTest)
	assert.Nil(t, err)

	c := local.Configurator()
	c.ProducerDisableSYNC()
	c.WriteCommunicationPeriod(0)
	c.WriteCounterOverflow(0)

	collector := &FrameCollector{}
	rxCancel, err := otherNet.Subscribe(0x80, 0x7FF, false, collector)
	assert.Nil(t, err)
	defer rxCancel()

	t.Run("sync counter increments", func(t *testing.T) {
		err = c.WriteCommunicationPeriod(0)
		assert.Nil(t, err)
		// Set Counter Overflow to 5
		err = c.WriteCounterOverflow(5)
		assert.Nil(t, err)
		err = c.WriteCommunicationPeriod(50 * time.Millisecond)

		err = c.ProducerEnableSYNC()
		assert.Nil(t, err)

		time.Sleep(350*time.Millisecond + 20*time.Millisecond) // ~7 frames

		frames := collector.GetFrames(0x80)
		assert.GreaterOrEqual(t, len(frames), 6)

		// Check payload
		for i, f := range frames {
			assert.Equal(t, 1, int(f.DLC))
			// Counter starts at 1
			// Sequence: 1, 2, 3, 4, 5, 1, 2...
			expectedCounter := uint8((i % 5) + 1)
			assert.Equal(t, expectedCounter, f.Data[0], "Frame %d should have counter %d", i, expectedCounter)
		}
	})
}

func TestSyncConsumer(t *testing.T) {
	net := CreateNetworkTest()
	otherNet := CreateNetworkEmptyTest()
	defer net.Disconnect()
	defer otherNet.Disconnect()

	local, err := net.Local(NodeIdTest)
	assert.Nil(t, err)

	// Ensure local node is NOT producer
	c := local.Configurator()
	c.ProducerDisableSYNC()
	c.WriteCommunicationPeriod(0)
	c.WriteCounterOverflow(0)

	t.Run("consumer receives sync", func(t *testing.T) {
		toggleBefore := local.SYNC.RxToggle()

		err = otherNet.Send(canopen.Frame{ID: 0x80, DLC: 0})
		assert.Nil(t, err)

		time.Sleep(50 * time.Millisecond)
		toggleAfter := local.SYNC.RxToggle()

		assert.NotEqual(t, toggleBefore, toggleAfter)
	})

	t.Run("consumer receives sync with counter", func(t *testing.T) {
		// Configure local node to expect counter overflow
		err = c.WriteCounterOverflow(5)
		assert.Nil(t, err)

		// Send SYNC with DLC 1
		err = otherNet.Send(canopen.Frame{ID: 0x80, DLC: 1, Data: [8]byte{3}})
		assert.Nil(t, err)

		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, uint8(3), local.SYNC.Counter())
	})
}
