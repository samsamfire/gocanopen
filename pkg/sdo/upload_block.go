package sdo

import (
	"encoding/binary"
	"fmt"

	"github.com/samsamfire/gocanopen/internal/crc"
)

func (s *SDOServer) rxUploadBlockInitiate(rx SDOMessage) error {

	// If protocol switch threshold (byte 5) is larger than data
	// size of OD var, then switch to segmented
	if s.sizeIndicated > 0 && rx.raw[5] > 0 && uint32(rx.raw[5]) >= s.sizeIndicated {
		return s.rxUploadInitiate(rx)
	}

	// Check if CRC enabled
	if (rx.raw[0] & 0x04) != 0 {
		s.blockCRCEnabled = true
		s.blockCRC = crc.CRC16(0)
		s.blockCRC.Block(s.buf.Bytes())
	} else {
		s.blockCRCEnabled = false
	}

	// Get block size and check okay
	s.blockSize = rx.GetBlockSize()
	s.logger.Debug("[RX] block init req",
		"index", fmt.Sprintf("x%x", s.index),
		"subindex", fmt.Sprintf("x%x", s.subindex),
		"crc", s.blockCRCEnabled,
		"blksize", s.blockSize,
		"raw", rx.raw,
	)
	if s.blockSize < 1 || s.blockSize > BlockMaxSize {
		return AbortBlockSize
	}

	// Check that we have enough data for sending a complete sub-block with the requested size
	if !s.finished && uint32(s.buf.Len()) < uint32(s.blockSize)*BlockSeqSize {
		return AbortBlockSize
	}
	s.state = stateUploadBlkInitiateRsp
	return nil
}

func (s *SDOServer) rxUploadSubBlock(rx SDOMessage) error {
	if rx.raw[0] != 0xA2 {
		return AbortCmd
	}
	ackseq := rx.raw[1]

	s.logger.Debug("[RX] block upload sub-block req",
		"index", fmt.Sprintf("x%x", s.index),
		"subindex", fmt.Sprintf("x%x", s.subindex),
		"blksize", rx.raw[2],
		"ackseq", ackseq,
		"seqno", s.blockSequenceNb,
		"raw", rx.raw,
	)

	// Check block size
	s.blockSize = rx.raw[2]
	if s.blockSize < 1 || s.blockSize > BlockMaxSize {
		return AbortBlockSize
	}

	// If server acknowledges more than what was sent, error straight away
	if ackseq > s.blockSequenceNb {
		s.logger.Debug("[RX] server acked more than sent, will abort")
		return AbortCmd
	}

	// Check client acknowledged all packets sent
	if ackseq < s.blockSequenceNb {
		// We go back to the last acknowledged packet
		// Because some data might still be in buffer, we must first remove it
		nbFailed := uint32(s.blockSize-ackseq)*BlockSeqSize - uint32(s.blockNoData)
		nbPending := uint32(s.buf.Len())
		s.sizeTransferred -= uint32(nbFailed)
		s.logger.Debug("server acked less than sent, will rewind & retransmit",
			"nBytes", nbFailed+nbPending,
			"nbFailed", nbFailed,
			"nbPending", nbPending,
		)
		s.streamer.DataOffset -= (nbFailed + nbPending)
		s.buf.Reset()

		// Refill buffer with previous data without re-calculating CRC (already calculated before)
		// This needs to be the exact size to not cause CRC errors
		err := s.readObjectDictionary(nbFailed+nbPending, int(nbPending+nbFailed), false)
		if err != nil {
			return err
		}
	}
	// Refill buffer for next block
	err := s.readObjectDictionary(uint32(s.blockSize)*BlockSeqSize, -1, true)
	if err != nil {
		return err
	}

	// No more data to be read
	if s.buf.Len() == 0 {
		s.state = stateUploadBlkEndSreq
		return nil
	}

	// Proceed to send the block
	s.blockSequenceNb = 0
	s.state = stateUploadBlkSubblockSreq
	return nil
}

func (s *SDOServer) txUploadBlockInitiate() {
	s.txBuffer.Data[0] = 0xC4
	s.txBuffer.Data[1] = byte(s.index)
	s.txBuffer.Data[2] = byte(s.index >> 8)
	s.txBuffer.Data[3] = s.subindex
	// Add data size
	if s.sizeIndicated > 0 {
		s.txBuffer.Data[0] |= 0x02
		binary.LittleEndian.PutUint32(s.txBuffer.Data[4:], s.sizeIndicated)
	}
	// Reset timer & send
	s.logger.Debug("[TX] block upload init resp",
		"index", fmt.Sprintf("x%x", s.index),
		"subindex", fmt.Sprintf("x%x", s.subindex),
		"raw", s.txBuffer.Data,
	)
	s.Send(s.txBuffer)
	s.state = stateUploadBlkInitiateReq2
}

func (s *SDOServer) txUploadBlockSubBlock() error {
	s.blockSequenceNb += 1
	s.txBuffer.Data[0] = s.blockSequenceNb

	unread := s.buf.Len()

	// Check if last segment (can be less that BlockSeqSize)
	if unread < BlockSeqSize || (s.finished && unread == BlockSeqSize) {
		s.txBuffer.Data[0] |= 0x80
	} else {
		unread = BlockSeqSize
	}
	s.buf.Read(s.txBuffer.Data[1 : 1+unread])

	s.blockNoData = byte(BlockSeqSize - unread)
	s.sizeTransferred += uint32(unread)

	// Check if too short or too large in last segment
	if s.sizeIndicated > 0 {
		if s.sizeTransferred > s.sizeIndicated {
			return AbortDataLong
		} else if s.buf.Len() == 0 && s.sizeTransferred < s.sizeIndicated {
			return AbortDataShort
		}
	}

	// Check if last segment or all segments in current block transferred
	if s.buf.Len() == 0 || s.blockSequenceNb >= s.blockSize {
		s.state = stateUploadBlkSubblockCrsp
		s.logger.Debug("[TX] block upload sub-block end req",
			"index", fmt.Sprintf("x%x", s.index),
			"subindex", fmt.Sprintf("x%x", s.subindex),
			"raw", s.txBuffer.Data,
		)
	} else {
		s.logger.Debug("[TX] block upload sub-block segment",
			"index", fmt.Sprintf("x%x", s.index),
			"subindex", fmt.Sprintf("x%x", s.subindex),
			"raw", s.txBuffer.Data,
		)
	}
	s.Send(s.txBuffer)
	return nil
}

func (s *SDOServer) txUploadBlockEnd() {
	s.txBuffer.Data[0] = 0xC1 | (s.blockNoData << 2)
	s.txBuffer.Data[1] = byte(s.blockCRC)
	s.txBuffer.Data[2] = byte(s.blockCRC >> 8)
	s.logger.Debug("[TX] block upload end resp",
		"index", fmt.Sprintf("x%x", s.index),
		"subindex", fmt.Sprintf("x%x", s.subindex),
		"size", s.sizeTransferred,
		"crc", s.blockCRC,
		"raw", s.txBuffer.Data,
	)
	s.Send(s.txBuffer)
	s.state = stateUploadBlkEndCrsp
}
