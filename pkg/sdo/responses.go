package sdo

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

	case stateUploadExpeditedRsp:
		s.txUploadExpedited()

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

func (s *SDOServer) txAbort(err error) {
	if sdoAbort, ok := err.(Abort); !ok {
		s.logger.Error("[TX] Abort internal error : unknown abort code", "err", err)
		s.SendAbort(AbortGeneral)
	} else {
		s.SendAbort(sdoAbort)
	}
	s.state = stateIdle
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

	if ret == waitingResponse {
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

		case stateDownloadBlkEndRsp:
			s.txBuffer.Data[0] = 0xA1
			log.Debugf("[SERVER][TX] BLOCK DOWNLOAD END | x%x:x%x %v", s.index, s.subindex, s.txBuffer.Data)
			s.Send(s.txBuffer)
			s.state = stateIdle
			ret = success

		case stateUploadBlkInitiateRsp:
			s.txUploadBlockInitiate()

		case stateUploadBlkSubblockSreq:
			abortCode = s.txUploadBlockSubBlock()
			if abortCode != nil {
				s.state = stateAbort
			}

		case stateUploadBlkEndSreq:
			s.txUploadBlockEnd()
		}
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
