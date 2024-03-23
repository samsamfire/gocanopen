package network

import (
	"testing"
	"time"

	"github.com/samsamfire/gocanopen/pkg/node"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/stretchr/testify/assert"
)

func TestTimeSynchronization(t *testing.T) {
	const slaveId = 0x66
	network := CreateNetworkTest()
	defer network.Disconnect()
	// Create 10 slave nodes that will update there internal time
	slaveNodes := make([]*node.LocalNode, 0)
	for i := 0; i < 10; i++ {
		slaveNode, err := network.CreateLocalNode(slaveId+uint8(i), od.Default())
		assert.Nil(t, err)
		err = slaveNode.Configurator().TIME.ProducerDisable()
		assert.Nil(t, err)
		err = slaveNode.Configurator().TIME.ConsumerEnable()
		assert.Nil(t, err)
		// Set internal time of slave to now - 24h (wrong time)
		slaveNode.TIME.SetInternalTime(time.Now().Add(24 * time.Hour))
		slaveNodes = append(slaveNodes, slaveNode)
	}
	// Set master node as time producer with interval 100ms
	masterNode := network.nodes[NODE_ID_TEST].(*node.LocalNode)
	masterNode.TIME.SetProducerIntervalMs(100)
	// Check that time difference between slaves and master is 24h
	for _, slaveNode := range slaveNodes {
		timeDiff := slaveNode.TIME.InternalTime().Sub(masterNode.TIME.InternalTime())
		assert.InDelta(t, 24, timeDiff.Hours(), 1)
	}
	// Start publishing time
	err := masterNode.Configurator().TIME.ProducerEnable()
	assert.Nil(t, err)
	// After enabling producer, time should be updated inside all slave nodes
	time.Sleep(150 * time.Millisecond)
	for _, slaveNode := range slaveNodes {
		timeDiff := slaveNode.TIME.InternalTime().Sub(masterNode.TIME.InternalTime())
		assert.InDelta(t, 0, timeDiff.Milliseconds(), 2)
	}
}
