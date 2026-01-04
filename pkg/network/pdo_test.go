package network

import (
	"sync"
	"testing"
	"time"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/config"
	"github.com/samsamfire/gocanopen/pkg/pdo"
	"github.com/stretchr/testify/assert"
)

type FrameCollector struct {
	frames []canopen.Frame
	mu     sync.Mutex
}

func (fc *FrameCollector) Handle(frame canopen.Frame) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.frames = append(fc.frames, frame)
}

func (fc *FrameCollector) HasFrame(id uint32) bool {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	for _, f := range fc.frames {
		if f.ID == id {
			return true
		}
	}
	return false
}

func (fc *FrameCollector) Count(id uint32) int {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	count := 0
	for _, f := range fc.frames {
		if f.ID == id {
			count++
		}
	}
	return count
}

func (fc *FrameCollector) GetFrames(id uint32) []canopen.Frame {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	var frames []canopen.Frame
	for _, f := range fc.frames {
		if f.ID == id {
			frames = append(frames, f)
		}
	}
	return frames
}

func (fc *FrameCollector) Clear() {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.frames = nil
}

func TestRPDO(t *testing.T) {
	net := CreateNetworkTest()
	otherNet := CreateNetworkEmptyTest()
	defer net.Disconnect()
	defer otherNet.Disconnect()
	local, err := net.Local(NodeIdTest)
	assert.Nil(t, err)

	c := local.Configurator()

	t.Run("update rpdo transmission type", func(t *testing.T) {
		for i := range uint8(100) {
			err := c.WriteTransmissionType(1, i)
			assert.Nil(t, err)
			transType, err := c.ReadTransmissionType(1)
			assert.Nil(t, err)
			assert.Equal(t, i, transType)
		}
	})

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

	t.Run("send wrong rpdo length creates error", func(t *testing.T) {
		// Reset value
		err := local.WriteAnyExact("UNSIGNED8 value", 0, uint8(0))
		assert.Nil(t, err)

		// PDO length too low
		err = otherNet.Send(canopen.Frame{ID: 0x244, DLC: 4, Data: [8]byte{0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}})
		assert.Nil(t, err)
		time.Sleep(100 * time.Millisecond)

		// Verify value was NOT updated
		val, err := local.ReadUint8("UNSIGNED8 value", 0)
		assert.Nil(t, err)
		assert.Equal(t, uint8(0), val)
	})

	t.Run("synchronous rpdo", func(t *testing.T) {
		// Reset value
		err := local.WriteAnyExact("UNSIGNED8 value", 0, uint8(0))
		assert.Nil(t, err)

		// Configure RPDO as Synchronous (Transmission Type 1)
		c.DisablePDO(1)
		err = c.WriteConfigurationPDO(1,
			config.PDOConfigurationParameter{
				CanId:            0x255,
				TransmissionType: 1, // Sync
				InhibitTime:      0,
				EventTimer:       0,
				Mappings: []config.PDOMappingParameter{
					{Index: 0x2005, Subindex: 0, LengthBits: 8},
				},
			})
		assert.Nil(t, err)
		err = c.EnablePDO(1)
		assert.Nil(t, err)

		time.Sleep(100 * time.Millisecond)

		// Send RPDO with new value 0x42
		err = otherNet.Send(canopen.Frame{ID: 0x255, DLC: 1, Data: [8]byte{0x42}})
		assert.Nil(t, err)

		time.Sleep(100 * time.Millisecond)
		// Value should STILL be 0
		val, err := local.ReadUint8("UNSIGNED8 value", 0)
		assert.Nil(t, err)
		assert.Equal(t, uint8(0), val)

		// Send SYNC
		err = otherNet.Send(canopen.Frame{ID: 0x80, DLC: 0})
		assert.Nil(t, err)

		time.Sleep(100 * time.Millisecond)
		// Value should NOW be 0x42
		val, err = local.ReadUint8("UNSIGNED8 value", 0)
		assert.Nil(t, err)
		assert.Equal(t, uint8(0x42), val)
	})

	t.Run("rpdo timeout deadline monitoring", func(t *testing.T) {
		emcyCollector := &FrameCollector{}
		_, err := otherNet.Subscribe(0x80+uint32(NodeIdTest), 0x7FF, false, emcyCollector)
		assert.Nil(t, err)

		c.DisablePDO(1)
		err = c.WriteConfigurationPDO(1,
			config.PDOConfigurationParameter{
				CanId:            0x255,
				TransmissionType: pdo.TransmissionTypeSyncEventHi,
				InhibitTime:      0,
				EventTimer:       200 * time.Millisecond,
				Mappings: []config.PDOMappingParameter{
					{Index: 0x2005, Subindex: 0, LengthBits: 8},
				},
			})
		assert.Nil(t, err)
		err = c.EnablePDO(1)
		assert.Nil(t, err)

		// Send RPDO with value, this will enable timeout monitoring
		err = otherNet.Send(canopen.Frame{ID: 0x255, DLC: 1, Data: [8]byte{0x33}})
		assert.Nil(t, err)

		// Reset collector
		emcyCollector.Clear()

		// Wait 100ms - No timeout yet
		time.Sleep(100 * time.Millisecond)
		assert.Equal(t, 0, emcyCollector.Count(0x80+uint32(NodeIdTest)))

		// Wait > 200ms.
		time.Sleep(200 * time.Millisecond) // Total 300ms

		// Should have received 1 EMCY
		assert.Equal(t, 1, emcyCollector.Count(0x80+uint32(NodeIdTest)))

		// Verify Content
		frames := emcyCollector.GetFrames(0x80 + uint32(NodeIdTest))
		if len(frames) > 0 {
			f := frames[0]
			// ErrRpdoTimeout = 0x8250. Little Endian: 50 82
			assert.Equal(t, uint8(0x50), f.Data[0])
			assert.Equal(t, uint8(0x82), f.Data[1])
		}

		// Wait again, should not receive more EMCY
		time.Sleep(200 * time.Millisecond)
		assert.Equal(t, 1, emcyCollector.Count(0x80+uint32(NodeIdTest)))

		err = otherNet.Send(canopen.Frame{ID: 0x255, DLC: 1, Data: [8]byte{0x33}})
		assert.Nil(t, err)

		// Should reset timeout monitoring, so no new EMCY after 200ms
		emcyCollector.Clear()
		time.Sleep(250 * time.Millisecond)
		assert.Equal(t, 1, emcyCollector.Count(0x80+uint32(NodeIdTest)))
	})
}

func TestTPDO(t *testing.T) {
	net := CreateNetworkTest()
	otherNet := CreateNetworkEmptyTest()
	defer net.Disconnect()
	defer otherNet.Disconnect()

	local, err := net.Local(NodeIdTest)
	assert.Nil(t, err)

	c := local.Configurator()
	err = c.ProducerDisableSYNC()
	c.WriteCommunicationPeriod(0)
	assert.Nil(t, err)
	tpdo1 := pdo.MaxRpdoNumber + 1
	canId := uint32(0x180 + int(NodeIdTest)) // Default TPDO1 ID

	collector := &FrameCollector{}
	_, err = otherNet.Subscribe(canId, 0x7FF, false, collector)
	assert.Nil(t, err)

	t.Run("send on sync reception", func(t *testing.T) {
		c.DisablePDO(tpdo1)
		collector.Clear()
		err = c.WriteConfigurationPDO(tpdo1,
			config.PDOConfigurationParameter{
				CanId:            uint16(canId),
				TransmissionType: 1, // Sync every cycle
				Mappings: []config.PDOMappingParameter{
					{Index: 0x2005, Subindex: 0, LengthBits: 8},
				},
			})
		assert.Nil(t, err)
		err = c.EnablePDO(tpdo1)
		assert.Nil(t, err)

		time.Sleep(100 * time.Millisecond)
		assert.Equal(t, 0, collector.Count(canId))

		// Send SYNC
		err = otherNet.Send(canopen.Frame{ID: 0x80, DLC: 0})
		assert.Nil(t, err)

		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, 1, collector.Count(canId))
	})

	t.Run("send on 10th sync", func(t *testing.T) {

		const syncStartCount = 10

		c.DisablePDO(tpdo1)
		collector.Clear()
		err = c.WriteConfigurationPDO(tpdo1,
			config.PDOConfigurationParameter{
				CanId:            uint16(canId),
				TransmissionType: syncStartCount,
				SyncStart:        0,
				Mappings: []config.PDOMappingParameter{
					{Index: 0x2008, Subindex: 0, LengthBits: 8},
				},
			})
		assert.Nil(t, err)
		err = c.EnablePDO(tpdo1)
		assert.Nil(t, err)
		assert.Equal(t, 0, collector.Count(canId))

		// From SYNC 1 to 9 we should have 0 frames
		for range syncStartCount - 1 {
			// Send SYNC
			err = otherNet.Send(canopen.Frame{ID: 0x80, DLC: 0})
			time.Sleep(100 * time.Millisecond)
			assert.Nil(t, err)
			assert.Equal(t, 0, collector.Count(canId))
		}

		err = otherNet.Send(canopen.Frame{ID: 0x80, DLC: 0})
		assert.Nil(t, err)
		time.Sleep(100 * time.Millisecond)
		assert.Equal(t, 1, collector.Count(canId))
	})

	t.Run("send on sync reception with sync start value", func(t *testing.T) {

		// This also requires sync counter overflow to be set
		err = c.WriteCommunicationPeriod(0)
		assert.Nil(t, err)
		err = c.WriteCounterOverflow(3)
		assert.Nil(t, err)
		err = c.WriteCommunicationPeriod(10_000)
		assert.Nil(t, err)

		c.DisablePDO(tpdo1)
		collector.Clear()
		err = c.WriteConfigurationPDO(tpdo1,
			config.PDOConfigurationParameter{
				CanId:            uint16(canId),
				TransmissionType: 1, // Sync every cycle
				SyncStart:        3,
				Mappings: []config.PDOMappingParameter{
					{Index: 0x2005, Subindex: 0, LengthBits: 8},
				},
			})
		assert.Nil(t, err)
		err = c.EnablePDO(tpdo1)
		assert.Nil(t, err)

		time.Sleep(100 * time.Millisecond)
		collector.Clear()

		// Send SYNC 1
		err = otherNet.Send(canopen.Frame{ID: 0x80, DLC: 1, Data: [8]byte{0}})
		assert.Nil(t, err)
		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, 0, collector.Count(canId))

		// Send SYNC 2
		err = otherNet.Send(canopen.Frame{ID: 0x80, DLC: 1, Data: [8]byte{1}})
		assert.Nil(t, err)
		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, 0, collector.Count(canId))

		// Send SYNC 3 - Should trigger
		err = otherNet.Send(canopen.Frame{ID: 0x80, DLC: 1, Data: [8]byte{3}})
		assert.Nil(t, err)
		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, 1, collector.Count(canId))

		// Send SYNC 4 - Should trigger (as transmission type is 1)
		err = otherNet.Send(canopen.Frame{ID: 0x80, DLC: 1, Data: [8]byte{4}})
		assert.Nil(t, err)
		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, 2, collector.Count(canId))

		// Send SYNC 5 - Should trigger (as transmission type is 1)
		err = otherNet.Send(canopen.Frame{ID: 0x80, DLC: 1, Data: [8]byte{5}})
		assert.Nil(t, err)
		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, 3, collector.Count(canId))

	})

	t.Run("event timer", func(t *testing.T) {

		c.DisablePDO(tpdo1)
		collector.Clear()
		err = c.WriteConfigurationPDO(tpdo1,
			config.PDOConfigurationParameter{
				CanId:            uint16(canId),
				TransmissionType: pdo.TransmissionTypeSyncEventLo,
				EventTimer:       500 * time.Millisecond,
				Mappings: []config.PDOMappingParameter{
					{Index: 0x2005, Subindex: 0, LengthBits: 8},
				},
			})
		assert.Nil(t, err)
		err = c.EnablePDO(tpdo1)
		assert.Nil(t, err)
		// Clear any sent PDO because of transmission type change
		time.Sleep(100 * time.Millisecond)
		collector.Clear()
		assert.Equal(t, 0, collector.Count(canId))

		// Wait another 450ms
		time.Sleep(450 * time.Millisecond)
		assert.Equal(t, 1, collector.Count(canId))

		// Wait another 500ms
		time.Sleep(550 * time.Millisecond)
		assert.Equal(t, 2, collector.Count(canId))
	})

	t.Run("event timer with inhibit time", func(t *testing.T) {
		c.DisablePDO(tpdo1)
		collector.Clear()
		err = c.WriteConfigurationPDO(tpdo1,
			config.PDOConfigurationParameter{
				CanId:            uint16(canId),
				TransmissionType: pdo.TransmissionTypeSyncEventHi,
				InhibitTime:      300 * time.Millisecond,
				EventTimer:       100 * time.Millisecond,
				Mappings: []config.PDOMappingParameter{
					{Index: 0x2005, Subindex: 0, LengthBits: 8},
				},
			})
		assert.Nil(t, err)
		err = c.EnablePDO(tpdo1)
		assert.Nil(t, err)

		// Wait for first PDO (triggered by enable)
		time.Sleep(50 * time.Millisecond)
		collector.Clear()

		// Wait 100ms: Event timer triggered (at 100ms), but inhibit time is 200ms
		time.Sleep(100 * time.Millisecond)
		assert.Equal(t, 0, collector.Count(canId))

		// Wait another 200ms (total 350ms): inhibit time should have elapsed
		time.Sleep(200 * time.Millisecond)
		assert.Equal(t, 1, collector.Count(canId))
	})
}
