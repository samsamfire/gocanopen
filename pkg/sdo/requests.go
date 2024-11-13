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
