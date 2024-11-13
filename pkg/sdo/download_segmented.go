package sdo

import (
	"fmt"
)

func (s *SDOServer) rxDownloadSegment(rx SDOMessage) error {
	if (rx.raw[0] & 0xE0) != 0x00 {
		return AbortCmd
	}

	s.logger.Debug("[RX] segmented download",
		"index", fmt.Sprintf("x%x", s.index),
		"subindex", fmt.Sprintf("x%x", s.subindex),
		"raw", s.txBuffer.Data,
	)

	s.finished = (rx.raw[0] & 0x01) != 0
	toggle := rx.GetToggle()
	if toggle != s.toggle {
		return AbortToggleBit
	}
	// Get size and write to buffer
	count := BlockSeqSize - ((rx.raw[0] >> 1) & 0x07)

	n, err := s.buf.Write(rx.raw[1 : 1+count])
	if err != nil || n != int(count) {
		return AbortDeviceIncompat
	}
	s.sizeTransferred += uint32(count)

	if s.streamer.DataLength > 0 && s.sizeTransferred > s.streamer.DataLength {
		return AbortDataLong
	}

	if s.finished || s.buf.Available() < (BlockSeqSize+2) {
		err := s.writeObjectDictionary(0, 0)
		if err != nil {
			return err
		}
	}
	s.state = stateDownloadSegmentRsp

	return nil
}

func (s *SDOServer) txDownloadSegment() {
	// Pepare segment
	s.txBuffer.Data[0] = 0x20 | s.toggle
	s.toggle ^= 0x10
	s.logger.Debug("[TX] segmented download",
		"index", fmt.Sprintf("x%x", s.index),
		"subindex", fmt.Sprintf("x%x", s.subindex),
		"raw", s.txBuffer.Data,
	)
	_ = s.Send(s.txBuffer)
	if s.finished {
		s.state = stateIdle
		return
	}
	s.state = stateDownloadSegmentReq
}
