package sdo

import (
	"encoding/binary"
	"fmt"

	"github.com/samsamfire/gocanopen/internal/crc"
	log "github.com/sirupsen/logrus"
)

func (s *SDOServer) txDownloadInitiate() uint8 {
	// Prepare response packet
	s.txBuffer.Data[0] = 0x60
	s.txBuffer.Data[1] = byte(s.index)
	s.txBuffer.Data[2] = byte(s.index >> 8)
	s.txBuffer.Data[3] = s.subindex
	s.timeoutTimer = 0
	_ = s.Send(s.txBuffer)
	if s.finished {
		log.Debugf("[SERVER][TX] DOWNLOAD EXPEDITED | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
		s.state = stateIdle
		return success
	}

	log.Debugf("[SERVER][TX] DOWNLOAD SEGMENT INIT | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
	s.toggle = 0x00
	s.sizeTransferred = 0
	s.bufWriteOffset = 0
	s.bufReadOffset = 0
	s.state = stateDownloadSegmentReq
	return waitingResponse
}

func (s *SDOServer) txDownloadSegment() uint8 {
	// Pepare segment
	s.txBuffer.Data[0] = 0x20 | s.toggle
	s.toggle ^= 0x10
	s.timeoutTimer = 0
	log.Debugf("[SERVER][TX] DOWNLOAD SEGMENT | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
	_ = s.Send(s.txBuffer)
	if s.finished {
		s.state = stateIdle
		return success
	}
	s.state = stateDownloadSegmentReq
	return waitingResponse
}

func (s *SDOServer) txUploadInitiate() uint8 {
	if s.sizeIndicated > 0 && s.sizeIndicated <= 4 {
		s.txBuffer.Data[0] = 0x43 | ((4 - byte(s.sizeIndicated)) << 2)
		copy(s.txBuffer.Data[4:], s.buffer[:s.sizeIndicated])
		s.state = stateIdle
		s.txBuffer.Data[1] = byte(s.index)
		s.txBuffer.Data[2] = byte(s.index >> 8)
		s.txBuffer.Data[3] = s.subindex
		_ = s.Send(s.txBuffer)
		log.Debugf("[SERVER][TX] UPLOAD EXPEDITED | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
		return success

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
	s.timeoutTimer = 0
	s.state = stateUploadSegmentReq
	log.Debugf("[SERVER][TX] UPLOAD SEGMENTED | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
	s.txBuffer.Data[1] = byte(s.index)
	s.txBuffer.Data[2] = byte(s.index >> 8)
	s.txBuffer.Data[3] = s.subindex
	_ = s.Send(s.txBuffer)
	return waitingResponse
}

func (s *SDOServer) txUploadSegment() (error, uint8) {
	ret := waitingResponse
	// Refill buffer if needed
	err := s.readObjectDictionary(BlockSeqSize, false)
	if err != nil {
		return err, waitingResponse
	}
	s.txBuffer.Data[0] = s.toggle
	s.toggle ^= 0x10
	count := s.bufWriteOffset - s.bufReadOffset

	// Check if last segment
	if count < BlockSeqSize || (s.finished && count == BlockSeqSize) {
		s.txBuffer.Data[0] |= (byte((BlockSeqSize - count) << 1)) | 0x01
		s.state = stateIdle
		ret = success
	} else {
		s.timeoutTimer = 0
		s.state = stateUploadSegmentReq
		count = BlockSeqSize
	}
	copy(s.txBuffer.Data[1:], s.buffer[s.bufReadOffset:s.bufReadOffset+count])
	s.bufReadOffset += count
	s.sizeTransferred += count
	// Check if too shor or too large in last segment
	if s.sizeIndicated > 0 {
		if s.sizeTransferred > s.sizeIndicated {
			s.state = stateAbort
			return AbortDataLong, ret
		} else if ret == success && s.sizeTransferred < s.sizeIndicated {
			s.state = stateAbort
			return AbortDataShort, waitingResponse
		}
	}
	log.Debugf("[SERVER][TX] UPLOAD SEGMENTED | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
	_ = s.Send(s.txBuffer)
	return nil, ret
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
	s.timeoutTimer = 0
	s.timeoutTimerBlock = 0
	s.state = stateDownloadBlkSubblockReq
	s.rxNew = false
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
		s.rxNew = false
	}
	s.txBuffer.Data[2] = s.blockSize
	s.timeoutTimerBlock = 0
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
	s.timeoutTimer = 0
	log.Debugf("[SERVER][TX] BLOCK UPLOAD INIT | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
	s.Send(s.txBuffer)
	s.state = stateUploadBlkInitiateReq2
}

func (s *SDOServer) txUploadBlockSubBlock() error {
	// Write header & gend current count
	s.blockSequenceNb += 1
	s.txBuffer.Data[0] = s.blockSequenceNb
	count := s.bufWriteOffset - s.bufReadOffset
	// Check if last segment
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
	// Reset timer & send
	s.timeoutTimer = 0
	s.Send(s.txBuffer)
	return nil
}

func (s *SDOServer) txUploadBlockEnd() {
	s.txBuffer.Data[0] = 0xC1 | (s.blockNoData << 2)
	s.txBuffer.Data[1] = byte(s.blockCRC)
	s.txBuffer.Data[2] = byte(s.blockCRC >> 8)
	s.timeoutTimer = 0
	log.Debugf("[SERVER][TX] BLOCK UPLOAD END | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
	s.Send(s.txBuffer)
	s.state = stateUploadBlkEndCrsp
}

func (s *SDOServer) processOutgoing() {
	var ret = waitingResponse
	var abortCode error

	s.txBuffer.Data = [8]byte{0}

	switch s.state {
	case stateDownloadInitiateRsp:
		ret = s.txDownloadInitiate()

	case stateDownloadSegmentRsp:
		ret = s.txDownloadSegment()

	case stateUploadInitiateRsp:
		ret = s.txUploadInitiate()

	case stateUploadSegmentRsp:
		abortCode, ret = s.txUploadSegment()

	case stateDownloadBlkInitiateRsp:
		s.txDownloadBlockInitiate()

	case stateDownloadBlkSubblockRsp:
		abortCode = s.txDownloadBlockSubBlock()
		fmt.Println("abort", abortCode)

	case stateDownloadBlkEndRsp:
		s.txBuffer.Data[0] = 0xA1
		log.Debugf("[SERVER][TX] BLOCK DOWNLOAD END | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
		s.Send(s.txBuffer)
		s.state = stateIdle
		ret = success

	case stateUploadBlkInitiateRsp:
		s.txUploadBlockInitiate()

	case stateUploadBlkSubblockSreq:
		// Send block straight away
		for {
			abortCode = s.txUploadBlockSubBlock()
			if abortCode != nil {
				s.state = stateAbort
				break
			}
			if s.state == stateUploadBlkSubblockCrsp {
				break
			}
		}

	case stateUploadBlkEndSreq:
		s.txUploadBlockEnd()
	}

	// Error handling
	if ret == waitingResponse {
		switch s.state {
		case stateAbort:
			if sdoAbort, ok := abortCode.(Abort); !ok {
				log.Errorf("[SERVER][TX] Abort internal error : unknown abort code : %v", abortCode)
				s.Abort(AbortGeneral)
			} else {
				s.Abort(sdoAbort)
			}
			s.state = stateIdle
		case stateDownloadBlkSubblockReq:
			ret = blockDownloadInProgress
		case stateUploadBlkSubblockSreq:
			ret = blockUploadInProgress
		}
	}
}
