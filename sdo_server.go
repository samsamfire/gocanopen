package canopen

import (
	"encoding/binary"
	"fmt"

	log "github.com/sirupsen/logrus"
)

type SDOServer struct {
	OD                         *ObjectDictionary
	Streamer                   *ObjectStreamer
	NodeId                     uint8
	BusManager                 *BusManager
	txBuffer                   Frame
	CobIdClientToServer        uint32
	CobIdServerToClient        uint32
	Valid                      bool
	Index                      uint16
	Subindex                   uint8
	Finished                   bool
	SizeIndicated              uint32
	SizeTransferred            uint32
	State                      SDOState
	TimeoutTimeUs              uint32
	TimeoutTimer               uint32
	buffer                     []byte
	bufWriteOffset             uint32
	bufReadOffset              uint32
	RxNew                      bool
	Response                   SDOResponse
	Toggle                     uint8
	TimeoutTimeBlockTransferUs uint32
	TimeoutTimerBlock          uint32
	BlockSequenceNb            uint8
	BlockSize                  uint8
	BlockNoData                uint8
	BlockCRCEnabled            bool
	BlockDataUploadLast        [7]byte
	BlockCRC                   CRC16
	ErrorExtraInfo             error
}

// Handle received messages
func (server *SDOServer) Handle(frame Frame) {
	if frame.DLC != 8 {
		return
	}
	if frame.Data[0] == 0x80 {
		// Client abort
		server.State = SDO_STATE_IDLE
	} else if server.RxNew {
		// Ignore message if previous message not processed
		log.Info("[SERVER][RX] ignoring message because still processing")
	} else if server.State == SDO_STATE_UPLOAD_BLK_END_CRSP && frame.Data[0] == 0xA1 {
		// Block transferred ! go to idle
		server.State = SDO_STATE_IDLE
	} else if server.State == SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ {
		// Condition should always pass but check
		if int(server.bufWriteOffset) <= (len(server.buffer) - (7 + 2)) {
			// Block download, copy data in handle
			state := SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ
			seqno := frame.Data[0] & 0x7F
			server.TimeoutTimer = 0
			server.TimeoutTimerBlock = 0
			// Check correct sequence number
			if seqno <= server.BlockSize && seqno == (server.BlockSequenceNb+1) {
				server.BlockSequenceNb = seqno
				// Copy data
				copy(server.buffer[server.bufWriteOffset:], frame.Data[1:])
				server.bufWriteOffset += 7
				server.SizeTransferred += 7
				// Check if last segment
				if (frame.Data[0] & 0x80) != 0 {
					server.Finished = true
					state = SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_RSP
					log.Debugf("[SERVER][RX] BLOCK DOWNLOAD END | x%x:x%x %v", server.Index, server.Subindex, frame.Data)
				} else if seqno == server.BlockSize {
					// All segments in sub block transferred
					state = SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_RSP
					log.Debugf("[SERVER][RX] BLOCK DOWNLOAD SUB-BLOCK | x%x:x%x %v", server.Index, server.Subindex, frame.Data)
				} else {
					log.Debugf("[SERVER][RX] BLOCK DOWNLOAD SUB-BLOCK | x%x:x%x %v", server.Index, server.Subindex, frame.Data)
				}
				// If duplicate or sequence didn't start ignore, otherwise
				// seqno is wrong
			} else if seqno != server.BlockSequenceNb && server.BlockSequenceNb != 0 {
				state = SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_RSP
				log.Warnf("[SERVER][RX] BLOCK DOWNLOAD SUB-BLOCK | Wrong sequence number (got %v, previous %v) | x%x:x%x %v",
					seqno,
					server.BlockSequenceNb,
					server.Index,
					server.Subindex,
					frame.Data,
				)
			} else {
				log.Warnf("[SERVER][RX] BLOCK DOWNLOAD SUB-BLOCK | Ignoring (got %v, expecting %v) | x%x:x%x %v",
					seqno,
					server.BlockSequenceNb+1,
					server.Index,
					server.Subindex,
					frame.Data,
				)
			}

			if state != SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ {
				server.RxNew = false
				server.State = state
			}
		}
	} else if server.State == SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_RSP {
		//Ignore other server messages if response requested
	} else {
		// Copy data and set new flag
		server.Response.raw = frame.Data
		server.RxNew = true
	}
}

func (server *SDOServer) Init(od *ObjectDictionary, entry12xx *Entry, nodeId uint8, timeoutTimeMs uint16, busManager *BusManager) error {
	if od == nil || busManager == nil {
		return ErrIllegalArgument
	}
	server.OD = od
	server.Streamer = &ObjectStreamer{}
	server.buffer = make([]byte, 1000)
	server.bufReadOffset = 0
	server.bufWriteOffset = 0
	server.NodeId = nodeId
	server.TimeoutTimeUs = uint32(timeoutTimeMs) * 1000
	server.TimeoutTimeBlockTransferUs = uint32(timeoutTimeMs) * 700
	var canIdClientToServer uint16
	var canIdServerToClient uint16
	if entry12xx == nil {
		/*Configure sdo channel*/
		if nodeId < 1 || nodeId > 127 {
			log.Errorf("SDO server node id is not valid : %x", nodeId)
			return ErrIllegalArgument
		}
		canIdClientToServer = SDO_CLIENT_ID + uint16(nodeId)
		canIdServerToClient = SDO_SERVER_ID + uint16(nodeId)
		server.Valid = true
	} else {
		if entry12xx.Index == 0x1200 {
			// Default channels
			if nodeId < 1 || nodeId > 127 {
				log.Errorf("SDO server node id is not valid : %x", nodeId)
				return ErrIllegalArgument
			}
			canIdClientToServer = SDO_CLIENT_ID + uint16(nodeId)
			canIdServerToClient = SDO_SERVER_ID + uint16(nodeId)
			server.Valid = true
			entry12xx.SetUint32(1, uint32(canIdClientToServer), true)
			entry12xx.SetUint32(2, uint32(canIdServerToClient), true)
		} else if entry12xx.Index > 0x1200 && entry12xx.Index <= 0x1200+0x7F {
			// Configure other channels
			var maxSubIndex uint8
			var cobIdClientToServer32, cobIdServerToClient32 uint32
			err0 := entry12xx.GetUint8(0, &maxSubIndex)
			err1 := entry12xx.GetUint32(1, &cobIdClientToServer32)
			err2 := entry12xx.GetUint32(2, &cobIdServerToClient32)
			if err0 != nil || (maxSubIndex != 2 && maxSubIndex != 3) ||
				err1 != nil || err2 != nil {
				log.Errorf("Error when retreiving sdo server parameters : %v, %v, %v, %v", err0, err1, err2, maxSubIndex)
				return ErrOdParameters
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
			entry12xx.AddExtension(server, ReadEntryOriginal, WriteEntry1201)

		} else {
			return ErrIllegalArgument
		}
	}
	server.RxNew = false
	server.BusManager = busManager
	server.CobIdClientToServer = 0
	server.CobIdServerToClient = 0
	return server.InitRxTx(server.BusManager, uint32(canIdClientToServer), uint32(canIdServerToClient))

}

func (server *SDOServer) InitRxTx(busManager *BusManager, cobIdClientToServer uint32, cobIdServerToClient uint32) error {
	var ret error
	// Only proceed if parameters change (i.e. different client)
	if cobIdServerToClient == server.CobIdServerToClient && cobIdClientToServer == server.CobIdClientToServer {
		return nil
	}
	server.CobIdServerToClient = cobIdServerToClient
	server.CobIdClientToServer = cobIdClientToServer

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
		server.Valid = true
	} else {
		CanIdC2S = 0
		CanIdS2C = 0
		server.Valid = false
	}
	// Configure buffers, if initializing then insert in buffer, otherwise, update
	ret = server.BusManager.Subscribe(uint32(CanIdC2S), 0x7FF, false, server)
	if ret != nil {
		server.Valid = false
		return ret
	}
	server.txBuffer = NewFrame(uint32(CanIdS2C), 0, 8)
	return ret

}

func (server *SDOServer) writeObjectDictionary(abortCode *SDOAbortCode, crcOperation uint, crcClient CRC16) bool {

	bufferOffsetWriteOriginal := server.bufWriteOffset

	if server.Finished {
		// Check size
		if server.SizeIndicated > 0 && server.SizeTransferred > server.SizeIndicated {
			*abortCode = SDO_ABORT_DATA_LONG
			server.State = SDO_STATE_ABORT
			return false
		} else if server.SizeIndicated > 0 && server.SizeTransferred < server.SizeIndicated {
			*abortCode = SDO_ABORT_DATA_SHORT
			server.State = SDO_STATE_ABORT
			return false
		}
		// Golang does not have null termination characters so nothing particular to do
		// Stream data should be limited to the sent value

		varSizeInOd := server.Streamer.stream.DataLength
		if server.Streamer.stream.Attribute&ATTRIBUTE_STR != 0 &&
			(varSizeInOd == 0 || server.SizeTransferred < varSizeInOd) &&
			int(server.bufWriteOffset+2) <= len(server.buffer) {
			server.buffer[server.bufWriteOffset] = 0x00
			server.bufWriteOffset++
			server.SizeTransferred++
			if varSizeInOd == 0 || server.SizeTransferred < varSizeInOd {
				server.buffer[server.bufWriteOffset] = 0x00
				server.bufWriteOffset++
				server.SizeTransferred++
			}
			server.Streamer.stream.DataLength = server.SizeTransferred
		} else if varSizeInOd == 0 {
			server.Streamer.stream.DataLength = server.SizeTransferred
		} else if server.SizeTransferred != varSizeInOd {
			if server.SizeTransferred > varSizeInOd {
				*abortCode = SDO_ABORT_DATA_LONG
				server.State = SDO_STATE_ABORT
				return false
			} else if server.SizeTransferred < varSizeInOd {
				*abortCode = SDO_ABORT_DATA_SHORT
				server.State = SDO_STATE_ABORT
				return false
			}
		}

	} else {
		// Still check if not bigger than max size
		if server.SizeIndicated > 0 && server.SizeTransferred > server.SizeIndicated {
			*abortCode = SDO_ABORT_DATA_LONG
			server.State = SDO_STATE_ABORT
			return false
		}
	}

	// Calculate CRC
	if server.BlockCRCEnabled && crcOperation > 0 {
		server.BlockCRC.ccittBlock(server.buffer[:bufferOffsetWriteOriginal])
		if crcOperation == 2 && crcClient != server.BlockCRC {
			*abortCode = SDO_ABORT_CRC
			server.State = SDO_STATE_ABORT
			server.ErrorExtraInfo = fmt.Errorf("server was expecting %v but got %v", server.BlockCRC, crcClient)
			return false
		}
	}

	// Write the data
	_, ret := server.Streamer.Write(server.buffer[:server.bufWriteOffset])
	server.bufWriteOffset = 0
	if ret != nil && ret != ODR_PARTIAL {
		*abortCode = ret.(ODR).GetSDOAbordCode()
		server.State = SDO_STATE_ABORT
		return false
	} else if server.Finished && ret == ODR_PARTIAL {
		*abortCode = SDO_ABORT_DATA_SHORT
		server.State = SDO_STATE_ABORT
		return false
	} else if !server.Finished && ret == nil {
		*abortCode = SDO_ABORT_DATA_LONG
		server.State = SDO_STATE_ABORT
		return false
	}
	return true

}

func (server *SDOServer) readObjectDictionary(abortCode *SDOAbortCode, countMinimum uint32, calculateCRC bool) bool {
	buffered := server.bufWriteOffset - server.bufReadOffset
	if !server.Finished && buffered < countMinimum {
		// Move buffered bytes to begining
		copy(server.buffer, server.buffer[server.bufReadOffset:server.bufReadOffset+buffered])
		server.bufReadOffset = 0
		server.bufWriteOffset = buffered

		// Read from OD into the buffer
		countRd, err := server.Streamer.Read(server.buffer[buffered:])

		if err != nil && err != ODR_PARTIAL {
			*abortCode = err.(ODR).GetSDOAbordCode()
			server.State = SDO_STATE_ABORT
			return false
		}

		// Stop sending at null termination if string
		if countRd > 0 && (server.Streamer.stream.Attribute&ATTRIBUTE_STR) != 0 {
			server.buffer[countRd+int(buffered)] = 0
			countStr := int(server.Streamer.stream.DataLength)
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
				server.Streamer.stream.DataLength = server.SizeTransferred + uint32(countRd)
			}
		}

		server.bufWriteOffset = buffered + uint32(countRd) // Move offset write by countRd (number of read bytes)
		if server.bufWriteOffset == 0 || err == ODR_PARTIAL {
			server.Finished = false
			if server.bufWriteOffset < countMinimum {
				*abortCode = SDO_ABORT_DEVICE_INCOMPAT
				server.State = SDO_STATE_ABORT
				server.ErrorExtraInfo = fmt.Errorf("buffer offset write %v is less than the minimum count %v", server.bufWriteOffset, countMinimum)
				return false
			}
		} else {
			server.Finished = true
		}
		if calculateCRC && server.BlockCRCEnabled {
			// Calculate CRC for the read data
			server.BlockCRC.ccittBlock(server.buffer[buffered:server.bufWriteOffset])
		}

	}

	return true
}

func updateStateFromRequest(stateReq uint8, state *SDOState, upload *bool) SDOAbortCode {
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
	return SDO_ABORT_NONE
}

func (server *SDOServer) process(nmtIsPreOrOperationnal bool, timeDifferenceUs uint32, timerNextUs *uint32) (err error) {
	ret := SDO_WAITING_RESPONSE
	abortCode := SDO_ABORT_NONE
	if server.Valid && server.State == SDO_STATE_IDLE && !server.RxNew {
		ret = SDO_SUCCESS
	} else if !nmtIsPreOrOperationnal || !server.Valid {
		server.State = SDO_STATE_IDLE
		server.RxNew = false
		ret = SDO_SUCCESS
	} else if server.RxNew {
		response := server.Response
		if server.State == SDO_STATE_IDLE {
			upload := false
			abortCode = updateStateFromRequest(response.raw[0], &server.State, &upload)

			// Check object exists and accessible
			if abortCode == SDO_ABORT_NONE {
				var err error
				server.Index = response.GetIndex()
				server.Subindex = response.GetSubindex()
				server.Streamer, err = server.OD.Index(server.Index).CreateStreamer(server.Subindex, false)
				if err != nil {
					abortCode = err.(ODR).GetSDOAbordCode()
					server.State = SDO_STATE_ABORT
				} else {
					if server.Streamer.stream.Attribute&ATTRIBUTE_SDO_RW == 0 {
						abortCode = SDO_ABORT_UNSUPPORTED_ACCESS
						server.State = SDO_STATE_ABORT
					} else if upload && (server.Streamer.stream.Attribute&ATTRIBUTE_SDO_R) == 0 {
						abortCode = SDO_ABORT_WRITEONLY
						server.State = SDO_STATE_ABORT
					} else if !upload && (server.Streamer.stream.Attribute&ATTRIBUTE_SDO_W) == 0 {
						abortCode = SDO_ABORT_READONLY
						server.State = SDO_STATE_ABORT
					}
				}
			}
			// Load data from OD
			if upload && abortCode == SDO_ABORT_NONE {
				server.bufReadOffset = 0
				server.bufWriteOffset = 0
				server.SizeTransferred = 0
				server.Finished = false
				if server.readObjectDictionary(&abortCode, 7, false) {
					if server.Finished {
						server.SizeIndicated = server.Streamer.stream.DataLength
						if server.SizeIndicated == 0 {
							server.SizeIndicated = server.bufWriteOffset
						} else if server.SizeIndicated != server.bufWriteOffset {
							server.ErrorExtraInfo = fmt.Errorf("size indicated %v != to buffer write offset %v", server.SizeIndicated, server.bufWriteOffset)
							abortCode = SDO_ABORT_DEVICE_INCOMPAT
							server.State = SDO_STATE_ABORT
						}
					} else {
						if server.Streamer.stream.Attribute&ATTRIBUTE_STR == 0 {
							server.SizeIndicated = server.Streamer.stream.DataLength
						} else {
							server.SizeIndicated = 0
						}
					}
				}

			}
		}

		if server.State != SDO_STATE_IDLE && server.State != SDO_STATE_ABORT {
			switch server.State {
			case SDO_STATE_DOWNLOAD_INITIATE_REQ:
				if (response.raw[0] & 0x02) != 0 {
					log.Debugf("[SERVER][RX] DOWNLOAD EXPEDITED | x%x:x%x %v", server.Index, server.Subindex, response.raw)
					//Expedited 4 bytes of data max
					varSizeInOd := server.Streamer.stream.DataLength
					dataSizeToWrite := 4
					if (response.raw[0] & 0x01) != 0 {
						dataSizeToWrite -= (int(response.raw[0]) >> 2) & 0x03
					} else if varSizeInOd > 0 && varSizeInOd < 4 {
						dataSizeToWrite = int(varSizeInOd)
					}
					//Create temporary buffer
					buf := make([]byte, 6)
					copy(buf, response.raw[4:4+dataSizeToWrite])
					if (server.Streamer.stream.Attribute&ATTRIBUTE_STR) != 0 &&
						(varSizeInOd == 0 || uint32(dataSizeToWrite) < varSizeInOd) {
						delta := varSizeInOd - uint32(dataSizeToWrite)
						if delta == 1 {
							dataSizeToWrite += 1
						} else {
							dataSizeToWrite += 2
						}
						server.Streamer.stream.DataLength = uint32(dataSizeToWrite)
					} else if varSizeInOd == 0 {
						server.Streamer.stream.DataLength = uint32(dataSizeToWrite)
					} else if dataSizeToWrite != int(varSizeInOd) {
						if dataSizeToWrite > int(varSizeInOd) {
							abortCode = SDO_ABORT_DATA_LONG
						} else {
							abortCode = SDO_ABORT_DATA_SHORT
						}
						server.State = SDO_STATE_ABORT
						break
					}
					_, err := server.Streamer.Write(buf[:dataSizeToWrite])
					if err != nil {
						abortCode = err.(ODR).GetSDOAbordCode()
						server.State = SDO_STATE_ABORT
						break
					} else {
						server.State = SDO_STATE_DOWNLOAD_INITIATE_RSP
						server.Finished = true

					}
				} else {
					if (response.raw[0] & 0x01) != 0 {
						log.Debugf("[SERVER][RX] DOWNLOAD SEGMENTED | x%x:x%x %v", server.Index, server.Subindex, response.raw)
						// Segmented transfer check if size indicated
						sizeInOd := server.Streamer.stream.DataLength
						server.SizeIndicated = binary.LittleEndian.Uint32(response.raw[4:])
						// Check if size matches
						if sizeInOd > 0 {
							if server.SizeIndicated > uint32(sizeInOd) {
								abortCode = SDO_ABORT_DATA_LONG
								server.State = SDO_STATE_ABORT
								break
							} else if server.SizeIndicated < uint32(sizeInOd) && (server.Streamer.stream.Attribute&ATTRIBUTE_STR) == 0 {
								abortCode = SDO_ABORT_DATA_SHORT
								server.State = SDO_STATE_ABORT
								break
							}
						}
					} else {
						server.SizeIndicated = 0
					}
					server.State = SDO_STATE_DOWNLOAD_INITIATE_RSP
					server.Finished = false
				}

			case SDO_STATE_DOWNLOAD_SEGMENT_REQ:
				if (response.raw[0] & 0xE0) == 0x00 {
					log.Debugf("[SERVER][RX] DOWNLOAD SEGMENT | x%x:x%x %v", server.Index, server.Subindex, response.raw)
					server.Finished = (response.raw[0] & 0x01) != 0
					toggle := response.GetToggle()
					if toggle != server.Toggle {
						abortCode = SDO_ABORT_TOGGLE_BIT
						server.State = SDO_STATE_ABORT
						break
					}
					// Get size and write to buffer
					count := 7 - ((response.raw[0] >> 1) & 0x07)
					copy(server.buffer[server.bufWriteOffset:], response.raw[1:1+count])
					server.bufWriteOffset += uint32(count)
					server.SizeTransferred += uint32(count)

					if server.Streamer.stream.DataLength > 0 && server.SizeTransferred > server.Streamer.stream.DataLength {
						abortCode = SDO_ABORT_DATA_LONG
						server.State = SDO_STATE_ABORT
						break
					}
					if server.Finished || (len(server.buffer)-int(server.bufWriteOffset) < (7 + 2)) {
						if !server.writeObjectDictionary(&abortCode, 0, 0) {
							break
						}
					}
					server.State = SDO_STATE_DOWNLOAD_SEGMENT_RSP
				} else {
					abortCode = SDO_ABORT_CMD
					server.State = SDO_STATE_ABORT
				}

			case SDO_STATE_UPLOAD_INITIATE_REQ:
				log.Debugf("[SERVER][RX] UPLOAD EXPEDITED | x%x:x%x %v", server.Index, server.Subindex, response.raw)
				server.State = SDO_STATE_UPLOAD_INITIATE_RSP

			case SDO_STATE_UPLOAD_SEGMENT_REQ:
				log.Debugf("[SERVER][RX] UPLOAD SEGMENTED | x%x:x%x %v", server.Index, server.Subindex, response.raw)
				if (response.raw[0] & 0xEF) != 0x60 {
					abortCode = SDO_ABORT_CMD
					server.State = SDO_STATE_ABORT
					break
				}
				toggle := response.GetToggle()
				if toggle != server.Toggle {
					abortCode = SDO_ABORT_TOGGLE_BIT
					server.State = SDO_STATE_ABORT
					break
				}
				server.State = SDO_STATE_UPLOAD_SEGMENT_RSP

			case SDO_STATE_DOWNLOAD_BLK_INITIATE_REQ:
				server.BlockCRCEnabled = response.IsCRCEnabled()
				// Check if size indicated
				if (response.raw[0] & 0x02) != 0 {
					sizeInOd := server.Streamer.stream.DataLength
					server.SizeIndicated = binary.LittleEndian.Uint32(response.raw[4:])
					// Check if size matches
					if sizeInOd > 0 {
						if server.SizeIndicated > uint32(sizeInOd) {
							abortCode = SDO_ABORT_DATA_LONG
							server.State = SDO_STATE_ABORT
							break
						} else if server.SizeIndicated < uint32(sizeInOd) && (server.Streamer.stream.Attribute&ATTRIBUTE_STR) == 0 {
							abortCode = SDO_ABORT_DATA_SHORT
							server.State = SDO_STATE_ABORT
							break
						}
					}
				} else {
					server.SizeIndicated = 0
				}
				log.Debugf("[SERVER][RX] BLOCK DOWNLOAD INIT | x%x:x%x | crc enabled : %v expected size : %v | %v",
					server.Index,
					server.Subindex,
					server.BlockCRCEnabled,
					server.SizeIndicated,
					response.raw,
				)
				server.State = SDO_STATE_DOWNLOAD_BLK_INITIATE_RSP
				server.Finished = false

			case SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ:
				// This is done in receive handler

			case SDO_STATE_DOWNLOAD_BLK_END_REQ:
				log.Debugf("[SERVER][RX] BLOCK DOWNLOAD END | x%x:x%x %v", server.Index, server.Subindex, response.raw)
				if (response.raw[0] & 0xE3) != 0xC1 {
					abortCode = SDO_ABORT_CMD
					server.State = SDO_STATE_ABORT
					break
				}
				//Get number of data bytes in last segment, that do not
				//contain data. Then reduce buffer
				noData := (response.raw[0] >> 2) & 0x07
				if server.bufWriteOffset <= uint32(noData) {
					server.ErrorExtraInfo = fmt.Errorf("internal buffer and end of block download are inconsitent")
					abortCode = SDO_ABORT_DEVICE_INCOMPAT
					server.State = SDO_STATE_ABORT
					break
				}
				server.SizeTransferred -= uint32(noData)
				server.bufWriteOffset -= uint32(noData)
				var crcClient = CRC16(0)
				if server.BlockCRCEnabled {
					crcClient = response.GetCRCClient()
				}
				if !server.writeObjectDictionary(&abortCode, 2, crcClient) {
					break
				}
				server.State = SDO_STATE_DOWNLOAD_BLK_END_RSP

			case SDO_STATE_UPLOAD_BLK_INITIATE_REQ:
				// If protocol switch threshold (byte 5) is larger than data
				// size of OD var, then switch to segmented
				if server.SizeIndicated > 0 && response.raw[5] > 0 && uint32(response.raw[5]) >= server.SizeIndicated {
					server.State = SDO_STATE_UPLOAD_INITIATE_RSP
					break
				}
				if (response.raw[0] & 0x04) != 0 {
					server.BlockCRCEnabled = true
					server.BlockCRC = CRC16(0)
					server.BlockCRC.ccittBlock(server.buffer[:server.bufWriteOffset])
				} else {
					server.BlockCRCEnabled = false
				}
				// Get block size and check okay
				server.BlockSize = response.GetBlockSize()
				log.Debugf("[SERVER][RX] UPLOAD BLOCK INIT | x%x:x%x %v | crc : %v, blksize :%v", server.Index, server.Subindex, response.raw, server.BlockCRCEnabled, server.BlockSize)
				if server.BlockSize < 1 || server.BlockSize > 127 {
					abortCode = SDO_ABORT_BLOCK_SIZE
					server.State = SDO_STATE_ABORT
					break
				}

				// Check that we have enough data for sending a complete sub-block with the requested size
				if !server.Finished && server.bufWriteOffset < uint32(server.BlockSize)*7 {
					abortCode = SDO_ABORT_BLOCK_SIZE
					server.State = SDO_STATE_ABORT
					break
				}
				server.State = SDO_STATE_UPLOAD_BLK_INITIATE_RSP

			case SDO_STATE_UPLOAD_BLK_INITIATE_REQ2:
				if response.raw[0] == 0xA3 {
					server.BlockSequenceNb = 0
					server.State = SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ
				} else {
					abortCode = SDO_ABORT_CMD
					server.State = SDO_STATE_ABORT
				}

			case SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ, SDO_STATE_UPLOAD_BLK_SUBBLOCK_CRSP:
				if response.raw[0] != 0xA2 {
					abortCode = SDO_ABORT_CMD
					server.State = SDO_STATE_ABORT
					break
				}
				log.Debugf("[SERVER][RX] BLOCK UPLOAD END SUB-BLOCK | blksize %v | x%x:x%x %v",
					response.raw[2],
					server.Index,
					server.Subindex,
					response.raw,
				)
				// Check block size
				server.BlockSize = response.raw[2]
				if server.BlockSize < 1 || server.BlockSize > 127 {
					abortCode = SDO_ABORT_BLOCK_SIZE
					server.State = SDO_STATE_ABORT
					break
				}
				// Check number of segments
				if response.raw[1] < server.BlockSequenceNb {
					// Some error occurd, re-transmit missing chunks
					cntFailed := server.BlockSequenceNb - response.raw[1]
					cntFailed = cntFailed*7 - server.BlockNoData
					server.bufReadOffset -= uint32(cntFailed)
					server.SizeTransferred -= uint32(cntFailed)
				} else if response.raw[1] > server.BlockSequenceNb {
					abortCode = SDO_ABORT_CMD
					server.State = SDO_STATE_ABORT
					break
				}
				// Refill buffer if needed
				if !server.readObjectDictionary(&abortCode, uint32(server.BlockSize)*7, true) {
					break
				}

				if server.bufWriteOffset == server.bufReadOffset {
					server.State = SDO_STATE_UPLOAD_BLK_END_SREQ
				} else {
					server.BlockSequenceNb = 0
					server.State = SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ
				}

			default:
				abortCode = SDO_ABORT_CMD
				server.State = SDO_STATE_ABORT

			}
		}
		server.TimeoutTimer = 0
		timeDifferenceUs = 0
		server.RxNew = false
	}

	if ret == SDO_WAITING_RESPONSE {
		if server.TimeoutTimer < server.TimeoutTimeUs {
			server.TimeoutTimer += timeDifferenceUs
		}
		if server.TimeoutTimer >= server.TimeoutTimeUs {
			abortCode = SDO_ABORT_TIMEOUT
			server.State = SDO_STATE_ABORT
			log.Errorf("[SERVER] TIMEOUT %v, State : %v", server.TimeoutTimer, server.State)

		} else if timerNextUs != nil {
			diff := server.TimeoutTimeUs - server.TimeoutTimer
			if *timerNextUs > diff {
				*timerNextUs = diff
			}
		}
		// Timeout for subblocks
		if server.State == SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ {
			if server.TimeoutTimerBlock < server.TimeoutTimeBlockTransferUs {
				server.TimeoutTimerBlock += timeDifferenceUs
			}
			if server.TimeoutTimerBlock >= server.TimeoutTimeBlockTransferUs {
				server.State = SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_RSP
				server.RxNew = false
			} else if timerNextUs != nil {
				diff := server.TimeoutTimeBlockTransferUs - server.TimeoutTimerBlock
				if *timerNextUs > diff {
					*timerNextUs = diff
				}
			}
		}
	}

	if ret == SDO_WAITING_RESPONSE {
		server.txBuffer.Data = [8]byte{0}

		switch server.State {
		case SDO_STATE_DOWNLOAD_INITIATE_RSP:
			server.txBuffer.Data[0] = 0x60
			server.txBuffer.Data[1] = byte(server.Index)
			server.txBuffer.Data[2] = byte(server.Index >> 8)
			server.txBuffer.Data[3] = server.Subindex
			server.TimeoutTimer = 0
			server.BusManager.Send(server.txBuffer)
			if server.Finished {
				log.Debugf("[SERVER][TX] DOWNLOAD EXPEDITED | x%x:x%x %v", server.Index, server.Subindex, server.txBuffer.Data)
				server.State = SDO_STATE_IDLE
				ret = SDO_SUCCESS
			} else {
				log.Debugf("[SERVER][TX] DOWNLOAD SEGMENT INIT | x%x:x%x %v", server.Index, server.Subindex, server.txBuffer.Data)
				server.Toggle = 0x00
				server.SizeTransferred = 0
				server.bufWriteOffset = 0
				server.bufReadOffset = 0
				server.State = SDO_STATE_DOWNLOAD_SEGMENT_REQ
			}

		case SDO_STATE_DOWNLOAD_SEGMENT_RSP:
			server.txBuffer.Data[0] = 0x20 | server.Toggle
			server.Toggle ^= 0x10
			server.TimeoutTimer = 0
			log.Debugf("[SERVER][TX] DOWNLOAD SEGMENT | x%x:x%x %v", server.Index, server.Subindex, server.txBuffer.Data)
			server.BusManager.Send(server.txBuffer)
			if server.Finished {
				server.State = SDO_STATE_IDLE
				ret = SDO_SUCCESS
			} else {
				server.State = SDO_STATE_DOWNLOAD_SEGMENT_REQ
			}

		case SDO_STATE_UPLOAD_INITIATE_RSP:
			if server.SizeIndicated > 0 && server.SizeIndicated <= 4 {
				server.txBuffer.Data[0] = 0x43 | ((4 - byte(server.SizeIndicated)) << 2)
				copy(server.txBuffer.Data[4:], server.buffer[:server.SizeIndicated])
				server.State = SDO_STATE_IDLE
				ret = SDO_SUCCESS
				log.Debugf("[SERVER][TX] UPLOAD EXPEDITED | x%x:x%x %v", server.Index, server.Subindex, server.txBuffer.Data)
			} else {
				// Segmented transfer
				if server.SizeIndicated > 0 {
					server.txBuffer.Data[0] = 0x41
					// Add data size
					binary.LittleEndian.PutUint32(server.txBuffer.Data[4:], server.SizeIndicated)

				} else {
					server.txBuffer.Data[0] = 0x40
				}
				server.Toggle = 0x00
				server.TimeoutTimer = 0
				server.State = SDO_STATE_UPLOAD_SEGMENT_REQ
				log.Debugf("[SERVER][TX] UPLOAD SEGMENTED | x%x:x%x %v", server.Index, server.Subindex, server.txBuffer.Data)
			}
			server.txBuffer.Data[1] = byte(server.Index)
			server.txBuffer.Data[2] = byte(server.Index >> 8)
			server.txBuffer.Data[3] = server.Subindex
			server.BusManager.Send(server.txBuffer)

		case SDO_STATE_UPLOAD_SEGMENT_RSP:
			// Refill buffer if needed
			if !server.readObjectDictionary(&abortCode, 7, false) {
				break
			}
			server.txBuffer.Data[0] = server.Toggle
			server.Toggle ^= 0x10
			count := server.bufWriteOffset - server.bufReadOffset
			// Check if last segment
			if count < 7 || (server.Finished && count == 7) {
				server.txBuffer.Data[0] |= (byte((7 - count) << 1)) | 0x01
				server.State = SDO_STATE_IDLE
				ret = SDO_SUCCESS
			} else {
				server.TimeoutTimer = 0
				server.State = SDO_STATE_UPLOAD_SEGMENT_REQ
				count = 7
			}
			copy(server.txBuffer.Data[1:], server.buffer[server.bufReadOffset:server.bufReadOffset+count])
			server.bufReadOffset += count
			server.SizeTransferred += count
			// Check if too shor or too large in last segment
			if server.SizeIndicated > 0 {
				if server.SizeTransferred > server.SizeIndicated {
					abortCode = SDO_ABORT_DATA_LONG
					server.State = SDO_STATE_ABORT
					break
				} else if ret == SDO_SUCCESS && server.SizeTransferred < server.SizeIndicated {
					abortCode = SDO_ABORT_DATA_SHORT
					ret = SDO_WAITING_RESPONSE
					server.State = SDO_STATE_ABORT
					break
				}
			}
			log.Debugf("[SERVER][TX] UPLOAD SEGMENTED | x%x:x%x %v", server.Index, server.Subindex, server.txBuffer.Data)
			server.BusManager.Send(server.txBuffer)

		case SDO_STATE_DOWNLOAD_BLK_INITIATE_RSP:
			server.txBuffer.Data[0] = 0xA4
			server.txBuffer.Data[1] = byte(server.Index)
			server.txBuffer.Data[2] = byte(server.Index >> 8)
			server.txBuffer.Data[3] = server.Subindex
			// Calculate blocks from free space
			count := (len(server.buffer) - 2) / 7
			if count > 127 {
				count = 127
			}
			server.BlockSize = uint8(count)
			server.txBuffer.Data[4] = server.BlockSize
			// Reset variables
			server.SizeTransferred = 0
			server.Finished = false
			server.bufReadOffset = 0
			server.bufWriteOffset = 0
			server.BlockSequenceNb = 0
			server.BlockCRC = CRC16(0)
			server.TimeoutTimer = 0
			server.TimeoutTimerBlock = 0
			server.State = SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ
			server.RxNew = false
			log.Debugf("[SERVER][TX] BLOCK DOWNLOAD INIT | x%x:x%x %v", server.Index, server.Subindex, server.txBuffer.Data)
			server.BusManager.Send(server.txBuffer)

		case SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_RSP:
			server.txBuffer.Data[0] = 0xA2
			server.txBuffer.Data[1] = server.BlockSequenceNb
			transferShort := server.BlockSequenceNb != server.BlockSize
			seqnoStart := server.BlockSequenceNb
			// Is it last segment ?
			if server.Finished {
				server.State = SDO_STATE_DOWNLOAD_BLK_END_REQ
			} else {
				// Calclate from free buffer space
				count := (len(server.buffer) - 2 - int(server.bufWriteOffset)) / 7
				if count > 127 {
					count = 127
				} else if server.bufWriteOffset > 0 {
					// Empty buffer
					if !server.writeObjectDictionary(&abortCode, 1, 0) {
						break
					}
					count = (len(server.buffer) - 2 - int(server.bufWriteOffset)) / 7
					if count > 127 {
						count = 127
					}
				}
				server.BlockSize = uint8(count)
				server.BlockSequenceNb = 0
				server.State = SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ
				server.RxNew = false
			}
			server.txBuffer.Data[2] = server.BlockSize
			server.TimeoutTimerBlock = 0
			server.BusManager.Send(server.txBuffer)

			if transferShort && !server.Finished {
				log.Debugf("[SERVER][TX] BLOCK DOWNLOAD RESTART seqno prev=%v, blksize=%v", seqnoStart, server.BlockSize)
			} else {
				log.Debugf("[SERVER][TX] BLOCK DOWNLOAD SUB-BLOCK RES | x%x:x%x blksize %v %v",
					server.Index,
					server.Subindex,
					server.BlockSize,
					server.txBuffer.Data,
				)
			}

		case SDO_STATE_DOWNLOAD_BLK_END_RSP:
			server.txBuffer.Data[0] = 0xA1
			log.Debugf("[SERVER][TX] BLOCK DOWNLOAD END | x%x:x%x %v", server.Index, server.Subindex, server.txBuffer.Data)
			server.BusManager.Send(server.txBuffer)
			server.State = SDO_STATE_IDLE
			ret = SDO_SUCCESS

		case SDO_STATE_UPLOAD_BLK_INITIATE_RSP:
			server.txBuffer.Data[0] = 0xC4
			server.txBuffer.Data[1] = byte(server.Index)
			server.txBuffer.Data[2] = byte(server.Index >> 8)
			server.txBuffer.Data[3] = server.Subindex
			// Add data size
			if server.SizeIndicated > 0 {
				server.txBuffer.Data[0] |= 0x02
				binary.LittleEndian.PutUint32(server.txBuffer.Data[4:], server.SizeIndicated)
			}
			// Reset timer & send
			server.TimeoutTimer = 0
			log.Debugf("[SERVER][TX] BLOCK UPLOAD INIT | x%x:x%x %v", server.Index, server.Subindex, server.txBuffer.Data)
			server.BusManager.Send(server.txBuffer)
			server.State = SDO_STATE_UPLOAD_BLK_INITIATE_REQ2

		case SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ:
			// Write header & gend current count
			server.BlockSequenceNb += 1
			server.txBuffer.Data[0] = server.BlockSequenceNb
			count := server.bufWriteOffset - server.bufReadOffset
			// Check if last segment
			if count < 7 || (server.Finished && count == 7) {
				server.txBuffer.Data[0] |= 0x80
			} else {
				count = 7
			}
			copy(server.txBuffer.Data[1:], server.buffer[server.bufReadOffset:server.bufReadOffset+count])
			server.bufReadOffset += count
			server.BlockNoData = byte(7 - count)
			server.SizeTransferred += count
			// Check if too short or too large in last segment
			if server.SizeIndicated > 0 {
				if server.SizeTransferred > server.SizeIndicated {
					abortCode = SDO_ABORT_DATA_LONG
					server.State = SDO_STATE_ABORT
					break
				} else if server.bufReadOffset == server.bufWriteOffset && server.SizeTransferred < server.SizeIndicated {
					abortCode = SDO_ABORT_DATA_SHORT
					server.State = SDO_STATE_ABORT
					break
				}
			}
			// Check if last segment or all segments in current block transferred
			if server.bufWriteOffset == server.bufReadOffset || server.BlockSequenceNb >= server.BlockSize {
				server.State = SDO_STATE_UPLOAD_BLK_SUBBLOCK_CRSP
				log.Debugf("[SERVER][TX] BLOCK UPLOAD END SUB-BLOCK | x%x:x%x %v", server.Index, server.Subindex, server.txBuffer.Data)
			} else {
				log.Debugf("[SERVER][TX] BLOCK UPLOAD SUB-BLOCK | x%x:x%x %v", server.Index, server.Subindex, server.txBuffer.Data)
				if timerNextUs != nil {
					*timerNextUs = 0
				}
			}
			// Reset timer & send
			server.TimeoutTimer = 0
			server.BusManager.Send(server.txBuffer)

		case SDO_STATE_UPLOAD_BLK_END_SREQ:
			server.txBuffer.Data[0] = 0xC1 | (server.BlockNoData << 2)
			server.txBuffer.Data[1] = byte(server.BlockCRC)
			server.txBuffer.Data[2] = byte(server.BlockCRC >> 8)
			server.TimeoutTimer = 0
			log.Debugf("[SERVER][TX] BLOCK UPLOAD END | x%x:x%x %v", server.Index, server.Subindex, server.txBuffer.Data)
			server.BusManager.Send(server.txBuffer)
			server.State = SDO_STATE_UPLOAD_BLK_END_CRSP

		default:

		}

	}

	if ret == SDO_WAITING_RESPONSE {
		switch server.State {
		case SDO_STATE_ABORT:
			server.Abort(abortCode)
			server.State = SDO_STATE_IDLE
			err = ErrSDOEndedWithServerAbort
		case SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ:
			ret = SDO_BLOCK_DOWNLOAD_IN_PROGRESS
		case SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ:
			ret = SDO_BLOCK_UPLOAD_IN_PROGRESS
		}
	}
	return
}

// Create & send abort on bus
func (server *SDOServer) Abort(abortCode SDOAbortCode) {
	code := uint32(abortCode)
	server.txBuffer.Data[0] = 0x80
	server.txBuffer.Data[1] = uint8(server.Index)
	server.txBuffer.Data[2] = uint8(server.Index >> 8)
	server.txBuffer.Data[3] = server.Subindex
	binary.LittleEndian.PutUint32(server.txBuffer.Data[4:], code)
	server.BusManager.Send(server.txBuffer)
	log.Warnf("[SERVER][TX] SERVER ABORT | x%x:x%x | %v (x%x)", server.Index, server.Subindex, abortCode, uint32(abortCode))
	if server.ErrorExtraInfo != nil {
		log.Warnf("[SERVER][TX] SERVER ABORT | %v", server.ErrorExtraInfo)
		server.ErrorExtraInfo = nil
	}
}
