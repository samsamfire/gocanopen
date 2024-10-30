package sdo

import (
	"encoding/binary"

	"github.com/samsamfire/gocanopen/internal/crc"
	log "github.com/sirupsen/logrus"
)

func (s *SDOServer) processOutgoing() error {
	var err error

	s.txBuffer.Data = [8]byte{0}

	switch s.state {
	case stateDownloadInitiateRsp:
		s.txDownloadInitiate()

	case stateDownloadSegmentRsp:
		s.txDownloadSegment()

	case stateUploadInitiateRsp:
		s.txUploadInitiate()

	case stateUploadSegmentRsp:
		err = s.txUploadSegment()

	case stateDownloadBlkInitiateRsp:
		s.txDownloadBlockInitiate()

	case stateDownloadBlkSubblockRsp:
		err = s.txDownloadBlockSubBlock()

	case stateDownloadBlkEndRsp:
		s.txDownloadBlockEnd()

	case stateUploadBlkInitiateRsp:
		s.txUploadBlockInitiate()

	case stateUploadBlkSubblockSreq:
		err = s.txUploadBlockSubBlock()
		if err != nil {
			return err
		}
		s.processOutgoing()

	case stateUploadBlkEndSreq:
		s.txUploadBlockEnd()
	}
	return err
}

func (s *SDOServer) txDownloadInitiate() {
	// Prepare response packet
	s.txBuffer.Data[0] = 0x60
	s.txBuffer.Data[1] = byte(s.index)
	s.txBuffer.Data[2] = byte(s.index >> 8)
	s.txBuffer.Data[3] = s.subindex
	_ = s.Send(s.txBuffer)
	if s.finished {
		log.Debugf("[SERVER][TX] DOWNLOAD EXPEDITED | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
		s.state = stateIdle
		return
	}

	log.Debugf("[SERVER][TX] DOWNLOAD SEGMENT INIT | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
	s.toggle = 0x00
	s.sizeTransferred = 0
	s.bufWriteOffset = 0
	s.bufReadOffset = 0
	s.state = stateDownloadSegmentReq
}

func (s *SDOServer) txDownloadSegment() {
	// Pepare segment
	s.txBuffer.Data[0] = 0x20 | s.toggle
	s.toggle ^= 0x10
	log.Debugf("[SERVER][TX] DOWNLOAD SEGMENT | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
	_ = s.Send(s.txBuffer)
	if s.finished {
		s.state = stateIdle
		return
	}
	s.state = stateDownloadSegmentReq
}

func (s *SDOServer) txUploadInitiate() {
	if s.sizeIndicated > 0 && s.sizeIndicated <= 4 {
		s.txBuffer.Data[0] = 0x43 | ((4 - byte(s.sizeIndicated)) << 2)
		copy(s.txBuffer.Data[4:], s.buffer[:s.sizeIndicated])
		s.state = stateIdle
		s.txBuffer.Data[1] = byte(s.index)
		s.txBuffer.Data[2] = byte(s.index >> 8)
		s.txBuffer.Data[3] = s.subindex
		_ = s.Send(s.txBuffer)
		log.Debugf("[SERVER][TX] UPLOAD EXPEDITED | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
		return

	}
	// Segmented transfer
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
	// Refill buffer if needed
	err := s.readObjectDictionary(BlockSeqSize, false)
	if err != nil {
		return err
	}
	s.txBuffer.Data[0] = s.toggle
	s.toggle ^= 0x10
	count := s.bufWriteOffset - s.bufReadOffset

	// Check if last segment
	if count < BlockSeqSize || (s.finished && count == BlockSeqSize) {
		s.txBuffer.Data[0] |= (byte((BlockSeqSize - count) << 1)) | 0x01
		s.state = stateIdle
	} else {
		s.state = stateUploadSegmentReq
		count = BlockSeqSize
	}
	copy(s.txBuffer.Data[1:], s.buffer[s.bufReadOffset:s.bufReadOffset+count])
	s.bufReadOffset += count
	s.sizeTransferred += count
	// Check if too short or too large in last segment
	if s.sizeIndicated > 0 {
		if s.sizeTransferred > s.sizeIndicated {
			s.state = stateAbort
			return AbortDataLong
		} else if s.state == stateIdle && s.sizeTransferred < s.sizeIndicated {
			s.state = stateAbort
			return AbortDataShort
		}
	}
	log.Debugf("[SERVER][TX] UPLOAD SEGMENTED | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
	_ = s.Send(s.txBuffer)
	return nil
}

func (s *SDOServer) txDownloadBlockInitiate() {
	s.txBuffer.Data[0] = 0xA4
	s.txBuffer.Data[1] = byte(s.index)
	s.txBuffer.Data[2] = byte(s.index >> 8)
	s.txBuffer.Data[3] = s.subindex
	// Calculate blocks from free space
	count := (len(s.buffer) - 2) / BlockSeqSize
	if count > BlockMaxSize {
		count = BlockMaxSize
	}
	s.blockSize = uint8(count)
	s.txBuffer.Data[4] = s.blockSize
	// Reset variables
	s.sizeTransferred = 0
	s.finished = false
	s.bufReadOffset = 0
	s.bufWriteOffset = 0
	s.blockSequenceNb = 0
	s.blockCRC = crc.CRC16(0)
	s.state = stateDownloadBlkSubblockReq
	log.Debugf("[SERVER][TX] BLOCK DOWNLOAD INIT | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
	s.Send(s.txBuffer)
}

func (s *SDOServer) txDownloadBlockSubBlock() error {
	s.txBuffer.Data[0] = 0xA2
	s.txBuffer.Data[1] = s.blockSequenceNb
	transferShort := s.blockSequenceNb != s.blockSize
	seqnoStart := s.blockSequenceNb
	// Is it last segment ?
	if s.finished {
		s.state = stateDownloadBlkEndReq
	} else {
		// Calclate from free buffer space
		count := (len(s.buffer) - 2 - int(s.bufWriteOffset)) / BlockSeqSize
		if count > BlockMaxSize {
			count = BlockMaxSize
		} else if s.bufWriteOffset > 0 {
			// Empty buffer
			err := s.writeObjectDictionary(1, 0)
			if err != nil {
				return err
			}
			count = (len(s.buffer) - 2 - int(s.bufWriteOffset)) / BlockSeqSize
			if count > BlockMaxSize {
				count = BlockMaxSize
			}
		}
		s.blockSize = uint8(count)
		s.blockSequenceNb = 0
		s.state = stateDownloadBlkSubblockReq
	}
	s.txBuffer.Data[2] = s.blockSize
	s.Send(s.txBuffer)

	if transferShort && !s.finished {
		log.Debugf("[SERVER][TX] BLOCK DOWNLOAD RESTART seqno prev=%v, blksize=%v", seqnoStart, s.blockSize)
	} else {
		log.Debugf("[SERVER][TX] BLOCK DOWNLOAD SUB-BLOCK RES | x%x:x%x blksize %v %v",
			s.index,
			s.subindex,
			s.blockSize,
			s.txBuffer.Data,
		)
	}
	return nil
}

func (s *SDOServer) txDownloadBlockEnd() {
	s.txBuffer.Data[0] = 0xA1
	log.Debugf("[SERVER][TX] BLOCK DOWNLOAD END | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
	s.Send(s.txBuffer)
	s.state = stateIdle
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
	count := s.bufWriteOffset - s.bufReadOffset

	// Check if last segment (can be less that BlockSeqSize)
	if count < BlockSeqSize || (s.finished && count == BlockSeqSize) {
		s.txBuffer.Data[0] |= 0x80
	} else {
		count = BlockSeqSize
	}

	copy(s.txBuffer.Data[1:], s.buffer[s.bufReadOffset:s.bufReadOffset+count])
	s.bufReadOffset += count
	s.blockNoData = byte(BlockSeqSize - count)
	s.sizeTransferred += count

	// Check if too short or too large in last segment
	if s.sizeIndicated > 0 {
		if s.sizeTransferred > s.sizeIndicated {
			return AbortDataLong
		} else if s.bufReadOffset == s.bufWriteOffset && s.sizeTransferred < s.sizeIndicated {
			return AbortDataShort
		}
	}

	// Check if last segment or all segments in current block transferred
	if s.bufWriteOffset == s.bufReadOffset || s.blockSequenceNb >= s.blockSize {
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
	log.Debugf("[SERVER][TX] BLOCK UPLOAD END | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
	s.Send(s.txBuffer)
	s.state = stateUploadBlkEndCrsp
}

func (s *SDOServer) txAbort(err error) {
	if sdoAbort, ok := err.(Abort); !ok {
		log.Errorf("[SERVER][TX] Abort internal error : unknown abort code : %v", err)
		s.SendAbort(AbortGeneral)
	} else {
		s.SendAbort(sdoAbort)
	}
	s.state = stateIdle
}
