package canopen

import (
	"encoding/binary"
	"errors"
	"math"
	"time"

	log "github.com/sirupsen/logrus"
)

const SDO_CLI_BUFFER_SIZE = 1000
const CO_CONFIG_SDO_CLI_PST = 21
const DEFAULT_SDO_CLIENT_TIMEOUT_MS = 1000

type SDOReturn int8

var ErrSDOInvalidArguments = errors.New("error in arguments")

const (
	SDO_WAITING_LOCAL_TRANSFER     uint8 = 6 // Waiting in client local transfer.
	SDO_UPLOAD_DATA_FULL           uint8 = 5 // Data buffer is full.SDO client: data must be read before next upload cycle begins.
	SDO_TRANSMIT_BUFFER_FULL       uint8 = 4 // CAN transmit buffer is full. Waiting.
	SDO_BLOCK_DOWNLOAD_IN_PROGRESS uint8 = 3 // Block download is in progress. Sending train of messages.
	SDO_BLOCK_UPLOAD_IN_PROGRESS   uint8 = 2 // Block upload is in progress. Receiving train of messages.SDO client: Data must not be read in this state.
	SDO_WAITING_RESPONSE           uint8 = 1 // Waiting server or client response.
	SDO_SUCCESS                    uint8 = 0 // Success, end of communication. SDO client: uploaded data must be read.

)

type SDOClient struct {
	od                         *ObjectDictionary
	streamer                   *Streamer
	NodeId                     uint8
	busManager                 *BusManager
	txBuffer                   Frame
	CobIdClientToServer        uint32
	CobIdServerToClient        uint32
	NodeIdServer               uint8
	Valid                      bool
	Index                      uint16
	Subindex                   uint8
	Finished                   bool
	SizeIndicated              uint32
	SizeTransferred            uint32
	State                      SDOState
	TimeoutTimeUs              uint32
	TimeoutTimer               uint32
	fifo                       *Fifo
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

func (client *SDOClient) Handle(frame Frame) {

	if client.State != SDO_STATE_IDLE && frame.DLC == 8 && (!client.RxNew || frame.Data[0] == 0x80) {
		if frame.Data[0] == 0x80 || (client.State != SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ && client.State != SDO_STATE_UPLOAD_BLK_SUBBLOCK_CRSP) {
			// Copy data in response
			client.Response.raw = frame.Data
			client.RxNew = true
		} else if client.State == SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ {
			state := SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ
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
					state = SDO_STATE_UPLOAD_BLK_SUBBLOCK_CRSP
				} else {
					client.fifo.Write(frame.Data[1:], &client.BlockCRC)
					client.SizeTransferred += 7
					if seqno == client.BlockSize {
						log.Debugf("[CLIENT][RX][x%x] BLOCK UPLOAD END SUB-BLOCK | x%x:x%x | %v", client.NodeIdServer, client.Index, client.Subindex, frame.Data)
						state = SDO_STATE_UPLOAD_BLK_SUBBLOCK_CRSP
					}
				}
			} else if seqno != client.BlockSequenceNb && client.BlockSequenceNb != 0 {
				state = SDO_STATE_UPLOAD_BLK_SUBBLOCK_CRSP
				log.Warnf("Wrong sequence number in rx sub-block. seqno %x, previous %x", seqno, client.BlockSequenceNb)
			} else {
				log.Warnf("Wrong sequence number in rx ignored. seqno %x, expected %x", seqno, client.BlockSequenceNb+1)
			}

			if state != SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ {
				client.RxNew = false
				client.State = state

			}

		}
	}

}

// Setup the client for communication with an SDO server
func (client *SDOClient) setupServer(cobIdClientToServer uint32, cobIdServerToClient uint32, nodeIdServer uint8) error {
	client.State = SDO_STATE_IDLE
	client.RxNew = false
	client.NodeIdServer = nodeIdServer
	// If server is the same don't re-initialize the buffers
	if client.CobIdClientToServer == cobIdClientToServer && client.CobIdServerToClient == cobIdServerToClient {
		return nil
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
	err := client.busManager.Subscribe(uint32(CanIdS2C), 0x7FF, false, client)
	if err != nil {
		return err
	}
	client.txBuffer = NewFrame(uint32(CanIdC2S), 0, 8)
	return nil
}

// Start a new download sequence
func (client *SDOClient) downloadSetup(index uint16, subindex uint8, sizeIndicated uint32, blockEnabled bool) error {
	if !client.Valid {
		return ErrSDOInvalidArguments
	}
	client.Index = index
	client.Subindex = subindex
	client.SizeIndicated = sizeIndicated
	client.SizeTransferred = 0
	client.Finished = false
	client.TimeoutTimer = 0
	client.fifo.Reset()

	if client.od != nil && client.NodeId != 0 && client.NodeIdServer == client.NodeId {
		client.streamer.write = nil
		client.State = SDO_STATE_DOWNLOAD_LOCAL_TRANSFER
	} else if blockEnabled && (sizeIndicated == 0 || sizeIndicated > CO_CONFIG_SDO_CLI_PST) {
		client.State = SDO_STATE_DOWNLOAD_BLK_INITIATE_REQ
	} else {
		client.State = SDO_STATE_DOWNLOAD_INITIATE_REQ
	}
	client.RxNew = false
	return nil
}

func (client *SDOClient) downloadMain(
	timeDifferenceUs uint32,
	abort bool,
	bufferPartial bool,
	sizeTransferred *uint32,
	timerNextUs *uint32,
	forceSegmented bool,
) (uint8, error) {

	ret := SDO_WAITING_RESPONSE
	var err error
	var abortCode error

	if !client.Valid {
		abortCode = SDO_ABORT_DEVICE_INCOMPAT
		err = ErrSDOInvalidArguments
	} else if client.State == SDO_STATE_IDLE {
		ret = SDO_SUCCESS
	} else if client.State == SDO_STATE_DOWNLOAD_LOCAL_TRANSFER && !abort {
		ret, abortCode = client.downloadLocal(bufferPartial, nil)
		if ret != SDO_WAITING_LOCAL_TRANSFER {
			client.State = SDO_STATE_IDLE
		} else if timerNextUs != nil {
			*timerNextUs = 0
		}
	} else if client.RxNew {
		response := client.Response
		if response.IsAbort() {
			abortCode = response.GetAbortCode()
			log.Debugf("[CLIENT][RX][x%x] SERVER ABORT | x%x:x%x | %v (x%x)", client.NodeIdServer, client.Index, client.Subindex, abortCode, uint32(response.GetAbortCode()))
			client.State = SDO_STATE_IDLE
			err = abortCode
			// Abort from the client
		} else if abort {
			abortCode = SDO_ABORT_DEVICE_INCOMPAT
			client.State = SDO_STATE_ABORT

		} else if !response.isResponseValid(client.State) {
			log.Warnf("Unexpected response code from server : %x", response.raw[0])
			client.State = SDO_STATE_ABORT
			abortCode = SDO_ABORT_CMD

		} else {
			switch client.State {
			case SDO_STATE_DOWNLOAD_INITIATE_RSP:

				index := response.GetIndex()
				subIndex := response.GetSubindex()
				if index != client.Index || subIndex != client.Subindex {
					abortCode = SDO_ABORT_PRAM_INCOMPAT
					client.State = SDO_STATE_ABORT
					break
				}
				// Expedited transfer
				if client.Finished {
					client.State = SDO_STATE_IDLE
					ret = SDO_SUCCESS
					log.Debugf("[CLIENT][RX][x%x] DOWNLOAD EXPEDITED | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, response.raw)
					// Segmented transfer
				} else {
					client.Toggle = 0x00
					client.State = SDO_STATE_DOWNLOAD_SEGMENT_REQ
					log.Debugf("[CLIENT][RX][x%x] DOWNLOAD SEGMENT | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, response.raw)
				}

			case SDO_STATE_DOWNLOAD_SEGMENT_RSP:

				// Verify and alternate toggle bit
				toggle := response.GetToggle()
				if toggle != client.Toggle {
					abortCode = SDO_ABORT_TOGGLE_BIT
					client.State = SDO_STATE_ABORT
					break
				}
				client.Toggle ^= 0x10
				if client.Finished {
					client.State = SDO_STATE_IDLE
					ret = SDO_SUCCESS
				} else {
					client.State = SDO_STATE_DOWNLOAD_SEGMENT_REQ
				}
				log.Debugf("[CLIENT][RX][x%x] DOWNLOAD SEGMENT | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, response.raw)

			case SDO_STATE_DOWNLOAD_BLK_INITIATE_RSP:

				index := response.GetIndex()
				subIndex := response.GetSubindex()
				if index != client.Index || subIndex != client.Subindex {
					abortCode = SDO_ABORT_PRAM_INCOMPAT
					client.State = SDO_STATE_ABORT
					break
				}
				client.BlockCRC = CRC16(0)
				client.BlockSize = response.GetBlockSize()
				if client.BlockSize < 1 || client.BlockSize > 127 {
					client.BlockSize = 127
				}
				client.BlockSequenceNb = 0
				client.fifo.AltBegin(0)
				client.State = SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ
				log.Debugf("[CLIENT][RX][x%x] DOWNLOAD BLOCK | x%x:x%x %v | blksize %v", client.NodeIdServer, client.Index, client.Subindex, response.raw, client.BlockSize)

			case SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ, SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_RSP:

				if response.GetNumberOfSegments() < client.BlockSequenceNb {
					log.Error("Not all segments transferred successfully")
					client.fifo.AltBegin(int(response.raw[1]) * 7)
					client.Finished = false

				} else if response.GetNumberOfSegments() > client.BlockSequenceNb {
					abortCode = SDO_ABORT_CMD
					client.State = SDO_STATE_ABORT
					break
				}
				client.fifo.AltFinish(&client.BlockCRC)
				if client.Finished {
					client.State = SDO_STATE_DOWNLOAD_BLK_END_REQ
				} else {
					client.BlockSize = response.raw[2]
					client.BlockSequenceNb = 0
					client.fifo.AltBegin(0)
					client.State = SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ
				}

			case SDO_STATE_DOWNLOAD_BLK_END_RSP:

				client.State = SDO_STATE_IDLE
				ret = SDO_SUCCESS

			}

			client.TimeoutTimer = 0
			timeDifferenceUs = 0
			client.RxNew = false

		}

	} else if abort {
		abortCode = SDO_ABORT_DEVICE_INCOMPAT
		client.State = SDO_STATE_ABORT
	}

	if ret == SDO_WAITING_RESPONSE {
		if client.TimeoutTimer < client.TimeoutTimeUs {
			client.TimeoutTimer += timeDifferenceUs
		}
		if client.TimeoutTimer >= client.TimeoutTimeUs {
			abortCode = SDO_ABORT_TIMEOUT
			client.State = SDO_STATE_ABORT
		} else if timerNextUs != nil {
			diff := client.TimeoutTimeUs - client.TimeoutTimer
			if *timerNextUs > diff {
				*timerNextUs = diff
			}
		}
	}

	if ret == SDO_WAITING_RESPONSE {
		client.txBuffer.Data = [8]byte{0}
		switch client.State {
		case SDO_STATE_DOWNLOAD_INITIATE_REQ:
			abortCode = client.downloadInitiate(forceSegmented)
			if abortCode != nil {
				client.State = SDO_STATE_IDLE
				err = abortCode
				break
			}
			client.State = SDO_STATE_DOWNLOAD_INITIATE_RSP

		case SDO_STATE_DOWNLOAD_SEGMENT_REQ:
			abortCode = client.downloadSegment(bufferPartial)
			if abortCode != nil {
				client.State = SDO_STATE_ABORT
				err = abortCode
				break
			}
			client.State = SDO_STATE_DOWNLOAD_SEGMENT_RSP

		case SDO_STATE_DOWNLOAD_BLK_INITIATE_REQ:
			client.downloadBlockInitiate()
			client.State = SDO_STATE_DOWNLOAD_BLK_INITIATE_RSP

		case SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ:
			abortCode = client.downloadBlock(bufferPartial, timerNextUs)
			if abortCode != nil {
				client.State = SDO_STATE_ABORT
			}

		case SDO_STATE_DOWNLOAD_BLK_END_REQ:
			client.downloadBlockEnd()
			client.State = SDO_STATE_DOWNLOAD_BLK_END_RSP

		default:
			break

		}

	}

	if ret == SDO_WAITING_RESPONSE {

		switch client.State {
		case SDO_STATE_ABORT:
			client.abort(abortCode.(SDOAbortCode))
			err = abortCode
			client.State = SDO_STATE_IDLE
		case SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ:
			ret = SDO_BLOCK_DOWNLOAD_IN_PROGRESS
		}
	}

	if sizeTransferred != nil {
		*sizeTransferred = client.SizeTransferred
	}
	return ret, err
}

// Helper function for starting download
// Valid for expedited or segmented transfer
func (client *SDOClient) downloadInitiate(forceSegmented bool) error {

	client.txBuffer.Data[0] = 0x20
	client.txBuffer.Data[1] = byte(client.Index)
	client.txBuffer.Data[2] = byte(client.Index >> 8)
	client.txBuffer.Data[3] = client.Subindex

	count := uint32(client.fifo.GetOccupied())
	if (client.SizeIndicated == 0 && count <= 4) || (client.SizeIndicated > 0 && client.SizeIndicated <= 4) && !forceSegmented {
		client.txBuffer.Data[0] |= 0x02
		// Check length
		if count == 0 || (client.SizeIndicated > 0 && client.SizeIndicated != count) {
			client.State = SDO_STATE_IDLE
			return SDO_ABORT_TYPE_MISMATCH
		}
		if client.SizeIndicated > 0 {
			client.txBuffer.Data[0] |= byte(0x01 | ((4 - count) << 2))
		}
		// Copy the data in queue and add the count
		count = uint32(client.fifo.Read(client.txBuffer.Data[4:], nil))
		client.SizeTransferred = count
		client.Finished = true
		log.Debugf("[CLIENT][TX][x%x] DOWNLOAD EXPEDITED | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, client.txBuffer.Data)

	} else {
		/* segmented transfer, indicate data size */
		if client.SizeIndicated > 0 {
			size := client.SizeIndicated
			client.txBuffer.Data[0] |= 0x01
			binary.LittleEndian.PutUint32(client.txBuffer.Data[4:], size)
		}
		log.Debugf("[CLIENT][TX][x%x] DOWNLOAD SEGMENT | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, client.txBuffer.Data)
	}
	client.TimeoutTimer = 0
	client.busManager.Send(client.txBuffer)
	return nil

}

// Write value to OD locally
func (client *SDOClient) downloadLocal(bufferPartial bool, timerNextUs *uint32) (ret uint8, abortCode error) {
	var err error

	if client.streamer.write == nil {
		client.streamer, err = NewStreamer(client.od.Index(client.Index), client.Subindex, false)
		odErr, ok := err.(ODR)
		if err != nil {
			if !ok {
				return 0, SDO_ABORT_GENERAL
			}
			return 0, odErr.GetSDOAbordCode()
		} else if (client.streamer.stream.Attribute & ATTRIBUTE_SDO_RW) == 0 {
			return 0, SDO_ABORT_UNSUPPORTED_ACCESS
		} else if (client.streamer.stream.Attribute & ATTRIBUTE_SDO_W) == 0 {
			return 0, SDO_ABORT_READONLY
		} else if client.streamer.write == nil {
			return 0, SDO_ABORT_DEVICE_INCOMPAT
		}
	} else {
		buffer := make([]byte, SDO_CLI_BUFFER_SIZE+2)
		count := client.fifo.Read(buffer, nil)
		client.SizeTransferred += uint32(count)
		// No data error
		if count == 0 {
			abortCode = SDO_ABORT_DEVICE_INCOMPAT
			// Size transferred is too large
		} else if client.SizeIndicated > 0 && client.SizeTransferred > client.SizeIndicated {
			client.SizeTransferred -= uint32(count)
			abortCode = SDO_ABORT_DATA_LONG
			// Size transferred is too small (check on last call)
		} else if !bufferPartial && client.SizeIndicated > 0 && client.SizeTransferred < client.SizeIndicated {
			abortCode = SDO_ABORT_DATA_SHORT
			// Last part of data !
		} else if !bufferPartial {
			odVarSize := client.streamer.stream.DataLength
			// Special case for strings where the downloaded data may be shorter (nul character can be omitted)
			if client.streamer.stream.Attribute&ATTRIBUTE_STR != 0 && odVarSize == 0 || client.SizeTransferred < uint32(odVarSize) {
				count += 1
				buffer[count] = 0
				client.SizeTransferred += 1
				if odVarSize == 0 || odVarSize > client.SizeTransferred {
					count += 1
					buffer[count] = 0
					client.SizeTransferred += 1
				}
				client.streamer.stream.DataLength = client.SizeTransferred
			} else if odVarSize == 0 {
				client.streamer.stream.DataLength = client.SizeTransferred
			} else if client.SizeTransferred > uint32(odVarSize) {
				abortCode = SDO_ABORT_DATA_LONG
			} else if client.SizeTransferred < uint32(odVarSize) {
				abortCode = SDO_ABORT_DATA_SHORT
			}
		}

		if abortCode == nil {
			_, err := client.streamer.Write(buffer)
			odErr, ok := err.(ODR)
			if err != nil && odErr != ODR_PARTIAL {
				if !ok {
					return 0, SDO_ABORT_GENERAL
				}
				return 0, odErr.GetSDOAbordCode()
			} else if bufferPartial && err == nil {
				return 0, SDO_ABORT_DATA_LONG
			} else if !bufferPartial {
				// Error if not written completely but download end
				if odErr == ODR_PARTIAL {
					return 0, SDO_ABORT_DATA_SHORT
				} else {
					return SDO_SUCCESS, nil
				}
			} else {
				return SDO_WAITING_LOCAL_TRANSFER, nil
			}
		}
	}
	return 0, nil
}

// Helper function for downloading a segement of segmented transfer
func (client *SDOClient) downloadSegment(bufferPartial bool) error {
	// Fill data part
	count := uint32(client.fifo.Read(client.txBuffer.Data[1:], nil))
	client.SizeTransferred += count
	if client.SizeIndicated > 0 && client.SizeTransferred > client.SizeIndicated {
		client.SizeTransferred -= count
		return SDO_ABORT_DATA_LONG
	}

	//Command specifier
	client.txBuffer.Data[0] = uint8(uint32(client.Toggle) | ((7 - count) << 1))
	if client.fifo.GetOccupied() == 0 && !bufferPartial {
		if client.SizeIndicated > 0 && client.SizeTransferred < client.SizeIndicated {
			return SDO_ABORT_DATA_SHORT
		}
		client.txBuffer.Data[0] |= 0x01
		client.Finished = true
	}

	client.TimeoutTimer = 0
	log.Debugf("[CLIENT][TX][x%x] DOWNLOAD SEGMENT | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, client.txBuffer.Data)
	client.busManager.Send(client.txBuffer)
	return nil
}

// Helper function for initiating a block download
func (client *SDOClient) downloadBlockInitiate() error {
	client.txBuffer.Data[0] = 0xC4
	client.txBuffer.Data[1] = byte(client.Index)
	client.txBuffer.Data[2] = byte(client.Index >> 8)
	client.txBuffer.Data[3] = client.Subindex
	if client.SizeIndicated > 0 {
		client.txBuffer.Data[0] |= 0x02
		binary.LittleEndian.PutUint32(client.txBuffer.Data[4:], client.SizeIndicated)
	}
	client.TimeoutTimer = 0
	client.busManager.Send(client.txBuffer)
	return nil

}

// Helper function for downloading a sub-block
func (client *SDOClient) downloadBlock(bufferPartial bool, timerNext *uint32) error {
	if client.fifo.AltGetOccupied() < 7 && bufferPartial {
		// No data yet
		return nil
	}
	client.BlockSequenceNb++
	client.txBuffer.Data[0] = client.BlockSequenceNb
	count := uint32(client.fifo.AltRead(client.txBuffer.Data[1:]))
	client.BlockNoData = uint8(7 - count)
	client.SizeTransferred += count
	if client.SizeIndicated > 0 && client.SizeTransferred > client.SizeIndicated {
		client.SizeTransferred -= count
		return SDO_ABORT_DATA_LONG
	}
	if client.fifo.AltGetOccupied() == 0 && !bufferPartial {
		if client.SizeIndicated > 0 && client.SizeTransferred < client.SizeIndicated {
			return SDO_ABORT_DATA_SHORT
		}
		client.txBuffer.Data[0] |= 0x80
		client.Finished = true
		client.State = SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_RSP
	} else if client.BlockSequenceNb >= client.BlockSize {
		client.State = SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_RSP
	} else {
		if timerNext != nil {
			*timerNext = 0
		}
	}
	client.TimeoutTimer = 0
	client.busManager.Send(client.txBuffer)
	return nil

}

// Helper function for end of block
func (client *SDOClient) downloadBlockEnd() {
	client.txBuffer.Data[0] = 0xC1 | (client.BlockNoData << 2)
	client.txBuffer.Data[1] = byte(client.BlockCRC)
	client.txBuffer.Data[2] = byte(client.BlockCRC >> 8)
	client.TimeoutTimer = 0
	client.busManager.Send(client.txBuffer)
}

// Create & send abort on bus
func (client *SDOClient) abort(abortCode SDOAbortCode) {
	code := uint32(abortCode)
	client.txBuffer.Data[0] = 0x80
	client.txBuffer.Data[1] = uint8(client.Index)
	client.txBuffer.Data[2] = uint8(client.Index >> 8)
	client.txBuffer.Data[3] = client.Subindex
	binary.LittleEndian.PutUint32(client.txBuffer.Data[4:], code)
	log.Warnf("[CLIENT][TX][x%x] CLIENT ABORT | x%x:x%x | %v (x%x)", client.NodeIdServer, client.Index, client.Subindex, abortCode, code)
	client.busManager.Send(client.txBuffer)

}

/////////////////////////////////////
////////////SDO UPLOAD///////////////
/////////////////////////////////////

func (client *SDOClient) uploadSetup(index uint16, subindex uint8, blockEnabled bool) error {
	if !client.Valid {
		return ErrSDOInvalidArguments
	}
	client.Index = index
	client.Subindex = subindex
	client.SizeIndicated = 0
	client.SizeTransferred = 0
	client.Finished = false
	client.fifo.Reset()
	if client.od != nil && client.NodeId != 0 && client.NodeIdServer == client.NodeId {
		client.streamer.read = nil
		client.State = SDO_STATE_UPLOAD_LOCAL_TRANSFER
	} else if blockEnabled {
		client.State = SDO_STATE_UPLOAD_BLK_INITIATE_REQ
	} else {
		client.State = SDO_STATE_UPLOAD_INITIATE_REQ
	}
	client.RxNew = false
	return nil
}

// Main state machine
func (client *SDOClient) upload(
	timeDifferenceUs uint32,
	abort bool,
	sizeIndicated *uint32,
	sizeTransferred *uint32,
	timerNextUs *uint32,
) (uint8, error) {

	ret := SDO_WAITING_RESPONSE
	var err error
	var abortCode error

	if !client.Valid {
		abortCode = SDO_ABORT_DEVICE_INCOMPAT
		err = ErrSDOInvalidArguments
	} else if client.State == SDO_STATE_IDLE {
		ret = SDO_SUCCESS
	} else if client.State == SDO_STATE_UPLOAD_LOCAL_TRANSFER && !abort {
		// TODO
	} else if client.RxNew {
		response := client.Response
		if response.IsAbort() {
			abortCode = response.GetAbortCode()
			log.Debugf("[CLIENT][RX][x%x] SERVER ABORT | x%x:x%x | %v (x%x)", client.NodeIdServer, client.Index, client.Subindex, abortCode, uint32(response.GetAbortCode()))
			client.State = SDO_STATE_IDLE
			err = abortCode
		} else if abort {
			abortCode = SDO_ABORT_DEVICE_INCOMPAT
			client.State = SDO_STATE_ABORT

		} else if !response.isResponseValid(client.State) {
			log.Warnf("Unexpected response code from server : %x", response.raw[0])
			client.State = SDO_STATE_ABORT
			abortCode = SDO_ABORT_CMD

		} else {
			switch client.State {
			case SDO_STATE_UPLOAD_INITIATE_RSP:
				index := response.GetIndex()
				subIndex := response.GetSubindex()
				if index != client.Index || subIndex != client.Subindex {
					abortCode = SDO_ABORT_PRAM_INCOMPAT
					client.State = SDO_STATE_ABORT
					break
				}
				if (response.raw[0] & 0x02) != 0 {
					//Expedited
					var count uint32 = 4
					// Size indicated ?
					if (response.raw[0] & 0x01) != 0 {
						count -= uint32((response.raw[0] >> 2) & 0x03)
					}
					client.fifo.Write(response.raw[4:4+count], nil)
					client.SizeTransferred = count
					client.State = SDO_STATE_IDLE
					ret = SDO_SUCCESS
					log.Debugf("[CLIENT][RX][x%x] UPLOAD EXPEDITED | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, response.raw)
					// Segmented
				} else {
					// Size indicated ?
					if (response.raw[0] & 0x01) != 0 {
						client.SizeIndicated = binary.LittleEndian.Uint32(response.raw[4:])
					}
					client.Toggle = 0
					client.State = SDO_STATE_UPLOAD_SEGMENT_REQ
					log.Debugf("[CLIENT][RX][x%x] UPLOAD SEGMENT | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, response.raw)

				}

			case SDO_STATE_UPLOAD_SEGMENT_RSP:
				// Verify and alternate toggle bit
				log.Debugf("[CLIENT][RX][x%x] UPLOAD SEGMENT | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, response.raw)
				toggle := response.GetToggle()
				if toggle != client.Toggle {
					abortCode = SDO_ABORT_TOGGLE_BIT
					client.State = SDO_STATE_ABORT
					break
				}
				client.Toggle ^= 0x10
				count := 7 - (response.raw[0]>>1)&0x07
				countWr := client.fifo.Write(response.raw[1:1+count], nil)
				client.SizeTransferred += uint32(countWr)
				// Check enough space if fifo
				if countWr != int(count) {
					abortCode = SDO_ABORT_OUT_OF_MEM
					client.State = SDO_STATE_ABORT
					break
				}

				//Check size uploaded
				if client.SizeIndicated > 0 && client.SizeTransferred > client.SizeIndicated {
					abortCode = SDO_ABORT_DATA_LONG
					client.State = SDO_STATE_ABORT
					break
				}

				//No more segments ?
				if (response.raw[0] & 0x01) != 0 {
					// Check size uploaded
					if client.SizeIndicated > 0 && client.SizeTransferred < client.SizeIndicated {
						abortCode = SDO_ABORT_DATA_LONG
						client.State = SDO_STATE_ABORT
					} else {
						client.State = SDO_STATE_IDLE
						ret = SDO_SUCCESS
					}
				} else {
					client.State = SDO_STATE_UPLOAD_SEGMENT_REQ
				}

			case SDO_STATE_UPLOAD_BLK_INITIATE_RSP:

				index := response.GetIndex()
				subindex := response.GetSubindex()
				if index != client.Index || subindex != client.Subindex {
					abortCode = SDO_ABORT_PRAM_INCOMPAT
					client.State = SDO_STATE_ABORT
					break
				}
				// Block is supported
				if (response.raw[0] & 0xF9) == 0xC0 {
					client.BlockCRCEnabled = response.IsCRCEnabled()
					if (response.raw[0] & 0x02) != 0 {
						client.SizeIndicated = uint32(response.GetBlockSize())
					}
					client.State = SDO_STATE_UPLOAD_BLK_INITIATE_REQ2
					log.Debugf("[CLIENT][RX][x%x] BLOCK UPLOAD INIT | x%x:x%x | crc enabled : %v expected size : %v | %v",
						client.NodeIdServer,
						client.Index,
						client.Subindex,
						response.IsCRCEnabled(),
						client.SizeIndicated,
						response.raw,
					)

					//Switch to normal transfer
				} else if (response.raw[0] & 0xF0) == 0x40 {
					if (response.raw[0] & 0x02) != 0 {
						//Expedited
						count := 4
						if (response.raw[0] & 0x01) != 0 {
							count -= (int(response.raw[0]>>2) & 0x03)
						}
						client.fifo.Write(response.raw[4:4+count], nil)
						client.SizeTransferred = uint32(count)
						client.State = SDO_STATE_IDLE
						ret = SDO_SUCCESS
						log.Debugf("[CLIENT][RX][x%x] BLOCK UPLOAD SWITCHING EXPEDITED | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, response.raw)

					} else {
						if (response.raw[0] & 0x01) != 0 {
							client.SizeIndicated = uint32(response.GetBlockSize())
						}
						client.Toggle = 0x00
						client.State = SDO_STATE_UPLOAD_SEGMENT_REQ
						log.Debugf("[CLIENT][RX][x%x] BLOCK UPLOAD SWITCHING SEGMENTED | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, response.raw)
					}

				}
			case SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ:
				// Handled directly in Rx callback
				break

			case SDO_STATE_UPLOAD_BLK_END_SREQ:
				//Get number of data bytes in last segment, that do not
				//contain data. Then copy remaining data into fifo
				noData := (response.raw[0] >> 2) & 0x07
				client.fifo.Write(client.BlockDataUploadLast[:7-noData], &client.BlockCRC)
				client.SizeTransferred += uint32(7 - noData)

				if client.SizeIndicated > 0 && client.SizeTransferred > client.SizeIndicated {
					abortCode = SDO_ABORT_DATA_LONG
					client.State = SDO_STATE_ABORT
					break
				} else if client.SizeIndicated > 0 && client.SizeTransferred < client.SizeIndicated {
					abortCode = SDO_ABORT_DATA_SHORT
					client.State = SDO_STATE_ABORT
					break
				}
				if client.BlockCRCEnabled {
					crcServer := CRC16(binary.LittleEndian.Uint16(response.raw[1:3]))
					if crcServer != client.BlockCRC {
						abortCode = SDO_ABORT_CRC
						client.State = SDO_STATE_ABORT
						break
					}
				}
				client.State = SDO_STATE_UPLOAD_BLK_END_CRSP
				log.Debugf("[CLIENT][RX][x%x] BLOCK UPLOAD END | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, response.raw)

			default:
				abortCode = SDO_ABORT_CMD
				client.State = SDO_STATE_ABORT
			}

		}
		client.TimeoutTimer = 0
		timeDifferenceUs = 0
		client.RxNew = false
	} else if abort {
		abortCode = SDO_ABORT_DEVICE_INCOMPAT
		client.State = SDO_STATE_ABORT
	}

	if ret == SDO_WAITING_RESPONSE {
		if client.TimeoutTimer < client.TimeoutTimeUs {
			client.TimeoutTimer += timeDifferenceUs
		}
		if client.TimeoutTimer >= client.TimeoutTimeUs {
			if client.State == SDO_STATE_UPLOAD_SEGMENT_REQ || client.State == SDO_STATE_UPLOAD_BLK_SUBBLOCK_CRSP {
				abortCode = SDO_ABORT_GENERAL
			} else {
				abortCode = SDO_ABORT_TIMEOUT
			}
			client.State = SDO_STATE_ABORT

		} else if timerNextUs != nil {
			diff := client.TimeoutTimeUs - client.TimeoutTimer
			if *timerNextUs > diff {
				*timerNextUs = diff
			}
		}
		// Timeout for subblocks
		if client.State == SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ {
			if client.TimeoutTimerBlock < client.TimeoutTimeBlockTransferUs {
				client.TimeoutTimerBlock += timeDifferenceUs
			}
			if client.TimeoutTimerBlock >= client.TimeoutTimeBlockTransferUs {
				client.State = SDO_STATE_UPLOAD_BLK_SUBBLOCK_CRSP
				client.RxNew = false
			} else if timerNextUs != nil {
				diff := client.TimeoutTimeBlockTransferUs - client.TimeoutTimerBlock
				if *timerNextUs > diff {
					*timerNextUs = diff
				}
			}
		}
	}

	if ret == SDO_WAITING_RESPONSE {
		client.txBuffer.Data = [8]byte{0}
		switch client.State {
		case SDO_STATE_UPLOAD_INITIATE_REQ:
			client.txBuffer.Data[0] = 0x40
			client.txBuffer.Data[1] = byte(client.Index)
			client.txBuffer.Data[2] = byte(client.Index >> 8)
			client.txBuffer.Data[3] = client.Subindex
			client.TimeoutTimer = 0
			client.busManager.Send(client.txBuffer)
			client.State = SDO_STATE_UPLOAD_INITIATE_RSP
			log.Debugf("[CLIENT][TX][x%x] UPLOAD SEGMENT | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, client.txBuffer.Data)

		case SDO_STATE_UPLOAD_SEGMENT_REQ:
			if client.fifo.GetSpace() < 7 {
				ret = SDO_UPLOAD_DATA_FULL
				break
			}
			client.txBuffer.Data[0] = 0x60 | client.Toggle
			client.TimeoutTimer = 0
			client.busManager.Send(client.txBuffer)
			client.State = SDO_STATE_UPLOAD_SEGMENT_RSP
			log.Debugf("[CLIENT][TX][x%x] UPLOAD SEGMENT | x%x:x%x %v", client.NodeIdServer, client.Index, client.Subindex, client.txBuffer.Data)

		case SDO_STATE_UPLOAD_BLK_INITIATE_REQ:
			client.txBuffer.Data[0] = 0xA4
			client.txBuffer.Data[1] = byte(client.Index)
			client.txBuffer.Data[2] = byte(client.Index >> 8)
			client.txBuffer.Data[3] = client.Subindex
			// Calculate number of block segments from free space
			count := client.fifo.GetSpace() / 7
			if count >= 127 {
				count = 127
			} else if count == 0 {
				abortCode = SDO_ABORT_OUT_OF_MEM
				client.State = SDO_STATE_ABORT
				break
			}
			client.BlockSize = uint8(count)
			client.txBuffer.Data[4] = client.BlockSize
			client.txBuffer.Data[5] = CO_CONFIG_SDO_CLI_PST
			client.TimeoutTimer = 0
			client.busManager.Send(client.txBuffer)
			client.State = SDO_STATE_UPLOAD_BLK_INITIATE_RSP
			log.Debugf("[CLIENT][TX][x%x] BLOCK UPLOAD INITIATE | x%x:x%x %v blksize : %v", client.NodeIdServer, client.Index, client.Subindex, client.txBuffer.Data, client.BlockSize)

		case SDO_STATE_UPLOAD_BLK_INITIATE_REQ2:
			client.txBuffer.Data[0] = 0xA3
			client.TimeoutTimer = 0
			client.TimeoutTimerBlock = 0
			client.BlockSequenceNb = 0
			client.BlockCRC = CRC16(0)
			client.State = SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ
			client.RxNew = false
			client.busManager.Send(client.txBuffer)

		case SDO_STATE_UPLOAD_BLK_SUBBLOCK_CRSP:
			client.txBuffer.Data[0] = 0xA2
			client.txBuffer.Data[1] = client.BlockSequenceNb
			transferShort := client.BlockSequenceNb != client.BlockSize
			seqnoStart := client.BlockSequenceNb
			if client.Finished {
				client.State = SDO_STATE_UPLOAD_BLK_END_SREQ
			} else {
				// Check size too large
				if client.SizeIndicated > 0 && client.SizeTransferred > client.SizeIndicated {
					abortCode = SDO_ABORT_DATA_LONG
					client.State = SDO_STATE_ABORT
					break
				}
				// Calculate number of block segments from free space
				count := client.fifo.GetSpace() / 7
				if count >= 127 {
					count = 127

				} else if client.fifo.GetOccupied() > 0 {
					ret = SDO_UPLOAD_DATA_FULL
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
				client.State = SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ
				client.RxNew = false
			}
			client.txBuffer.Data[2] = client.BlockSize
			client.TimeoutTimerBlock = 0
			client.busManager.Send(client.txBuffer)
			if transferShort && !client.Finished {
				log.Warnf("sub-block restarted: seqnoPrev=%v, blksize=%v", seqnoStart, client.BlockSize)
			}

		case SDO_STATE_UPLOAD_BLK_END_CRSP:
			client.txBuffer.Data[0] = 0xA1
			client.busManager.Send(client.txBuffer)
			client.State = SDO_STATE_IDLE
			ret = SDO_SUCCESS

		default:
			break
		}

	}

	if ret == SDO_WAITING_RESPONSE {
		switch client.State {
		case SDO_STATE_ABORT:
			client.abort(abortCode.(SDOAbortCode))
			err = abortCode
			client.State = SDO_STATE_IDLE
		case SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ:
			ret = SDO_BLOCK_UPLOAD_IN_PROGRESS
		}
	}
	if sizeIndicated != nil {
		*sizeIndicated = client.SizeIndicated
	}

	if sizeTransferred != nil {
		*sizeTransferred = client.SizeTransferred
	}

	return ret, err

}

// Read a given index/subindex from node into data
// Similar to io.Read
func (client *SDOClient) ReadRaw(nodeId uint8, index uint16, subindex uint8, data []byte) (int, error) {
	var timeDifferenceUs uint32 = 10000

	err := client.setupServer(
		uint32(SDO_CLIENT_ID)+uint32(nodeId),
		uint32(SDO_SERVER_ID)+uint32(nodeId),
		nodeId,
	)
	if err != nil {
		return 0, err
	}
	err = client.uploadSetup(index, subindex, false)
	if err != nil {
		return 0, err
	}

	for {
		ret, err := client.upload(timeDifferenceUs, false, nil, nil, nil)
		if err != nil {
			return 0, err
		} else if ret == SDO_SUCCESS {
			break
		}
		time.Sleep(time.Duration(timeDifferenceUs) * time.Microsecond)
	}
	if err != nil {
		return 0, err
	}
	return client.fifo.Read(data, nil), nil
}

// Read uint32
func (client *SDOClient) ReadUint32(nodeId uint8, index uint16, subindex uint8) (uint32, error) {
	buf := make([]byte, 4)
	n, err := client.ReadRaw(nodeId, index, subindex, buf)
	if err != nil {
		return 0, err
	} else if n != 4 {
		return 0, ODR_TYPE_MISMATCH
	}
	return binary.LittleEndian.Uint32(buf), nil
}

// Read everything from a given index/subindex from node and return all bytes
// Similar to io.ReadAll
func (client *SDOClient) ReadAll(nodeId uint8, index uint16, subindex uint8) ([]byte, error) {
	var timeDifferenceUs uint32 = 10000
	err := client.setupServer(
		uint32(SDO_CLIENT_ID)+uint32(nodeId),
		uint32(SDO_SERVER_ID)+uint32(nodeId),
		nodeId,
	)
	if err != nil {
		return nil, err
	}
	err = client.uploadSetup(index, subindex, true)
	if err != nil {
		return nil, err
	}

	buffer := make([]byte, 1000)
	singleRead := 0
	returnBuffer := make([]byte, 0)

	for {
		ret, err := client.upload(timeDifferenceUs, false, nil, nil, nil)
		if err != nil {
			return nil, err
		} else if ret == SDO_SUCCESS {
			break
		} else if ret == SDO_UPLOAD_DATA_FULL {
			singleRead = client.fifo.Read(buffer, nil)
			returnBuffer = append(returnBuffer, buffer[0:singleRead]...)
		}
		time.Sleep(time.Duration(timeDifferenceUs) * time.Microsecond)
	}
	singleRead = client.fifo.Read(buffer, nil)
	returnBuffer = append(returnBuffer, buffer[0:singleRead]...)
	return returnBuffer, err
}

// Write to a given index/subindex to node using raw data
// Similar to io.Write
func (client *SDOClient) WriteRaw(nodeId uint8, index uint16, subindex uint8, data any, forceSegmented bool) error {
	bufferPartial := false
	err := client.setupServer(
		uint32(SDO_CLIENT_ID)+uint32(nodeId),
		uint32(SDO_SERVER_ID)+uint32(nodeId),
		nodeId,
	)
	if err != nil {
		return err
	}
	var encoded []byte
	switch val := data.(type) {
	case uint8:
		encoded = []byte{val}
	case int8:
		encoded = []byte{byte(val)}
	case uint16:
		encoded = make([]byte, 2)
		binary.LittleEndian.PutUint16(encoded, val)
	case int16:
		encoded = make([]byte, 2)
		binary.LittleEndian.PutUint16(encoded, uint16(val))
	case uint32:
		encoded = make([]byte, 4)
		binary.LittleEndian.PutUint32(encoded, val)
	case int32:
		encoded = make([]byte, 4)
		binary.LittleEndian.PutUint32(encoded, uint32(val))
	case uint64:
		encoded = make([]byte, 8)
		binary.LittleEndian.PutUint64(encoded, val)
	case int64:
		encoded = make([]byte, 8)
		binary.LittleEndian.PutUint64(encoded, uint64(val))
	case string:
		encoded = []byte(val)
	case float32:
		encoded = make([]byte, 4)
		binary.LittleEndian.PutUint32(encoded, math.Float32bits(val))
	case float64:
		encoded = make([]byte, 8)
		binary.LittleEndian.PutUint64(encoded, math.Float64bits(val))
	case []byte:
		encoded = val
	default:
		return ODR_TYPE_MISMATCH
	}

	err = client.downloadSetup(index, subindex, uint32(len(encoded)), true)
	if err != nil {
		return err
	}
	// Fill buffer
	totalWritten := client.fifo.Write(encoded, nil)
	if totalWritten < len(encoded) {
		bufferPartial = true
	}
	var timeDifferenceUs uint32 = 10000

	for {
		ret, err := client.downloadMain(
			timeDifferenceUs,
			false,
			bufferPartial,
			nil,
			nil,
			forceSegmented,
		)
		if err != nil {
			return err
		} else if ret == SDO_BLOCK_DOWNLOAD_IN_PROGRESS && bufferPartial {
			totalWritten += client.fifo.Write(encoded[totalWritten:], nil)
			if totalWritten == len(encoded) {
				bufferPartial = false
			}
		} else if ret == SDO_SUCCESS {
			break
		}
		time.Sleep(time.Duration(timeDifferenceUs) * time.Microsecond)
	}
	return nil
}

func NewSDOClient(
	busManager *BusManager,
	od *ObjectDictionary,
	nodeId uint8,
	timeoutMs uint32,
	entry1280 *Entry,
) (*SDOClient, error) {

	if busManager == nil {
		return nil, ErrIllegalArgument
	}
	if entry1280 != nil && (entry1280.Index < 0x1280 || entry1280.Index > (0x1280+0x7F)) {
		log.Errorf("[SDO CLIENT] invalid index for sdo client : x%v", entry1280.Index)
		return nil, ErrIllegalArgument
	}
	client := &SDOClient{}
	client.busManager = busManager
	client.od = od
	client.NodeId = nodeId
	client.TimeoutTimeUs = 1000 * timeoutMs
	client.TimeoutTimeBlockTransferUs = client.TimeoutTimeUs
	client.streamer = &Streamer{}
	client.fifo = NewFifo(1000) // At least 127*7

	var nodeIdServer uint8
	var CobIdClientToServer, CobIdServerToClient uint32
	if entry1280 != nil {
		var maxSubindex uint8
		err1 := entry1280.Uint8(0, &maxSubindex)
		err2 := entry1280.Uint32(1, &CobIdClientToServer)
		err3 := entry1280.Uint32(2, &CobIdServerToClient)
		err4 := entry1280.Uint8(3, &nodeIdServer)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || maxSubindex != 3 {
			log.Errorf("[SDO CLIENT] error when reading SDO client parameters in OD 0:%v,1:%v,2:%v,3:%v,max sub-index(should be 3) : %v", err1, err2, err3, err4, maxSubindex)
			return nil, ErrOdParameters
		}
	} else {
		nodeIdServer = 0
	}
	if entry1280 != nil {
		entry1280.AddExtension(client, ReadEntryDefault, WriteEntry1280)
	}
	client.CobIdClientToServer = 0
	client.CobIdServerToClient = 0

	err := client.setupServer(CobIdClientToServer, CobIdServerToClient, nodeIdServer)
	if err != nil {
		return nil, ErrIllegalArgument
	}
	return client, nil
}
