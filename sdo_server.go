package canopen

import (
	"encoding/binary"

	log "github.com/sirupsen/logrus"
)

/*TODOs:
- Add locking mechanisms for reading/writing to OD when accessing PDO mappable OD vars
- Re-check string support
- Add dynamic SDO write configuration
*/

type SDOServer struct {
	OD                         *ObjectDictionary
	Streamer                   *ObjectStreamer
	NodeId                     uint8
	BusManager                 *BusManager
	CANtxBuff                  *BufferTxFrame
	idRxBuff                   int
	idTxBuff                   int
	CobIdClientToServer        uint32
	CobIdServerToClient        uint32
	ExtensionEntry1200         *Extension
	Valid                      bool
	Index                      uint16
	Subindex                   uint8
	Finished                   bool
	SizeIndicated              uint32
	SizeTransferred            uint32
	State                      uint8
	TimeoutTimeUs              uint32
	TimeoutTimer               uint32
	Buffer                     []byte
	BufferOffsetWrite          uint32
	BufferOffsetRead           uint32
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
}

// Handle received messages
func (server *SDOServer) Handle(frame Frame) {
	if frame.DLC != 8 {
		log.Debugf("Ignoring client message because wrong length x%x %v; Server state : x%x", frame.ID, frame.Data, server.State)
		return
	}
	if frame.Data[0] == 0x80 {
		// Client abort
		log.Debugf("Abort from client")
		server.State = CO_SDO_ST_IDLE
	} else if server.RxNew {
		// Ignore message if previous message not processed
		log.Info("Ignoring because already received message")
	} else if server.State == CO_SDO_ST_UPLOAD_BLK_END_CRSP && frame.Data[0] == 0xA1 {
		// Block transferred ! go to idle
		server.State = CO_SDO_ST_IDLE
	} else if server.State == CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_REQ {
		// Condition should always pass but check
		if int(server.BufferOffsetWrite) <= (len(server.Buffer) - (7 + 2)) {
			// Block download, copy data in handle
			state := CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_REQ
			seqno := frame.Data[0] & 0x7F
			server.TimeoutTimer = 0
			server.TimeoutTimerBlock = 0
			// Check correct sequence number
			if seqno <= server.BlockSize && seqno == (server.BlockSequenceNb+1) {
				server.BlockSequenceNb = seqno
				// Copy data
				copy(server.Buffer[server.BufferOffsetWrite:], frame.Data[1:])
				server.BufferOffsetWrite += 7
				server.SizeTransferred += 7
				// Check if last segment
				if (frame.Data[0] & 0x80) != 0 {
					server.Finished = true
					state = CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_RSP
				} else if seqno == server.BlockSize {
					// All segments in sub block transferred
					state = CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_RSP
				}
				// If duplicate or sequence didn't start ignore, otherwise
				// seqno is wron
			} else if seqno != server.BlockSequenceNb && server.BlockSequenceNb != 0 {
				state = CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_RSP
				log.Warnf("Wrong sequence number in rx sub-block. seqno %v, previous %v", seqno, server.BlockSequenceNb)
			} else {
				log.Warnf("Wrong sequence number in rx ignored. seqno %v, expected %v", seqno, server.BlockSequenceNb+1)
			}

			if state != CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_REQ {
				server.RxNew = false
				server.State = state
			}
		}
	} else if server.State == CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_RSP {
		//Ignore other server messages if response requested
	} else {
		// Copy data and set new flag
		server.Response.raw = frame.Data
		server.RxNew = true
	}
}

func (server *SDOServer) Init(od *ObjectDictionary, entry12xx *Entry, nodeId uint8, timeoutTimeMs uint16, busManager *BusManager) error {
	if od == nil || busManager == nil {
		return CO_ERROR_ILLEGAL_ARGUMENT
	}
	server.OD = od
	server.Streamer = &ObjectStreamer{}
	server.Buffer = make([]byte, 500)
	server.BufferOffsetRead = 0
	server.BufferOffsetWrite = 0
	server.NodeId = nodeId
	server.TimeoutTimeUs = uint32(timeoutTimeMs) * 1000
	server.TimeoutTimeBlockTransferUs = uint32(timeoutTimeMs) * 700
	var canIdClientToServer uint16
	var canIdServerToClient uint16
	if entry12xx == nil {
		/*Configure sdo channel*/
		if nodeId < 1 || nodeId > 127 {
			log.Errorf("SDO server node id is not valid : %x", nodeId)
			return CO_ERROR_ILLEGAL_ARGUMENT
		}
		canIdClientToServer = SDO_CLIENT_ID + uint16(nodeId)
		canIdServerToClient = SDO_SERVER_ID + uint16(nodeId)
		server.Valid = true
	} else {
		if entry12xx.Index == 0x1200 {
			// Default channels
			if nodeId < 1 || nodeId > 127 {
				log.Errorf("SDO server node id is not valid : %x", nodeId)
				return CO_ERROR_ILLEGAL_ARGUMENT
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
				return CO_ERROR_OD_PARAMETERS
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
			server.ExtensionEntry1200.Object = server
			server.ExtensionEntry1200.Read = ReadEntryOriginal
			server.ExtensionEntry1200.Write = WriteEntryOriginal

		} else {
			return CO_ERROR_ILLEGAL_ARGUMENT
		}
	}
	server.RxNew = false
	server.BusManager = busManager
	server.idRxBuff = 0
	server.idTxBuff = 0
	server.CobIdClientToServer = 0
	server.CobIdServerToClient = 0
	return server.InitRxTx(server.BusManager, 0, 0, uint32(canIdClientToServer), uint32(canIdServerToClient))

}

func (server *SDOServer) InitRxTx(canModule *BusManager, idRx uint16, idTx uint16, cobIdClientToServer uint32, cobIdServerToClient uint32) error {
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
	if idRx == idTx && idTx == 0 {
		server.idRxBuff, ret = server.BusManager.InsertRxBuffer(uint32(CanIdC2S), 0x7FF, false, server)
		server.CANtxBuff, server.idTxBuff, _ = server.BusManager.InsertTxBuffer(uint32(CanIdS2C), false, 8, false)
	} else {
		ret = server.BusManager.UpdateRxBuffer(server.idRxBuff, uint32(CanIdC2S), 0x7FF, false, server)
		server.CANtxBuff, _ = server.BusManager.UpdateTxBuffer(server.idTxBuff, uint32(CanIdS2C), false, 8, false)
	}
	if server.CANtxBuff == nil {
		ret = CO_ERROR_ILLEGAL_ARGUMENT
		server.Valid = false
	}
	return ret

}

func (server *SDOServer) writeObjectDictionary(abortCode *SDOAbortCode, crcOperation uint, crcClient uint16) bool {
	if server.Finished {
		// Check size
		if server.SizeIndicated > 0 && server.SizeTransferred > server.SizeIndicated {
			*abortCode = CO_SDO_AB_DATA_LONG
			server.State = CO_SDO_ST_ABORT
			return false
		} else if server.SizeIndicated > 0 && server.SizeTransferred < server.SizeIndicated {
			*abortCode = CO_SDO_AB_DATA_SHORT
			server.State = CO_SDO_ST_ABORT
			return false
		}
		// Golang does not have null termination characters so nothing particular to do
		// Stream data should be limited to the sent value
		varSizeStream := len(server.Streamer.Stream.Data)
		if server.Streamer.Stream.Attribute&ODA_STR != 0 &&
			(varSizeStream == 0 || int(server.SizeTransferred) < varSizeStream) &&
			int(server.BufferOffsetWrite+2) <= len(server.Buffer) {
			// Reduce the size of the buffer to the transferred size
			server.Streamer.Stream.Data = server.Streamer.Stream.Data[:server.SizeTransferred]

		} else if int(server.SizeTransferred) != varSizeStream {
			if int(server.SizeTransferred) > varSizeStream {
				*abortCode = CO_SDO_AB_DATA_LONG
				server.State = CO_SDO_ST_ABORT
				return false
			} else if int(server.SizeTransferred) < varSizeStream {
				*abortCode = CO_SDO_AB_DATA_SHORT
				server.State = CO_SDO_ST_ABORT
				return false
			}
		}

	} else {
		// Still check if not bigger than max size
		if server.SizeIndicated > 0 && server.SizeTransferred > server.SizeIndicated {
			*abortCode = CO_SDO_AB_DATA_LONG
			server.State = CO_SDO_ST_ABORT
			return false
		}
	}

	// Calculate CRC
	if server.BlockCRCEnabled && crcOperation > 0 {
		server.BlockCRC.ccitt_block(server.Buffer[:server.BufferOffsetWrite])
		if crcOperation == 2 && crcClient != server.BlockCRC.crc {
			*abortCode = CO_SDO_AB_CRC
			server.State = CO_SDO_ST_ABORT
			return false
		}
	}

	// Write the data
	var countWritten uint16 = 0
	ret := server.Streamer.Write(&server.Streamer.Stream, server.Buffer[:server.BufferOffsetWrite], &countWritten)
	server.BufferOffsetWrite = 0
	if ret != nil && ret != ODR_PARTIAL {
		*abortCode = ret.(ODR).GetSDOAbordCode()
		server.State = CO_SDO_ST_ABORT
		return false
	} else if server.Finished && ret == ODR_PARTIAL {
		*abortCode = CO_SDO_AB_DATA_SHORT
		server.State = CO_SDO_ST_ABORT
		return false
	} else if !server.Finished && ret == nil {
		*abortCode = CO_SDO_AB_DATA_LONG
		server.State = CO_SDO_ST_ABORT
		return false
	}
	return true

}

func (server *SDOServer) readObjectDictionary(abortCode *SDOAbortCode, countMinimum uint32, calculateCRC bool) bool {
	remainingCount := server.BufferOffsetWrite - server.BufferOffsetRead
	if !server.Finished && remainingCount < countMinimum {
		copy(server.Buffer, server.Buffer[server.BufferOffsetRead:])
		server.BufferOffsetRead = 0
		server.BufferOffsetWrite = remainingCount
		var countRd uint16 = 0

		err := server.Streamer.Read(&server.Streamer.Stream, server.Buffer[remainingCount:], &countRd)

		if err != nil && err != ODR_PARTIAL {
			*abortCode = err.(ODR).GetSDOAbordCode()
			server.State = CO_SDO_ST_ABORT
			return false
		}

		server.BufferOffsetWrite = remainingCount + uint32(countRd)
		if server.BufferOffsetWrite == 0 || err == ODR_PARTIAL {
			server.Finished = false
			if server.BufferOffsetWrite < countMinimum {
				*abortCode = CO_SDO_AB_DEVICE_INCOMPAT
				server.State = CO_SDO_ST_ABORT
				return false
			}
		} else {
			server.Finished = true
		}
		if calculateCRC && server.BlockCRCEnabled {
			server.BlockCRC.ccitt_block(server.Buffer[remainingCount:])
		}

	}

	return true
}

func updateStateFromRequest(stateReq uint8, state *uint8, upload *bool) {
	*upload = false
	if (stateReq & 0xF0) == 0x20 {
		*state = CO_SDO_ST_DOWNLOAD_INITIATE_REQ
	} else if stateReq == 0x40 {
		*upload = true
		*state = CO_SDO_ST_UPLOAD_INITIATE_REQ
	} else if (stateReq & 0xF9) == 0xC0 {
		*state = CO_SDO_ST_DOWNLOAD_BLK_INITIATE_REQ
	} else if (stateReq & 0xFB) == 0xA0 {
		*upload = true
		*state = CO_SDO_ST_UPLOAD_BLK_INITIATE_REQ
	} else {
		*state = CO_SDO_ST_ABORT
	}
}

func (server *SDOServer) Process(nmtIsPreOrOperationnal bool, timeDifferenceUs uint32, timerNextUs *uint32) SDOReturn {
	ret := CO_SDO_RT_waitingResponse
	abortCode := CO_SDO_AB_NONE
	if server.Valid && server.State == CO_SDO_ST_IDLE && !server.RxNew {
		ret = CO_SDO_RT_ok_communicationEnd
	} else if !nmtIsPreOrOperationnal || !server.Valid {
		server.State = CO_SDO_ST_IDLE
		server.RxNew = false
		ret = CO_SDO_RT_ok_communicationEnd
	} else if server.RxNew {
		response := server.Response
		if server.State == CO_SDO_ST_IDLE {
			upload := false
			updateStateFromRequest(response.raw[0], &server.State, &upload)
			if server.State == CO_SDO_ST_ABORT {
				abortCode = CO_SDO_AB_CMD
			}

			// Check object exists and accessible
			if abortCode == CO_SDO_AB_NONE {
				server.Index = response.GetIndex()
				server.Subindex = response.GetSubindex()
				err := server.OD.Find(server.Index).Sub(server.Subindex, false, server.Streamer)
				if err != nil {
					abortCode = err.(ODR).GetSDOAbordCode()
					server.State = CO_SDO_ST_ABORT
				} else {
					if server.Streamer.Stream.Attribute&ODA_SDO_RW == 0 {
						abortCode = CO_SDO_AB_UNSUPPORTED_ACCESS
						server.State = CO_SDO_ST_ABORT
					} else if upload && (server.Streamer.Stream.Attribute&ODA_SDO_R) == 0 {
						abortCode = CO_SDO_AB_WRITEONLY
						server.State = CO_SDO_ST_ABORT
					} else if !upload && (server.Streamer.Stream.Attribute&ODA_SDO_W) == 0 {
						abortCode = CO_SDO_AB_READONLY
						server.State = CO_SDO_ST_ABORT
					}
				}
			}
			// Load data from OD
			if upload && abortCode == CO_SDO_AB_NONE {
				server.BufferOffsetRead = 0
				server.BufferOffsetWrite = 0
				server.SizeTransferred = 0
				server.Finished = false
				if server.readObjectDictionary(&abortCode, 7, false) {
					// Size may not be known yet
					if server.Finished {
						server.SizeIndicated = uint32(len(server.Streamer.Stream.Data))
						if server.SizeIndicated == 0 {
							server.SizeIndicated = server.BufferOffsetWrite
						} else if server.SizeIndicated != server.BufferOffsetWrite {
							abortCode = CO_SDO_AB_DEVICE_INCOMPAT
							server.State = CO_SDO_ST_ABORT
						}
					} else {
						server.SizeIndicated = uint32(len(server.Streamer.Stream.Data))
					}
				}

			}
		}

		if server.State != CO_SDO_ST_IDLE && server.State != CO_SDO_ST_ABORT {
			switch server.State {
			case CO_SDO_ST_DOWNLOAD_INITIATE_REQ:
				if (response.raw[0] & 0x02) != 0 {
					log.Debugf("[SERVER] <==Rx | DOWNLOAD EXPEDITED | x%x:x%x %v", server.Index, server.Subindex, response.raw)
					//Expedited 4 bytes of data max
					sizeInOd := uint32(len(server.Streamer.Stream.Data))
					dataSizeToWrite := 4
					if (response.raw[0] & 0x01) != 0 {
						dataSizeToWrite -= (int(response.raw[0]) >> 2) & 0x03
					} else if sizeInOd > 0 && sizeInOd < 4 {
						dataSizeToWrite = int(sizeInOd)
					}
					//Create temporary buffer
					buf := response.raw[4:]
					buf = append(buf, []byte{0, 0}...)
					// TODO add checks if size is 0 & if string
					if dataSizeToWrite != int(sizeInOd) {
						if dataSizeToWrite > int(sizeInOd) {
							abortCode = CO_SDO_AB_DATA_LONG
						} else {
							abortCode = CO_SDO_AB_DATA_SHORT
						}
						server.State = CO_SDO_ST_ABORT
						break
					}
					var countWritten uint16 = 0
					err := server.Streamer.Write(&server.Streamer.Stream, buf[:dataSizeToWrite], &countWritten)
					if err != nil {
						abortCode = err.(ODR).GetSDOAbordCode()
						server.State = CO_SDO_ST_ABORT
						break
					} else {
						server.State = CO_SDO_ST_DOWNLOAD_INITIATE_RSP
						server.Finished = true

					}
				} else {
					if (response.raw[0] & 0x01) != 0 {
						log.Debugf("[SERVER] <==Rx | DOWNLOAD SEGMENTED | x%x:x%x %v", server.Index, server.Subindex, response.raw)
						// Segmented transfer check if size indicated
						sizeInOd := len(server.Streamer.Stream.Data)
						server.SizeIndicated = binary.LittleEndian.Uint32(response.raw[4:])
						// Check if size matches
						if sizeInOd > 0 {
							if server.SizeIndicated > uint32(sizeInOd) {
								abortCode = CO_SDO_AB_DATA_LONG
								server.State = CO_SDO_ST_ABORT
								break
							} else if server.SizeIndicated < uint32(sizeInOd) && (server.Streamer.Stream.Attribute&ODA_STR) == 0 {
								abortCode = CO_SDO_AB_DATA_SHORT
								server.State = CO_SDO_ST_ABORT
								break
							}
						}
					} else {
						server.SizeIndicated = 0
					}
					server.State = CO_SDO_ST_DOWNLOAD_INITIATE_RSP
					server.Finished = false
				}

			case CO_SDO_ST_DOWNLOAD_SEGMENT_REQ:
				if (response.raw[0] & 0xE0) == 0x00 {
					log.Debugf("[SERVER] <==Rx | DOWNLOAD SEGMENT | x%x:x%x %v", server.Index, server.Subindex, response.raw)
					server.Finished = (response.raw[0] & 0x01) != 0
					toggle := response.GetToggle()
					if toggle != server.Toggle {
						abortCode = CO_SDO_AB_TOGGLE_BIT
						server.State = CO_SDO_ST_ABORT
						break
					}
					// Get size and write to buffer
					count := 7 - ((response.raw[0] >> 1) & 0x07)
					copy(server.Buffer[server.BufferOffsetWrite:], response.raw[1:1+count])
					server.BufferOffsetWrite += uint32(count)
					server.SizeTransferred += uint32(count)

					if len(server.Streamer.Stream.Data) > 0 && int(server.SizeTransferred) > len(server.Streamer.Stream.Data) {
						abortCode = CO_SDO_AB_DATA_LONG
						server.State = CO_SDO_ST_ABORT
						break
					}
					if server.Finished || (len(server.Buffer)-int(server.BufferOffsetWrite) < (7 + 2)) {
						if !server.writeObjectDictionary(&abortCode, 0, 0) {
							break
						}
					}
					server.State = CO_SDO_ST_DOWNLOAD_SEGMENT_RSP
				} else {
					abortCode = CO_SDO_AB_CMD
					server.State = CO_SDO_ST_ABORT
				}

			case CO_SDO_ST_UPLOAD_INITIATE_REQ:
				log.Debugf("[SERVER] <==Rx | UPLOAD EXPEDITED | x%x:x%x %v", server.Index, server.Subindex, response.raw)
				server.State = CO_SDO_ST_UPLOAD_INITIATE_RSP

			case CO_SDO_ST_UPLOAD_SEGMENT_REQ:
				log.Debugf("[SERVER] <==Rx | UPLOAD SEGMENTED | x%x:x%x %v", server.Index, server.Subindex, response.raw)
				if (response.raw[0] & 0xEF) != 0x60 {
					abortCode = CO_SDO_AB_CMD
					server.State = CO_SDO_ST_ABORT
					break
				}
				toggle := response.GetToggle()
				if toggle != server.Toggle {
					abortCode = CO_SDO_AB_TOGGLE_BIT
					server.State = CO_SDO_ST_ABORT
					break
				}
				server.State = CO_SDO_ST_UPLOAD_SEGMENT_RSP

			case CO_SDO_ST_DOWNLOAD_BLK_INITIATE_REQ:
				log.Debugf("[SERVER] <==Rx | DOWNLOAD BLOCK INIT | x%x:x%x %v", server.Index, server.Subindex, response.raw)
				server.BlockCRCEnabled = response.IsCRCEnabled()
				// Check if size indicated
				if (response.raw[0] & 0x02) != 0 {
					sizeInOd := len(server.Streamer.Stream.Data)
					server.SizeIndicated = binary.LittleEndian.Uint32(response.raw[4:])
					// Check if size matches
					if sizeInOd > 0 {
						if server.SizeIndicated > uint32(sizeInOd) {
							abortCode = CO_SDO_AB_DATA_LONG
							server.State = CO_SDO_ST_ABORT
							break
						} else if server.SizeIndicated < uint32(sizeInOd) && (server.Streamer.Stream.Attribute&ODA_STR) == 0 {
							abortCode = CO_SDO_AB_DATA_SHORT
							server.State = CO_SDO_ST_ABORT
							break
						}
					}
				} else {
					server.SizeIndicated = 0
				}
				server.State = CO_SDO_ST_DOWNLOAD_BLK_INITIATE_RSP
				server.Finished = false

			case CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_REQ:

			case CO_SDO_ST_DOWNLOAD_BLK_END_REQ:
				log.Debugf("[SERVER] <==Rx | DOWNLOAD BLOCK END | x%x:x%x %v", server.Index, server.Subindex, response.raw)
				if (response.raw[0] & 0xE3) != 0xC1 {
					abortCode = CO_SDO_AB_CMD
					server.State = CO_SDO_ST_ABORT
					break
				}
				//Get number of data bytes in last segment, that do not
				//contain data. Then reduce buffer
				noData := (response.raw[0] >> 2) & 0x07
				if server.BufferOffsetWrite <= uint32(noData) {
					abortCode = CO_SDO_AB_DEVICE_INCOMPAT
					server.State = CO_SDO_ST_ABORT
					break
				}
				server.SizeTransferred -= uint32(noData)
				server.BufferOffsetWrite -= uint32(noData)
				var crcClient uint16 = 0
				if server.BlockCRCEnabled {
					crcClient = response.GetCRCClient()
				}
				if !server.writeObjectDictionary(&abortCode, 2, crcClient) {
					break
				}
				server.State = CO_SDO_ST_DOWNLOAD_BLK_END_RSP

			case CO_SDO_ST_UPLOAD_BLK_INITIATE_REQ:
				// If protocol switch threshold (byte 5) is larger than data
				// size of OD var, then switch to segmented
				if server.SizeIndicated > 0 && response.raw[5] > 0 && uint32(response.raw[5]) >= server.SizeIndicated {
					server.State = CO_SDO_ST_UPLOAD_INITIATE_RSP
				} else {
					if (response.raw[0] & 0x04) != 0 {
						server.BlockCRCEnabled = true
						server.BlockCRC = CRC16{0}
						server.BlockCRC.ccitt_block(server.Buffer[:server.BufferOffsetWrite])
					} else {
						server.BlockCRCEnabled = false
					}
					// Get block size and check okay
					server.BlockSize = response.GetBlockSize()
					if server.BlockSize < 1 || server.BlockSize > 127 {
						abortCode = CO_SDO_AB_BLOCK_SIZE
						server.State = CO_SDO_ST_ABORT
						break
					}

					// Check if enough space in buffer
					if !server.Finished && server.BufferOffsetWrite < uint32(server.BlockSize)*7 {
						abortCode = CO_SDO_AB_DEVICE_INCOMPAT
						server.State = CO_SDO_ST_ABORT
						break
					}
					server.State = CO_SDO_ST_UPLOAD_BLK_INITIATE_RSP
					log.Debugf("[SERVER] <==Rx | UPLOAD BLOCK INIT | x%x:x%x %v | crc : %v, blksize :%v", server.Index, server.Subindex, response.raw, server.BlockCRCEnabled, server.BlockSize)

				}

			case CO_SDO_ST_UPLOAD_BLK_INITIATE_REQ2:
				if response.raw[0] == 0xA3 {
					server.BlockSequenceNb = 0
					server.State = CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_SREQ
				} else {
					abortCode = CO_SDO_AB_CMD
					server.State = CO_SDO_ST_ABORT
				}

			case CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_SREQ, CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_CRSP:
				if response.raw[0] != 0xA2 {
					abortCode = CO_SDO_AB_CMD
					server.State = CO_SDO_ST_ABORT
					break
				}
				// Check block size
				server.BlockSize = response.raw[2]
				if server.BlockSize < 1 || server.BlockSize > 127 {
					abortCode = CO_SDO_AB_BLOCK_SIZE
					server.State = CO_SDO_ST_ABORT
					break
				}
				// Check number of segments
				if response.raw[1] < server.BlockSequenceNb {
					// Some error occurd, re-transmit missing chunks
					cntFailed := server.BlockSequenceNb - response.raw[1]
					cntFailed = cntFailed*7 - server.BlockNoData
					server.BufferOffsetRead -= uint32(cntFailed)
					server.SizeTransferred -= uint32(cntFailed)
				} else if response.raw[1] > server.BlockSequenceNb {
					abortCode = CO_SDO_AB_CMD
					server.State = CO_SDO_ST_ABORT
					break
				}
				// Refill buffer if needed
				if !server.readObjectDictionary(&abortCode, uint32(server.BlockSize)*7, true) {
					break
				}

				if server.BufferOffsetWrite == server.BufferOffsetRead {
					server.State = CO_SDO_ST_UPLOAD_BLK_END_SREQ
				} else {
					server.BlockSequenceNb = 0
					server.State = CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_SREQ
				}

			default:
				abortCode = CO_SDO_AB_CMD
				server.State = CO_SDO_ST_ABORT

			}
		}
		server.TimeoutTimer = 0
		timeDifferenceUs = 0
		server.RxNew = false
	}

	if ret == CO_SDO_RT_waitingResponse {
		if server.TimeoutTimer < server.TimeoutTimeUs {
			server.TimeoutTimer += timeDifferenceUs
		}
		if server.TimeoutTimer >= server.TimeoutTimeUs {
			abortCode = CO_SDO_AB_TIMEOUT
			server.State = CO_SDO_ST_ABORT
			log.Errorf("[SERVER] Timeout %v, State : %v", server.TimeoutTimer, server.State)

		} else if timerNextUs != nil {
			diff := server.TimeoutTimeUs - server.TimeoutTimer
			if *timerNextUs > diff {
				*timerNextUs = diff
			}
		}
		// Timeout for subblocks
		if server.State == CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_REQ {
			if server.TimeoutTimerBlock < server.TimeoutTimeBlockTransferUs {
				server.TimeoutTimerBlock += timeDifferenceUs
			}
			if server.TimeoutTimerBlock >= server.TimeoutTimeBlockTransferUs {
				server.State = CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_RSP
				server.RxNew = false
			} else if timerNextUs != nil {
				diff := server.TimeoutTimeBlockTransferUs - server.TimeoutTimerBlock
				if *timerNextUs > diff {
					*timerNextUs = diff
				}
			}
		}
		if server.CANtxBuff.BufferFull {
			ret = CO_SDO_RT_transmittBufferFull
		}
	}

	if ret == CO_SDO_RT_waitingResponse {
		server.CANtxBuff.Data = [8]byte{0}

		switch server.State {
		case CO_SDO_ST_DOWNLOAD_INITIATE_RSP:
			server.CANtxBuff.Data[0] = 0x60
			server.CANtxBuff.Data[1] = byte(server.Index)
			server.CANtxBuff.Data[2] = byte(server.Index >> 8)
			server.CANtxBuff.Data[3] = server.Subindex
			server.TimeoutTimer = 0
			server.BusManager.Send(*server.CANtxBuff)
			if server.Finished {
				log.Debugf("[SERVER] ==>Tx | DOWNLOAD EXPEDITED | x%x:x%x %v", server.Index, server.Subindex, server.CANtxBuff.Data)
				server.State = CO_SDO_ST_IDLE
				ret = CO_SDO_RT_ok_communicationEnd
			} else {
				log.Debugf("[SERVER] ==>Tx | DOWNLOAD SEGMENT INIT | x%x:x%x %v", server.Index, server.Subindex, server.CANtxBuff.Data)
				server.Toggle = 0x00
				server.SizeTransferred = 0
				server.BufferOffsetWrite = 0
				server.BufferOffsetRead = 0
				server.State = CO_SDO_ST_DOWNLOAD_SEGMENT_REQ
			}

		case CO_SDO_ST_DOWNLOAD_SEGMENT_RSP:
			server.CANtxBuff.Data[0] = 0x20 | server.Toggle
			server.Toggle ^= 0x10
			server.TimeoutTimer = 0
			log.Debugf("[SERVER] ==>Tx | DOWNLOAD SEGMENT | x%x:x%x %v", server.Index, server.Subindex, server.CANtxBuff.Data)
			server.BusManager.Send(*server.CANtxBuff)
			if server.Finished {
				server.State = CO_SDO_ST_IDLE
				ret = CO_SDO_RT_ok_communicationEnd
			} else {
				server.State = CO_SDO_ST_DOWNLOAD_SEGMENT_REQ
			}

		case CO_SDO_ST_UPLOAD_INITIATE_RSP:
			if server.SizeIndicated > 0 && server.SizeIndicated <= 4 {
				server.CANtxBuff.Data[0] = 0x43 | ((4 - byte(server.SizeIndicated)) << 2)
				copy(server.CANtxBuff.Data[4:], server.Buffer[:server.SizeIndicated])
				server.State = CO_SDO_ST_IDLE
				ret = CO_SDO_RT_ok_communicationEnd
				log.Debugf("[SERVER] ==>Tx | UPLOAD EXPEDITED | x%x:x%x %v", server.Index, server.Subindex, server.CANtxBuff.Data)
			} else {
				// Segmented transfer
				if server.SizeIndicated > 0 {
					server.CANtxBuff.Data[0] = 0x41
					// Add data size
					binary.LittleEndian.PutUint32(server.CANtxBuff.Data[4:], server.SizeIndicated)

				} else {
					server.CANtxBuff.Data[0] = 0x40
				}
				server.Toggle = 0x00
				server.TimeoutTimer = 0
				server.State = CO_SDO_ST_UPLOAD_SEGMENT_REQ
				log.Debugf("[SERVER] ==>Tx | UPLOAD SEGMENTED | x%x:x%x %v", server.Index, server.Subindex, server.CANtxBuff.Data)
			}
			server.CANtxBuff.Data[1] = byte(server.Index)
			server.CANtxBuff.Data[2] = byte(server.Index >> 8)
			server.CANtxBuff.Data[3] = server.Subindex
			server.BusManager.Send(*server.CANtxBuff)

		case CO_SDO_ST_UPLOAD_SEGMENT_RSP:
			// Refill buffer if needed
			if !server.readObjectDictionary(&abortCode, 7, false) {
				break
			}
			server.CANtxBuff.Data[0] = server.Toggle
			server.Toggle ^= 0x10
			count := server.BufferOffsetWrite - server.BufferOffsetRead
			// Check if last segment
			if count < 7 || (server.Finished && count == 7) {
				server.CANtxBuff.Data[0] |= (byte((7 - count) << 1)) | 0x01
				server.State = CO_SDO_ST_IDLE
				ret = CO_SDO_RT_ok_communicationEnd
			} else {
				server.TimeoutTimer = 0
				server.State = CO_SDO_ST_UPLOAD_SEGMENT_REQ
				count = 7
			}
			copy(server.CANtxBuff.Data[1:], server.Buffer[server.BufferOffsetRead:server.BufferOffsetRead+count])
			server.BufferOffsetRead += count
			server.SizeTransferred += count
			// Check if too shor or too large in last segment
			if server.SizeIndicated > 0 {
				if server.SizeTransferred > server.SizeIndicated {
					abortCode = CO_SDO_AB_DATA_LONG
					server.State = CO_SDO_ST_ABORT
					break
				} else if ret == CO_SDO_RT_ok_communicationEnd && server.SizeTransferred < server.SizeIndicated {
					abortCode = CO_SDO_AB_DATA_SHORT
					ret = CO_SDO_RT_waitingResponse
					server.State = CO_SDO_ST_ABORT
					break
				}
			}
			log.Debugf("[SERVER] ==>Tx | UPLOAD SEGMENTED | x%x:x%x %v", server.Index, server.Subindex, server.CANtxBuff.Data)
			server.BusManager.Send(*server.CANtxBuff)

		case CO_SDO_ST_DOWNLOAD_BLK_INITIATE_RSP:
			server.CANtxBuff.Data[0] = 0xA4
			server.CANtxBuff.Data[1] = byte(server.Index)
			server.CANtxBuff.Data[2] = byte(server.Index >> 8)
			server.CANtxBuff.Data[3] = server.Subindex
			// Calculate blocks from free space
			count := (len(server.Buffer) - 2) / 7
			if count > 127 {
				count = 127
			}
			server.BlockSize = uint8(count)
			server.CANtxBuff.Data[4] = server.BlockSize
			// Reset variables
			server.SizeTransferred = 0
			server.Finished = false
			server.BufferOffsetRead = 0
			server.BufferOffsetWrite = 0
			server.BlockSequenceNb = 0
			server.BlockCRC = CRC16{0}
			server.TimeoutTimer = 0
			server.TimeoutTimerBlock = 0
			server.State = CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_REQ
			server.RxNew = false
			log.Debugf("[SERVER] ==>Tx | BLOCK DOWNLOAD INIT | x%x:x%x %v", server.Index, server.Subindex, server.CANtxBuff.Data)
			server.BusManager.Send(*server.CANtxBuff)

		case CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_RSP:
			server.CANtxBuff.Data[0] = 0xA2
			server.CANtxBuff.Data[1] = server.BlockSequenceNb
			transferShort := server.BlockSequenceNb != server.BlockSize
			seqnoStart := server.BlockSequenceNb
			// Is it last segment ?
			if server.Finished {
				server.State = CO_SDO_ST_DOWNLOAD_BLK_END_REQ
			} else {
				// Calclate from free buffer space
				count := (len(server.Buffer) - 2 - int(server.BufferOffsetWrite)) / 7
				if count > 127 {
					count = 127
				} else if server.BufferOffsetWrite > 0 {
					// Empty buffer
					if !server.writeObjectDictionary(&abortCode, 1, 0) {
						break
					}
					count = (len(server.Buffer) - 2 - int(server.BufferOffsetWrite))
					if count > 127 {
						count = 127
					}
				}
				server.BlockSize = uint8(count)
				server.BlockSequenceNb = 0
				server.State = CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_REQ
				server.RxNew = false
			}
			server.CANtxBuff.Data[2] = server.BlockSize
			server.TimeoutTimerBlock = 0
			log.Debugf("[SERVER] ==>Tx | BLOCK DOWNLOAD SUB-BLOCK | x%x:x%x %v", server.Index, server.Subindex, server.CANtxBuff.Data)
			server.BusManager.Send(*server.CANtxBuff)
			if transferShort && !server.Finished {
				log.Debugf("sub-block restarte : seqno prev=%v, blksize=%v", seqnoStart, server.BlockSize)
			}

		case CO_SDO_ST_DOWNLOAD_BLK_END_RSP:
			server.CANtxBuff.Data[0] = 0xA1
			log.Debugf("[SERVER] ==>Tx | BLOCK DOWNLOAD END | x%x:x%x %v", server.Index, server.Subindex, server.CANtxBuff.Data)
			server.BusManager.Send(*server.CANtxBuff)
			server.State = CO_SDO_ST_IDLE
			ret = CO_SDO_RT_ok_communicationEnd

		case CO_SDO_ST_UPLOAD_BLK_INITIATE_RSP:
			server.CANtxBuff.Data[0] = 0xC4
			server.CANtxBuff.Data[1] = byte(server.Index)
			server.CANtxBuff.Data[2] = byte(server.Index >> 8)
			server.CANtxBuff.Data[3] = server.Subindex
			// Add data size
			if server.SizeIndicated > 0 {
				server.CANtxBuff.Data[0] |= 0x02
				binary.LittleEndian.PutUint32(server.CANtxBuff.Data[4:], server.SizeIndicated)
			}
			// Reset timer & send
			server.TimeoutTimer = 0
			log.Debugf("[SERVER] ==>Tx | BLOCK UPLOAD INIT | x%x:x%x %v", server.Index, server.Subindex, server.CANtxBuff.Data)
			server.BusManager.Send(*server.CANtxBuff)
			server.State = CO_SDO_ST_UPLOAD_BLK_INITIATE_REQ2

		case CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_SREQ:
			// Write header & gend current count
			server.BlockSequenceNb += 1
			server.CANtxBuff.Data[0] = server.BlockSequenceNb
			count := server.BufferOffsetWrite - server.BufferOffsetRead
			// Check if last segment
			if count < 7 || (server.Finished && count == 7) {
				server.CANtxBuff.Data[0] |= 0x80
			} else {
				count = 7
			}
			copy(server.CANtxBuff.Data[1:], server.Buffer[server.BufferOffsetRead:server.BufferOffsetRead+count])
			server.BufferOffsetRead += count
			server.BlockNoData = byte(7 - count)
			server.SizeTransferred += count
			// Check if too shor or too large in last segment
			if server.SizeIndicated > 0 {
				if server.SizeTransferred > server.SizeIndicated {
					abortCode = CO_SDO_AB_DATA_LONG
					server.State = CO_SDO_ST_ABORT
					break
				} else if server.BufferOffsetRead == server.BufferOffsetWrite && server.SizeTransferred < server.SizeIndicated {
					abortCode = CO_SDO_AB_DATA_SHORT
					server.State = CO_SDO_ST_ABORT
					break
				}
			}
			// Check if last segment or all semgents in current block transferred
			if server.BufferOffsetWrite == server.BufferOffsetRead || server.BlockSequenceNb >= server.BlockSize {
				server.State = CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_CRSP
			} else {
				if timerNextUs != nil {
					*timerNextUs = 0
				}
			}
			// Reset timer & send
			server.TimeoutTimer = 0
			log.Debugf("[SERVER] ==>Tx | BLOCK UPLOAD SUB-BLOCK | x%x:x%x %v", server.Index, server.Subindex, server.CANtxBuff.Data)
			server.BusManager.Send(*server.CANtxBuff)

		case CO_SDO_ST_UPLOAD_BLK_END_SREQ:
			server.CANtxBuff.Data[0] = 0xC1 | (server.BlockNoData << 2)
			server.CANtxBuff.Data[1] = byte(server.BlockCRC.crc)
			server.CANtxBuff.Data[2] = byte(server.BlockCRC.crc >> 8)
			server.TimeoutTimer = 0
			log.Debugf("[SERVER] ==>Tx | BLOCK DOWNLOAD END | x%x:x%x %v", server.Index, server.Subindex, server.CANtxBuff.Data)
			server.BusManager.Send(*server.CANtxBuff)
			server.State = CO_SDO_ST_UPLOAD_BLK_END_CRSP

		default:

		}

	}

	if ret == CO_SDO_RT_waitingResponse {
		switch server.State {
		case CO_SDO_ST_ABORT:
			server.Abort(abortCode)
			server.State = CO_SDO_ST_IDLE
			ret = CO_SDO_RT_endedWithServerAbort
		case CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_REQ:
			ret = CO_SDO_RT_blockDownldInProgress
		case CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_SREQ:
			ret = CO_SDO_RT_blockUploadInProgress
		}
	}
	return ret
}

// Create & send abort on bus
func (server *SDOServer) Abort(abortCode SDOAbortCode) {
	code := uint32(abortCode)
	server.CANtxBuff.Data[0] = 0x80
	server.CANtxBuff.Data[1] = uint8(server.Index)
	server.CANtxBuff.Data[2] = uint8(server.Index >> 8)
	server.CANtxBuff.Data[3] = server.Subindex
	binary.LittleEndian.PutUint32(server.CANtxBuff.Data[4:], code)
	server.BusManager.Send(*server.CANtxBuff)
	log.Warnf("[SERVER] ==>Tx | SERVER ABORT | x%x:x%x | %v (x%x)", server.Index, server.Subindex, abortCode, uint32(abortCode))

}
