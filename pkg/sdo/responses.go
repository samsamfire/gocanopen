package sdo

import (
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
		log.Errorf("[SERVER][TX] Abort internal error : unknown abort code : %v", err)
		s.SendAbort(AbortGeneral)
	} else {
		s.SendAbort(sdoAbort)
	}
	s.state = stateIdle
}
