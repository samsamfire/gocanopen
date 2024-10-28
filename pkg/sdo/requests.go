package sdo

import (
	"encoding/binary"
	"fmt"

	"github.com/samsamfire/gocanopen/internal/crc"
	"github.com/samsamfire/gocanopen/pkg/od"
	log "github.com/sirupsen/logrus"
)

func (s *SDOServer) processIncoming(rx SDOMessage) error {

	if rx.raw[0] == CSAbort {
		s.state = stateIdle
		abortCode := binary.LittleEndian.Uint32(rx.raw[4:])
		log.Warnf("[SERVER][RX] abort received from client : x%x (%v)", abortCode, Abort(abortCode))
		return nil
	}

	// Copy data and set new flag
	header := rx.raw[0]
	var abortCode error

	// Determine if we need to read / write to OD.
	// If we are in idle, we need to create a streamer object to
	// access the relevant OD entry.
	if s.state == stateIdle {
		switch header & MaskCS {

		case CSDownloadInitiate:
			s.state = stateDownloadInitiateReq

		case CSUploadInitiate:
			s.state = stateUploadInitiateReq

		case CSDownloadBlockInitiate:
			if (header & MaskClientSubcommand) == initiateDownloadRequest {
				s.state = stateDownloadBlkInitiateReq
			} else {
				return AbortCmd
			}

		case CSUploadBlockInitiate:
			if (header & MaskClientSubcommandBlockUpload) == initiateUploadRequest {
				s.state = stateUploadBlkInitiateReq
			} else {
				return AbortCmd
			}

		default:
			return AbortCmd
		}

		// Check object exists and has correct attributes
		// i.e. readable or writable depending on what has been
		// requested
		err := s.updateStreamer(rx)
		if err != nil {
			return err
		}
	}

	// Process receive state machine
	var err error = nil

	switch s.state {

	case stateDownloadInitiateReq:
		err = s.rxDownloadInitiate(rx)

	case stateDownloadSegmentReq:
		err = s.rxDownloadSegment(rx)

	case stateUploadInitiateReq:
		log.Debugf("[SERVER][RX] UPLOAD EXPEDITED | x%x:x%x %v", s.index, s.subindex, rx.raw)
		s.state = stateUploadInitiateRsp

	case stateUploadSegmentReq:
		err = s.rxUploadSegment(rx)

	case stateDownloadBlkInitiateReq:
		err = s.rxDownloadBlockInitiate(rx)

	case stateDownloadBlkSubblockReq:
		err = s.rxDownloadBlockSubBlock(rx)

	case stateDownloadBlkEndReq:
		err = s.rxDownloadBlockEnd(rx)

	case stateUploadBlkInitiateReq:
		err = s.rxUploadBlockInitiate(rx)

	case stateUploadBlkInitiateReq2:
		if rx.raw[0] == 0xA3 {
			s.blockSequenceNb = 0
			s.state = stateUploadBlkSubblockSreq
		} else {
			return AbortCmd
		}

	case stateUploadBlkSubblockSreq, stateUploadBlkSubblockCrsp:
		err = s.rxUploadSubBlock(rx)
		if err != nil {
			return err
		} else {
			// Refill buffer if needed
			abortCode = s.readObjectDictionary(uint32(s.blockSize)*BlockSeqSize, true)
			if abortCode != nil {
				return abortCode
			}

			if s.bufWriteOffset == s.bufReadOffset {
				s.state = stateUploadBlkEndSreq
			} else {
				s.blockSequenceNb = 0
				s.state = stateUploadBlkSubblockSreq
			}
		}
	case stateUploadBlkEndCrsp:
		if rx.raw[0] == 0xA1 {
			// Block transferred ! go to idle
			s.state = stateIdle
			return nil
		} else {
			return AbortCmd
		}

	default:
		return AbortCmd

	}

	return err
}

func (s *SDOServer) rxDownloadInitiate(rx SDOMessage) error {

	// Segmented transfer type
	if !rx.IsExpedited() {
		log.Debugf("[SERVER][RX] DOWNLOAD SEGMENTED | x%x:x%x %v", s.index, s.subindex, rx.raw)

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
	log.Debugf("[SERVER][RX] DOWNLOAD EXPEDITED | x%x:x%x %v", s.index, s.subindex, rx.raw)

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

func (s *SDOServer) rxDownloadSegment(rx SDOMessage) error {
	if (rx.raw[0] & 0xE0) != 0x00 {
		return AbortCmd
	}

	log.Debugf("[SERVER][RX] DOWNLOAD SEGMENT | x%x:x%x %v", s.index, s.subindex, rx.raw)
	s.finished = (rx.raw[0] & 0x01) != 0
	toggle := rx.GetToggle()
	if toggle != s.toggle {
		return AbortToggleBit
	}
	// Get size and write to buffer
	count := BlockSeqSize - ((rx.raw[0] >> 1) & 0x07)
	copy(s.buffer[s.bufWriteOffset:], rx.raw[1:1+count])
	s.bufWriteOffset += uint32(count)
	s.sizeTransferred += uint32(count)

	if s.streamer.DataLength > 0 && s.sizeTransferred > s.streamer.DataLength {
		return AbortDataLong
	}

	if s.finished || (len(s.buffer)-int(s.bufWriteOffset) < (BlockSeqSize + 2)) {
		err := s.writeObjectDictionary(0, 0)
		if err != nil {
			return nil
		}
	}
	s.state = stateDownloadSegmentRsp

	return nil
}

func (s *SDOServer) rxUploadSegment(rx SDOMessage) error {
	log.Debugf("[SERVER][RX] UPLOAD SEGMENTED | x%x:x%x %v", s.index, s.subindex, rx.raw)
	if (rx.raw[0] & 0xEF) != 0x60 {
		return AbortCmd
	}
	toggle := rx.GetToggle()
	if toggle != s.toggle {
		return AbortToggleBit
	}
	s.state = stateUploadSegmentRsp
	return nil
}

func (s *SDOServer) rxDownloadBlockInitiate(rx SDOMessage) error {
	s.blockCRCEnabled = rx.IsCRCEnabled()
	// Check if size indicated
	if (rx.raw[0] & 0x02) != 0 {
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
	} else {
		s.sizeIndicated = 0
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
	// Condition should always pass but still check just in case
	if int(s.bufWriteOffset) > (len(s.buffer) - (BlockSeqSize + 2)) {
		return AbortGeneral
	}

	// Block download, copy data in handle
	seqno := rx.raw[0] & 0x7F

	// Check correct sequence number
	if seqno <= s.blockSize && seqno == (s.blockSequenceNb+1) {
		s.blockSequenceNb = seqno
		// Copy data
		copy(s.buffer[s.bufWriteOffset:], rx.raw[1:])
		s.bufWriteOffset += BlockSeqSize
		s.sizeTransferred += BlockSeqSize

		// Check if last block
		if (rx.raw[0] & 0x80) != 0 {
			s.finished = true
			s.state = stateDownloadBlkSubblockRsp
			log.Debugf("[SERVER][RX] BLOCK DOWNLOAD END | x%x:x%x %v", s.index, s.subindex, rx.raw)
			return nil
		}
		// Check if end of sub-block
		if seqno == s.blockSize {
			s.state = stateDownloadBlkSubblockRsp
			log.Debugf("[SERVER][RX] BLOCK DOWNLOAD SUB-BLOCK | x%x:x%x %v", s.index, s.subindex, rx.raw)
			return nil
		}
		// Regular sub-block
		log.Debugf("[SERVER][RX] BLOCK DOWNLOAD SUB-BLOCK | x%x:x%x %v", s.index, s.subindex, rx.raw)

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
	// Ignore
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
	if s.bufWriteOffset <= uint32(noData) {
		s.errorExtraInfo = fmt.Errorf("internal buffer and end of block download are inconsitent")
		return AbortDeviceIncompat
	}
	s.sizeTransferred -= uint32(noData)
	s.bufWriteOffset -= uint32(noData)

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

func (s *SDOServer) rxUploadBlockInitiate(rx SDOMessage) error {
	// If protocol switch threshold (byte 5) is larger than data
	// size of OD var, then switch to segmented
	if s.sizeIndicated > 0 && rx.raw[5] > 0 && uint32(rx.raw[5]) >= s.sizeIndicated {
		s.state = stateUploadInitiateRsp
		return nil
	}
	if (rx.raw[0] & 0x04) != 0 {
		s.blockCRCEnabled = true
		s.blockCRC = crc.CRC16(0)
		s.blockCRC.Block(s.buffer[:s.bufWriteOffset])
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
	if !s.finished && s.bufWriteOffset < uint32(s.blockSize)*BlockSeqSize {
		return AbortBlockSize
	}
	s.state = stateUploadBlkInitiateRsp
	return nil
}

func (s *SDOServer) rxUploadSubBlock(rx SDOMessage) error {
	if rx.raw[0] != 0xA2 {
		return AbortCmd
	}
	log.Debugf("[SERVER][RX] BLOCK UPLOAD END SUB-BLOCK | blksize %v | x%x:x%x %v",
		rx.raw[2],
		s.index,
		s.subindex,
		rx.raw,
	)
	// Check block size
	s.blockSize = rx.raw[2]
	if s.blockSize < 1 || s.blockSize > BlockMaxSize {
		return AbortBlockSize
	}
	// Check number of segments
	if rx.raw[1] < s.blockSequenceNb {
		// Some error occurd, re-transmit missing chunks
		cntFailed := s.blockSequenceNb - rx.raw[1]
		cntFailed = cntFailed*BlockSeqSize - s.blockNoData
		s.bufReadOffset -= uint32(cntFailed)
		s.sizeTransferred -= uint32(cntFailed)
	} else if rx.raw[1] > s.blockSequenceNb {
		return AbortCmd
	}
	return nil
}
