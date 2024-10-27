package sdo

import (
	"encoding/binary"
	"fmt"
	"sync"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/internal/crc"
	"github.com/samsamfire/gocanopen/pkg/od"
	log "github.com/sirupsen/logrus"
)

type SDOServer struct {
	*canopen.BusManager
	mu                         sync.Mutex
	od                         *od.ObjectDictionary
	streamer                   od.Streamer
	nodeId                     uint8
	txBuffer                   canopen.Frame
	cobIdClientToServer        uint32
	cobIdServerToClient        uint32
	valid                      bool
	index                      uint16
	subindex                   uint8
	finished                   bool
	sizeIndicated              uint32
	sizeTransferred            uint32
	state                      internalState
	timeoutTimeUs              uint32
	timeoutTimer               uint32
	buffer                     []byte
	bufWriteOffset             uint32
	bufReadOffset              uint32
	rxNew                      bool
	response                   SDOResponse
	toggle                     uint8
	timeoutTimeBlockTransferUs uint32
	timeoutTimerBlock          uint32
	blockSequenceNb            uint8
	blockSize                  uint8
	blockNoData                uint8
	blockCRCEnabled            bool
	blockCRC                   crc.CRC16
	errorExtraInfo             error
}

// Handle [SDOServer] related RX CAN frames
func (server *SDOServer) Handle(frame canopen.Frame) {
	server.mu.Lock()
	defer server.mu.Unlock()
	if frame.DLC != 8 {
		return
	}
	if frame.Data[0] == 0x80 {
		// Client abort
		server.state = stateIdle
		abortCode := binary.LittleEndian.Uint32(frame.Data[4:])
		log.Warnf("[SERVER][RX] abort received from client : x%x (%v)", abortCode, Abort(abortCode))
	} else if server.rxNew {
		// Ignore message if previous message not processed
		log.Info("[SERVER][RX] ignoring message because still processing")
	} else if server.state == stateUploadBlkEndCrsp && frame.Data[0] == 0xA1 {
		// Block transferred ! go to idle
		server.state = stateIdle
	} else if server.state == stateDownloadBlkSubblockReq {
		// Condition should always pass but check
		if int(server.bufWriteOffset) <= (len(server.buffer) - (BlockSeqSize + 2)) {
			// Block download, copy data in handle
			state := stateDownloadBlkSubblockReq
			seqno := frame.Data[0] & 0x7F
			server.timeoutTimer = 0
			server.timeoutTimerBlock = 0
			// Check correct sequence number
			if seqno <= server.blockSize && seqno == (server.blockSequenceNb+1) {
				server.blockSequenceNb = seqno
				// Copy data
				copy(server.buffer[server.bufWriteOffset:], frame.Data[1:])
				server.bufWriteOffset += BlockSeqSize
				server.sizeTransferred += BlockSeqSize
				// Check if last segment
				if (frame.Data[0] & 0x80) != 0 {
					server.finished = true
					state = stateDownloadBlkSubblockRsp
					log.Debugf("[SERVER][RX] BLOCK DOWNLOAD END | x%x:x%x %v", server.index, server.subindex, frame.Data)
				} else if seqno == server.blockSize {
					// All segments in sub block transferred
					state = stateDownloadBlkSubblockRsp
					log.Debugf("[SERVER][RX] BLOCK DOWNLOAD SUB-BLOCK | x%x:x%x %v", server.index, server.subindex, frame.Data)
				} else {
					log.Debugf("[SERVER][RX] BLOCK DOWNLOAD SUB-BLOCK | x%x:x%x %v", server.index, server.subindex, frame.Data)
				}
				// If duplicate or sequence didn't start ignore, otherwise
				// seqno is wrong
			} else if seqno != server.blockSequenceNb && server.blockSequenceNb != 0 {
				state = stateDownloadBlkSubblockRsp
				log.Warnf("[SERVER][RX] BLOCK DOWNLOAD SUB-BLOCK | Wrong sequence number (got %v, previous %v) | x%x:x%x %v",
					seqno,
					server.blockSequenceNb,
					server.index,
					server.subindex,
					frame.Data,
				)
			} else {
				log.Warnf("[SERVER][RX] BLOCK DOWNLOAD SUB-BLOCK | Ignoring (got %v, expecting %v) | x%x:x%x %v",
					seqno,
					server.blockSequenceNb+1,
					server.index,
					server.subindex,
					frame.Data,
				)
			}

			if state != stateDownloadBlkSubblockReq {
				server.rxNew = false
				server.state = state
			}
		}
	} else if server.state == stateDownloadBlkSubblockRsp {
		// Ignore other server messages if response requested
	} else {
		// Copy data and set new flag
		server.response.raw = frame.Data
		server.rxNew = true
	}
}

// Process [SDOServer] state machine and TX CAN frames
// It returns the global server state and error if any
// This should be called periodically
func (server *SDOServer) Process(
	nmtIsPreOrOperationnal bool,
	timeDifferenceUs uint32,
	timerNextUs *uint32,
) (state uint8, err error) {
	server.mu.Lock()
	defer server.mu.Unlock()
	ret := waitingResponse
	var abortCode error
	if server.valid && server.state == stateIdle && !server.rxNew {
		ret = success
	} else if !nmtIsPreOrOperationnal || !server.valid {
		server.state = stateIdle
		server.rxNew = false
		ret = success
	} else if server.rxNew {
		response := server.response
		if server.state == stateIdle {
			upload := false
			abortCode = updateStateFromRequest(response.raw[0], &server.state, &upload)

			// Check object exists and accessible
			if abortCode == nil {
				var err error
				server.index = response.GetIndex()
				server.subindex = response.GetSubindex()
				server.streamer, err = od.NewStreamer(server.od.Index(server.index), server.subindex, false)
				if err != nil {
					abortCode = ConvertOdToSdoAbort(err.(od.ODR))
					server.state = stateAbort
				} else {
					if !server.streamer.HasAttribute(od.AttributeSdoRw) {
						abortCode = AbortUnsupportedAccess
						server.state = stateAbort
					} else if upload && !server.streamer.HasAttribute(od.AttributeSdoR) {
						abortCode = AbortWriteOnly
						server.state = stateAbort
					} else if !upload && !server.streamer.HasAttribute(od.AttributeSdoW) {
						abortCode = AbortReadOnly
						server.state = stateAbort
					}
				}
			}
			// Load data from OD
			if upload && abortCode == nil {
				server.bufReadOffset = 0
				server.bufWriteOffset = 0
				server.sizeTransferred = 0
				server.finished = false
				abortCode = server.readObjectDictionary(BlockSeqSize, false)
				if abortCode == nil {
					if server.finished {
						server.sizeIndicated = server.streamer.DataLength
						if server.sizeIndicated == 0 {
							server.sizeIndicated = server.bufWriteOffset
						} else if server.sizeIndicated != server.bufWriteOffset {
							server.errorExtraInfo = fmt.Errorf("size indicated %v != to buffer write offset %v", server.sizeIndicated, server.bufWriteOffset)
							abortCode = AbortDeviceIncompat
							server.state = stateAbort
						}
					} else {
						if !server.streamer.HasAttribute(od.AttributeStr) {
							server.sizeIndicated = server.streamer.DataLength
						} else {
							server.sizeIndicated = 0
						}
					}
				}

			}
		}

		if server.state != stateIdle && server.state != stateAbort {
			switch server.state {

			case stateDownloadInitiateReq:
				err := server.rxDownloadInitiate(response)
				if err != nil {
					server.state = stateAbort
					abortCode = err
				}

			case stateDownloadSegmentReq:
				err := server.rxDownloadSegment(response)
				if err != nil {
					server.state = stateAbort
					abortCode = err
				} else {
					if server.finished || (len(server.buffer)-int(server.bufWriteOffset) < (BlockSeqSize + 2)) {
						abortCode = server.writeObjectDictionary(0, 0)
						if abortCode != nil {
							break
						}
					}
					server.state = stateDownloadSegmentRsp
				}

			case stateUploadInitiateReq:
				log.Debugf("[SERVER][RX] UPLOAD EXPEDITED | x%x:x%x %v", server.index, server.subindex, response.raw)
				server.state = stateUploadInitiateRsp

			case stateUploadSegmentReq:
				err := server.rxUploadSegment(response)
				if err != nil {
					server.state = stateAbort
					abortCode = err
				}

			case stateDownloadBlkInitiateReq:
				err := server.rxDownloadBlockInitiate(response)
				if err != nil {
					server.state = stateAbort
					abortCode = err
				}

			case stateDownloadBlkSubblockReq:
				// This is done in receive handler

			case stateDownloadBlkEndReq:
				err := server.rxDownloadBlockEnd(response)
				var crcClient = crc.CRC16(0)
				if server.blockCRCEnabled {
					crcClient = response.GetCRCClient()
				}
				if err != nil {
					server.state = stateAbort
					abortCode = err
				} else {
					abortCode = server.writeObjectDictionary(2, crcClient)
					if abortCode != nil {
						break
					}
					server.state = stateDownloadBlkEndRsp
				}

			case stateUploadBlkInitiateReq:
				err := server.rxUploadBlockInitiate(response)
				if err != nil {
					server.state = stateAbort
					abortCode = err
				}

			case stateUploadBlkInitiateReq2:
				if response.raw[0] == 0xA3 {
					server.blockSequenceNb = 0
					server.state = stateUploadBlkSubblockSreq
				} else {
					abortCode = AbortCmd
					server.state = stateAbort
				}

			case stateUploadBlkSubblockSreq, stateUploadBlkSubblockCrsp:
				err := server.rxUploadSubBlock(response)
				if err != nil {
					server.state = stateAbort
					abortCode = err
				} else {
					// Refill buffer if needed
					abortCode = server.readObjectDictionary(uint32(server.blockSize)*BlockSeqSize, true)
					if abortCode != nil {
						break
					}

					if server.bufWriteOffset == server.bufReadOffset {
						server.state = stateUploadBlkEndSreq
					} else {
						server.blockSequenceNb = 0
						server.state = stateUploadBlkSubblockSreq
					}
				}

			default:
				abortCode = AbortCmd
				server.state = stateAbort

			}
		}
		server.timeoutTimer = 0
		timeDifferenceUs = 0
		server.rxNew = false
	}

	if ret == waitingResponse {
		if server.timeoutTimer < server.timeoutTimeUs {
			server.timeoutTimer += timeDifferenceUs
		}
		if server.timeoutTimer >= server.timeoutTimeUs {
			abortCode = AbortTimeout
			server.state = stateAbort
			log.Errorf("[SERVER] TIMEOUT %v, State : %v", server.timeoutTimer, server.state)

		} else if timerNextUs != nil {
			diff := server.timeoutTimeUs - server.timeoutTimer
			if *timerNextUs > diff {
				*timerNextUs = diff
			}
		}
		// Timeout for subblocks
		if server.state == stateDownloadBlkSubblockReq {
			if server.timeoutTimerBlock < server.timeoutTimeBlockTransferUs {
				server.timeoutTimerBlock += timeDifferenceUs
			}
			if server.timeoutTimerBlock >= server.timeoutTimeBlockTransferUs {
				server.state = stateDownloadBlkSubblockRsp
				server.rxNew = false
			} else if timerNextUs != nil {
				diff := server.timeoutTimeBlockTransferUs - server.timeoutTimerBlock
				if *timerNextUs > diff {
					*timerNextUs = diff
				}
			}
		}
	}

	if ret == waitingResponse {
		server.txBuffer.Data = [8]byte{0}

		switch server.state {
		case stateDownloadInitiateRsp:
			ret = server.txDownloadInitiate()

		case stateDownloadSegmentRsp:
			ret = server.txDownloadSegment()

		case stateUploadInitiateRsp:
			ret = server.txUploadInitiate()

		case stateUploadSegmentRsp:
			abortCode, ret = server.txUploadSegment()

		case stateDownloadBlkInitiateRsp:
			server.txDownloadBlockInitiate()

		case stateDownloadBlkSubblockRsp:
			abortCode = server.txDownloadBlockSubBlock()

		case stateDownloadBlkEndRsp:
			server.txBuffer.Data[0] = 0xA1
			log.Debugf("[SERVER][TX] BLOCK DOWNLOAD END | x%x:x%x %v", server.index, server.subindex, server.txBuffer.Data)
			server.Send(server.txBuffer)
			server.state = stateIdle
			ret = success

		case stateUploadBlkInitiateRsp:
			server.txUploadBlockInitiate()

		case stateUploadBlkSubblockSreq:
			abortCode = server.txUploadBlockSubBlock()
			if abortCode != nil {
				server.state = stateAbort
			}

		case stateUploadBlkEndSreq:
			server.txBuffer.Data[0] = 0xC1 | (server.blockNoData << 2)
			server.txBuffer.Data[1] = byte(server.blockCRC)
			server.txBuffer.Data[2] = byte(server.blockCRC >> 8)
			server.timeoutTimer = 0
			log.Debugf("[SERVER][TX] BLOCK UPLOAD END | x%x:x%x %v", server.index, server.subindex, server.txBuffer.Data)
			server.Send(server.txBuffer)
			server.state = stateUploadBlkEndCrsp
		}
	}

	if ret == waitingResponse {
		switch server.state {
		case stateAbort:
			if sdoAbort, ok := abortCode.(Abort); !ok {
				log.Errorf("[SERVER][TX] Abort internal error : unknown abort code : %v", abortCode)
				server.mu.Unlock()
				server.Abort(AbortGeneral)
				server.mu.Lock()
			} else {
				server.mu.Unlock()
				server.Abort(sdoAbort)
				server.mu.Lock()
			}
			server.state = stateIdle
		case stateDownloadBlkSubblockReq:
			ret = blockDownloadInProgress
		case stateUploadBlkSubblockSreq:
			ret = blockUploadInProgress
		}
	}
	return ret, abortCode
}

func (server *SDOServer) initRxTx(cobIdClientToServer uint32, cobIdServerToClient uint32) error {
	var ret error
	// Only proceed if parameters change (i.e. different client)
	if cobIdServerToClient == server.cobIdServerToClient && cobIdClientToServer == server.cobIdClientToServer {
		return nil
	}
	server.cobIdServerToClient = cobIdServerToClient
	server.cobIdClientToServer = cobIdClientToServer

	// Check the valid bit
	var CanIdC2S, CanIdS2C uint16
	if cobIdClientToServer&0x80000000 == 0 {
		CanIdC2S = uint16(cobIdClientToServer & 0x7FF)
	} else {
		CanIdC2S = 0
	}
	if cobIdServerToClient&0x80000000 == 0 {
		CanIdS2C = uint16(cobIdServerToClient & 0x7FF)
	} else {
		CanIdS2C = 0
	}
	if CanIdC2S != 0 && CanIdS2C != 0 {
		server.valid = true
	} else {
		CanIdC2S = 0
		CanIdS2C = 0
		server.valid = false
	}
	// Configure buffers, if initializing then insert in buffer, otherwise, update
	ret = server.Subscribe(uint32(CanIdC2S), 0x7FF, false, server)
	if ret != nil {
		server.valid = false
		return ret
	}
	server.txBuffer = canopen.NewFrame(uint32(CanIdS2C), 0, 8)
	return ret
}

func (server *SDOServer) writeObjectDictionary(crcOperation uint, crcClient crc.CRC16) error {

	bufferOffsetWriteOriginal := server.bufWriteOffset

	if server.finished {
		// Check size
		if server.sizeIndicated > 0 && server.sizeTransferred > server.sizeIndicated {
			server.state = stateAbort
			return AbortDataLong
		} else if server.sizeIndicated > 0 && server.sizeTransferred < server.sizeIndicated {
			server.state = stateAbort
			return AbortDataShort
		}
		// Golang does not have null termination characters so nothing particular to do
		// Stream data should be limited to the sent value

		varSizeInOd := server.streamer.DataLength
		if server.streamer.HasAttribute(od.AttributeStr) &&
			(varSizeInOd == 0 || server.sizeTransferred < varSizeInOd) &&
			int(server.bufWriteOffset+2) <= len(server.buffer) {
			server.buffer[server.bufWriteOffset] = 0x00
			server.bufWriteOffset++
			server.sizeTransferred++
			if varSizeInOd == 0 || server.sizeTransferred < varSizeInOd {
				server.buffer[server.bufWriteOffset] = 0x00
				server.bufWriteOffset++
				server.sizeTransferred++
			}
			server.streamer.DataLength = server.sizeTransferred
		} else if varSizeInOd == 0 {
			server.streamer.DataLength = server.sizeTransferred
		} else if server.sizeTransferred != varSizeInOd {
			if server.sizeTransferred > varSizeInOd {
				server.state = stateAbort
				return AbortDataLong
			} else if server.sizeTransferred < varSizeInOd {
				server.state = stateAbort
				return AbortDataShort
			}
		}

	} else if server.sizeIndicated > 0 && server.sizeTransferred > server.sizeIndicated {
		// Still check if not bigger than max size
		server.state = stateAbort
		return AbortDataLong
	}

	// Calculate CRC
	if server.blockCRCEnabled && crcOperation > 0 {
		server.blockCRC.Block(server.buffer[:bufferOffsetWriteOriginal])
		if crcOperation == 2 && crcClient != server.blockCRC {
			server.state = stateAbort
			server.errorExtraInfo = fmt.Errorf("server was expecting %v but got %v", server.blockCRC, crcClient)
			return AbortCRC
		}
	}

	// Write the data
	_, ret := server.streamer.Write(server.buffer[:server.bufWriteOffset])
	server.bufWriteOffset = 0
	if ret != nil && ret != od.ErrPartial {
		server.state = stateAbort
		return ConvertOdToSdoAbort(ret.(od.ODR))
	} else if server.finished && ret == od.ErrPartial {
		server.state = stateAbort
		return AbortDataShort
	} else if !server.finished && ret == nil {
		server.state = stateAbort
		return AbortDataLong
	}
	return nil
}

func (server *SDOServer) readObjectDictionary(countMinimum uint32, calculateCRC bool) error {
	buffered := server.bufWriteOffset - server.bufReadOffset
	if !server.finished && buffered < countMinimum {
		// Move buffered bytes to beginning
		copy(server.buffer, server.buffer[server.bufReadOffset:server.bufReadOffset+buffered])
		server.bufReadOffset = 0
		server.bufWriteOffset = buffered

		// Read from OD into the buffer
		countRd, err := server.streamer.Read(server.buffer[buffered:])

		if err != nil && err != od.ErrPartial {
			server.state = stateAbort
			return ConvertOdToSdoAbort(err.(od.ODR))
		}

		// Stop sending at null termination if string
		if countRd > 0 && server.streamer.HasAttribute(od.AttributeStr) {
			server.buffer[countRd+int(buffered)] = 0
			countStr := int(server.streamer.DataLength)
			for i, v := range server.buffer[buffered:] {
				if v == 0 {
					countStr = i
					break
				}
			}
			if countStr == 0 {
				countStr = 1
			}
			if countStr < countRd {
				// String terminator found
				countRd = countStr
				err = nil
				server.streamer.DataLength = server.sizeTransferred + uint32(countRd)
			}
		}

		server.bufWriteOffset = buffered + uint32(countRd) // Move offset write by countRd (number of read bytes)
		if server.bufWriteOffset == 0 || err == od.ErrPartial {
			server.finished = false
			if server.bufWriteOffset < countMinimum {
				server.state = stateAbort
				server.errorExtraInfo = fmt.Errorf("buffer offset write %v is less than the minimum count %v", server.bufWriteOffset, countMinimum)
				return AbortDeviceIncompat
			}
		} else {
			server.finished = true
		}
		if calculateCRC && server.blockCRCEnabled {
			// Calculate CRC for the read data
			server.blockCRC.Block(server.buffer[buffered:server.bufWriteOffset])
		}

	}

	return nil
}

func updateStateFromRequest(stateReq uint8, state *internalState, upload *bool) error {
	*upload = false
	if (stateReq & 0xF0) == 0x20 {
		*state = stateDownloadInitiateReq
	} else if stateReq == 0x40 {
		*upload = true
		*state = stateUploadInitiateReq
	} else if (stateReq & 0xF9) == 0xC0 {
		*state = stateDownloadBlkInitiateReq
	} else if (stateReq & 0xFB) == 0xA0 {
		*upload = true
		*state = stateUploadBlkInitiateReq
	} else {
		*state = stateAbort
		return AbortCmd
	}
	return nil
}

// Create & send abort on bus
func (server *SDOServer) Abort(abortCode Abort) {
	server.mu.Lock()
	defer server.mu.Unlock()
	code := uint32(abortCode)
	server.txBuffer.Data[0] = 0x80
	server.txBuffer.Data[1] = uint8(server.index)
	server.txBuffer.Data[2] = uint8(server.index >> 8)
	server.txBuffer.Data[3] = server.subindex
	binary.LittleEndian.PutUint32(server.txBuffer.Data[4:], code)
	server.Send(server.txBuffer)
	log.Warnf("[SERVER][TX] SERVER ABORT | x%x:x%x | %v (x%x)", server.index, server.subindex, abortCode, code)
	if server.errorExtraInfo != nil {
		log.Warnf("[SERVER][TX] SERVER ABORT | %v", server.errorExtraInfo)
		server.errorExtraInfo = nil
	}
}

func NewSDOServer(
	bm *canopen.BusManager,
	odict *od.ObjectDictionary,
	nodeId uint8,
	timeoutMs uint32,
	entry12xx *od.Entry,
) (*SDOServer, error) {
	server := &SDOServer{BusManager: bm}
	if odict == nil || bm == nil || entry12xx == nil {
		return nil, canopen.ErrIllegalArgument
	}
	server.od = odict
	server.streamer = od.Streamer{}
	server.buffer = make([]byte, 1000)
	server.bufReadOffset = 0
	server.bufWriteOffset = 0
	server.nodeId = nodeId
	server.timeoutTimeUs = timeoutMs * 1000
	server.timeoutTimeBlockTransferUs = timeoutMs * 700
	var canIdClientToServer uint16
	var canIdServerToClient uint16
	if entry12xx.Index == 0x1200 {
		// Default channels
		if nodeId < 1 || nodeId > BlockMaxSize {
			log.Errorf("SDO server node id is not valid : %x", nodeId)
			return nil, canopen.ErrIllegalArgument
		}
		canIdClientToServer = ClientServiceId + uint16(nodeId)
		canIdServerToClient = ServerServiceId + uint16(nodeId)
		server.valid = true
		entry12xx.PutUint32(1, uint32(canIdClientToServer), true)
		entry12xx.PutUint32(2, uint32(canIdServerToClient), true)
	} else if entry12xx.Index > 0x1200 && entry12xx.Index <= 0x1200+0x7F {
		// Configure other channels
		maxSubIndex, err0 := entry12xx.Uint8(0)
		cobIdClientToServer32, err1 := entry12xx.Uint32(1)
		cobIdServerToClient32, err2 := entry12xx.Uint32(2)
		if err0 != nil || (maxSubIndex != 2 && maxSubIndex != 3) ||
			err1 != nil || err2 != nil {
			log.Errorf("Error when retreiving sdo server parameters : %v, %v, %v, %v", err0, err1, err2, maxSubIndex)
			return nil, canopen.ErrOdParameters
		}
		if (cobIdClientToServer32 & 0x80000000) == 0 {
			canIdClientToServer = uint16(cobIdClientToServer32 & 0x7FF)
		} else {
			canIdClientToServer = 0
		}
		if (cobIdServerToClient32 & 0x80000000) == 0 {
			canIdServerToClient = uint16(cobIdServerToClient32 & 0x7FF)
		} else {
			canIdServerToClient = 0
		}
		entry12xx.AddExtension(server, od.ReadEntryDefault, writeEntry1201)

	} else {
		return nil, canopen.ErrIllegalArgument
	}
	server.rxNew = false
	server.cobIdClientToServer = 0
	server.cobIdServerToClient = 0
	return server, server.initRxTx(uint32(canIdClientToServer), uint32(canIdServerToClient))

}
