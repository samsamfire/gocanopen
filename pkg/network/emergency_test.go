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
