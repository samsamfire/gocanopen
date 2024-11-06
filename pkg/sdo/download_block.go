package sdo

import (
	"fmt"

	"github.com/samsamfire/gocanopen/internal/crc"
	"github.com/samsamfire/gocanopen/pkg/od"
	log "github.com/sirupsen/logrus"
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
	log.Debugf("[SERVER][RX] BLOCK DOWNLOAD INIT | x%x:x%x | crc enabled : %v expected size : %v | %v",
		s.index,
		s.subindex,
		s.blockCRCEnabled,
		s.sizeIndicated,
		rx.raw,
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
			log.Debugf("[SERVER][RX] BLOCK DOWNLOAD END | x%x:x%x %v", s.index, s.subindex, rx.raw)
			return nil
		}

		// Check if end of a segment
		if seqno == s.blockSize {
			s.state = stateDownloadBlkSubblockRsp
			log.Debugf("[SERVER][RX] BLOCK DOWNLOAD SUB-BLOCK | x%x:x%x %v", s.index, s.subindex, rx.raw)
			return nil
		}

		// Regular frame of a segment
		log.Debugf("[SERVER][RX] BLOCK DOWNLOAD SUB-BLOCK | x%x:x%x %v", s.index, s.subindex, rx.raw)
		return nil

	}

	// If duplicate or sequence didn't start ignore, otherwise
	// seqno is wrong
	if seqno != s.blockSequenceNb && s.blockSequenceNb != 0 {
		s.state = stateDownloadBlkSubblockRsp
		log.Warnf("[SERVER][RX] BLOCK DOWNLOAD SUB-BLOCK | Wrong sequence number (got %v, previous %v) | x%x:x%x %v",
			seqno,
			s.blockSequenceNb,
			s.index,
			s.subindex,
			rx.raw,
		)
		return nil
	}

	// If an error occurs, client can continue sending frames before it sees that
	// there is a problem. So ignore frames in the meantime
	log.Warnf("[SERVER][RX] BLOCK DOWNLOAD SUB-BLOCK | Ignoring (got %v, expecting %v) | x%x:x%x %v",
		seqno,
		s.blockSequenceNb+1,
		s.index,
		s.subindex,
		rx.raw,
	)
	return nil
}

func (s *SDOServer) rxDownloadBlockEnd(rx SDOMessage) error {
	log.Debugf("[SERVER][RX] BLOCK DOWNLOAD END | x%x:x%x %v", s.index, s.subindex, rx.raw)
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
	//s.bufWriteOffset -= uint32(noData)
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
	log.Debugf("[SERVER][TX] BLOCK DOWNLOAD INIT | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
	s.Send(s.txBuffer)
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
		s.Send(s.txBuffer)
		log.Debugf("[SERVER][TX] BLOCK DOWNLOAD SUB-BLOCK RES | x%x:x%x blksize %v %v",
			s.index,
			s.subindex,
			s.blockSize,
			s.txBuffer.Data,
		)
		return nil
	}

	// Determine the next block size depending on the free buffer space
	// If not enough space, try to empty buffer once by writting to OD
	count := s.buf.Available()
	if count > BlockMaxSize {
		count = BlockMaxSize
	} else if s.buf.Len() > 0 {
		// We have something in the buffer
		err := s.writeObjectDictionary(1, 0)
		if err != nil {
			return err
		}
		count := s.buf.Available()
		if count > BlockMaxSize {
			count = BlockMaxSize
		}
	}

	// Update parameters for next block
	s.blockSize = uint8(count)
	s.blockSequenceNb = 0
	s.txBuffer.Data[2] = s.blockSize
	s.state = stateDownloadBlkSubblockReq
	s.Send(s.txBuffer)

	if retransmit {
		log.Debugf("[SERVER][TX] BLOCK DOWNLOAD RESTART seqno prev=%v, blksize=%v", seqnoStart, s.blockSize)
		return nil
	}

	log.Debugf("[SERVER][TX] BLOCK DOWNLOAD SUB-BLOCK RES | x%x:x%x blksize %v %v",
		s.index,
		s.subindex,
		s.blockSize,
		s.txBuffer.Data,
	)

	return nil
}

func (s *SDOServer) txDownloadBlockEnd() {
	s.txBuffer.Data[0] = 0xA1
	log.Debugf("[SERVER][TX] BLOCK DOWNLOAD END | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
	s.Send(s.txBuffer)
	s.state = stateIdle
}
