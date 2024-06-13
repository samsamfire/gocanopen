package network

import (
	"fmt"
	"testing"
	"time"

	"github.com/samsamfire/gocanopen/pkg/heartbeat"
	"github.com/samsamfire/gocanopen/pkg/nmt"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/stretchr/testify/assert"
)

type EventHandler struct {
	nbTimeout int
	nbReset   int
	nbChanged int
	nbStarted int
}

func (e *EventHandler) OnEvent(event uint8, index uint8, nodeId uint8, nmtState uint8) {
	switch event {
	case heartbeat.EventTimeout:
		e.nbTimeout++
	case heartbeat.EventBoot:
		e.nbReset++
	case heartbeat.EventChanged:
		fmt.Println("changed", index, nodeId, nmtState)
		e.nbChanged++
	case heartbeat.EventStarted:
		e.nbStarted++
	}

}

const minDelayHeartbeat = 200 * time.Millisecond

func TestHeartbeatEventCallback(t *testing.T) {
	network := CreateNetworkEmptyTest()
	network.Connect()
	defer network.Disconnect()

	// Create node that will emit heartbeats (producer)
	producer, err := network.CreateLocalNode(0x22, od.Default())
	assert.Nil(t, err)
	configProducer := producer.Configurator()

	// Create node that will receive heartbeats (consumer)
	consumer, err := network.CreateLocalNode(0x23, od.Default())
	assert.Nil(t, err)

	// Make consumer monitor producer
	configConsumer := consumer.Configurator()
	err = configConsumer.WriteMonitoredNode(1, 0x22, 100)
	assert.Nil(t, err)

	t.Run("heartbeat lost event", func(t *testing.T) {
		// Start heartbeat
		eventHandler := EventHandler{}
		consumer.HBConsumer.OnEvent(eventHandler.OnEvent)
		err = configProducer.WriteHeartbeatPeriod(20)
		assert.Nil(t, err)
		time.Sleep(minDelayHeartbeat)

		// Disable heartbeat of producer
		err = configProducer.WriteHeartbeatPeriod(0)
		assert.Nil(t, err)

		// Wait for timeout
		time.Sleep(minDelayHeartbeat)
		assert.Equal(t, 1, eventHandler.nbTimeout)

		// Enable / Disable heartbeat multiple times
		for i := 0; i < 5; i++ {
			configProducer.WriteHeartbeatPeriod(20)
			time.Sleep(minDelayHeartbeat)
			configProducer.WriteHeartbeatPeriod(0)
			time.Sleep(minDelayHeartbeat)
		}
		assert.Equal(t, 6, eventHandler.nbTimeout)
	})

	t.Run("heartbeat reset & started event", func(t *testing.T) {
		eventHandler := EventHandler{}
		consumer.HBConsumer.OnEvent(eventHandler.OnEvent)
		configProducer.WriteHeartbeatPeriod(100)
		configConsumer.WriteMonitoredNode(1, 0x22, 150)
		time.Sleep(minDelayHeartbeat)

		// Remove node, and start it again
		err = network.RemoveNode(0x22)
		assert.Nil(t, err)
		producer, err = network.CreateLocalNode(0x22, od.Default())
		assert.Nil(t, err)
		configProducer.WriteHeartbeatPeriod(100)
		time.Sleep(minDelayHeartbeat)
		assert.Equal(t, 1, eventHandler.nbReset)
		assert.Equal(t, 2, eventHandler.nbStarted)
	})

	t.Run("heartbeat nmt changed event", func(t *testing.T) {
		configProducer.WriteHeartbeatPeriod(100)
		configConsumer.WriteMonitoredNode(1, 0x22, 150)
		time.Sleep(minDelayHeartbeat)
		eventHandler := EventHandler{}
		consumer.HBConsumer.OnEvent(eventHandler.OnEvent)
		// Send pre-operational command to trigger nmt change
		err := network.Command(0x22, nmt.CommandEnterPreOperational)
		assert.Nil(t, err)
		time.Sleep(minDelayHeartbeat)
		assert.Equal(t, 1, eventHandler.nbChanged)

	})
}
