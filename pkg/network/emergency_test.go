package network

import (
	"sync"
	"testing"
	"time"

	"github.com/samsamfire/gocanopen/v2/pkg/emergency"
	"github.com/samsamfire/gocanopen/v2/pkg/od"
	"github.com/stretchr/testify/assert"
)

// An EMCY sent by another node should be received by the EMCY consumer
func TestEmergencyRemoteReception(t *testing.T) {
	net := CreateNetworkTest()
	defer net.Disconnect()
	net2 := CreateNetworkEmptyTest()
	defer net2.Disconnect()
	remote, err := net2.CreateLocalNode(NodeIdTest+1, od.Default())
	assert.Nil(t, err)
	local, err := net.Local(NodeIdTest)
	assert.Nil(t, err)

	var mu sync.Mutex
	received := []uint16{}
	local.EMCY.SetCallback(func(ident uint16, errorCode uint16, errorRegister byte, errorBit byte, infoCode uint32) {
		mu.Lock()
		defer mu.Unlock()
		if ident == uint16(emergency.ServiceId)+uint16(NodeIdTest+1) && errorCode == emergency.ErrGeneric {
			received = append(received, ident)
		}
	})
	remote.EMCY.ErrorReport(emergency.EmGenericError, emergency.ErrGeneric, 0x1234)
	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) == 1
	}, 1*time.Second, 20*time.Millisecond)
}

// The error register (0x1001 and inside EMCY frames) should reflect the active errors
func TestEmergencyErrorRegister(t *testing.T) {
	net := CreateNetworkTest()
	defer net.Disconnect()
	net2 := CreateNetworkEmptyTest()
	defer net2.Disconnect()
	local, err := net.Local(NodeIdTest)
	assert.Nil(t, err)
	collector := &FrameCollector{}
	cancel, err := net2.Subscribe(uint32(emergency.ServiceId)+uint32(NodeIdTest), 0x7FF, false, collector)
	assert.Nil(t, err)
	defer cancel()

	readRegister := func() uint8 {
		value, err := net2.ReadUint8(NodeIdTest, 0x1001, 0)
		assert.Nil(t, err)
		return value
	}
	lastFrameRegister := func() uint8 {
		frames := collector.GetFrames(uint32(emergency.ServiceId) + uint32(NodeIdTest))
		if len(frames) == 0 {
			return 0xFF
		}
		return frames[len(frames)-1].Data[2]
	}
	assert.Equal(t, uint8(0), readRegister())

	// Generic error
	local.EMCY.ErrorReport(emergency.EmGenericError, emergency.ErrGeneric, 0)
	assert.Eventually(t, func() bool { return lastFrameRegister() == emergency.ErrRegGeneric }, 1*time.Second, 20*time.Millisecond)
	assert.Equal(t, uint8(emergency.ErrRegGeneric), readRegister())

	// Communication error on top
	local.EMCY.ErrorReport(emergency.EmHeartbeatConsumer, emergency.ErrHeartbeat, 0)
	expected := uint8(emergency.ErrRegGeneric | emergency.ErrRegCommunication)
	assert.Eventually(t, func() bool { return lastFrameRegister() == expected }, 1*time.Second, 20*time.Millisecond)
	assert.Equal(t, expected, readRegister())

	// Everything reset
	local.EMCY.ErrorReset(emergency.EmGenericError, 0)
	local.EMCY.ErrorReset(emergency.EmHeartbeatConsumer, 0)
	assert.Eventually(t, func() bool {
		frames := collector.GetFrames(uint32(emergency.ServiceId) + uint32(NodeIdTest))
		return len(frames) == 4 && lastFrameRegister() == 0
	}, 1*time.Second, 20*time.Millisecond)
	assert.Equal(t, uint8(0), readRegister())
}
