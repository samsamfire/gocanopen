package sdo

import (
	"encoding/binary"

	log "github.com/sirupsen/logrus"
)

func (s *SDOServer) rxUploadSegment(rx SDOMessage) error {
	log.Debugf("[SERVER][RX] UPLOAD SEGMENTED | x%x:x%x %v", s.index, s.subindex, rx.raw)
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

	if s.sizeIndicated > 0 {
		s.txBuffer.Data[0] = 0x41
		// Add data size
		binary.LittleEndian.PutUint32(s.txBuffer.Data[4:], s.sizeIndicated)

	} else {
		s.txBuffer.Data[0] = 0x40
	}
	s.toggle = 0x00

	s.state = stateUploadSegmentReq
	log.Debugf("[SERVER][TX] UPLOAD SEGMENTED | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
	s.txBuffer.Data[1] = byte(s.index)
	s.txBuffer.Data[2] = byte(s.index >> 8)
	s.txBuffer.Data[3] = s.subindex
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

	log.Debugf("[SERVER][TX] UPLOAD SEGMENTED | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
	_ = s.Send(s.txBuffer)
	return nil
}
