package sdo

import (
	"fmt"

	"github.com/samsamfire/gocanopen/internal/crc"
	"github.com/samsamfire/gocanopen/pkg/od"
)

func (s *SDOServer) rxDownloadBlockInitiate(rx SDOMessage) error {
	s.blockCRCEnabled = rx.IsCRCEnabled()
	s.sizeIndicated = 0 // TODO : Shouldn't this be reset everywhere ?

	// Check if size indicated
	if rx.IsSizeIndicatedBlock() {
		sizeInOd := s.streamer.DataLength
		s.sizeIndicated = rx.SizeIndicated()

		// Check if size matches
		if sizeInOd > 0 {
			if s.sizeIndicated > sizeInOd {
				return AbortDataLong
			} else if s.sizeIndicated < sizeInOd && !s.streamer.HasAttribute(od.AttributeStr) {
				return AbortDataShort
			}
		}
	}
	s.logger.Debug("[RX] block download init",
		"index", fmt.Sprintf("x%x", s.index),
		"subindex", fmt.Sprintf("x%x", s.subindex),
		"crc", s.blockCRCEnabled,
		"size", s.sizeIndicated,
		"raw", rx.raw,
	)
	s.state = stateDownloadBlkInitiateRsp
	s.finished = false
	return nil
}

func (s *SDOServer) rxDownloadBlockSubBlock(rx SDOMessage) error {

	seqno := rx.Seqno()

	// Check correct sequence number
	if seqno <= s.blockSize && seqno == (s.blockSequenceNb+1) {

		// Copy data
		s.buf.Write(rx.raw[1:])
		s.blockSequenceNb = seqno
		s.sizeTransferred += BlockSeqSize

		// Check if last segment
		if !rx.SegmentRemaining() {
			s.finished = true
			s.state = stateDownloadBlkSubblockRsp
			s.logger.Debug("[RX] block download end",
				"index", fmt.Sprintf("x%x", s.index),
				"subindex", fmt.Sprintf("x%x", s.subindex),
				"raw", rx.raw,
			)
			return nil
		}

		// Check if end of a segment
		if seqno == s.blockSize {
			s.state = stateDownloadBlkSubblockRsp
			s.logger.Debug("[RX] block download segment end",
				"index", fmt.Sprintf("x%x", s.index),
				"subindex", fmt.Sprintf("x%x", s.subindex),
				"blksize", s.blockSize,
				"raw", rx.raw,
			)
			return nil
		}

		// Regular frame of a segment
		s.logger.Debug("[RX] block download segment",
			"index", fmt.Sprintf("x%x", s.index),
			"subindex", fmt.Sprintf("x%x", s.subindex),
			"seqno", seqno,
			"blksize", s.blockSize,
			"raw", rx.raw,
		)
		return nil

	}

	// If duplicate or sequence didn't start ignore, otherwise
	// seqno is wrong
	if seqno != s.blockSequenceNb && s.blockSequenceNb != 0 {
		s.state = stateDownloadBlkSubblockRsp
		s.logger.Warn("[RX] block download segment error",
			"index", fmt.Sprintf("x%x", s.index),
			"subindex", fmt.Sprintf("x%x", s.subindex),
			"seqno", seqno,
			"ackseq", s.blockSequenceNb,
			"blksize", s.blockSize,
			"raw", rx.raw,
		)
		return nil
	}

	// If an error occurs, client can continue sending frames before it sees that
	// there is a problem. So ignore frames in the meantime
	s.logger.Debug("[RX] block download segment (ignoring, error occured)",
		"index", fmt.Sprintf("x%x", s.index),
		"subindex", fmt.Sprintf("x%x", s.subindex),
		"raw", rx.raw,
	)
	return nil
}

func (s *SDOServer) rxDownloadBlockEnd(rx SDOMessage) error {
	s.logger.Debug("[RX] block download end",
		"index", fmt.Sprintf("x%x", s.index),
		"subindex", fmt.Sprintf("x%x", s.subindex),
		"raw", rx.raw,
	)
	if (rx.raw[0] & 0xE3) != 0xC1 {
		return AbortCmd
	}

	// Get number of data bytes in last segment, that do not
	// contain data. Then reduce buffer
	noData := (rx.raw[0] >> 2) & 0x07
	if uint32(s.buf.Len()) <= uint32(noData) {
		s.errorExtraInfo = fmt.Errorf("internal buffer and end of block download are inconsitent")
		return AbortDeviceIncompat
	}
	s.sizeTransferred -= uint32(noData)
	s.buf.Truncate(s.buf.Len() - int(noData))

	var crcClient = crc.CRC16(0)
	if s.blockCRCEnabled {
		crcClient = rx.GetCRCClient()
	}
	err := s.writeObjectDictionary(2, crcClient)
	if err != nil {
		return err
	}
	s.state = stateDownloadBlkEndRsp
	return nil
}

func (s *SDOServer) txDownloadBlockInitiate() {
	s.txBuffer.Data[0] = 0xA4
	s.txBuffer.Data[1] = byte(s.index)
	s.txBuffer.Data[2] = byte(s.index >> 8)
	s.txBuffer.Data[3] = s.subindex

	// Reset variables
	s.sizeTransferred = 0
	s.finished = false
	s.buf.Reset()
	s.blockSequenceNb = 0
	s.blockCRC = crc.CRC16(0)

	// Calculate blocks from free space
	count := (s.buf.Available() - 2) / BlockSeqSize
	if count > BlockMaxSize {
		count = BlockMaxSize
	}
	s.blockSize = uint8(count)
	s.txBuffer.Data[4] = s.blockSize

	s.state = stateDownloadBlkSubblockReq
	s.logger.Debug("[TX] block download init",
		"index", fmt.Sprintf("x%x", s.index),
		"subindex", fmt.Sprintf("x%x", s.subindex),
		"raw", s.txBuffer.Data,
	)
	_ = s.send(s.txBuffer)
}

func (s *SDOServer) txDownloadBlockSubBlock() error {

	s.txBuffer.Data[0] = 0xA2
	s.txBuffer.Data[1] = s.blockSequenceNb
	s.txBuffer.Data[2] = s.blockSize

	retransmit := s.blockSequenceNb != s.blockSize
	seqnoStart := s.blockSequenceNb

	// Check if last segment to send
	if s.finished {
		s.state = stateDownloadBlkEndReq
		_ = s.send(s.txBuffer)
		s.logger.Debug("[TX] block download segment",
			"index", fmt.Sprintf("x%x", s.index),
			"subindex", fmt.Sprintf("x%x", s.subindex),
			"blksize", s.blockSize,
			"raw", s.txBuffer.Data,
		)
		return nil
	}

	// Determine the next block size depending on the free buffer space
	// If not enough space, try to empty buffer once by writting to OD
	if s.buf.Len() > 0 {
		// We have something in the buffer
		err := s.writeObjectDictionary(1, 0)
		if err != nil {
			return err
		}
	}
	count := s.buf.Available()
	if count > BlockMaxSize {
		count = BlockMaxSize
	}

	// Update parameters for next block
	s.blockSize = uint8(count)
	s.blockSequenceNb = 0
	s.txBuffer.Data[2] = s.blockSize
	s.state = stateDownloadBlkSubblockReq
	_ = s.send(s.txBuffer)

	if retransmit {
		s.logger.Debug("[TX] block download restart",
			"index", fmt.Sprintf("x%x", s.index),
			"subindex", fmt.Sprintf("x%x", s.subindex),
			"seqno prev", seqnoStart,
			"blksize", s.blockSize,
			"raw", s.txBuffer.Data,
		)
		return nil
	}

	s.logger.Debug("[TX] block download segment",
		"index", fmt.Sprintf("x%x", s.index),
		"subindex", fmt.Sprintf("x%x", s.subindex),
		"blksize", s.blockSize,
		"raw", s.txBuffer.Data,
	)
	return nil
}

func (s *SDOServer) txDownloadBlockEnd() {
	s.txBuffer.Data[0] = 0xA1
	s.logger.Debug("[TX] block download end",
		"index", fmt.Sprintf("x%x", s.index),
		"subindex", fmt.Sprintf("x%x", s.subindex),
		"raw", s.txBuffer.Data,
	)
	_ = s.send(s.txBuffer)
	s.state = stateIdle
}
