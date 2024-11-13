package sdo

import (
	"encoding/binary"
	"fmt"
)

func (s *SDOServer) rxUploadSegment(rx SDOMessage) error {
	s.logger.Debug("[RX] segmented upload req",
		"index", fmt.Sprintf("x%x", s.index),
		"subindex", fmt.Sprintf("x%x", s.subindex),
		"raw", rx.raw,
	)
	if (rx.raw[0] & 0xEF) != 0x60 {
		return AbortCmd
	}
	toggle := rx.GetToggle()
	if toggle != s.toggle {
		return AbortToggleBit
	}
	s.state = stateUploadSegmentRsp
	return nil
}

func (s *SDOServer) txUploadInitiate() {

	s.toggle = 0x00
	s.state = stateUploadSegmentReq
	s.logger.Debug("[TX] segmented upload initiate resp",
		"index", fmt.Sprintf("x%x", s.index),
		"subindex", fmt.Sprintf("x%x", s.subindex),
		"raw", s.txBuffer.Data,
	)
	s.txBuffer.Data[0] = byte(s.sizeIndicated&0b1) + 0x40
	s.txBuffer.Data[1] = byte(s.index)
	s.txBuffer.Data[2] = byte(s.index >> 8)
	s.txBuffer.Data[3] = s.subindex
	binary.LittleEndian.PutUint32(s.txBuffer.Data[4:], s.sizeIndicated)
	_ = s.Send(s.txBuffer)
}

func (s *SDOServer) txUploadSegment() error {

	unread := s.buf.Len()

	// Refill buffer if needed
	err := s.readObjectDictionary(BlockSeqSize, 0, false)
	if err != nil {
		return err
	}

	// Add toggle bit
	s.txBuffer.Data[0] = s.toggle
	s.toggle ^= 0x10

	// Check if last segment
	if unread < BlockSeqSize || (s.finished && unread == BlockSeqSize) {
		s.txBuffer.Data[0] |= (byte((BlockSeqSize - unread) << 1)) | 0x01
		s.state = stateIdle
	} else {
		s.state = stateUploadSegmentReq
		unread = BlockSeqSize
	}

	s.buf.Read(s.txBuffer.Data[1 : 1+unread])
	s.sizeTransferred += uint32(unread)

	// Check if too short or too large in last segment
	err = s.checkSizeConsitency()
	if err != nil {
		return err
	}

	s.logger.Debug("[TX] segmented upload",
		"index", fmt.Sprintf("x%x", s.index),
		"subindex", fmt.Sprintf("x%x", s.subindex),
		"raw", s.txBuffer.Data,
	)
	_ = s.Send(s.txBuffer)
	return nil
}
