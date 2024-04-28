package emergency

import (
	"encoding/binary"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/od"
)

func readEntryStatusBits(stream *od.Stream, data []byte, countRead *uint16) error {
	if stream == nil || stream.Subindex != 0 || data == nil || countRead == nil {
		return od.ErrDevIncompat
	}
	em, ok := stream.Object.(*EMCY)
	if !ok {
		return od.ErrDevIncompat
	}

	countReadLocal := EmergencyErrorStatusBits / 8
	if countReadLocal > len(data) {
		countReadLocal = len(data)
	}
	if len(stream.Data) != 0 && countReadLocal > len(stream.Data) {
		countReadLocal = len(stream.Data)
	} // Unclear why we change datalength
	copy(data, em.errorStatusBits[:countReadLocal])
	*countRead = uint16(countReadLocal)
	return nil
}

func writeEntryStatusBits(stream *od.Stream, data []byte, countWritten *uint16) error {
	if stream == nil || stream.Subindex != 0 || countWritten == nil || data == nil {
		return od.ErrDevIncompat
	}
	em, ok := stream.Object.(*EMCY)
	if !ok {
		return od.ErrDevIncompat
	}
	countWriteLocal := EmergencyErrorStatusBits / 8
	if countWriteLocal > len(data) {
		countWriteLocal = len(data)
	}
	if len(stream.Data) != 0 && countWriteLocal > len(stream.Data) {
		countWriteLocal = len(stream.Data)
	} // Unclear why we change datalength
	copy(em.errorStatusBits[:], data[:countWriteLocal])
	*countWritten = uint16(countWriteLocal)
	return nil
}

// [EMCY] read emergency history
func readEntry1003(stream *od.Stream, data []byte, countRead *uint16) error {
	if stream == nil || data == nil || countRead == nil ||
		(len(data) < 4 && stream.Subindex > 0) ||
		len(data) < 1 {
		return od.ErrDevIncompat
	}
	em, ok := stream.Object.(*EMCY)
	if !ok {
		return od.ErrDevIncompat
	}
	em.mu.Lock()
	defer em.mu.Unlock()

	if len(em.fifo) < 2 {
		return od.ErrDevIncompat
	}
	if stream.Subindex == 0 {
		data[0] = em.fifoCount
		*countRead = 1
		return nil
	}
	if stream.Subindex > em.fifoCount {
		return od.ErrNoData
	}
	// Most recent error is in subindex 1 and stored behind fifoWrPtr
	index := int(em.fifoWrPtr) - int(stream.Subindex)
	if index >= len(em.fifo) {
		return od.ErrDevIncompat
	}
	if index < 0 {
		index += len(em.fifo)
	}
	binary.LittleEndian.PutUint32(data, em.fifo[index].msg)
	*countRead = 4
	return nil
}

// [EMCY] clear emergency history
func writeEntry1003(stream *od.Stream, data []byte, countWritten *uint16) error {
	if stream == nil || stream.Subindex != 0 || data == nil || len(data) != 1 || countWritten == nil {
		return od.ErrDevIncompat
	}
	if data[0] != 0 {
		return od.ErrInvalidValue
	}
	em, ok := stream.Object.(*EMCY)
	if !ok {
		return od.ErrDevIncompat
	}
	em.mu.Lock()
	defer em.mu.Unlock()

	// Clear error history
	em.fifoCount = 0
	*countWritten = 1
	return nil
}

// [EMCY] read emergency cob id
func readEntry1014(stream *od.Stream, data []byte, countRead *uint16) error {
	if stream == nil || data == nil || countRead == nil || len(data) < 4 || stream.Subindex != 0 {
		return od.ErrDevIncompat
	}
	em, ok := stream.Object.(*EMCY)
	if !ok {
		return od.ErrDevIncompat
	}
	em.mu.Lock()
	defer em.mu.Unlock()

	var canId uint16
	if em.producerIdent == ServiceId {
		canId = ServiceId + uint16(em.nodeId)
	} else {
		canId = em.producerIdent
	}
	var cobId uint32
	if em.producerEnabled {
		cobId = 0
	} else {
		cobId = 0x80000000
	}
	cobId |= uint32(canId)
	binary.LittleEndian.PutUint32(data, cobId)
	*countRead = 4
	return nil
}

// [EMCY] update emergency producer cob id
func writeEntry1014(stream *od.Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || countWritten == nil || len(data) != 4 || stream.Subindex != 0 {
		return od.ErrDevIncompat
	}
	em, ok := stream.Object.(*EMCY)
	if !ok {
		return od.ErrDevIncompat
	}
	em.mu.Lock()
	defer em.mu.Unlock()

	// Check written value, cob id musn't change when enabled
	cobId := binary.LittleEndian.Uint32(data)
	newCanId := cobId & 0x7FF
	var currentCanId uint16
	if em.producerIdent == ServiceId {
		currentCanId = ServiceId + uint16(em.nodeId)
	} else {
		currentCanId = em.producerIdent
	}
	newEnabled := (cobId&uint32(currentCanId)) == 0 && newCanId != 0
	if cobId&0x7FFFF800 != 0 || canopen.IsIDRestricted(uint16(newCanId)) ||
		(em.producerEnabled && newEnabled && newCanId != uint32(currentCanId)) {
		return od.ErrInvalidValue
	}
	em.producerEnabled = newEnabled
	if newCanId == uint32(ServiceId+uint16(em.nodeId)) {
		em.producerIdent = ServiceId
	} else {
		em.producerIdent = uint16(newCanId)
	}

	if newEnabled {
		em.txBuffer = canopen.NewFrame(newCanId, 0, 8)
	}
	return od.WriteEntryDefault(stream, data, countWritten)

}

// [EMCY] update inhibite time
func writeEntry1015(stream *od.Stream, data []byte, countWritten *uint16) error {
	if stream == nil || stream.Subindex != 0 || data == nil || len(data) != 2 || countWritten == nil {
		return od.ErrDevIncompat
	}
	em, ok := stream.Object.(*EMCY)
	if !ok {
		return od.ErrDevIncompat
	}
	em.mu.Lock()
	defer em.mu.Unlock()

	em.inhibitTimeUs = uint32(binary.LittleEndian.Uint16(data)) * 100
	em.inhibitTimer = 0

	return od.WriteEntryDefault(stream, data, countWritten)
}
