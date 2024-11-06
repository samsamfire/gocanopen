package sdo

import (
	"encoding/binary"

	"github.com/samsamfire/gocanopen/internal/crc"
	log "github.com/sirupsen/logrus"
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
	log.Debugf("[SERVER][RX] UPLOAD BLOCK INIT | x%x:x%x %v | crc : %v, blksize :%v", s.index, s.subindex, rx.raw, s.blockCRCEnabled, s.blockSize)
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
	log.Debugf("[SERVER][RX] BLOCK UPLOAD END SUB-BLOCK | blksize %v ackseq %v | x%x:x%x %v",
		rx.raw[2],
		ackseq,
		s.index,
		s.subindex,
		rx.raw,
	)

	// Check block size
	s.blockSize = rx.raw[2]
	if s.blockSize < 1 || s.blockSize > BlockMaxSize {
		return AbortBlockSize
	}

	// If server acknowledges more than what was sent, error straight away
	if ackseq > s.blockSequenceNb {
		log.Debugf("[SERVER][RX] BLOCK UPLOAD END SUB-BLOCK error | blksize %v | x%x:x%x %v",
			rx.raw[2],
			s.index,
			s.subindex,
			rx.raw,
		)
		return AbortCmd
	}

	// Check client acknowledged all packets sent
	if ackseq < s.blockSequenceNb {
		log.Debugf("[SERVER][RX] BLOCK UPLOAD END SUB-BLOCK error, retransmitting | blksize %v | x%x:x%x %v | ackseq %v | seqno %v",
			rx.raw[2],
			s.index,
			s.subindex,
			rx.raw,
			ackseq,
			s.blockSequenceNb,
		)
		// We go back to the last acknowledged packet
		// Because some data might still be in buffer, we must first remove it
		nbFailed := uint32(s.blockSequenceNb-ackseq)*BlockSeqSize - uint32(s.blockNoData)
		nbPending := uint32(s.buf.Len())
		s.sizeTransferred -= uint32(nbFailed)
		s.streamer.DataOffset -= (nbFailed + nbPending)
		s.buf.Reset()

		// Refill buffer for next block, without re-calculating CRC (already done)
		err := s.readObjectDictionary(nbFailed+nbPending, int(nbPending)+int(nbFailed), false)
		if err != nil {
			return err
		}
	}

	// Refill buffer for next block
	err := s.readObjectDictionary(uint32(s.blockSize)*BlockSeqSize, 0, true)
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
	log.Debugf("[SERVER][TX] BLOCK UPLOAD INIT | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
	s.Send(s.txBuffer)
	s.state = stateUploadBlkInitiateReq2
}

func (s *SDOServer) txUploadBlockSubBlock() error {
	// Write header & gend current count
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
		log.Debugf("[SERVER][TX] BLOCK UPLOAD END SUB-BLOCK | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
	} else {
		log.Debugf("[SERVER][TX] BLOCK UPLOAD SUB-BLOCK | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
	}
	s.Send(s.txBuffer)
	return nil
}

func (s *SDOServer) txUploadBlockEnd() {
	s.txBuffer.Data[0] = 0xC1 | (s.blockNoData << 2)
	s.txBuffer.Data[1] = byte(s.blockCRC)
	s.txBuffer.Data[2] = byte(s.blockCRC >> 8)
	log.Debugf("[SERVER][TX] BLOCK UPLOAD END | x%x:x%x %v | size %v | crc %v", s.index, s.subindex, s.txBuffer.Data, s.sizeTransferred, s.blockCRC)
	s.Send(s.txBuffer)
	s.state = stateUploadBlkEndCrsp
}
