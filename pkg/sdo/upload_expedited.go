package sdo

import (
	log "github.com/sirupsen/logrus"
)

func (s *SDOServer) rxUploadInitiate(rx SDOMessage) error {
	log.Debugf("[SERVER][RX] UPLOAD EXPEDITED | x%x:x%x %v", s.index, s.subindex, rx.raw)
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
	log.Debugf("[SERVER][TX] UPLOAD EXPEDITED | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
}
