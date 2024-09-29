package network

import (
	"testing"
	"time"

	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/samsamfire/gocanopen/pkg/pdo"
	"github.com/stretchr/testify/assert"
)

func TestTpdo(t *testing.T) {
	networkLocal := CreateNetworkEmptyTest()
	networkRemote := CreateNetworkEmptyTest()
	defer networkLocal.Disconnect()
	defer networkRemote.Disconnect()

	t.Run("check send tdpo on sync", func(t *testing.T) {
		// Will act as the "real" node that should send TPDOs
		// Disable SYNC & set communication period to 0
		local, err := networkLocal.CreateLocalNode(80, od.Default())
		assert.Nil(t, err)
		config := local.Configurator()
		err = config.WriteCommunicationPeriod(0)
		assert.Nil(t, err)
		err = config.ProducerDisableSYNC()
		assert.Nil(t, err)
		err = config.EnablePDO(pdo.MinTpdoNumber)
		assert.Nil(t, err)

		// Create a second local node that will send SYNC
		localSync, _ := networkLocal.CreateLocalNode(81, od.Default())
		assert.NotNil(t, localSync)
		configSync := local.Configurator()
		err = configSync.ProducerEnableSYNC()
		assert.Nil(t, err)

		// Will act as the master that will receive the TPDOs (RPDOs)
		// Enable SYNC on remote node
		remote, err := networkRemote.AddRemoteNode(80, od.Default())
		assert.Nil(t, err)
		err = remote.StartPDOs(false)
		assert.Nil(t, err)

		// Update local node, wait for some time & check value
		// received on other side
		err = local.Write(0x2002, 0, int8(10))
		assert.Nil(t, err)
		time.Sleep(400 * time.Millisecond)

		// Internal value received via PDO
		val, err := remote.ReadUint8(0, 0x2002, 0)
		assert.Nil(t, err)
		assert.EqualValues(t, 10, val)

	})
}
