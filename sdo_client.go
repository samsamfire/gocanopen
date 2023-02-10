package canopen

import (
	"encoding/binary"
	"time"

	"github.com/brutella/can"
	log "github.com/sirupsen/logrus"
)

/* TODOs
- Locking mechanisms
- string with different size if no null character
- extension management
- block download
- refactor/document
*/

const SDO_CLI_BUFFER_SIZE = 1000
const CO_CONFIG_SDO_CLI_PST = 21

type SDOClient struct {
	OD                         *ObjectDictionary
	Streamer                   *ObjectStreamer
	NodeId                     uint8
	CANModule                  *CANModule
	CANtxBuff                  *BufferTxFrame
	CobIdClientToServer        uint32
	CobIdServerToClient        uint32
	ExtensionEntry1280         *Extension
	NodeIdServer               uint8
	Valid                      bool
	Index                      uint16
	Subindex                   uint8
	Finished                   bool
	SizeIndicated              uint32
	SizeTransferred            uint32
	State                      uint8
	TimeoutTimeUs              uint32
	TimeoutTimer               uint32
	Fifo                       *Fifo
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

const (
	CO_SDO_ST_IDLE                      uint8 = 0x00
	CO_SDO_ST_ABORT                     uint8 = 0x0
	CO_SDO_ST_DOWNLOAD_LOCAL_TRANSFER   uint8 = 0x10
	CO_SDO_ST_DOWNLOAD_INITIATE_REQ     uint8 = 0x11
	CO_SDO_ST_DOWNLOAD_INITIATE_RSP     uint8 = 0x12
	CO_SDO_ST_DOWNLOAD_SEGMENT_REQ      uint8 = 0x13
	CO_SDO_ST_DOWNLOAD_SEGMENT_RSP      uint8 = 0x14
	CO_SDO_ST_UPLOAD_LOCAL_TRANSFER     uint8 = 0x20
	CO_SDO_ST_UPLOAD_INITIATE_REQ       uint8 = 0x21
	CO_SDO_ST_UPLOAD_INITIATE_RSP       uint8 = 0x22
	CO_SDO_ST_UPLOAD_SEGMENT_REQ        uint8 = 0x23
	CO_SDO_ST_UPLOAD_SEGMENT_RSP        uint8 = 0x24
	CO_SDO_ST_DOWNLOAD_BLK_INITIATE_REQ uint8 = 0x51
	CO_SDO_ST_DOWNLOAD_BLK_INITIATE_RSP uint8 = 0x52
	CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_REQ uint8 = 0x53
	CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_RSP uint8 = 0x54
	CO_SDO_ST_DOWNLOAD_BLK_END_REQ      uint8 = 0x55
	CO_SDO_ST_DOWNLOAD_BLK_END_RSP      uint8 = 0x56
	CO_SDO_ST_UPLOAD_BLK_INITIATE_REQ   uint8 = 0x61
	CO_SDO_ST_UPLOAD_BLK_INITIATE_RSP   uint8 = 0x62
	CO_SDO_ST_UPLOAD_BLK_INITIATE_REQ2  uint8 = 0x63
	CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_SREQ  uint8 = 0x64
	CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_CRSP  uint8 = 0x65
	CO_SDO_ST_UPLOAD_BLK_END_SREQ       uint8 = 0x66
	CO_SDO_ST_UPLOAD_BLK_END_CRSP       uint8 = 0x67
)

type SDOReturn int8

const (
	CO_SDO_RT_waitingLocalTransfer  SDOReturn = 6   // Waiting in client local transfer.
	CO_SDO_RT_uploadDataBufferFull  SDOReturn = 5   // Data buffer is full.SDO client: data must be read before next upload cycle begins.
	CO_SDO_RT_transmittBufferFull   SDOReturn = 4   // CAN transmit buffer is full. Waiting.
	CO_SDO_RT_blockDownldInProgress SDOReturn = 3   // Block download is in progress. Sending train of messages.
	CO_SDO_RT_blockUploadInProgress SDOReturn = 2   // Block upload is in progress. Receiving train of messages.SDO client: Data must not be read in this state.
	CO_SDO_RT_waitingResponse       SDOReturn = 1   // Waiting server or client response.
	CO_SDO_RT_ok_communicationEnd   SDOReturn = 0   // Success, end of communication. SDO client: uploaded data must be read.
	CO_SDO_RT_wrongArguments        SDOReturn = -2  // Error in arguments
	CO_SDO_RT_endedWithClientAbort  SDOReturn = -9  // Communication ended with client abort
	CO_SDO_RT_endedWithServerAbort  SDOReturn = -10 // Communication ended with server abort
)

const ()

func (client *SDOClient) Init(od *ObjectDictionary, entry1280 *Entry, nodeId uint8, canmodule *CANModule) error {

	if entry1280.Index < 0x1280 || entry1280.Index > (0x1280+0x7F) {
		log.Errorf("Invalid index for SDO communication parameters %v", entry1280.Index)
		return CO_ERROR_ILLEGAL_ARGUMENT
	}
	if entry1280 == nil || canmodule == nil || od == nil {
		return CO_ERROR_ILLEGAL_ARGUMENT
	}
	client.OD = od
	client.NodeId = nodeId
	client.CANModule = canmodule
	client.Streamer = &ObjectStreamer{}

	fifo := NewFifo(300)
	client.Fifo = fifo

	var maxSubindex, nodeIdServer uint8
	var CobIdClientToServer, CobIdServerToClient uint32
	err1 := entry1280.GetUint8(0, &maxSubindex)
	err2 := entry1280.GetUint32(1, &CobIdClientToServer)
	err3 := entry1280.GetUint32(2, &CobIdServerToClient)
	err4 := entry1280.GetUint8(3, &nodeIdServer)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || maxSubindex != 3 {
		log.Errorf("Invalid parameters inside SDO client entry. [0: %v,1: %v, 2: %v, 3: %v]. Max sub index : %v", err1, err2, err3, err4, maxSubindex)
		return CO_ERROR_OD_PARAMETERS
	}

	// TODO Initialize extension

	client.CobIdClientToServer = 0
	client.CobIdServerToClient = 0

	sdoReturn := client.Setup(CobIdClientToServer, CobIdServerToClient, nodeIdServer)
	if sdoReturn != CO_SDO_RT_ok_communicationEnd {
		return CO_ERROR_ILLEGAL_ARGUMENT
	}
	return nil

}

// Setup the client for a new communication
func (client *SDOClient) Setup(cobIdClientToServer uint32, cobIdServerToClient uint32, nodeIdServer uint8) SDOReturn {
	client.State = CO_SDO_ST_IDLE
	client.RxNew = false
	client.NodeIdServer = nodeIdServer
	// If server is the same don't re-initialize the buffers
	if client.CobIdClientToServer == cobIdClientToServer && client.CobIdServerToClient == cobIdServerToClient {
		return CO_SDO_RT_ok_communicationEnd
	}
	client.CobIdClientToServer = cobIdClientToServer
	client.CobIdServerToClient = cobIdServerToClient
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
		client.Valid = true
	} else {
		CanIdC2S = 0
		CanIdS2C = 0
		client.Valid = false
	}
	_, err1 := client.CANModule.InsertRxBuffer(uint32(CanIdS2C), 0x7FF, false, client)
	if err1 != nil {
		log.Errorf("Error initializing RX buffer for SDO client %v", err1)
		client.Valid = false
	}
	var err2 error
	client.CANtxBuff, _, err2 = client.CANModule.InsertTxBuffer(uint32(CanIdC2S), false, 8, false)
	if err2 != nil {
		log.Errorf("Error initializing TX buffer for SDO client %v", err2)
		client.Valid = false
	}
	if err1 != nil || err2 != nil {
		return CO_SDO_RT_wrongArguments
	}

	return CO_SDO_RT_ok_communicationEnd

}

func (client *SDOClient) Handle(frame can.Frame) {

	if client.State != CO_SDO_ST_IDLE && frame.Length == 8 && (!client.RxNew || frame.Data[0] == 0x80) {
		if frame.Data[0] == 0x80 || (client.State != CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_SREQ && client.State != CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_CRSP) {
			// Copy data in response
			client.Response.raw = frame.Data
			client.RxNew = true
		} else if client.State == CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_SREQ {
			state := CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_SREQ
			seqno := frame.Data[0] & 0x7F
			client.TimeoutTimer = 0
			client.TimeoutTimerBlock = 0
			// Checks on the Sequence number
			if seqno <= client.BlockSize && seqno == (client.BlockSequenceNb+1) {
				client.BlockSequenceNb = seqno
				// Is it last segment
				if (frame.Data[0] & 0x80) != 0 {
					copy(client.BlockDataUploadLast[:], frame.Data[1:])
					client.Finished = true
					state = CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_CRSP
				} else {
					client.Fifo.Write(frame.Data[1:], &client.BlockCRC)
					client.SizeTransferred += 7
					if seqno == client.BlockSize {
						log.Debugf("<==Rx (x%x) | BLOCK UPLOAD | Last sub-block received (%v)", client.NodeIdServer, seqno)
						state = CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_CRSP
					}
				}
			} else if seqno != client.BlockSequenceNb && client.BlockSequenceNb != 0 {
				state = CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_CRSP
				log.Warnf("Wrong sequence number in rx sub-block. seqno %x, previous %x", seqno, client.BlockSequenceNb)
			} else {
				log.Warnf("Wrong sequence number in rx ignored. seqno %x, expected %x", seqno, client.BlockSequenceNb+1)
			}

			if state != CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_SREQ {
				client.RxNew = false
				client.State = state

			}

		}
	} else {
		log.Debugf("Ignoring response x%x %v; Client state : x%x", frame.ID, frame.Data, client.State)
	}

}

// Start a new download sequence
func (client *SDOClient) DownloadInitiate(index uint16, subindex uint8, sizeIndicated uint32, timeoutMs uint16, blockEnabled bool) SDOReturn {
	if !client.Valid {
		return CO_SDO_RT_wrongArguments
	}
	client.Index = index
	client.Subindex = subindex
	client.SizeIndicated = sizeIndicated
	client.SizeTransferred = 0
	client.Finished = false
	client.TimeoutTimeUs = uint32(timeoutMs) * 1000
	client.TimeoutTimer = 0
	client.Fifo.Reset()

	if client.OD != nil && client.NodeId != 0 && client.NodeIdServer == client.NodeId {
		client.Streamer.Write = nil
		client.State = CO_SDO_ST_DOWNLOAD_LOCAL_TRANSFER
	} else if blockEnabled && (sizeIndicated == 0 || sizeIndicated > CO_CONFIG_SDO_CLI_PST) {
		client.State = CO_SDO_ST_DOWNLOAD_BLK_INITIATE_REQ
	} else {
		client.State = CO_SDO_ST_DOWNLOAD_INITIATE_REQ
	}
	client.RxNew = false
	return CO_SDO_RT_ok_communicationEnd
}

func (client *SDOClient) DownloadInitiateSize(sizeIndicated uint32) {
	client.SizeIndicated = sizeIndicated
	if client.State == CO_SDO_ST_DOWNLOAD_BLK_INITIATE_REQ && sizeIndicated > 0 && sizeIndicated <= CO_CONFIG_SDO_CLI_PST {
		client.State = CO_SDO_ST_DOWNLOAD_INITIATE_REQ
	}
}

func (client *SDOClient) DownloadBufWrite(buffer []byte) int {
	if buffer == nil {
		return 0
	}
	return client.Fifo.Write(buffer, nil)
}

// Write value to OD locally
// func (client *SDOClient) DownloadLocal(bufferPartial bool, timerNextUs *uint32) {
// 	ret := CO_SDO_RT_waitingResponse
// 	var abortCode SDOAbortCode = CO_SDO_AB_NONE

// 	if client.Streamer.Write == nil {
// 		log.Info("Downloading via local transfer")
// 		// Get the object on first function call
// 		err := client.OD.Find(client.Index).Sub(client.Subindex, false, client.Streamer)
// 		odr_err, _ := err.(ODR)
// 		if err != nil {
// 			abortCode = odr_err.GetSDOAbordCode()
// 			ret = CO_SDO_RT_endedWithClientAbort
// 		} else if (client.Streamer.Stream.Attribute & ODA_SDO_RW) == 0 {
// 			abortCode = CO_SDO_AB_UNSUPPORTED_ACCESS
// 			ret = CO_SDO_RT_endedWithClientAbort
// 		} else if (client.Streamer.Stream.Attribute & ODA_SDO_W) == 0 {
// 			abortCode = CO_SDO_AB_READONLY
// 			ret = CO_SDO_RT_endedWithClientAbort
// 		} else if client.Streamer.Write == nil {
// 			abortCode = CO_SDO_AB_DEVICE_INCOMPAT
// 			ret = CO_SDO_RT_endedWithClientAbort
// 		}
// 	} else {
// 		// Do the real stuff
// 		buffer := client.Queue
// 		count := len(buffer)
// 		client.SizeTransferred += uint32(count)
// 		// No data error
// 		if count == 0 {
// 			abortCode = CO_SDO_AB_DEVICE_INCOMPAT
// 			ret = CO_SDO_RT_endedWithClientAbort
// 			// Size transferred is too large
// 		} else if client.SizeIndicated > 0 && client.SizeTransferred > client.SizeIndicated {
// 			client.SizeTransferred -= uint32(count)
// 			abortCode = CO_SDO_AB_DATA_LONG
// 			ret = CO_SDO_RT_endedWithClientAbort
// 			// Size transferred is too small (check on last call)
// 		} else if !bufferPartial && client.SizeIndicated > 0 && client.SizeTransferred < client.SizeIndicated {
// 			abortCode = CO_SDO_AB_DATA_SHORT
// 			ret = CO_SDO_RT_endedWithClientAbort
// 			// Last part of data !
// 		} else if !bufferPartial {
// 			odVarSize := len(client.Streamer.Stream.Data)
// 			// Special case for strings where the downloaded data may be shorter (nul character can be omitted)
// 			// TODO, finish this because unclear with how go stores strings
// 			if client.Streamer.Stream.Attribute&ODA_STR != 0 && odVarSize == 0 || client.SizeTransferred < uint32(odVarSize) {
// 				buffer = append(buffer, byte(0))
// 				client.SizeTransferred += 1
// 				if odVarSize == 0 || odVarSize > int(client.SizeTransferred) {
// 					buffer = append(buffer, byte(0))
// 				} else if odVarSize == 0 {
// 					log.Warn("odvarsize 0 case not handled for now")
// 					abortCode = CO_SDO_AB_DEVICE_INCOMPAT
// 					ret = CO_SDO_RT_endedWithClientAbort

// 				} else if client.SizeTransferred > uint32(odVarSize) {
// 					abortCode = CO_SDO_AB_DATA_LONG
// 					ret = CO_SDO_RT_endedWithClientAbort
// 				} else if client.SizeTransferred < uint32(odVarSize) {
// 					abortCode = CO_SDO_AB_DATA_SHORT
// 					ret = CO_SDO_RT_endedWithClientAbort
// 				}
// 			}
// 		}

// 		if abortCode == CO_SDO_AB_NONE {
// 			var countWritten uint16 = 0
// 			// TODO l
// 			//lock := client.Streamer.Stream.Mappable()
// 			err := client.Streamer.Write(client.Streamer.Stream, buffer, &countWritten)
// 			odr_err, _ := err.(ODR)
// 			// Check errors when writing
// 			if err != nil && odr_err != ODR_PARTIAL {
// 				abortCode = odr_err.GetSDOAbordCode()
// 				ret = CO_SDO_RT_endedWithClientAbort
// 				// Error if written completely but download still has data
// 			} else if bufferPartial && err == nil {
// 				abortCode = CO_SDO_AB_DATA_LONG
// 				ret = CO_SDO_RT_endedWithClientAbort
// 			} else if !bufferPartial {
// 				// Error if not written completely but download end
// 				if odr_err == ODR_PARTIAL {
// 					abortCode = CO_SDO_AB_DATA_SHORT
// 					ret = CO_SDO_RT_endedWithClientAbort
// 				} else {
// 					ret = CO_SDO_RT_ok_communicationEnd
// 				}
// 			} else {
// 				ret = CO_SDO_RT_waitingLocalTransfer
// 			}

// 		}

// 		if ret != CO_SDO_RT_waitingLocalTransfer {
// 			client.State = CO_SDO_ST_IDLE
// 		} else if timerNextUs != nil {
// 			*timerNextUs = 0
// 		}
// 	}

// }

func (client *SDOClient) Download(timeDifferenceUs uint32, abort bool, bufferPartial bool, sdoAbortCode *SDOAbortCode, sizeTransferred *uint32, timerNextUs *uint32, forceSegmented *bool) SDOReturn {

	ret := CO_SDO_RT_waitingResponse
	var abortCode error

	if !client.Valid {
		abortCode = CO_SDO_AB_DEVICE_INCOMPAT
		ret = CO_SDO_RT_wrongArguments
	} else if client.State == CO_SDO_ST_IDLE {
		ret = CO_SDO_RT_ok_communicationEnd
	} else if client.State == CO_SDO_ST_DOWNLOAD_LOCAL_TRANSFER && !abort {
		log.Info("Downloading via local transfer")
		// TODO Add Download Local function O

	} else if client.RxNew {
		response := client.Response
		if response.IsAbort() {
			abortCode = response.GetAbortCode()
			log.Debugf("<==Rx (x%x) | SERVER ABORT | %v (x%x)", client.NodeIdServer, abortCode, abortCode)
			client.State = CO_SDO_ST_IDLE
			ret = CO_SDO_RT_endedWithServerAbort
			// Abort from the client
		} else if abort {
			if sdoAbortCode == nil {
				abortCode = CO_SDO_AB_DEVICE_INCOMPAT
			} else {
				abortCode = *sdoAbortCode
			}
			log.Warnf("Client is aborting : %x", abortCode)
			client.State = CO_SDO_ST_ABORT

		} else if !response.isResponseValid(client.State) {
			log.Warnf("Unexpected response code from server : %x", response.raw[0])
			client.State = CO_SDO_ST_ABORT
			abortCode = CO_SDO_AB_CMD

		} else {
			switch client.State {
			case CO_SDO_ST_DOWNLOAD_INITIATE_RSP:

				index := response.GetIndex()
				subIndex := response.GetSubindex()
				if index != client.Index || subIndex != client.Subindex {
					abortCode = CO_SDO_AB_PRAM_INCOMPAT
					client.State = CO_SDO_ST_ABORT
					break
				}
				// Expedited transfer
				if client.Finished {
					client.State = CO_SDO_ST_IDLE
					ret = CO_SDO_RT_ok_communicationEnd
					log.Debugf("<==Rx (x%x) | DOWNLOAD EXPEDITED | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, response.raw)
					// Segmented transfer
				} else {
					client.Toggle = 0x00
					client.State = CO_SDO_ST_DOWNLOAD_SEGMENT_REQ
					log.Debugf("<==Rx (x%x) | DOWNLOAD SEGMENT | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, response.raw)
				}

			case CO_SDO_ST_DOWNLOAD_SEGMENT_RSP:

				// Verify and alternate toggle bit
				toggle := response.GetToggle()
				if toggle != client.Toggle {
					abortCode = CO_SDO_AB_TOGGLE_BIT
					client.State = CO_SDO_ST_ABORT
					break
				}
				client.Toggle ^= 0x10
				if client.Finished {
					client.State = CO_SDO_ST_IDLE
					ret = CO_SDO_RT_ok_communicationEnd
				} else {
					client.State = CO_SDO_ST_DOWNLOAD_SEGMENT_REQ
				}
				log.Debugf("<==Rx (x%x) | DOWNLOAD SEGMENT | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, response.raw)

			case CO_SDO_ST_DOWNLOAD_BLK_INITIATE_RSP:

				index := response.GetIndex()
				subIndex := response.GetSubindex()
				if index != client.Index || subIndex != client.Subindex {
					abortCode = CO_SDO_AB_PRAM_INCOMPAT
					client.State = CO_SDO_ST_ABORT
					break
				}
				client.BlockCRC = CRC16{0}
				client.BlockSize = response.GetBlockSize()
				if client.BlockSize < 1 || client.BlockSize > 127 {
					client.BlockSize = 127
				}
				client.BlockSequenceNb = 0
				client.Fifo.AltBegin(0)
				client.State = CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_REQ

			case CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_REQ, CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_RSP:

				if response.GetNumberOfSegments() < client.BlockSequenceNb {
					log.Error("Not all segments transferred successfully")
					client.Fifo.AltBegin(int(response.raw[1]) * 7)
					client.Finished = false

				} else if response.GetNumberOfSegments() > client.BlockSequenceNb {
					abortCode = CO_SDO_AB_CMD
					client.State = CO_SDO_ST_ABORT
					break
				}
				// TODO alt finish
				client.Fifo.AltFinish(&client.BlockCRC)

				if client.Finished {
					client.State = CO_SDO_ST_DOWNLOAD_BLK_END_REQ
				} else {
					client.BlockSize = response.raw[2]
					client.BlockSequenceNb = 0
					client.Fifo.AltBegin(0)
					client.State = CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_REQ
				}

			case CO_SDO_ST_DOWNLOAD_BLK_END_RSP:

				client.State = CO_SDO_ST_IDLE
				ret = CO_SDO_RT_ok_communicationEnd

			}

			client.TimeoutTimer = 0
			timeDifferenceUs = 0
			client.RxNew = false

		}

	} else if abort {
		if sdoAbortCode == nil {
			abortCode = CO_SDO_AB_DEVICE_INCOMPAT
		} else {
			abortCode = *sdoAbortCode
		}
		log.Warnf("Client is aborting : %x", abortCode)
		client.State = CO_SDO_ST_ABORT
	}

	if ret == CO_SDO_RT_waitingResponse {
		if client.TimeoutTimer < client.TimeoutTimeUs {
			client.TimeoutTimer += timeDifferenceUs
		}
		if client.TimeoutTimer >= client.TimeoutTimeUs {
			abortCode = CO_SDO_AB_TIMEOUT
			client.State = CO_SDO_ST_ABORT
		} else if timerNextUs != nil {
			diff := client.TimeoutTimeUs - client.TimeoutTimer
			if *timerNextUs > diff {
				*timerNextUs = diff
			}
		}
		if client.CANtxBuff.BufferFull {
			ret = CO_SDO_RT_transmittBufferFull
		}
	}

	if ret == CO_SDO_RT_waitingResponse {

		client.CANtxBuff.Data = [8]byte{0}
		switch client.State {
		case CO_SDO_ST_DOWNLOAD_INITIATE_REQ:
			if forceSegmented == nil {
				abortCode = client.InitiateDownload(false)
			} else {
				abortCode = client.InitiateDownload(*forceSegmented)
			}
			if abortCode != nil {
				client.State = CO_SDO_ST_IDLE
				ret = CO_SDO_RT_endedWithClientAbort
				break
			}
			client.State = CO_SDO_ST_DOWNLOAD_INITIATE_RSP

		case CO_SDO_ST_DOWNLOAD_SEGMENT_REQ:
			// Fill data part
			abortCode = client.DownloadSegmented(bufferPartial)
			if abortCode != nil {
				client.State = CO_SDO_ST_ABORT
				break
			}
			client.State = CO_SDO_ST_DOWNLOAD_SEGMENT_RSP
		default:
			break

		}

		// case CO_SDO_ST_DOWNLOAD_BLK_INITIATE_REQ:
		// 	// TODO
		// case CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_REQ:
		// 	// TODO

		// case CO_SDO_ST_DOWNLOAD_BLK_END_REQ:
		// 	// TODO

	}

	if ret == CO_SDO_RT_waitingResponse {

		if client.State == CO_SDO_ST_ABORT {
			client.Abort(abortCode.(SDOAbortCode))
			ret = CO_SDO_RT_endedWithClientAbort
			client.State = CO_SDO_ST_IDLE

		} else if client.State == CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_REQ {
			ret = CO_SDO_RT_blockDownldInProgress
		}
	}

	if sizeTransferred != nil {
		*sizeTransferred = client.SizeTransferred
	}

	if sdoAbortCode != nil && abortCode != nil {
		*sdoAbortCode = abortCode.(SDOAbortCode)
	}

	return ret
}

func (client *SDOClient) WriteRaw(nodeId uint8, index uint16, subindex uint8, data []byte, forceSegmented bool) error {
	bufferPartial := false
	ret := client.Setup(uint32(SDO_CLIENT_ID)+uint32(nodeId), uint32(SDO_SERVER_ID)+uint32(nodeId), nodeId)
	if ret != CO_SDO_RT_ok_communicationEnd {
		log.Errorf("Error when setting up SDO client reason : %v", ret)
		return CO_SDO_AB_GENERAL
	}
	ret = client.DownloadInitiate(index, subindex, uint32(len(data)), 1000, false)
	if ret < 0 {
		log.Errorf("Failed to initiate SDO client : %v", ret)
	}

	// Fill buffer
	nWritten := client.DownloadBufWrite(data)
	if nWritten < len(data) {
		bufferPartial = true
		log.Info("Not enough space in buffer so using buffer partial")
	}
	var timeDifferenceUs uint32 = 10000
	abortCode := CO_SDO_AB_NONE

	for {
		ret = client.Download(timeDifferenceUs, false, bufferPartial, &abortCode, nil, nil, &forceSegmented)
		if ret < 0 {
			log.Errorf("SDO write failed : %v", ret)
			return CO_SDO_AB_GENERAL
		} else if uint8(ret) == 0 {
			break
		}
		time.Sleep(time.Duration(timeDifferenceUs) * time.Microsecond)
	}

	return CO_SDO_AB_NONE
}

// Helpers functions for different SDO messages
// Valid for both expedited and segmented
func (client *SDOClient) InitiateDownload(forceSegmented bool) error {

	client.CANtxBuff.Data[0] = 0x20
	client.CANtxBuff.Data[1] = byte(client.Index)
	client.CANtxBuff.Data[2] = byte(client.Index >> 8)
	client.CANtxBuff.Data[3] = client.Subindex

	count := uint32(client.Fifo.GetOccupied())
	if (client.SizeIndicated == 0 && count <= 4) || (client.SizeIndicated > 0 && client.SizeIndicated <= 4) && !forceSegmented {
		client.CANtxBuff.Data[0] |= 0x02
		// Check length
		if count == 0 || (client.SizeIndicated > 0 && client.SizeIndicated != count) {
			client.State = CO_SDO_ST_IDLE
			return CO_SDO_AB_TYPE_MISMATCH
		}
		if client.SizeIndicated > 0 {
			client.CANtxBuff.Data[0] |= byte(0x01 | ((4 - count) << 2))
		}
		// Copy the data in queue and add the count
		count = uint32(client.Fifo.Read(client.CANtxBuff.Data[4:], nil))
		client.SizeTransferred = count
		client.Finished = true
		log.Debugf("==>Tx (x%x) | DOWNLOAD EXPEDITED | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, client.CANtxBuff.Data)

	} else {
		/* segmented transfer, indicate data size */
		if client.SizeIndicated > 0 {
			size := client.SizeIndicated
			client.CANtxBuff.Data[0] |= 0x01
			binary.LittleEndian.PutUint32(client.CANtxBuff.Data[4:], size)
		}
		log.Debugf("==>Tx (x%x) | DOWNLOAD SEGMENT | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, client.CANtxBuff.Data)
	}
	client.TimeoutTimer = 0
	client.CANModule.Bus.Send(*client.CANtxBuff)
	return nil

}

// Called for each segment
func (client *SDOClient) DownloadSegmented(bufferPartial bool) error {
	// Fill data part
	count := uint32(client.Fifo.Read(client.CANtxBuff.Data[1:], nil))
	client.SizeTransferred += count
	if client.SizeIndicated > 0 && client.SizeTransferred > client.SizeIndicated {
		client.SizeTransferred -= count
		return CO_SDO_AB_DATA_LONG
	}

	//Command specifier
	client.CANtxBuff.Data[0] = uint8(uint32(client.Toggle) | ((7 - count) << 1))
	if client.Fifo.GetOccupied() == 0 && !bufferPartial {
		if client.SizeIndicated > 0 && client.SizeTransferred < client.SizeIndicated {
			return CO_SDO_AB_DATA_SHORT
		}
		client.CANtxBuff.Data[0] |= 0x01
		client.Finished = true
	}

	client.TimeoutTimer = 0
	log.Debugf("==>Tx (x%x) | DOWNLOAD SEGMENT | x%x:x%x %v | %v%%", client.NodeIdServer, client.Index, client.Subindex, client.CANtxBuff.Data, ((float64(client.SizeTransferred) / float64(client.SizeIndicated)) * 100))
	client.CANModule.Send(*client.CANtxBuff)
	return nil
}

// Create & send abort on bus
func (client *SDOClient) Abort(abortCode SDOAbortCode) {
	code := uint32(abortCode)
	client.CANtxBuff.Data[0] = 0x80
	client.CANtxBuff.Data[1] = uint8(client.Index)
	client.CANtxBuff.Data[2] = uint8(client.Index >> 8)
	client.CANtxBuff.Data[3] = client.Subindex
	binary.LittleEndian.PutUint32(client.CANtxBuff.Data[4:], code)
	log.Warnf("[CLIENT]==>Tx (x%x) | CLIENT ABORT | %v (x%x)", client.NodeIdServer, abortCode, abortCode)
	client.CANModule.Send(*client.CANtxBuff)

}

/////////////////////////////////////
////////////SDO UPLOAD///////////////
/////////////////////////////////////

func (client *SDOClient) UploadInitiate(index uint16, subindex uint8, timeoutTimeMs uint16, blockEnabled bool) SDOReturn {
	if !client.Valid {
		return CO_SDO_RT_wrongArguments
	}
	client.Index = index
	client.Subindex = subindex
	client.SizeIndicated = 0
	client.SizeTransferred = 0
	client.Finished = false
	client.Fifo.Reset()
	client.TimeoutTimeUs = uint32(timeoutTimeMs) * 1000
	client.TimeoutTimeBlockTransferUs = uint32(timeoutTimeMs) * 1000
	if client.OD != nil && client.NodeId != 0 && client.NodeIdServer == client.NodeId {
		client.Streamer.Read = nil
		client.State = CO_SDO_ST_UPLOAD_LOCAL_TRANSFER
	} else if blockEnabled {
		client.State = CO_SDO_ST_UPLOAD_BLK_INITIATE_REQ
	} else {
		client.State = CO_SDO_ST_UPLOAD_INITIATE_REQ
	}
	client.RxNew = false
	return CO_SDO_RT_ok_communicationEnd
}

// Main state machine
func (client *SDOClient) Upload(timeDifferenceUs uint32, abort bool, sdoAbortCode *SDOAbortCode, sizeIndicated *uint32, sizeTransferred *uint32, timerNextUs *uint32) SDOReturn {

	ret := CO_SDO_RT_waitingResponse
	var abortCode error

	if !client.Valid {
		abortCode = CO_SDO_AB_DEVICE_INCOMPAT
		ret = CO_SDO_RT_wrongArguments
	} else if client.State == CO_SDO_ST_IDLE {
		ret = CO_SDO_RT_ok_communicationEnd
	} else if client.State == CO_SDO_ST_UPLOAD_LOCAL_TRANSFER && !abort {
		// TODO
	} else if client.RxNew {
		response := client.Response
		if response.IsAbort() {
			abortCode = response.GetAbortCode()
			log.Debugf("<==Rx (x%x) | SERVER ABORT | %v (x%x)", client.NodeIdServer, abortCode, uint32(abortCode.(SDOAbortCode)))
			client.State = CO_SDO_ST_IDLE
			ret = CO_SDO_RT_endedWithServerAbort
			// Abort from the client
		} else if abort {
			if sdoAbortCode == nil {
				abortCode = CO_SDO_AB_DEVICE_INCOMPAT
			} else {
				abortCode = *sdoAbortCode
			}
			log.Warnf("Client is aborting : %x", abortCode)
			client.State = CO_SDO_ST_ABORT

		} else if !response.isResponseValid(client.State) {
			log.Warnf("Unexpected response code from server : %x", response.raw[0])
			client.State = CO_SDO_ST_ABORT
			abortCode = CO_SDO_AB_CMD

		} else {
			switch client.State {
			case CO_SDO_ST_UPLOAD_INITIATE_RSP:
				index := response.GetIndex()
				subIndex := response.GetSubindex()
				if index != client.Index || subIndex != client.Subindex {
					abortCode = CO_SDO_AB_PRAM_INCOMPAT
					client.State = CO_SDO_ST_ABORT
					break
				}
				if (response.raw[0] & 0x02) != 0 {
					//Expedited
					var count uint32 = 4
					// Size indicated ?
					if (response.raw[0] & 0x01) != 0 {
						count -= uint32((response.raw[0] >> 2) & 0x03)
					}
					client.Fifo.Write(response.raw[4:4+count], nil)
					client.SizeTransferred = count
					client.State = CO_SDO_ST_IDLE
					ret = CO_SDO_RT_ok_communicationEnd
					log.Debugf("<==Rx (x%x) | UPLOAD EXPEDITED | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, response.raw)
					// Segmented
				} else {
					// Size indicated ?
					if (response.raw[0] & 0x01) != 0 {
						client.SizeIndicated = binary.LittleEndian.Uint32(response.raw[4:])
					}
					client.Toggle = 0
					client.State = CO_SDO_ST_UPLOAD_SEGMENT_REQ
					log.Debugf("<==Rx (x%x) | UPLOAD SEGMENT | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, response.raw)

				}

			case CO_SDO_ST_UPLOAD_SEGMENT_RSP:
				// Verify and alternate toggle bit
				log.Debugf("<==Rx (x%x) | UPLOAD SEGMENT | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, response.raw)
				toggle := response.GetToggle()
				if toggle != client.Toggle {
					abortCode = CO_SDO_AB_TOGGLE_BIT
					client.State = CO_SDO_ST_ABORT
					break
				}
				client.Toggle ^= 0x10
				count := 7 - (response.raw[0]>>1)&0x07
				countWr := client.Fifo.Write(response.raw[1:1+count], nil)
				client.SizeTransferred += uint32(countWr)
				// Check enough space if fifo
				if countWr != int(count) {
					abortCode = CO_SDO_AB_OUT_OF_MEM
					client.State = CO_SDO_ST_ABORT
					break
				}

				//Check size uploaded
				if client.SizeIndicated > 0 && client.SizeTransferred > client.SizeIndicated {
					abortCode = CO_SDO_AB_DATA_LONG
					client.State = CO_SDO_ST_ABORT
					break
				}

				//No more segments ?
				if (response.raw[0] & 0x01) != 0 {
					// Check size uploaded
					if client.SizeIndicated > 0 && client.SizeTransferred < client.SizeIndicated {
						abortCode = CO_SDO_AB_DATA_LONG
						client.State = CO_SDO_ST_ABORT
					} else {
						client.State = CO_SDO_ST_IDLE
						ret = CO_SDO_RT_ok_communicationEnd
					}
				} else {
					client.State = CO_SDO_ST_UPLOAD_SEGMENT_REQ
				}

			case CO_SDO_ST_UPLOAD_BLK_INITIATE_RSP:

				index := response.GetIndex()
				subindex := response.GetSubindex()
				if index != client.Index || subindex != client.Subindex {
					abortCode = CO_SDO_AB_PRAM_INCOMPAT
					client.State = CO_SDO_ST_ABORT
					break
				}
				// Block is supported
				if (response.raw[0] & 0xF9) == 0xC0 {
					client.BlockCRCEnabled = response.IsCRCEnabled()
					if (response.raw[0] & 0x02) != 0 {
						client.SizeIndicated = uint32(response.GetBlockSize())
					}
					client.State = CO_SDO_ST_UPLOAD_BLK_INITIATE_REQ2
					log.Debugf("<==Rx (x%x) | BLOCK UPLOAD (CRC : %v) | x%x:x%x %v", client.NodeIdServer, response.IsCRCEnabled(), client.Index, client.Subindex, response.raw)

					//Switch to normal transfer
				} else if (response.raw[0] & 0xF0) == 0x40 {
					if (response.raw[0] & 0x02) != 0 {
						//Expedited
						count := 4
						if (response.raw[0] & 0x01) != 0 {
							count -= (int(response.raw[0]>>2) & 0x03)
						}
						client.Fifo.Write(response.raw[4:4+count], nil)
						client.SizeTransferred = uint32(count)
						client.State = CO_SDO_ST_IDLE
						ret = CO_SDO_RT_ok_communicationEnd
						log.Debugf("<==Rx (x%x) | BLOCK UPLOAD SWITCHING to EXPEDITED | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, response.raw)

					} else {
						if (response.raw[0] & 0x01) != 0 {
							client.SizeIndicated = uint32(response.GetBlockSize())
						}
						client.Toggle = 0x00
						client.State = CO_SDO_ST_UPLOAD_SEGMENT_REQ
						log.Debugf("<==Rx (x%x) | BLOCK UPLOAD SWITCHING to SEGMENTED | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, response.raw)
					}

				}
			case CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_SREQ:
				// Handled directly in Rx callback
				break

			case CO_SDO_ST_UPLOAD_BLK_END_SREQ:
				//Get number of data bytes in last segment, that do not
				//contain data. Then copy remaining data into fifo
				noData := (response.raw[0] >> 2) & 0x07
				client.Fifo.Write(client.BlockDataUploadLast[:7-noData], &client.BlockCRC)
				client.SizeTransferred += uint32(7 - noData)

				if client.SizeIndicated > 0 && client.SizeTransferred > client.SizeIndicated {
					abortCode = CO_SDO_AB_DATA_LONG
					client.State = CO_SDO_ST_ABORT
					break
				} else if client.SizeIndicated > 0 && client.SizeTransferred < client.SizeIndicated {
					abortCode = CO_SDO_AB_DATA_SHORT
					client.State = CO_SDO_ST_ABORT
					break
				}
				if client.BlockCRCEnabled {
					crcServer := binary.LittleEndian.Uint16(response.raw[1:3])
					if crcServer != client.BlockCRC.crc {
						abortCode = CO_SDO_AB_CRC
						client.State = CO_SDO_ST_ABORT
						break
					}
				}
				client.State = CO_SDO_ST_UPLOAD_BLK_END_CRSP
				log.Debugf("<==Rx (x%x) | BLOCK UPLOAD END | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, response.raw)

			default:
				abortCode = CO_SDO_AB_CMD
				client.State = CO_SDO_ST_ABORT
			}

		}
		client.TimeoutTimer = 0
		timeDifferenceUs = 0
		client.RxNew = false
	} else if abort {
		if sdoAbortCode == nil {
			abortCode = CO_SDO_AB_DEVICE_INCOMPAT
		} else {
			abortCode = *sdoAbortCode
		}
		client.State = CO_SDO_ST_ABORT
	}

	if ret == CO_SDO_RT_waitingResponse {
		if client.TimeoutTimer < client.TimeoutTimeUs {
			client.TimeoutTimer += timeDifferenceUs
		}
		if client.TimeoutTimer >= client.TimeoutTimeUs {
			if client.State == CO_SDO_ST_UPLOAD_SEGMENT_REQ || client.State == CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_CRSP {
				abortCode = CO_SDO_AB_GENERAL
			} else {
				abortCode = CO_SDO_AB_TIMEOUT
			}
			client.State = CO_SDO_ST_ABORT

		} else if timerNextUs != nil {
			diff := client.TimeoutTimeUs - client.TimeoutTimer
			if *timerNextUs > diff {
				*timerNextUs = diff
			}
		}
		// Timeout for subblocks
		if client.State == CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_SREQ {
			if client.TimeoutTimerBlock < client.TimeoutTimeBlockTransferUs {
				client.TimeoutTimerBlock += timeDifferenceUs
			}
			if client.TimeoutTimerBlock >= client.TimeoutTimeBlockTransferUs {
				client.State = CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_CRSP
				client.RxNew = false
			} else if timerNextUs != nil {
				diff := client.TimeoutTimeBlockTransferUs - client.TimeoutTimerBlock
				if *timerNextUs > diff {
					*timerNextUs = diff
				}
			}
		}
		if client.CANtxBuff.BufferFull {
			ret = CO_SDO_RT_transmittBufferFull
		}
	}

	if ret == CO_SDO_RT_waitingResponse {
		client.CANtxBuff.Data = [8]byte{0}
		switch client.State {
		case CO_SDO_ST_UPLOAD_INITIATE_REQ:
			client.CANtxBuff.Data[0] = 0x40
			client.CANtxBuff.Data[1] = byte(client.Index)
			client.CANtxBuff.Data[2] = byte(client.Index >> 8)
			client.CANtxBuff.Data[3] = client.Subindex
			client.TimeoutTimer = 0
			client.CANModule.Send(*client.CANtxBuff)
			client.State = CO_SDO_ST_UPLOAD_INITIATE_RSP
			log.Debugf("==>Tx (x%x) | UPLOAD SEGMENT | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, client.CANtxBuff.Data)

		case CO_SDO_ST_UPLOAD_SEGMENT_REQ:
			if client.Fifo.GetSpace() < 7 {
				ret = CO_SDO_RT_uploadDataBufferFull
				break
			}
			client.CANtxBuff.Data[0] = 0x60 | client.Toggle
			client.TimeoutTimer = 0
			client.CANModule.Send(*client.CANtxBuff)
			client.State = CO_SDO_ST_UPLOAD_SEGMENT_RSP
			log.Debugf("==>Tx (x%x) | UPLOAD SEGMENT | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, client.CANtxBuff.Data)

		case CO_SDO_ST_UPLOAD_BLK_INITIATE_REQ:
			client.CANtxBuff.Data[0] = 0xA4
			client.CANtxBuff.Data[1] = byte(client.Index)
			client.CANtxBuff.Data[2] = byte(client.Index >> 8)
			client.CANtxBuff.Data[3] = client.Subindex
			// Calculate number of block segments from free space
			count := client.Fifo.GetSpace() / 7
			if count >= 127 {
				count = 127
			} else if count == 0 {
				abortCode = CO_SDO_AB_OUT_OF_MEM
				client.State = CO_SDO_ST_ABORT
				break
			}
			client.BlockSize = uint8(count)
			client.CANtxBuff.Data[4] = client.BlockSize
			client.CANtxBuff.Data[5] = CO_CONFIG_SDO_CLI_PST
			client.TimeoutTimer = 0
			client.CANModule.Send(*client.CANtxBuff)
			client.State = CO_SDO_ST_UPLOAD_BLK_INITIATE_RSP
			log.Debugf("==>Tx (x%x) | BLOCK UPLOAD INITIATE | x%x:x%x %v blksize : %v", client.NodeIdServer, client.Index, client.Subindex, client.CANtxBuff.Data, client.BlockSize)

		case CO_SDO_ST_UPLOAD_BLK_INITIATE_REQ2:
			client.CANtxBuff.Data[0] = 0xA3
			client.TimeoutTimer = 0
			client.TimeoutTimerBlock = 0
			client.BlockSequenceNb = 0
			client.BlockCRC = CRC16{0}
			client.State = CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_SREQ
			client.RxNew = false
			client.CANModule.Send(*client.CANtxBuff)

		case CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_CRSP:
			client.CANtxBuff.Data[0] = 0xA2
			client.CANtxBuff.Data[1] = client.BlockSequenceNb
			transferShort := client.BlockSequenceNb != client.BlockSize
			seqnoStart := client.BlockSequenceNb
			if client.Finished {
				client.State = CO_SDO_ST_UPLOAD_BLK_END_SREQ
			} else {
				// Check size too large
				if client.SizeIndicated > 0 && client.SizeTransferred > client.SizeIndicated {
					abortCode = CO_SDO_AB_DATA_LONG
					client.State = CO_SDO_ST_ABORT
					break
				}
				// Calculate number of block segments from free space
				count := client.Fifo.GetSpace() / 7
				if count >= 127 {
					count = 127

				} else if client.Fifo.GetOccupied() > 0 {
					ret = CO_SDO_RT_uploadDataBufferFull
					log.Warnf("Fifo is full")
					if transferShort {
						log.Warnf("sub-block , upload data is full seqno=%v", seqnoStart)
					}
					if timerNextUs != nil {
						*timerNextUs = 0
					}
					break
				}
				client.BlockSize = uint8(count)
				client.BlockSequenceNb = 0
				client.State = CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_SREQ
				client.RxNew = false
			}
			client.CANtxBuff.Data[2] = client.BlockSize
			client.TimeoutTimerBlock = 0
			client.CANModule.Send(*client.CANtxBuff)
			if transferShort && !client.Finished {
				log.Warnf("sub-block restarted: seqnoPrev=%v, blksize=%v", seqnoStart, client.BlockSize)
			}

		case CO_SDO_ST_UPLOAD_BLK_END_CRSP:
			client.CANtxBuff.Data[0] = 0xA1
			client.CANModule.Send(*client.CANtxBuff)
			client.State = CO_SDO_ST_IDLE
			ret = CO_SDO_RT_ok_communicationEnd

		default:
			break
		}

	}

	if ret == CO_SDO_RT_waitingResponse {

		if client.State == CO_SDO_ST_ABORT {
			client.Abort(abortCode.(SDOAbortCode))
			ret = CO_SDO_RT_endedWithClientAbort
			client.State = CO_SDO_ST_IDLE

		} else if client.State == CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_SREQ {
			ret = CO_SDO_RT_blockUploadInProgress
		}
	}
	if sizeIndicated != nil {
		*sizeIndicated = client.SizeIndicated
	}

	if sizeTransferred != nil {
		*sizeTransferred = client.SizeTransferred
	}

	if sdoAbortCode != nil && abortCode != nil {
		*sdoAbortCode = abortCode.(SDOAbortCode)
	}

	return ret

}

func (client *SDOClient) UploadBufRead(buffer []byte) int {
	if buffer == nil {
		return 0
	}
	return client.Fifo.Read(buffer, nil)
}

// func (client *SDOClient) ReadRaw(nodeId uint8, index uint16, subindex uint8, data []byte, forceSegmented bool) error {

func (client *SDOClient) ReadRaw(nodeId uint8, index uint16, subindex uint8, data []byte) (int, error) {
	ret := client.Setup(uint32(SDO_CLIENT_ID)+uint32(nodeId), uint32(SDO_SERVER_ID)+uint32(nodeId), nodeId)
	if ret != CO_SDO_RT_ok_communicationEnd {
		log.Errorf("Error when setting up SDO client reason : %v", ret)
		return 0, CO_SDO_AB_GENERAL
	}
	ret = client.UploadInitiate(index, subindex, 1000, false)

	if ret != CO_SDO_RT_ok_communicationEnd {
		return 0, CO_SDO_AB_GENERAL
	}
	var timeDifferenceUs uint32 = 10000
	abortCode := CO_SDO_AB_NONE

	for {
		ret = client.Upload(timeDifferenceUs, false, &abortCode, nil, nil, nil)
		if ret < 0 {
			log.Errorf("SDO write failed : %v", ret)
			return 0, abortCode
		} else if uint8(ret) == 0 {
			break
		}
		time.Sleep(time.Duration(timeDifferenceUs) * time.Microsecond)
	}
	return client.UploadBufRead(data), abortCode
}

type BlockReader struct {
	Client       *SDOClient
	Index        uint16
	SubIndex     uint8
	NodeIdServer uint8
}

// Read hole block
func (reader *BlockReader) ReadAll() (data []byte, err error) {

	client := reader.Client
	ret := client.Setup(uint32(SDO_CLIENT_ID)+uint32(reader.NodeIdServer), uint32(SDO_SERVER_ID)+uint32(reader.NodeIdServer), reader.NodeIdServer)
	if ret != CO_SDO_RT_ok_communicationEnd {
		log.Errorf("Error when setting up SDO client reason : %v", ret)
		return nil, CO_SDO_AB_GENERAL
	}
	ret = client.UploadInitiate(reader.Index, reader.SubIndex, 1000, true)

	if ret != CO_SDO_RT_ok_communicationEnd {
		return nil, CO_SDO_AB_GENERAL
	}

	var timeDifferenceUs uint32 = 10000
	abortCode := CO_SDO_AB_NONE
	buffer := make([]byte, 1000)
	single_read := 0

	for {
		ret = client.Upload(timeDifferenceUs, false, &abortCode, nil, nil, nil)
		if ret < 0 {
			log.Errorf("SDO write failed : %v", ret)
			return nil, abortCode
		} else if uint8(ret) == 0 {
			break
		} else if ret == CO_SDO_RT_uploadDataBufferFull {
			single_read = client.UploadBufRead(buffer)
			data = append(data, buffer[0:single_read]...)
		}
		time.Sleep(time.Duration(timeDifferenceUs) * time.Microsecond)
	}
	single_read = client.UploadBufRead(buffer)
	data = append(data, buffer[0:single_read]...)
	return data, abortCode

}

func NewBlockReader(nodeid uint8, index uint16, subindex uint8, client *SDOClient) *BlockReader {
	return &BlockReader{Client: client, Index: index, SubIndex: subindex, NodeIdServer: nodeid}
}
