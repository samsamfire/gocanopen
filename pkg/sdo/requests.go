package sdo

import (
	"encoding/binary"
)

func (s *SDOServer) processIncoming(rx SDOMessage) error {

	if rx.raw[0] == CSAbort {
		s.state = stateIdle
		abortCode := binary.LittleEndian.Uint32(rx.raw[4:])
		s.logger.Warn("[RX] abort received from client", "code", abortCode, "description", Abort(abortCode))
		return nil
	}

	// Copy data
	header := rx.raw[0]

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
		err = s.rxUploadInitiate(rx)

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
				s.rxNew = false
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
	s.rxNew = true

	response := s.response
	var abortCode error

	// If we are in idle, we need to create a streamer object to
	// access the relevant OD entry.
	// Determine if we need to read / write to OD.
	if s.state == stateIdle {
		upload := false
		abortCode = updateStateFromRequest(response.raw[0], &s.state, &upload)

		// Check object exists and has correct attributes
		if abortCode == nil {
			abortCode = s.updateStreamer(response, upload)
			if abortCode != nil {
				s.state = stateAbort
			}
		}
		// In case of reading, we need to prepare data ASAP
		if upload && abortCode == nil {
			abortCode = s.prepareRx()
			if abortCode != nil {
				s.state = stateAbort
			}
		}
	}

	if s.state != stateIdle && s.state != stateAbort {
		switch s.state {

		case stateDownloadInitiateReq:
			err := s.rxDownloadInitiate(response)
			if err != nil {
				s.state = stateAbort
				abortCode = err
			}

		case stateDownloadSegmentReq:
			err := s.rxDownloadSegment(response)
			if err != nil {
				s.state = stateAbort
				abortCode = err
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
			err := s.rxUploadSegment(response)
			if err != nil {
				s.state = stateAbort
				abortCode = err
			}

		case stateDownloadBlkInitiateReq:
			err := s.rxDownloadBlockInitiate(response)
			if err != nil {
				s.state = stateAbort
				abortCode = err
			}

		case stateDownloadBlkSubblockReq:
			// This is done in receive handler

		case stateDownloadBlkEndReq:
			err := s.rxDownloadBlockEnd(response)
			var crcClient = crc.CRC16(0)
			if s.blockCRCEnabled {
				crcClient = response.GetCRCClient()
			}
			if err != nil {
				s.state = stateAbort
				abortCode = err
			} else {
				abortCode = s.writeObjectDictionary(2, crcClient)
				if abortCode != nil {
					break
				}
				s.state = stateDownloadBlkEndRsp
			}

		case stateUploadBlkInitiateReq:
			err := s.rxUploadBlockInitiate(response)
			if err != nil {
				s.state = stateAbort
				abortCode = err
			}

		case stateUploadBlkInitiateReq2:
			if response.raw[0] == 0xA3 {
				s.blockSequenceNb = 0
				s.state = stateUploadBlkSubblockSreq
			} else {
				abortCode = AbortCmd
				s.state = stateAbort
			}

		case stateUploadBlkSubblockSreq, stateUploadBlkSubblockCrsp:
			err := s.rxUploadSubBlock(response)
			if err != nil {
				s.state = stateAbort
				abortCode = err
			} else {
				// Refill buffer if needed
				abortCode = s.readObjectDictionary(uint32(s.blockSize)*BlockSeqSize, true)
				if abortCode != nil {
					break
				}

				if s.bufWriteOffset == s.bufReadOffset {
					s.state = stateUploadBlkEndSreq
				} else {
					s.blockSequenceNb = 0
					s.state = stateUploadBlkSubblockSreq
				}
			}

		default:
			abortCode = AbortCmd
			s.state = stateAbort

		}
	}
	s.timeoutTimer = 0
	s.rxNew = false

	return nil
}
