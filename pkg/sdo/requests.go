package sdo

import (
	"encoding/binary"
	"fmt"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/internal/crc"
	"github.com/samsamfire/gocanopen/pkg/od"
	log "github.com/sirupsen/logrus"
)

const (
	sizeIndicated      = 1 << 0
	sizeNotIndicated   = 0 << 0
	transferExpedited  = 1 << 1
	transferSegemented = 0 << 1
)

// type DownloadInitiate [8]byte

// // Check if expedited or segmented type
// // Field "e" in CiA 301
// func (d DownloadInitiate) TransferExpedited() bool {
// 	return (d[0] & TransferExpedited) > 0
// }

// // Check if size indicated
// // Field "s" in CiA 301
// func (d DownloadInitiate) SizeIndicated()

func (s *SDOServer) rxDownloadInitiate(response SDOResponse) error {
	cmd := response.raw[0]

	// Segmented transfer
	if (cmd & transferExpedited) == 0 {
		if (cmd & sizeIndicated) == 0 {
			s.sizeIndicated = 0
			s.state = stateDownloadInitiateRsp
			s.finished = false
			return nil
		}

		log.Debugf("[SERVER][RX] DOWNLOAD SEGMENTED | x%x:x%x %v", s.index, s.subindex, response.raw)
		// Segmented transfer check if size indicated
		sizeInOd := s.streamer.DataLength
		s.sizeIndicated = binary.LittleEndian.Uint32(response.raw[4:])

		// Check if size matches
		if sizeInOd > 0 {
			if s.sizeIndicated > uint32(sizeInOd) {
				return AbortDataLong
			} else if s.sizeIndicated < uint32(sizeInOd) && !s.streamer.HasAttribute(od.AttributeStr) {
				return AbortDataShort
			}
		}
		s.state = stateDownloadInitiateRsp
		s.finished = false
		return nil
	}

	// Expedited transfer
	log.Debugf("[SERVER][RX] DOWNLOAD EXPEDITED | x%x:x%x %v", s.index, s.subindex, response.raw)

	// Expedited 4 bytes of data max
	sizeInOd := s.streamer.DataLength
	dataSizeToWrite := 4
	if (cmd & sizeIndicated) != 0 {
		dataSizeToWrite -= (int(response.raw[0]) >> 2) & 0x03
	} else if sizeInOd > 0 && sizeInOd < 4 {
		dataSizeToWrite = int(sizeInOd)
	}
	// Create temporary buffer
	buf := make([]byte, 6)
	copy(buf, response.raw[4:4+dataSizeToWrite])
	if s.streamer.HasAttribute(od.AttributeStr) &&
		(sizeInOd == 0 || uint32(dataSizeToWrite) < sizeInOd) {
		delta := sizeInOd - uint32(dataSizeToWrite)
		if delta == 1 {
			dataSizeToWrite += 1
		} else {
			dataSizeToWrite += 2
		}
		s.streamer.DataLength = uint32(dataSizeToWrite)
	} else if sizeInOd == 0 {
		s.streamer.DataLength = uint32(dataSizeToWrite)
	} else if dataSizeToWrite != int(sizeInOd) {
		if dataSizeToWrite > int(sizeInOd) {
			return AbortDataLong
		} else {
			return AbortDataShort
		}
	}
	_, err := s.streamer.Write(buf[:dataSizeToWrite])
	if err != nil {
		return ConvertOdToSdoAbort(err.(od.ODR))
	}
	s.state = stateDownloadInitiateRsp
	s.finished = true
	return nil
}

func (s *SDOServer) rxDownloadSegment(response SDOResponse) error {
	if (response.raw[0] & 0xE0) != 0x00 {
		return AbortCmd
	}

	log.Debugf("[SERVER][RX] DOWNLOAD SEGMENT | x%x:x%x %v", s.index, s.subindex, response.raw)
	s.finished = (response.raw[0] & 0x01) != 0
	toggle := response.GetToggle()
	if toggle != s.toggle {
		return AbortToggleBit
	}
	// Get size and write to buffer
	count := BlockSeqSize - ((response.raw[0] >> 1) & 0x07)
	copy(s.buffer[s.bufWriteOffset:], response.raw[1:1+count])
	s.bufWriteOffset += uint32(count)
	s.sizeTransferred += uint32(count)

	if s.streamer.DataLength > 0 && s.sizeTransferred > s.streamer.DataLength {
		return AbortDataLong
	}
	return nil
}

func (s *SDOServer) rxUploadSegment(response SDOResponse) error {
	log.Debugf("[SERVER][RX] UPLOAD SEGMENTED | x%x:x%x %v", s.index, s.subindex, response.raw)
	if (response.raw[0] & 0xEF) != 0x60 {
		return AbortCmd
	}
	toggle := response.GetToggle()
	if toggle != s.toggle {
		return AbortToggleBit
	}
	s.state = stateUploadSegmentRsp
	return nil
}

func (s *SDOServer) rxDownloadBlockInitiate(response SDOResponse) error {
	s.blockCRCEnabled = response.IsCRCEnabled()
	// Check if size indicated
	if (response.raw[0] & 0x02) != 0 {
		sizeInOd := s.streamer.DataLength
		s.sizeIndicated = binary.LittleEndian.Uint32(response.raw[4:])
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
		response.raw,
	)
	s.state = stateDownloadBlkInitiateRsp
	s.finished = false
	return nil
}

func (s *SDOServer) rxDownloadBlockEnd(response SDOResponse) error {
	log.Debugf("[SERVER][RX] BLOCK DOWNLOAD END | x%x:x%x %v", s.index, s.subindex, response.raw)
	if (response.raw[0] & 0xE3) != 0xC1 {
		return AbortCmd
	}
	// Get number of data bytes in last segment, that do not
	// contain data. Then reduce buffer
	noData := (response.raw[0] >> 2) & 0x07
	if s.bufWriteOffset <= uint32(noData) {
		s.errorExtraInfo = fmt.Errorf("internal buffer and end of block download are inconsitent")
		return AbortDeviceIncompat
	}
	s.sizeTransferred -= uint32(noData)
	s.bufWriteOffset -= uint32(noData)

	var crcClient = crc.CRC16(0)
	if s.blockCRCEnabled {
		crcClient = response.GetCRCClient()
	}
	err := s.writeObjectDictionary(2, crcClient)
	if err != nil {
		return err
	}
	s.state = stateDownloadBlkEndRsp
	return nil
}

func (s *SDOServer) rxUploadBlockInitiate(response SDOResponse) error {
	// If protocol switch threshold (byte 5) is larger than data
	// size of OD var, then switch to segmented
	if s.sizeIndicated > 0 && response.raw[5] > 0 && uint32(response.raw[5]) >= s.sizeIndicated {
		s.state = stateUploadInitiateRsp
		return nil
	}
	if (response.raw[0] & 0x04) != 0 {
		s.blockCRCEnabled = true
		s.blockCRC = crc.CRC16(0)
		s.blockCRC.Block(s.buffer[:s.bufWriteOffset])
	} else {
		s.blockCRCEnabled = false
	}
	// Get block size and check okay
	s.blockSize = response.GetBlockSize()
	log.Debugf("[SERVER][RX] UPLOAD BLOCK INIT | x%x:x%x %v | crc : %v, blksize :%v", s.index, s.subindex, response.raw, s.blockCRCEnabled, s.blockSize)
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

func (s *SDOServer) rxUploadSubBlock(response SDOResponse) error {
	if response.raw[0] != 0xA2 {
		return AbortCmd
	}
	log.Debugf("[SERVER][RX] BLOCK UPLOAD END SUB-BLOCK | blksize %v | x%x:x%x %v",
		response.raw[2],
		s.index,
		s.subindex,
		response.raw,
	)
	// Check block size
	s.blockSize = response.raw[2]
	if s.blockSize < 1 || s.blockSize > BlockMaxSize {
		return AbortBlockSize
	}
	// Check number of segments
	if response.raw[1] < s.blockSequenceNb {
		// Some error occurd, re-transmit missing chunks
		cntFailed := s.blockSequenceNb - response.raw[1]
		cntFailed = cntFailed*BlockSeqSize - s.blockNoData
		s.bufReadOffset -= uint32(cntFailed)
		s.sizeTransferred -= uint32(cntFailed)
	} else if response.raw[1] > s.blockSequenceNb {
		return AbortCmd
	}
	return nil
}

// Process an incoming request from client
// Depending on request type, determine if a response is expected
// from s
func (s *SDOServer) processIncoming(frame canopen.Frame) error {
	if frame.Data[0] == 0x80 {
		// Client abort
		s.state = stateIdle
		abortCode := binary.LittleEndian.Uint32(frame.Data[4:])
		log.Warnf("[SERVER][RX] abort received from client : x%x (%v)", abortCode, Abort(abortCode))
		return nil
	}

	if s.state == stateUploadBlkEndCrsp && frame.Data[0] == 0xA1 {
		// Block transferred ! go to idle
		s.state = stateIdle
		return nil
	}

	if s.state == stateDownloadBlkSubblockReq {
		// Condition should always pass but check
		if int(s.bufWriteOffset) <= (len(s.buffer) - (BlockSeqSize + 2)) {
			// Block download, copy data in handle
			state := stateDownloadBlkSubblockReq
			seqno := frame.Data[0] & 0x7F
			s.timeoutTimer = 0
			s.timeoutTimerBlock = 0
			// Check correct sequence number
			if seqno <= s.blockSize && seqno == (s.blockSequenceNb+1) {
				s.blockSequenceNb = seqno
				// Copy data
				copy(s.buffer[s.bufWriteOffset:], frame.Data[1:])
				s.bufWriteOffset += BlockSeqSize
				s.sizeTransferred += BlockSeqSize
				// Check if last segment
				if (frame.Data[0] & 0x80) != 0 {
					s.finished = true
					state = stateDownloadBlkSubblockRsp
					log.Debugf("[SERVER][RX] BLOCK DOWNLOAD END | x%x:x%x %v", s.index, s.subindex, frame.Data)
				} else if seqno == s.blockSize {
					// All segments in sub block transferred
					state = stateDownloadBlkSubblockRsp
					log.Debugf("[SERVER][RX] BLOCK DOWNLOAD SUB-BLOCK | x%x:x%x %v", s.index, s.subindex, frame.Data)
				} else {
					log.Debugf("[SERVER][RX] BLOCK DOWNLOAD SUB-BLOCK | x%x:x%x %v", s.index, s.subindex, frame.Data)
				}
				// If duplicate or sequence didn't start ignore, otherwise
				// seqno is wrong
			} else if seqno != s.blockSequenceNb && s.blockSequenceNb != 0 {
				state = stateDownloadBlkSubblockRsp
				log.Warnf("[SERVER][RX] BLOCK DOWNLOAD SUB-BLOCK | Wrong sequence number (got %v, previous %v) | x%x:x%x %v",
					seqno,
					s.blockSequenceNb,
					s.index,
					s.subindex,
					frame.Data,
				)
			} else {
				log.Warnf("[SERVER][RX] BLOCK DOWNLOAD SUB-BLOCK | Ignoring (got %v, expecting %v) | x%x:x%x %v",
					seqno,
					s.blockSequenceNb+1,
					s.index,
					s.subindex,
					frame.Data,
				)
			}

			if state != stateDownloadBlkSubblockReq {
				s.state = state
			}
		}
		return nil
	}
	if s.state == stateDownloadBlkSubblockRsp {
		// Ignore other s messages if response requested
		return nil
	}

	// Copy data and set new flag
	s.response.raw = frame.Data
	response := s.response
	var abortCode error

	// If we are in idle, we need to create a streamer object to
	// access the relevant OD entry.
	// Determine if we need to read / write to OD.
	if s.state == stateIdle {
		upload := false
		err := updateStateFromRequest(response.raw[0], &s.state, &upload)
		if err != nil {
			return err
		}

		// Check object exists and has correct attributes
		// i.e. readable or writable depending on what has been
		// requested
		err = s.updateStreamer(response, upload)
		if err != nil {
			return err
		}
		// In case of reading, we need to prepare data straigth
		// away
		if upload {
			err = s.prepareRx()
			if err != nil {
				return err
			}
		}
	}

	var err error = nil
	if s.state != stateIdle && s.state != stateAbort {
		switch s.state {

		case stateDownloadInitiateReq:
			err = s.rxDownloadInitiate(response)

		case stateDownloadSegmentReq:
			err = s.rxDownloadSegment(response)
			if err != nil {
				return err
			} else {
				if s.finished || (len(s.buffer)-int(s.bufWriteOffset) < (BlockSeqSize + 2)) {
					abortCode = s.writeObjectDictionary(0, 0)
					if abortCode != nil {
						break
					}
				}
				s.state = stateDownloadSegmentRsp
			}

		case stateUploadInitiateReq:
			log.Debugf("[SERVER][RX] UPLOAD EXPEDITED | x%x:x%x %v", s.index, s.subindex, response.raw)
			s.state = stateUploadInitiateRsp

		case stateUploadSegmentReq:
			err = s.rxUploadSegment(response)

		case stateDownloadBlkInitiateReq:
			err = s.rxDownloadBlockInitiate(response)

		case stateDownloadBlkSubblockReq:
			// This is done in receive handler

		case stateDownloadBlkEndReq:
			err = s.rxDownloadBlockEnd(response)

		case stateUploadBlkInitiateReq:
			err = s.rxUploadBlockInitiate(response)

		case stateUploadBlkInitiateReq2:
			if response.raw[0] == 0xA3 {
				s.blockSequenceNb = 0
				s.state = stateUploadBlkSubblockSreq
			} else {
				return AbortCmd
			}

		case stateUploadBlkSubblockSreq, stateUploadBlkSubblockCrsp:
			err = s.rxUploadSubBlock(response)
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

		default:
			return AbortCmd

		}
	}
	s.timeoutTimer = 0

	return err
}
