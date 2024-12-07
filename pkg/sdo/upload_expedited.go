package sdo

import (
	"fmt"
)

func (s *SDOServer) rxUploadInitiate(rx SDOMessage) error {
	s.logger.Debug("[RX] expedited upload initiate req",
		"index", fmt.Sprintf("x%x", s.index),
		"subindex", fmt.Sprintf("x%x", s.subindex),
		"raw", rx.raw,
	)
	// Expedited transfer
	if s.sizeIndicated > 0 && s.sizeIndicated <= 4 {
		s.state = stateUploadExpeditedRsp
		return nil
	}
	// Switch to segmented response
	s.state = stateUploadInitiateRsp
	return nil
}

func (s *SDOServer) txUploadExpedited() {
	s.txBuffer.Data[0] = 0x43 | ((4 - byte(s.sizeIndicated)) << 2)
	s.buf.Read(s.txBuffer.Data[4 : 4+s.sizeIndicated])
	s.state = stateIdle
	s.txBuffer.Data[1] = byte(s.index)
	s.txBuffer.Data[2] = byte(s.index >> 8)
	s.txBuffer.Data[3] = s.subindex
	_ = s.Send(s.txBuffer)
	s.logger.Debug("[TX] expedited upload resp",
		"index", fmt.Sprintf("x%x", s.index),
		"subindex", fmt.Sprintf("x%x", s.subindex),
		"raw", s.txBuffer.Data,
	)
}
