package network

import (
	"testing"
	"time"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/config"
	"github.com/samsamfire/gocanopen/pkg/pdo"
	"github.com/stretchr/testify/assert"
)

func TestPdo(t *testing.T) {

	net := CreateNetworkTest()
	otherNet := CreateNetworkEmptyTest()
	local, err := net.Local(NodeIdTest)
	assert.Nil(t, err)

	c := local.Configurator()

	t.Run("dynamically map async rpdo and send corresponding tpdo updates od", func(t *testing.T) {
		err := c.ClearMappings(1)
		assert.Nil(t, err)
		err = c.WriteConfigurationPDO(1,
			config.PDOConfigurationParameter{
				CanId:            0x255,
				TransmissionType: pdo.TransmissionTypeSyncEventHi,
				InhibitTime:      0,
				EventTimer:       0,
				Mappings: []config.PDOMappingParameter{
					{Index: 0x2005, Subindex: 0, LengthBits: 8},
					{Index: 0x2006, Subindex: 0, LengthBits: 16},
					{Index: 0x2007, Subindex: 0, LengthBits: 32},
				},
			})
		assert.Nil(t, err)

		err = c.EnablePDO(1)
		assert.Nil(t, err)

		time.Sleep(100 * time.Millisecond)
		// Send corresponding TPDO (total is 8+16+32 = 56 bits i.e. 7 bytes)
		err = otherNet.Send(canopen.Frame{ID: 0x255, DLC: 7, Data: [8]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77}})
		assert.Nil(t, err)

		// Read OD entries and check consistency
		time.Sleep(100 * time.Millisecond)
		val, err := local.ReadUint8("UNSIGNED8 value", 0)
		assert.Nil(t, err)
		assert.Equal(t, uint8(0x11), val)
		valU16, err := local.ReadUint16("UNSIGNED16 value", 0)
		assert.Nil(t, err)
		assert.Equal(t, uint16(0x3322), valU16)
		valU32, err := local.ReadUint32("UNSIGNED32 value", 0)
		assert.Nil(t, err)
		assert.Equal(t, uint32(0x77665544), valU32)

	})

		assert.Eventually(t,
			func() bool {
				// Internal value received via PDO
				val, err := remote.ReadInt8(0x2002, 0)
				if val == 10 && err == nil {
					return true
				}
				return false
			}, 5*time.Second, 100*time.Millisecond,
		)
	t.Run("remap rpdo with other order and different can id", func(t *testing.T) {
		c.DisablePDO(1)
		err = c.WriteConfigurationPDO(1,
			config.PDOConfigurationParameter{
				CanId:            0x244,
				TransmissionType: pdo.TransmissionTypeSyncEventHi,
				InhibitTime:      0,
				EventTimer:       0,
				Mappings: []config.PDOMappingParameter{
					{Index: 0x2007, Subindex: 0, LengthBits: 32},
					{Index: 0x2006, Subindex: 0, LengthBits: 16},
					{Index: 0x2005, Subindex: 0, LengthBits: 8},
				},
			})
		assert.Nil(t, err)
		err = c.EnablePDO(1)
		assert.Nil(t, err)

		time.Sleep(100 * time.Millisecond)

		// Send old TPDO id, make sure correctly unregistred
		err = otherNet.Send(canopen.Frame{ID: 0x255, DLC: 7, Data: [8]byte{0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}})
		assert.Nil(t, err)
		time.Sleep(100 * time.Millisecond)
		val, err := local.ReadUint8("UNSIGNED8 value", 0)
		assert.Nil(t, err)
		assert.Equal(t, uint8(0x11), val)

		// Send new TPDO id, should update correctly
		err = otherNet.Send(canopen.Frame{ID: 0x244, DLC: 7, Data: [8]byte{0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}})
		assert.Nil(t, err)
		time.Sleep(100 * time.Millisecond)
		val, err = local.ReadUint8("UNSIGNED8 value", 0)
		assert.Nil(t, err)
		assert.Equal(t, uint8(0x18), val)
	})
	})
}
