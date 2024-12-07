package sdo

import (
	"encoding/binary"
	"fmt"

	"github.com/samsamfire/gocanopen/pkg/od"
)

func (s *SDOServer) rxDownloadInitiate(rx SDOMessage) error {

	// Segmented transfer type
	if !rx.IsExpedited() {
		s.logger.Debug("[RX] segmented download",
			"index", fmt.Sprintf("x%x", s.index),
			"subindex", fmt.Sprintf("x%x", s.subindex),
			"raw", s.txBuffer.Data,
		)

		// If size is indicated, we need to check coherence
		// Between size in OD and requested size
		if rx.IsSizeIndicated() {

			sizeInOd := s.streamer.DataLength
			s.sizeIndicated = binary.LittleEndian.Uint32(rx.raw[4:])
			// Check if size matches
			if sizeInOd > 0 {
				if s.sizeIndicated > uint32(sizeInOd) {
					return AbortDataLong
				} else if s.sizeIndicated < uint32(sizeInOd) && !s.streamer.HasAttribute(od.AttributeStr) {
					return AbortDataShort
				}
			}
		}

		s.state = stateDownloadInitiateRsp
		s.finished = false
		return nil
	}

	// Expedited transfer type, we write directly inside OD
	s.logger.Debug("[RX] expedited download",
		"index", fmt.Sprintf("x%x", s.index),
		"subindex", fmt.Sprintf("x%x", s.subindex),
		"raw", s.txBuffer.Data,
	)
	sizeInOd := s.streamer.DataLength
	nbToWrite := 4
	// Determine number of bytes to write, depending on size flag
	// either undetermined or 4-n
	if rx.IsSizeIndicated() {
		nbToWrite -= (int(rx.raw[0]) >> 2) & 0x03
	} else if sizeInOd > 0 && sizeInOd < 4 {
		nbToWrite = int(sizeInOd)
	}

	if s.streamer.HasAttribute(od.AttributeStr) &&
		(sizeInOd == 0 || uint32(nbToWrite) < sizeInOd) {
		delta := sizeInOd - uint32(nbToWrite)
		if delta == 1 {
			nbToWrite += 1
		} else {
			nbToWrite += 2
		}
		s.streamer.DataLength = uint32(nbToWrite)
	} else if sizeInOd == 0 {
		s.streamer.DataLength = uint32(nbToWrite)
	} else if nbToWrite != int(sizeInOd) {
		if nbToWrite > int(sizeInOd) {
			return AbortDataLong
		} else {
			return AbortDataShort
		}
	}
	_, err := s.streamer.Write(rx.raw[4 : 4+nbToWrite])
	if err != nil {
		return ConvertOdToSdoAbort(err.(od.ODR))
	}
	s.state = stateDownloadInitiateRsp
	s.finished = true
	return nil
}

func (s *SDOServer) txDownloadInitiate() {
	// Prepare response packet
	s.txBuffer.Data[0] = 0x60
	s.txBuffer.Data[1] = byte(s.index)
	s.txBuffer.Data[2] = byte(s.index >> 8)
	s.txBuffer.Data[3] = s.subindex
	_ = s.Send(s.txBuffer)
	if s.finished {
		s.logger.Debug("[TX] expedited download",
			"index", fmt.Sprintf("x%x", s.index),
			"subindex", fmt.Sprintf("x%x", s.subindex),
			"raw", s.txBuffer.Data,
		)
		s.state = stateIdle
		return
	}
	s.logger.Debug("[TX] segmented download init",
		"index", fmt.Sprintf("x%x", s.index),
		"subindex", fmt.Sprintf("x%x", s.subindex),
		"raw", s.txBuffer.Data,
	)
	s.toggle = 0x00
	s.sizeTransferred = 0
	s.buf.Reset()
	s.state = stateDownloadSegmentReq
}
