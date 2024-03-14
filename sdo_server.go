package canopen

import (
	"encoding/binary"
	"fmt"

	can "github.com/samsamfire/gocanopen/pkg/can"
	log "github.com/sirupsen/logrus"
)

type SDOServer struct {
	*busManager
	od                         *ObjectDictionary
	streamer                   *streamer
	nodeId                     uint8
	txBuffer                   can.Frame
	cobIdClientToServer        uint32
	cobIdServerToClient        uint32
	valid                      bool
	index                      uint16
	subindex                   uint8
	finished                   bool
	sizeIndicated              uint32
	sizeTransferred            uint32
	state                      SDOState
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
	blockCRC                   crc16
	errorExtraInfo             error
}

// Handle received messages
func (server *SDOServer) Handle(frame can.Frame) {
	if frame.DLC != 8 {
		return
	}
	if frame.Data[0] == 0x80 {
		// Client abort
		server.state = SDO_STATE_IDLE
		abortCode := binary.LittleEndian.Uint32(frame.Data[4:])
		log.Warnf("[SERVER][RX] abort received from client : x%x (%v)", abortCode, SDOAbortCode(abortCode))
	} else if server.rxNew {
		// Ignore message if previous message not processed
		log.Info("[SERVER][RX] ignoring message because still processing")
	} else if server.state == SDO_STATE_UPLOAD_BLK_END_CRSP && frame.Data[0] == 0xA1 {
		// Block transferred ! go to idle
		server.state = SDO_STATE_IDLE
	} else if server.state == SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ {
		// Condition should always pass but check
		if int(server.bufWriteOffset) <= (len(server.buffer) - (7 + 2)) {
			// Block download, copy data in handle
			state := SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ
			seqno := frame.Data[0] & 0x7F
			server.timeoutTimer = 0
			server.timeoutTimerBlock = 0
			// Check correct sequence number
			if seqno <= server.blockSize && seqno == (server.blockSequenceNb+1) {
				server.blockSequenceNb = seqno
				// Copy data
				copy(server.buffer[server.bufWriteOffset:], frame.Data[1:])
				server.bufWriteOffset += 7
				server.sizeTransferred += 7
				// Check if last segment
				if (frame.Data[0] & 0x80) != 0 {
					server.finished = true
					state = SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_RSP
					log.Debugf("[SERVER][RX] BLOCK DOWNLOAD END | x%x:x%x %v", server.index, server.subindex, frame.Data)
				} else if seqno == server.blockSize {
					// All segments in sub block transferred
					state = SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_RSP
					log.Debugf("[SERVER][RX] BLOCK DOWNLOAD SUB-BLOCK | x%x:x%x %v", server.index, server.subindex, frame.Data)
				} else {
					log.Debugf("[SERVER][RX] BLOCK DOWNLOAD SUB-BLOCK | x%x:x%x %v", server.index, server.subindex, frame.Data)
				}
				// If duplicate or sequence didn't start ignore, otherwise
				// seqno is wrong
			} else if seqno != server.blockSequenceNb && server.blockSequenceNb != 0 {
				state = SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_RSP
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

			if state != SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ {
				server.rxNew = false
				server.state = state
			}
		}
	} else if server.state == SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_RSP {
		//Ignore other server messages if response requested
	} else {
		// Copy data and set new flag
		server.response.raw = frame.Data
		server.rxNew = true
	}
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
	server.txBuffer = can.NewFrame(uint32(CanIdS2C), 0, 8)
	return ret
}

func (server *SDOServer) writeObjectDictionary(crcOperation uint, crcClient crc16) error {

	bufferOffsetWriteOriginal := server.bufWriteOffset

	if server.finished {
		// Check size
		if server.sizeIndicated > 0 && server.sizeTransferred > server.sizeIndicated {
			server.state = SDO_STATE_ABORT
			return SDO_ABORT_DATA_LONG
		} else if server.sizeIndicated > 0 && server.sizeTransferred < server.sizeIndicated {
			server.state = SDO_STATE_ABORT
			return SDO_ABORT_DATA_SHORT
		}
		// Golang does not have null termination characters so nothing particular to do
		// Stream data should be limited to the sent value

		varSizeInOd := server.streamer.stream.DataLength
		if server.streamer.stream.Attribute&ATTRIBUTE_STR != 0 &&
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
			server.streamer.stream.DataLength = server.sizeTransferred
		} else if varSizeInOd == 0 {
			server.streamer.stream.DataLength = server.sizeTransferred
		} else if server.sizeTransferred != varSizeInOd {
			if server.sizeTransferred > varSizeInOd {
				server.state = SDO_STATE_ABORT
				return SDO_ABORT_DATA_LONG
			} else if server.sizeTransferred < varSizeInOd {
				server.state = SDO_STATE_ABORT
				return SDO_ABORT_DATA_SHORT
			}
		}

	} else {
		// Still check if not bigger than max size
		if server.sizeIndicated > 0 && server.sizeTransferred > server.sizeIndicated {
			server.state = SDO_STATE_ABORT
			return SDO_ABORT_DATA_LONG
		}
	}

	// Calculate CRC
	if server.blockCRCEnabled && crcOperation > 0 {
		server.blockCRC.ccittBlock(server.buffer[:bufferOffsetWriteOriginal])
		if crcOperation == 2 && crcClient != server.blockCRC {
			server.state = SDO_STATE_ABORT
			server.errorExtraInfo = fmt.Errorf("server was expecting %v but got %v", server.blockCRC, crcClient)
			return SDO_ABORT_CRC
		}
	}

	// Write the data
	_, ret := server.streamer.Write(server.buffer[:server.bufWriteOffset])
	server.bufWriteOffset = 0
	if ret != nil && ret != ODR_PARTIAL {
		server.state = SDO_STATE_ABORT
		return ret.(ODR).GetSDOAbordCode()
	} else if server.finished && ret == ODR_PARTIAL {
		server.state = SDO_STATE_ABORT
		return SDO_ABORT_DATA_SHORT
	} else if !server.finished && ret == nil {
		server.state = SDO_STATE_ABORT
		return SDO_ABORT_DATA_LONG
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

		if err != nil && err != ODR_PARTIAL {
			server.state = SDO_STATE_ABORT
			return err.(ODR).GetSDOAbordCode()
		}

		// Stop sending at null termination if string
		if countRd > 0 && (server.streamer.stream.Attribute&ATTRIBUTE_STR) != 0 {
			server.buffer[countRd+int(buffered)] = 0
			countStr := int(server.streamer.stream.DataLength)
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
				server.streamer.stream.DataLength = server.sizeTransferred + uint32(countRd)
			}
		}

		server.bufWriteOffset = buffered + uint32(countRd) // Move offset write by countRd (number of read bytes)
		if server.bufWriteOffset == 0 || err == ODR_PARTIAL {
			server.finished = false
			if server.bufWriteOffset < countMinimum {
				server.state = SDO_STATE_ABORT
				server.errorExtraInfo = fmt.Errorf("buffer offset write %v is less than the minimum count %v", server.bufWriteOffset, countMinimum)
				return SDO_ABORT_DEVICE_INCOMPAT
			}
		} else {
			server.finished = true
		}
		if calculateCRC && server.blockCRCEnabled {
			// Calculate CRC for the read data
			server.blockCRC.ccittBlock(server.buffer[buffered:server.bufWriteOffset])
		}

	}

	return nil
}

func updateStateFromRequest(stateReq uint8, state *SDOState, upload *bool) error {
	*upload = false
	if (stateReq & 0xF0) == 0x20 {
		*state = SDO_STATE_DOWNLOAD_INITIATE_REQ
	} else if stateReq == 0x40 {
		*upload = true
		*state = SDO_STATE_UPLOAD_INITIATE_REQ
	} else if (stateReq & 0xF9) == 0xC0 {
		*state = SDO_STATE_DOWNLOAD_BLK_INITIATE_REQ
	} else if (stateReq & 0xFB) == 0xA0 {
		*upload = true
		*state = SDO_STATE_UPLOAD_BLK_INITIATE_REQ
	} else {
		*state = SDO_STATE_ABORT
		return SDO_ABORT_CMD
	}
	return nil
}

func (server *SDOServer) process(nmtIsPreOrOperationnal bool, timeDifferenceUs uint32, timerNextUs *uint32) (state uint8, err error) {
	ret := SDO_WAITING_RESPONSE
	var abortCode error
	if server.valid && server.state == SDO_STATE_IDLE && !server.rxNew {
		ret = SDO_SUCCESS
	} else if !nmtIsPreOrOperationnal || !server.valid {
		server.state = SDO_STATE_IDLE
		server.rxNew = false
		ret = SDO_SUCCESS
	} else if server.rxNew {
		response := server.response
		if server.state == SDO_STATE_IDLE {
			upload := false
			abortCode = updateStateFromRequest(response.raw[0], &server.state, &upload)

			// Check object exists and accessible
			if abortCode == nil {
				var err error
				server.index = response.GetIndex()
				server.subindex = response.GetSubindex()
				server.streamer, err = newStreamer(server.od.Index(server.index), server.subindex, false)
				if err != nil {
					abortCode = err.(ODR).GetSDOAbordCode()
					server.state = SDO_STATE_ABORT
				} else {
					if server.streamer.stream.Attribute&ATTRIBUTE_SDO_RW == 0 {
						abortCode = SDO_ABORT_UNSUPPORTED_ACCESS
						server.state = SDO_STATE_ABORT
					} else if upload && (server.streamer.stream.Attribute&ATTRIBUTE_SDO_R) == 0 {
						abortCode = SDO_ABORT_WRITEONLY
						server.state = SDO_STATE_ABORT
					} else if !upload && (server.streamer.stream.Attribute&ATTRIBUTE_SDO_W) == 0 {
						abortCode = SDO_ABORT_READONLY
						server.state = SDO_STATE_ABORT
					}
				}
			}
			// Load data from OD
			if upload && abortCode == nil {
				server.bufReadOffset = 0
				server.bufWriteOffset = 0
				server.sizeTransferred = 0
				server.finished = false
				abortCode = server.readObjectDictionary(7, false)
				if abortCode == nil {
					if server.finished {
						server.sizeIndicated = server.streamer.stream.DataLength
						if server.sizeIndicated == 0 {
							server.sizeIndicated = server.bufWriteOffset
						} else if server.sizeIndicated != server.bufWriteOffset {
							server.errorExtraInfo = fmt.Errorf("size indicated %v != to buffer write offset %v", server.sizeIndicated, server.bufWriteOffset)
							abortCode = SDO_ABORT_DEVICE_INCOMPAT
							server.state = SDO_STATE_ABORT
						}
					} else {
						if server.streamer.stream.Attribute&ATTRIBUTE_STR == 0 {
							server.sizeIndicated = server.streamer.stream.DataLength
						} else {
							server.sizeIndicated = 0
						}
					}
				}

			}
		}

		if server.state != SDO_STATE_IDLE && server.state != SDO_STATE_ABORT {
			switch server.state {
			case SDO_STATE_DOWNLOAD_INITIATE_REQ:
				if (response.raw[0] & 0x02) != 0 {
					log.Debugf("[SERVER][RX] DOWNLOAD EXPEDITED | x%x:x%x %v", server.index, server.subindex, response.raw)
					//Expedited 4 bytes of data max
					varSizeInOd := server.streamer.stream.DataLength
					dataSizeToWrite := 4
					if (response.raw[0] & 0x01) != 0 {
						dataSizeToWrite -= (int(response.raw[0]) >> 2) & 0x03
					} else if varSizeInOd > 0 && varSizeInOd < 4 {
						dataSizeToWrite = int(varSizeInOd)
					}
					//Create temporary buffer
					buf := make([]byte, 6)
					copy(buf, response.raw[4:4+dataSizeToWrite])
					if (server.streamer.stream.Attribute&ATTRIBUTE_STR) != 0 &&
						(varSizeInOd == 0 || uint32(dataSizeToWrite) < varSizeInOd) {
						delta := varSizeInOd - uint32(dataSizeToWrite)
						if delta == 1 {
							dataSizeToWrite += 1
						} else {
							dataSizeToWrite += 2
						}
						server.streamer.stream.DataLength = uint32(dataSizeToWrite)
					} else if varSizeInOd == 0 {
						server.streamer.stream.DataLength = uint32(dataSizeToWrite)
					} else if dataSizeToWrite != int(varSizeInOd) {
						if dataSizeToWrite > int(varSizeInOd) {
							abortCode = SDO_ABORT_DATA_LONG
						} else {
							abortCode = SDO_ABORT_DATA_SHORT
						}
						server.state = SDO_STATE_ABORT
						break
					}
					_, err := server.streamer.Write(buf[:dataSizeToWrite])
					if err != nil {
						abortCode = err.(ODR).GetSDOAbordCode()
						server.state = SDO_STATE_ABORT
						break
					} else {
						server.state = SDO_STATE_DOWNLOAD_INITIATE_RSP
						server.finished = true

					}
				} else {
					if (response.raw[0] & 0x01) != 0 {
						log.Debugf("[SERVER][RX] DOWNLOAD SEGMENTED | x%x:x%x %v", server.index, server.subindex, response.raw)
						// Segmented transfer check if size indicated
						sizeInOd := server.streamer.stream.DataLength
						server.sizeIndicated = binary.LittleEndian.Uint32(response.raw[4:])
						// Check if size matches
						if sizeInOd > 0 {
							if server.sizeIndicated > uint32(sizeInOd) {
								abortCode = SDO_ABORT_DATA_LONG
								server.state = SDO_STATE_ABORT
								break
							} else if server.sizeIndicated < uint32(sizeInOd) && (server.streamer.stream.Attribute&ATTRIBUTE_STR) == 0 {
								abortCode = SDO_ABORT_DATA_SHORT
								server.state = SDO_STATE_ABORT
								break
							}
						}
					} else {
						server.sizeIndicated = 0
					}
					server.state = SDO_STATE_DOWNLOAD_INITIATE_RSP
					server.finished = false
				}

			case SDO_STATE_DOWNLOAD_SEGMENT_REQ:
				if (response.raw[0] & 0xE0) == 0x00 {
					log.Debugf("[SERVER][RX] DOWNLOAD SEGMENT | x%x:x%x %v", server.index, server.subindex, response.raw)
					server.finished = (response.raw[0] & 0x01) != 0
					toggle := response.GetToggle()
					if toggle != server.toggle {
						abortCode = SDO_ABORT_TOGGLE_BIT
						server.state = SDO_STATE_ABORT
						break
					}
					// Get size and write to buffer
					count := 7 - ((response.raw[0] >> 1) & 0x07)
					copy(server.buffer[server.bufWriteOffset:], response.raw[1:1+count])
					server.bufWriteOffset += uint32(count)
					server.sizeTransferred += uint32(count)

					if server.streamer.stream.DataLength > 0 && server.sizeTransferred > server.streamer.stream.DataLength {
						abortCode = SDO_ABORT_DATA_LONG
						server.state = SDO_STATE_ABORT
						break
					}
					if server.finished || (len(server.buffer)-int(server.bufWriteOffset) < (7 + 2)) {
						abortCode = server.writeObjectDictionary(0, 0)
						if abortCode != nil {
							break
						}
					}
					server.state = SDO_STATE_DOWNLOAD_SEGMENT_RSP
				} else {
					abortCode = SDO_ABORT_CMD
					server.state = SDO_STATE_ABORT
				}

			case SDO_STATE_UPLOAD_INITIATE_REQ:
				log.Debugf("[SERVER][RX] UPLOAD EXPEDITED | x%x:x%x %v", server.index, server.subindex, response.raw)
				server.state = SDO_STATE_UPLOAD_INITIATE_RSP

			case SDO_STATE_UPLOAD_SEGMENT_REQ:
				log.Debugf("[SERVER][RX] UPLOAD SEGMENTED | x%x:x%x %v", server.index, server.subindex, response.raw)
				if (response.raw[0] & 0xEF) != 0x60 {
					abortCode = SDO_ABORT_CMD
					server.state = SDO_STATE_ABORT
					break
				}
				toggle := response.GetToggle()
				if toggle != server.toggle {
					abortCode = SDO_ABORT_TOGGLE_BIT
					server.state = SDO_STATE_ABORT
					break
				}
				server.state = SDO_STATE_UPLOAD_SEGMENT_RSP

			case SDO_STATE_DOWNLOAD_BLK_INITIATE_REQ:
				server.blockCRCEnabled = response.IsCRCEnabled()
				// Check if size indicated
				if (response.raw[0] & 0x02) != 0 {
					sizeInOd := server.streamer.stream.DataLength
					server.sizeIndicated = binary.LittleEndian.Uint32(response.raw[4:])
					// Check if size matches
					if sizeInOd > 0 {
						if server.sizeIndicated > uint32(sizeInOd) {
							abortCode = SDO_ABORT_DATA_LONG
							server.state = SDO_STATE_ABORT
							break
						} else if server.sizeIndicated < uint32(sizeInOd) && (server.streamer.stream.Attribute&ATTRIBUTE_STR) == 0 {
							abortCode = SDO_ABORT_DATA_SHORT
							server.state = SDO_STATE_ABORT
							break
						}
					}
				} else {
					server.sizeIndicated = 0
				}
				log.Debugf("[SERVER][RX] BLOCK DOWNLOAD INIT | x%x:x%x | crc enabled : %v expected size : %v | %v",
					server.index,
					server.subindex,
					server.blockCRCEnabled,
					server.sizeIndicated,
					response.raw,
				)
				server.state = SDO_STATE_DOWNLOAD_BLK_INITIATE_RSP
				server.finished = false

			case SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ:
				// This is done in receive handler

			case SDO_STATE_DOWNLOAD_BLK_END_REQ:
				log.Debugf("[SERVER][RX] BLOCK DOWNLOAD END | x%x:x%x %v", server.index, server.subindex, response.raw)
				if (response.raw[0] & 0xE3) != 0xC1 {
					abortCode = SDO_ABORT_CMD
					server.state = SDO_STATE_ABORT
					break
				}
				//Get number of data bytes in last segment, that do not
				//contain data. Then reduce buffer
				noData := (response.raw[0] >> 2) & 0x07
				if server.bufWriteOffset <= uint32(noData) {
					server.errorExtraInfo = fmt.Errorf("internal buffer and end of block download are inconsitent")
					abortCode = SDO_ABORT_DEVICE_INCOMPAT
					server.state = SDO_STATE_ABORT
					break
				}
				server.sizeTransferred -= uint32(noData)
				server.bufWriteOffset -= uint32(noData)
				var crcClient = crc16(0)
				if server.blockCRCEnabled {
					crcClient = response.GetCRCClient()
				}
				abortCode = server.writeObjectDictionary(2, crcClient)
				if abortCode != nil {
					break
				}
				server.state = SDO_STATE_DOWNLOAD_BLK_END_RSP

			case SDO_STATE_UPLOAD_BLK_INITIATE_REQ:
				// If protocol switch threshold (byte 5) is larger than data
				// size of OD var, then switch to segmented
				if server.sizeIndicated > 0 && response.raw[5] > 0 && uint32(response.raw[5]) >= server.sizeIndicated {
					server.state = SDO_STATE_UPLOAD_INITIATE_RSP
					break
				}
				if (response.raw[0] & 0x04) != 0 {
					server.blockCRCEnabled = true
					server.blockCRC = crc16(0)
					server.blockCRC.ccittBlock(server.buffer[:server.bufWriteOffset])
				} else {
					server.blockCRCEnabled = false
				}
				// Get block size and check okay
				server.blockSize = response.GetBlockSize()
				log.Debugf("[SERVER][RX] UPLOAD BLOCK INIT | x%x:x%x %v | crc : %v, blksize :%v", server.index, server.subindex, response.raw, server.blockCRCEnabled, server.blockSize)
				if server.blockSize < 1 || server.blockSize > 127 {
					abortCode = SDO_ABORT_BLOCK_SIZE
					server.state = SDO_STATE_ABORT
					break
				}

				// Check that we have enough data for sending a complete sub-block with the requested size
				if !server.finished && server.bufWriteOffset < uint32(server.blockSize)*7 {
					abortCode = SDO_ABORT_BLOCK_SIZE
					server.state = SDO_STATE_ABORT
					break
				}
				server.state = SDO_STATE_UPLOAD_BLK_INITIATE_RSP

			case SDO_STATE_UPLOAD_BLK_INITIATE_REQ2:
				if response.raw[0] == 0xA3 {
					server.blockSequenceNb = 0
					server.state = SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ
				} else {
					abortCode = SDO_ABORT_CMD
					server.state = SDO_STATE_ABORT
				}

			case SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ, SDO_STATE_UPLOAD_BLK_SUBBLOCK_CRSP:
				if response.raw[0] != 0xA2 {
					abortCode = SDO_ABORT_CMD
					server.state = SDO_STATE_ABORT
					break
				}
				log.Debugf("[SERVER][RX] BLOCK UPLOAD END SUB-BLOCK | blksize %v | x%x:x%x %v",
					response.raw[2],
					server.index,
					server.subindex,
					response.raw,
				)
				// Check block size
				server.blockSize = response.raw[2]
				if server.blockSize < 1 || server.blockSize > 127 {
					abortCode = SDO_ABORT_BLOCK_SIZE
					server.state = SDO_STATE_ABORT
					break
				}
				// Check number of segments
				if response.raw[1] < server.blockSequenceNb {
					// Some error occurd, re-transmit missing chunks
					cntFailed := server.blockSequenceNb - response.raw[1]
					cntFailed = cntFailed*7 - server.blockNoData
					server.bufReadOffset -= uint32(cntFailed)
					server.sizeTransferred -= uint32(cntFailed)
				} else if response.raw[1] > server.blockSequenceNb {
					abortCode = SDO_ABORT_CMD
					server.state = SDO_STATE_ABORT
					break
				}
				// Refill buffer if needed
				abortCode = server.readObjectDictionary(uint32(server.blockSize)*7, true)
				if abortCode != nil {
					break
				}

				if server.bufWriteOffset == server.bufReadOffset {
					server.state = SDO_STATE_UPLOAD_BLK_END_SREQ
				} else {
					server.blockSequenceNb = 0
					server.state = SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ
				}

			default:
				abortCode = SDO_ABORT_CMD
				server.state = SDO_STATE_ABORT

			}
		}
		server.timeoutTimer = 0
		timeDifferenceUs = 0
		server.rxNew = false
	}

	if ret == SDO_WAITING_RESPONSE {
		if server.timeoutTimer < server.timeoutTimeUs {
			server.timeoutTimer += timeDifferenceUs
		}
		if server.timeoutTimer >= server.timeoutTimeUs {
			abortCode = SDO_ABORT_TIMEOUT
			server.state = SDO_STATE_ABORT
			log.Errorf("[SERVER] TIMEOUT %v, State : %v", server.timeoutTimer, server.state)

		} else if timerNextUs != nil {
			diff := server.timeoutTimeUs - server.timeoutTimer
			if *timerNextUs > diff {
				*timerNextUs = diff
			}
		}
		// Timeout for subblocks
		if server.state == SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ {
			if server.timeoutTimerBlock < server.timeoutTimeBlockTransferUs {
				server.timeoutTimerBlock += timeDifferenceUs
			}
			if server.timeoutTimerBlock >= server.timeoutTimeBlockTransferUs {
				server.state = SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_RSP
				server.rxNew = false
			} else if timerNextUs != nil {
				diff := server.timeoutTimeBlockTransferUs - server.timeoutTimerBlock
				if *timerNextUs > diff {
					*timerNextUs = diff
				}
			}
		}
	}

	if ret == SDO_WAITING_RESPONSE {
		server.txBuffer.Data = [8]byte{0}

		switch server.state {
		case SDO_STATE_DOWNLOAD_INITIATE_RSP:
			server.txBuffer.Data[0] = 0x60
			server.txBuffer.Data[1] = byte(server.index)
			server.txBuffer.Data[2] = byte(server.index >> 8)
			server.txBuffer.Data[3] = server.subindex
			server.timeoutTimer = 0
			server.Send(server.txBuffer)
			if server.finished {
				log.Debugf("[SERVER][TX] DOWNLOAD EXPEDITED | x%x:x%x %v", server.index, server.subindex, server.txBuffer.Data)
				server.state = SDO_STATE_IDLE
				ret = SDO_SUCCESS
			} else {
				log.Debugf("[SERVER][TX] DOWNLOAD SEGMENT INIT | x%x:x%x %v", server.index, server.subindex, server.txBuffer.Data)
				server.toggle = 0x00
				server.sizeTransferred = 0
				server.bufWriteOffset = 0
				server.bufReadOffset = 0
				server.state = SDO_STATE_DOWNLOAD_SEGMENT_REQ
			}

		case SDO_STATE_DOWNLOAD_SEGMENT_RSP:
			server.txBuffer.Data[0] = 0x20 | server.toggle
			server.toggle ^= 0x10
			server.timeoutTimer = 0
			log.Debugf("[SERVER][TX] DOWNLOAD SEGMENT | x%x:x%x %v", server.index, server.subindex, server.txBuffer.Data)
			server.Send(server.txBuffer)
			if server.finished {
				server.state = SDO_STATE_IDLE
				ret = SDO_SUCCESS
			} else {
				server.state = SDO_STATE_DOWNLOAD_SEGMENT_REQ
			}

		case SDO_STATE_UPLOAD_INITIATE_RSP:
			if server.sizeIndicated > 0 && server.sizeIndicated <= 4 {
				server.txBuffer.Data[0] = 0x43 | ((4 - byte(server.sizeIndicated)) << 2)
				copy(server.txBuffer.Data[4:], server.buffer[:server.sizeIndicated])
				server.state = SDO_STATE_IDLE
				ret = SDO_SUCCESS
				log.Debugf("[SERVER][TX] UPLOAD EXPEDITED | x%x:x%x %v", server.index, server.subindex, server.txBuffer.Data)
			} else {
				// Segmented transfer
				if server.sizeIndicated > 0 {
					server.txBuffer.Data[0] = 0x41
					// Add data size
					binary.LittleEndian.PutUint32(server.txBuffer.Data[4:], server.sizeIndicated)

				} else {
					server.txBuffer.Data[0] = 0x40
				}
				server.toggle = 0x00
				server.timeoutTimer = 0
				server.state = SDO_STATE_UPLOAD_SEGMENT_REQ
				log.Debugf("[SERVER][TX] UPLOAD SEGMENTED | x%x:x%x %v", server.index, server.subindex, server.txBuffer.Data)
			}
			server.txBuffer.Data[1] = byte(server.index)
			server.txBuffer.Data[2] = byte(server.index >> 8)
			server.txBuffer.Data[3] = server.subindex
			server.Send(server.txBuffer)

		case SDO_STATE_UPLOAD_SEGMENT_RSP:
			// Refill buffer if needed
			abortCode = server.readObjectDictionary(7, false)
			if abortCode != nil {
				break
			}
			server.txBuffer.Data[0] = server.toggle
			server.toggle ^= 0x10
			count := server.bufWriteOffset - server.bufReadOffset
			// Check if last segment
			if count < 7 || (server.finished && count == 7) {
				server.txBuffer.Data[0] |= (byte((7 - count) << 1)) | 0x01
				server.state = SDO_STATE_IDLE
				ret = SDO_SUCCESS
			} else {
				server.timeoutTimer = 0
				server.state = SDO_STATE_UPLOAD_SEGMENT_REQ
				count = 7
			}
			copy(server.txBuffer.Data[1:], server.buffer[server.bufReadOffset:server.bufReadOffset+count])
			server.bufReadOffset += count
			server.sizeTransferred += count
			// Check if too shor or too large in last segment
			if server.sizeIndicated > 0 {
				if server.sizeTransferred > server.sizeIndicated {
					abortCode = SDO_ABORT_DATA_LONG
					server.state = SDO_STATE_ABORT
					break
				} else if ret == SDO_SUCCESS && server.sizeTransferred < server.sizeIndicated {
					abortCode = SDO_ABORT_DATA_SHORT
					ret = SDO_WAITING_RESPONSE
					server.state = SDO_STATE_ABORT
					break
				}
			}
			log.Debugf("[SERVER][TX] UPLOAD SEGMENTED | x%x:x%x %v", server.index, server.subindex, server.txBuffer.Data)
			server.Send(server.txBuffer)

		case SDO_STATE_DOWNLOAD_BLK_INITIATE_RSP:
			server.txBuffer.Data[0] = 0xA4
			server.txBuffer.Data[1] = byte(server.index)
			server.txBuffer.Data[2] = byte(server.index >> 8)
			server.txBuffer.Data[3] = server.subindex
			// Calculate blocks from free space
			count := (len(server.buffer) - 2) / 7
			if count > 127 {
				count = 127
			}
			server.blockSize = uint8(count)
			server.txBuffer.Data[4] = server.blockSize
			// Reset variables
			server.sizeTransferred = 0
			server.finished = false
			server.bufReadOffset = 0
			server.bufWriteOffset = 0
			server.blockSequenceNb = 0
			server.blockCRC = crc16(0)
			server.timeoutTimer = 0
			server.timeoutTimerBlock = 0
			server.state = SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ
			server.rxNew = false
			log.Debugf("[SERVER][TX] BLOCK DOWNLOAD INIT | x%x:x%x %v", server.index, server.subindex, server.txBuffer.Data)
			server.Send(server.txBuffer)

		case SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_RSP:
			server.txBuffer.Data[0] = 0xA2
			server.txBuffer.Data[1] = server.blockSequenceNb
			transferShort := server.blockSequenceNb != server.blockSize
			seqnoStart := server.blockSequenceNb
			// Is it last segment ?
			if server.finished {
				server.state = SDO_STATE_DOWNLOAD_BLK_END_REQ
			} else {
				// Calclate from free buffer space
				count := (len(server.buffer) - 2 - int(server.bufWriteOffset)) / 7
				if count > 127 {
					count = 127
				} else if server.bufWriteOffset > 0 {
					// Empty buffer
					abortCode = server.writeObjectDictionary(1, 0)
					if abortCode != nil {
						break
					}
					count = (len(server.buffer) - 2 - int(server.bufWriteOffset)) / 7
					if count > 127 {
						count = 127
					}
				}
				server.blockSize = uint8(count)
				server.blockSequenceNb = 0
				server.state = SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ
				server.rxNew = false
			}
			server.txBuffer.Data[2] = server.blockSize
			server.timeoutTimerBlock = 0
			server.Send(server.txBuffer)

			if transferShort && !server.finished {
				log.Debugf("[SERVER][TX] BLOCK DOWNLOAD RESTART seqno prev=%v, blksize=%v", seqnoStart, server.blockSize)
			} else {
				log.Debugf("[SERVER][TX] BLOCK DOWNLOAD SUB-BLOCK RES | x%x:x%x blksize %v %v",
					server.index,
					server.subindex,
					server.blockSize,
					server.txBuffer.Data,
				)
			}

		case SDO_STATE_DOWNLOAD_BLK_END_RSP:
			server.txBuffer.Data[0] = 0xA1
			log.Debugf("[SERVER][TX] BLOCK DOWNLOAD END | x%x:x%x %v", server.index, server.subindex, server.txBuffer.Data)
			server.Send(server.txBuffer)
			server.state = SDO_STATE_IDLE
			ret = SDO_SUCCESS

		case SDO_STATE_UPLOAD_BLK_INITIATE_RSP:
			server.txBuffer.Data[0] = 0xC4
			server.txBuffer.Data[1] = byte(server.index)
			server.txBuffer.Data[2] = byte(server.index >> 8)
			server.txBuffer.Data[3] = server.subindex
			// Add data size
			if server.sizeIndicated > 0 {
				server.txBuffer.Data[0] |= 0x02
				binary.LittleEndian.PutUint32(server.txBuffer.Data[4:], server.sizeIndicated)
			}
			// Reset timer & send
			server.timeoutTimer = 0
			log.Debugf("[SERVER][TX] BLOCK UPLOAD INIT | x%x:x%x %v", server.index, server.subindex, server.txBuffer.Data)
			server.Send(server.txBuffer)
			server.state = SDO_STATE_UPLOAD_BLK_INITIATE_REQ2

		case SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ:
			// Write header & gend current count
			server.blockSequenceNb += 1
			server.txBuffer.Data[0] = server.blockSequenceNb
			count := server.bufWriteOffset - server.bufReadOffset
			// Check if last segment
			if count < 7 || (server.finished && count == 7) {
				server.txBuffer.Data[0] |= 0x80
			} else {
				count = 7
			}
			copy(server.txBuffer.Data[1:], server.buffer[server.bufReadOffset:server.bufReadOffset+count])
			server.bufReadOffset += count
			server.blockNoData = byte(7 - count)
			server.sizeTransferred += count
			// Check if too short or too large in last segment
			if server.sizeIndicated > 0 {
				if server.sizeTransferred > server.sizeIndicated {
					abortCode = SDO_ABORT_DATA_LONG
					server.state = SDO_STATE_ABORT
					break
				} else if server.bufReadOffset == server.bufWriteOffset && server.sizeTransferred < server.sizeIndicated {
					abortCode = SDO_ABORT_DATA_SHORT
					server.state = SDO_STATE_ABORT
					break
				}
			}
			// Check if last segment or all segments in current block transferred
			if server.bufWriteOffset == server.bufReadOffset || server.blockSequenceNb >= server.blockSize {
				server.state = SDO_STATE_UPLOAD_BLK_SUBBLOCK_CRSP
				log.Debugf("[SERVER][TX] BLOCK UPLOAD END SUB-BLOCK | x%x:x%x %v", server.index, server.subindex, server.txBuffer.Data)
			} else {
				log.Debugf("[SERVER][TX] BLOCK UPLOAD SUB-BLOCK | x%x:x%x %v", server.index, server.subindex, server.txBuffer.Data)
				if timerNextUs != nil {
					*timerNextUs = 0
				}
			}
			// Reset timer & send
			server.timeoutTimer = 0
			server.Send(server.txBuffer)

		case SDO_STATE_UPLOAD_BLK_END_SREQ:
			server.txBuffer.Data[0] = 0xC1 | (server.blockNoData << 2)
			server.txBuffer.Data[1] = byte(server.blockCRC)
			server.txBuffer.Data[2] = byte(server.blockCRC >> 8)
			server.timeoutTimer = 0
			log.Debugf("[SERVER][TX] BLOCK UPLOAD END | x%x:x%x %v", server.index, server.subindex, server.txBuffer.Data)
			server.Send(server.txBuffer)
			server.state = SDO_STATE_UPLOAD_BLK_END_CRSP

		default:

		}

	}

	if ret == SDO_WAITING_RESPONSE {
		switch server.state {
		case SDO_STATE_ABORT:
			if sdoAbort, ok := abortCode.(SDOAbortCode); !ok {
				log.Errorf("[SERVER][TX] Abort internal error : unknown abort code : %v", abortCode)
				server.Abort(SDO_ABORT_GENERAL)
			} else {
				server.Abort(sdoAbort)
			}
			server.state = SDO_STATE_IDLE
		case SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ:
			ret = SDO_BLOCK_DOWNLOAD_IN_PROGRESS
		case SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ:
			ret = SDO_BLOCK_UPLOAD_IN_PROGRESS
		}
	}
	return ret, abortCode
}

// Create & send abort on bus
func (server *SDOServer) Abort(abortCode SDOAbortCode) {
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
	bm *busManager,
	od *ObjectDictionary,
	nodeId uint8,
	timeoutMs uint32,
	entry12xx *Entry,
) (*SDOServer, error) {
	server := &SDOServer{busManager: bm}
	if od == nil || bm == nil || entry12xx == nil {
		return nil, ErrIllegalArgument
	}
	server.od = od
	server.streamer = &streamer{}
	server.buffer = make([]byte, 1000)
	server.bufReadOffset = 0
	server.bufWriteOffset = 0
	server.nodeId = nodeId
	server.timeoutTimeUs = timeoutMs * 1000
	server.timeoutTimeBlockTransferUs = timeoutMs * 700
	var canIdClientToServer uint16
	var canIdServerToClient uint16
	if entry12xx == nil {
		/*Configure sdo channel*/
		if nodeId < 1 || nodeId > 127 {
			log.Errorf("SDO server node id is not valid : %x", nodeId)
			return nil, ErrIllegalArgument
		}
		canIdClientToServer = SDO_CLIENT_ID + uint16(nodeId)
		canIdServerToClient = SDO_SERVER_ID + uint16(nodeId)
		server.valid = true
	} else {
		if entry12xx.Index == 0x1200 {
			// Default channels
			if nodeId < 1 || nodeId > 127 {
				log.Errorf("SDO server node id is not valid : %x", nodeId)
				return nil, ErrIllegalArgument
			}
			canIdClientToServer = SDO_CLIENT_ID + uint16(nodeId)
			canIdServerToClient = SDO_SERVER_ID + uint16(nodeId)
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
				return nil, ErrOdParameters
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
			entry12xx.AddExtension(server, ReadEntryDefault, writeEntry1201)

		} else {
			return nil, ErrIllegalArgument
		}
	}
	server.rxNew = false
	server.cobIdClientToServer = 0
	server.cobIdServerToClient = 0
	return server, server.initRxTx(uint32(canIdClientToServer), uint32(canIdServerToClient))

}
