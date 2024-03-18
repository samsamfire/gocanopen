package sdo

import (
	"encoding/binary"
	"errors"
	"math"
	"time"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/internal/crc"
	"github.com/samsamfire/gocanopen/internal/fifo"
	"github.com/samsamfire/gocanopen/pkg/can"
	"github.com/samsamfire/gocanopen/pkg/od"
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
	*canopen.BusManager
	od                         *od.ObjectDictionary
	streamer                   *od.Streamer
	nodeId                     uint8
	txBuffer                   can.Frame
	cobIdClientToServer        uint32
	cobIdServerToClient        uint32
	nodeIdServer               uint8
	valid                      bool
	index                      uint16
	subindex                   uint8
	finished                   bool
	sizeIndicated              uint32
	sizeTransferred            uint32
	state                      SDOState
	timeoutTimeUs              uint32
	timeoutTimer               uint32
	fifo                       *fifo.Fifo
	rxNew                      bool
	response                   SDOResponse
	toggle                     uint8
	timeoutTimeBlockTransferUs uint32
	timeoutTimerBlock          uint32
	blockSequenceNb            uint8
	blockSize                  uint8
	blockNoData                uint8
	blockCRCEnabled            bool
	blockDataUploadLast        [7]byte
	blockCRC                   crc.CRC16
}

func (client *SDOClient) Handle(frame can.Frame) {

	if client.state != SDO_STATE_IDLE && frame.DLC == 8 && (!client.rxNew || frame.Data[0] == 0x80) {
		if frame.Data[0] == 0x80 || (client.state != SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ && client.state != SDO_STATE_UPLOAD_BLK_SUBBLOCK_CRSP) {
			// Copy data in response
			client.response.raw = frame.Data
			client.rxNew = true
		} else if client.state == SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ {
			state := SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ
			seqno := frame.Data[0] & 0x7F
			client.timeoutTimer = 0
			client.timeoutTimerBlock = 0
			// Checks on the Sequence number
			switch {
			case seqno <= client.blockSize && seqno == (client.blockSequenceNb+1):
				client.blockSequenceNb = seqno
				// Is it last segment
				if (frame.Data[0] & 0x80) != 0 {
					copy(client.blockDataUploadLast[:], frame.Data[1:])
					client.finished = true
					state = SDO_STATE_UPLOAD_BLK_SUBBLOCK_CRSP
				} else {
					client.fifo.Write(frame.Data[1:], &client.blockCRC)
					client.sizeTransferred += 7
					if seqno == client.blockSize {
						log.Debugf("[CLIENT][RX][x%x] BLOCK UPLOAD END SUB-BLOCK | x%x:x%x | %v", client.nodeIdServer, client.index, client.subindex, frame.Data)
						state = SDO_STATE_UPLOAD_BLK_SUBBLOCK_CRSP
					}
				}
			case seqno != client.blockSequenceNb && client.blockSequenceNb != 0:
				state = SDO_STATE_UPLOAD_BLK_SUBBLOCK_CRSP
				log.Warnf("Wrong sequence number in rx sub-block. seqno %x, previous %x", seqno, client.blockSequenceNb)
			default:
				log.Warnf("Wrong sequence number in rx ignored. seqno %x, expected %x", seqno, client.blockSequenceNb+1)
			}
			if state != SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ {
				client.rxNew = false
				client.state = state
			}
		}
	}

}

// Setup the client for communication with an SDO server
func (client *SDOClient) setupServer(cobIdClientToServer uint32, cobIdServerToClient uint32, nodeIdServer uint8) error {
	client.state = SDO_STATE_IDLE
	client.rxNew = false
	client.nodeIdServer = nodeIdServer
	// If server is the same don't re-initialize the buffers
	if client.cobIdClientToServer == cobIdClientToServer && client.cobIdServerToClient == cobIdServerToClient {
		return nil
	}
	client.cobIdClientToServer = cobIdClientToServer
	client.cobIdServerToClient = cobIdServerToClient
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
		client.valid = true
	} else {
		CanIdC2S = 0
		CanIdS2C = 0
		client.valid = false
	}
	err := client.Subscribe(uint32(CanIdS2C), 0x7FF, false, client)
	if err != nil {
		return err
	}
	client.txBuffer = can.NewFrame(uint32(CanIdC2S), 0, 8)
	return nil
}

// Start a new download sequence
func (client *SDOClient) downloadSetup(index uint16, subindex uint8, sizeIndicated uint32, blockEnabled bool) error {
	if !client.valid {
		return ErrSDOInvalidArguments
	}
	client.index = index
	client.subindex = subindex
	client.sizeIndicated = sizeIndicated
	client.sizeTransferred = 0
	client.finished = false
	client.timeoutTimer = 0
	client.fifo.Reset()

	switch {
	case client.od != nil && client.nodeIdServer == client.nodeId:
		client.streamer.SetWriter(nil)
		client.state = SDO_STATE_DOWNLOAD_LOCAL_TRANSFER
	case blockEnabled && (sizeIndicated == 0 || sizeIndicated > CO_CONFIG_SDO_CLI_PST):
		client.state = SDO_STATE_DOWNLOAD_BLK_INITIATE_REQ
	default:
		client.state = SDO_STATE_DOWNLOAD_INITIATE_REQ
	}
	client.rxNew = false
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

	if !client.valid {
		abortCode = SDO_ABORT_DEVICE_INCOMPAT
		err = ErrSDOInvalidArguments
	} else if client.state == SDO_STATE_IDLE {
		ret = SDO_SUCCESS
	} else if client.state == SDO_STATE_DOWNLOAD_LOCAL_TRANSFER && !abort {
		ret, err = client.downloadLocal(bufferPartial, nil)
		if ret != SDO_WAITING_LOCAL_TRANSFER {
			client.state = SDO_STATE_IDLE
		} else if timerNextUs != nil {
			*timerNextUs = 0
		}
	} else if client.rxNew {
		response := client.response
		if response.IsAbort() {
			abortCode = response.GetAbortCode()
			log.Debugf("[CLIENT][RX][x%x] SERVER ABORT | x%x:x%x | %v (x%x)", client.nodeIdServer, client.index, client.subindex, abortCode, uint32(response.GetAbortCode()))
			client.state = SDO_STATE_IDLE
			err = abortCode
			// Abort from the client
		} else if abort {
			abortCode = SDO_ABORT_DEVICE_INCOMPAT
			client.state = SDO_STATE_ABORT

		} else if !response.isResponseValid(client.state) {
			log.Warnf("Unexpected response code from server : %x", response.raw[0])
			client.state = SDO_STATE_ABORT
			abortCode = SDO_ABORT_CMD

		} else {
			switch client.state {
			case SDO_STATE_DOWNLOAD_INITIATE_RSP:

				index := response.GetIndex()
				subIndex := response.GetSubindex()
				if index != client.index || subIndex != client.subindex {
					abortCode = SDO_ABORT_PRAM_INCOMPAT
					client.state = SDO_STATE_ABORT
					break
				}
				// Expedited transfer
				if client.finished {
					client.state = SDO_STATE_IDLE
					ret = SDO_SUCCESS
					log.Debugf("[CLIENT][RX][x%x] DOWNLOAD EXPEDITED | x%x:x%x %v", client.nodeIdServer, client.index, client.subindex, response.raw)
					// Segmented transfer
				} else {
					client.toggle = 0x00
					client.state = SDO_STATE_DOWNLOAD_SEGMENT_REQ
					log.Debugf("[CLIENT][RX][x%x] DOWNLOAD SEGMENT | x%x:x%x %v", client.nodeIdServer, client.index, client.subindex, response.raw)
				}

			case SDO_STATE_DOWNLOAD_SEGMENT_RSP:

				// Verify and alternate toggle bit
				toggle := response.GetToggle()
				if toggle != client.toggle {
					abortCode = SDO_ABORT_TOGGLE_BIT
					client.state = SDO_STATE_ABORT
					break
				}
				client.toggle ^= 0x10
				if client.finished {
					client.state = SDO_STATE_IDLE
					ret = SDO_SUCCESS
				} else {
					client.state = SDO_STATE_DOWNLOAD_SEGMENT_REQ
				}
				log.Debugf("[CLIENT][RX][x%x] DOWNLOAD SEGMENT | x%x:x%x %v", client.nodeIdServer, client.index, client.subindex, response.raw)

			case SDO_STATE_DOWNLOAD_BLK_INITIATE_RSP:

				index := response.GetIndex()
				subIndex := response.GetSubindex()
				if index != client.index || subIndex != client.subindex {
					abortCode = SDO_ABORT_PRAM_INCOMPAT
					client.state = SDO_STATE_ABORT
					break
				}
				client.blockCRC = crc.CRC16(0)
				client.blockSize = response.GetBlockSize()
				if client.blockSize < 1 || client.blockSize > 127 {
					client.blockSize = 127
				}
				client.blockSequenceNb = 0
				client.fifo.AltBegin(0)
				client.state = SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ
				log.Debugf("[CLIENT][RX][x%x] DOWNLOAD BLOCK | x%x:x%x %v | blksize %v", client.nodeIdServer, client.index, client.subindex, response.raw, client.blockSize)

			case SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ, SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_RSP:

				if response.GetNumberOfSegments() < client.blockSequenceNb {
					log.Error("Not all segments transferred successfully")
					client.fifo.AltBegin(int(response.raw[1]) * 7)
					client.finished = false

				} else if response.GetNumberOfSegments() > client.blockSequenceNb {
					abortCode = SDO_ABORT_CMD
					client.state = SDO_STATE_ABORT
					break
				}
				client.fifo.AltFinish(&client.blockCRC)
				if client.finished {
					client.state = SDO_STATE_DOWNLOAD_BLK_END_REQ
				} else {
					client.blockSize = response.raw[2]
					client.blockSequenceNb = 0
					client.fifo.AltBegin(0)
					client.state = SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ
				}

			case SDO_STATE_DOWNLOAD_BLK_END_RSP:

				client.state = SDO_STATE_IDLE
				ret = SDO_SUCCESS

			}

			client.timeoutTimer = 0
			timeDifferenceUs = 0
			client.rxNew = false

		}

	} else if abort {
		abortCode = SDO_ABORT_DEVICE_INCOMPAT
		client.state = SDO_STATE_ABORT
	}

	if ret == SDO_WAITING_RESPONSE {
		if client.timeoutTimer < client.timeoutTimeUs {
			client.timeoutTimer += timeDifferenceUs
		}
		if client.timeoutTimer >= client.timeoutTimeUs {
			abortCode = SDO_ABORT_TIMEOUT
			client.state = SDO_STATE_ABORT
		} else if timerNextUs != nil {
			diff := client.timeoutTimeUs - client.timeoutTimer
			if *timerNextUs > diff {
				*timerNextUs = diff
			}
		}
	}

	if ret == SDO_WAITING_RESPONSE {
		client.txBuffer.Data = [8]byte{0}
		switch client.state {
		case SDO_STATE_DOWNLOAD_INITIATE_REQ:
			abortCode = client.downloadInitiate(forceSegmented)
			if abortCode != nil {
				client.state = SDO_STATE_IDLE
				err = abortCode
				break
			}
			client.state = SDO_STATE_DOWNLOAD_INITIATE_RSP

		case SDO_STATE_DOWNLOAD_SEGMENT_REQ:
			abortCode = client.downloadSegment(bufferPartial)
			if abortCode != nil {
				client.state = SDO_STATE_ABORT
				err = abortCode
				break
			}
			client.state = SDO_STATE_DOWNLOAD_SEGMENT_RSP

		case SDO_STATE_DOWNLOAD_BLK_INITIATE_REQ:
			client.downloadBlockInitiate()
			client.state = SDO_STATE_DOWNLOAD_BLK_INITIATE_RSP

		case SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ:
			abortCode = client.downloadBlock(bufferPartial, timerNextUs)
			if abortCode != nil {
				client.state = SDO_STATE_ABORT
			}

		case SDO_STATE_DOWNLOAD_BLK_END_REQ:
			client.downloadBlockEnd()
			client.state = SDO_STATE_DOWNLOAD_BLK_END_RSP

		default:
			break

		}

	}

	if ret == SDO_WAITING_RESPONSE {

		switch client.state {
		case SDO_STATE_ABORT:
			client.abort(abortCode.(SDOAbortCode))
			err = abortCode
			client.state = SDO_STATE_IDLE
		case SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_REQ:
			ret = SDO_BLOCK_DOWNLOAD_IN_PROGRESS
		}
	}

	if sizeTransferred != nil {
		*sizeTransferred = client.sizeTransferred
	}
	return ret, err
}

// Helper function for starting download
// Valid for expedited or segmented transfer
func (client *SDOClient) downloadInitiate(forceSegmented bool) error {

	client.txBuffer.Data[0] = 0x20
	client.txBuffer.Data[1] = byte(client.index)
	client.txBuffer.Data[2] = byte(client.index >> 8)
	client.txBuffer.Data[3] = client.subindex

	count := uint32(client.fifo.GetOccupied())
	if (client.sizeIndicated == 0 && count <= 4) || (client.sizeIndicated > 0 && client.sizeIndicated <= 4) && !forceSegmented {
		client.txBuffer.Data[0] |= 0x02
		// Check length
		if count == 0 || (client.sizeIndicated > 0 && client.sizeIndicated != count) {
			client.state = SDO_STATE_IDLE
			return SDO_ABORT_TYPE_MISMATCH
		}
		if client.sizeIndicated > 0 {
			client.txBuffer.Data[0] |= byte(0x01 | ((4 - count) << 2))
		}
		// Copy the data in queue and add the count
		count = uint32(client.fifo.Read(client.txBuffer.Data[4:], nil))
		client.sizeTransferred = count
		client.finished = true
		log.Debugf("[CLIENT][TX][x%x] DOWNLOAD EXPEDITED | x%x:x%x %v", client.nodeIdServer, client.index, client.subindex, client.txBuffer.Data)

	} else {
		/* segmented transfer, indicate data size */
		if client.sizeIndicated > 0 {
			size := client.sizeIndicated
			client.txBuffer.Data[0] |= 0x01
			binary.LittleEndian.PutUint32(client.txBuffer.Data[4:], size)
		}
		log.Debugf("[CLIENT][TX][x%x] DOWNLOAD SEGMENT | x%x:x%x %v", client.nodeIdServer, client.index, client.subindex, client.txBuffer.Data)
	}
	client.timeoutTimer = 0
	client.Send(client.txBuffer)
	return nil

}

// Write value to OD locally
func (client *SDOClient) downloadLocal(bufferPartial bool, timerNextUs *uint32) (ret uint8, abortCode error) {
	var err error

	if client.streamer.Writer() == nil {
		log.Debugf("[CLIENT][TX][x%x] LOCAL TRANSFER WRITE | x%x:x%x", client.nodeId, client.index, client.subindex)
		streamer, err := od.NewStreamer(client.od.Index(client.index), client.subindex, false)
		if streamer != nil {
			client.streamer = streamer
		}
		odErr, ok := err.(od.ODR)
		if err != nil {
			if !ok {
				return 0, SDO_ABORT_GENERAL
			}
			return 0, ConvertOdToSdoAbort(odErr)
		} else if !client.streamer.CheckHasAttribute(od.ATTRIBUTE_SDO_RW) {
			return 0, SDO_ABORT_UNSUPPORTED_ACCESS
		} else if !client.streamer.CheckHasAttribute(od.ATTRIBUTE_SDO_W) {
			return 0, SDO_ABORT_READONLY
		} else if client.streamer.Writer() == nil {
			return 0, SDO_ABORT_DEVICE_INCOMPAT
		}
	}
	// If still nil, return
	if client.streamer.Writer() == nil {
		return
	}

	buffer := make([]byte, SDO_CLI_BUFFER_SIZE+2)
	count := client.fifo.Read(buffer, nil)
	client.sizeTransferred += uint32(count)
	// No data error
	if count == 0 {
		abortCode = SDO_ABORT_DEVICE_INCOMPAT
		// Size transferred is too large
	} else if client.sizeIndicated > 0 && client.sizeTransferred > client.sizeIndicated {
		client.sizeTransferred -= uint32(count)
		abortCode = SDO_ABORT_DATA_LONG
		// Size transferred is too small (check on last call)
	} else if !bufferPartial && client.sizeIndicated > 0 && client.sizeTransferred < client.sizeIndicated {
		abortCode = SDO_ABORT_DATA_SHORT
		// Last part of data !
	} else if !bufferPartial {
		odVarSize := client.streamer.DataLength()
		// Special case for strings where the downloaded data may be shorter (nul character can be omitted)
		if client.streamer.CheckHasAttribute(od.ATTRIBUTE_STR) && odVarSize == 0 || client.sizeTransferred < uint32(odVarSize) {
			count += 1
			buffer[count] = 0
			client.sizeTransferred += 1
			if odVarSize == 0 || odVarSize > client.sizeTransferred {
				count += 1
				buffer[count] = 0
				client.sizeTransferred += 1
			}
			client.streamer.SetDataLength(client.sizeTransferred)
		} else if odVarSize == 0 {
			client.streamer.SetDataLength(client.sizeTransferred)
		} else if client.sizeTransferred > uint32(odVarSize) {
			abortCode = SDO_ABORT_DATA_LONG
		} else if client.sizeTransferred < uint32(odVarSize) {
			abortCode = SDO_ABORT_DATA_SHORT
		}
	}
	if abortCode == nil {
		_, err = client.streamer.Write(buffer[:count])
		odErr, ok := err.(od.ODR)
		if err != nil && odErr != od.ODR_PARTIAL {
			if !ok {
				return 0, SDO_ABORT_GENERAL
			}
			return 0, ConvertOdToSdoAbort(odErr)
		} else if bufferPartial && err == nil {
			return 0, SDO_ABORT_DATA_LONG
		} else if !bufferPartial {
			// Error if not written completely but download end
			if odErr == od.ODR_PARTIAL {
				return 0, SDO_ABORT_DATA_SHORT
			} else {
				return SDO_SUCCESS, nil
			}
		} else {
			return SDO_WAITING_LOCAL_TRANSFER, nil
		}
	}

	return 0, abortCode
}

// Helper function for downloading a segement of segmented transfer
func (client *SDOClient) downloadSegment(bufferPartial bool) error {
	// Fill data part
	count := uint32(client.fifo.Read(client.txBuffer.Data[1:], nil))
	client.sizeTransferred += count
	if client.sizeIndicated > 0 && client.sizeTransferred > client.sizeIndicated {
		client.sizeTransferred -= count
		return SDO_ABORT_DATA_LONG
	}

	// Command specifier
	client.txBuffer.Data[0] = uint8(uint32(client.toggle) | ((7 - count) << 1))
	if client.fifo.GetOccupied() == 0 && !bufferPartial {
		if client.sizeIndicated > 0 && client.sizeTransferred < client.sizeIndicated {
			return SDO_ABORT_DATA_SHORT
		}
		client.txBuffer.Data[0] |= 0x01
		client.finished = true
	}

	client.timeoutTimer = 0
	log.Debugf("[CLIENT][TX][x%x] DOWNLOAD SEGMENT | x%x:x%x %v", client.nodeIdServer, client.index, client.subindex, client.txBuffer.Data)
	client.Send(client.txBuffer)
	return nil
}

// Helper function for initiating a block download
func (client *SDOClient) downloadBlockInitiate() error {
	client.txBuffer.Data[0] = 0xC4
	client.txBuffer.Data[1] = byte(client.index)
	client.txBuffer.Data[2] = byte(client.index >> 8)
	client.txBuffer.Data[3] = client.subindex
	if client.sizeIndicated > 0 {
		client.txBuffer.Data[0] |= 0x02
		binary.LittleEndian.PutUint32(client.txBuffer.Data[4:], client.sizeIndicated)
	}
	client.timeoutTimer = 0
	client.Send(client.txBuffer)
	return nil

}

// Helper function for downloading a sub-block
func (client *SDOClient) downloadBlock(bufferPartial bool, timerNext *uint32) error {
	if client.fifo.AltGetOccupied() < 7 && bufferPartial {
		// No data yet
		return nil
	}
	client.blockSequenceNb++
	client.txBuffer.Data[0] = client.blockSequenceNb
	count := uint32(client.fifo.AltRead(client.txBuffer.Data[1:]))
	client.blockNoData = uint8(7 - count)
	client.sizeTransferred += count
	if client.sizeIndicated > 0 && client.sizeTransferred > client.sizeIndicated {
		client.sizeTransferred -= count
		return SDO_ABORT_DATA_LONG
	}
	if client.fifo.AltGetOccupied() == 0 && !bufferPartial {
		if client.sizeIndicated > 0 && client.sizeTransferred < client.sizeIndicated {
			return SDO_ABORT_DATA_SHORT
		}
		client.txBuffer.Data[0] |= 0x80
		client.finished = true
		client.state = SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_RSP
	} else if client.blockSequenceNb >= client.blockSize {
		client.state = SDO_STATE_DOWNLOAD_BLK_SUBBLOCK_RSP
	} else if timerNext != nil {
		*timerNext = 0
	}
	client.timeoutTimer = 0
	client.Send(client.txBuffer)
	return nil

}

// Helper function for end of block
func (client *SDOClient) downloadBlockEnd() {
	client.txBuffer.Data[0] = 0xC1 | (client.blockNoData << 2)
	client.txBuffer.Data[1] = byte(client.blockCRC)
	client.txBuffer.Data[2] = byte(client.blockCRC >> 8)
	client.timeoutTimer = 0
	client.Send(client.txBuffer)
}

// Create & send abort on bus
func (client *SDOClient) abort(abortCode SDOAbortCode) {
	code := uint32(abortCode)
	client.txBuffer.Data[0] = 0x80
	client.txBuffer.Data[1] = uint8(client.index)
	client.txBuffer.Data[2] = uint8(client.index >> 8)
	client.txBuffer.Data[3] = client.subindex
	binary.LittleEndian.PutUint32(client.txBuffer.Data[4:], code)
	log.Warnf("[CLIENT][TX][x%x] CLIENT ABORT | x%x:x%x | %v (x%x)", client.nodeIdServer, client.index, client.subindex, abortCode, code)
	client.Send(client.txBuffer)

}

/////////////////////////////////////
////////////SDO UPLOAD///////////////
/////////////////////////////////////

func (client *SDOClient) uploadSetup(index uint16, subindex uint8, blockEnabled bool) error {
	if !client.valid {
		return ErrSDOInvalidArguments
	}
	client.index = index
	client.subindex = subindex
	client.sizeIndicated = 0
	client.sizeTransferred = 0
	client.finished = false
	client.fifo.Reset()
	if client.od != nil && client.nodeIdServer == client.nodeId {
		client.streamer.SetReader(nil)
		client.state = SDO_STATE_UPLOAD_LOCAL_TRANSFER
	} else if blockEnabled {
		client.state = SDO_STATE_UPLOAD_BLK_INITIATE_REQ
	} else {
		client.state = SDO_STATE_UPLOAD_INITIATE_REQ
	}
	client.rxNew = false
	return nil
}

func (client *SDOClient) uploadLocal() (ret uint8, err error) {

	if client.streamer.Reader() == nil {
		log.Debugf("[CLIENT][RX][x%x] LOCAL TRANSFER READ | x%x:x%x", client.nodeId, client.index, client.subindex)
		streamer, err := od.NewStreamer(client.od.Index(client.index), client.subindex, false)
		if streamer != nil {
			client.streamer = streamer
		}
		odErr, ok := err.(od.ODR)
		if err != nil {
			if !ok {
				return 0, SDO_ABORT_GENERAL
			}
			return 0, ConvertOdToSdoAbort(odErr)
		} else if !client.streamer.CheckHasAttribute(od.ATTRIBUTE_SDO_RW) {
			return 0, SDO_ABORT_UNSUPPORTED_ACCESS
		} else if !client.streamer.CheckHasAttribute(od.ATTRIBUTE_SDO_R) {
			return 0, SDO_ABORT_WRITEONLY
		} else if client.streamer.Reader() == nil {
			return 0, SDO_ABORT_DEVICE_INCOMPAT
		}
	}
	countFifo := client.fifo.GetSpace()
	if countFifo == 0 {
		ret = SDO_UPLOAD_DATA_FULL
	} else if client.streamer.Reader() != nil {
		countData := client.streamer.DataLength()
		countBuffer := uint32(0)
		countRead := 0
		if countData > 0 && countData <= uint32(countFifo) {
			countBuffer = countData
		} else {
			countBuffer = uint32(countFifo)
		}
		buffer := make([]byte, SDO_CLI_BUFFER_SIZE+1)
		countRead, err = client.streamer.Read(buffer[:countBuffer])
		odErr, ok := err.(od.ODR)
		if err != nil && err != od.ODR_PARTIAL {
			if !ok {
				return 0, SDO_ABORT_GENERAL
			}
			return 0, ConvertOdToSdoAbort(odErr)
		} else {
			if countRead > 0 && client.streamer.CheckHasAttribute(od.ATTRIBUTE_STR) {
				buffer[countRead] = 0
				countStr := 0
				for i, v := range buffer {
					if v == 0 {
						countStr = i
						break
					}
				}
				if countStr == 0 {
					countStr = 1
				}
				if countStr < countRead {
					countRead = countStr
					odErr = od.ODR_OK
					client.streamer.SetDataLength(client.sizeTransferred + uint32(countRead))
				}
			}
			client.fifo.Write(buffer[:countRead], nil)
			client.sizeTransferred += uint32(countRead)
			client.sizeIndicated = client.streamer.DataLength()
			if client.sizeIndicated > 0 && client.sizeTransferred > client.sizeIndicated {
				err = SDO_ABORT_DATA_LONG
			} else if odErr == od.ODR_OK {
				if client.sizeIndicated > 0 && client.sizeTransferred < client.sizeIndicated {
					err = SDO_ABORT_DATA_SHORT
				}
			} else {
				ret = SDO_WAITING_LOCAL_TRANSFER
			}
		}

	}
	return ret, err
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

	if !client.valid {
		abortCode = SDO_ABORT_DEVICE_INCOMPAT
		err = ErrSDOInvalidArguments
	} else if client.state == SDO_STATE_IDLE {
		ret = SDO_SUCCESS
	} else if client.state == SDO_STATE_UPLOAD_LOCAL_TRANSFER && !abort {
		ret, err = client.uploadLocal()
		if ret != SDO_UPLOAD_DATA_FULL && ret != SDO_WAITING_LOCAL_TRANSFER {
			client.state = SDO_STATE_IDLE
		} else if timerNextUs != nil {
			*timerNextUs = 0
		}
	} else if client.rxNew {
		response := client.response
		if response.IsAbort() {
			abortCode = response.GetAbortCode()
			log.Debugf("[CLIENT][RX][x%x] SERVER ABORT | x%x:x%x | %v (x%x)", client.nodeIdServer, client.index, client.subindex, abortCode, uint32(response.GetAbortCode()))
			client.state = SDO_STATE_IDLE
			err = abortCode
		} else if abort {
			abortCode = SDO_ABORT_DEVICE_INCOMPAT
			client.state = SDO_STATE_ABORT

		} else if !response.isResponseValid(client.state) {
			log.Warnf("Unexpected response code from server : %x", response.raw[0])
			client.state = SDO_STATE_ABORT
			abortCode = SDO_ABORT_CMD

		} else {
			switch client.state {
			case SDO_STATE_UPLOAD_INITIATE_RSP:
				index := response.GetIndex()
				subIndex := response.GetSubindex()
				if index != client.index || subIndex != client.subindex {
					abortCode = SDO_ABORT_PRAM_INCOMPAT
					client.state = SDO_STATE_ABORT
					break
				}
				if (response.raw[0] & 0x02) != 0 {
					// Expedited
					var count uint32 = 4
					// Size indicated ?
					if (response.raw[0] & 0x01) != 0 {
						count -= uint32((response.raw[0] >> 2) & 0x03)
					}
					client.fifo.Write(response.raw[4:4+count], nil)
					client.sizeTransferred = count
					client.state = SDO_STATE_IDLE
					ret = SDO_SUCCESS
					log.Debugf("[CLIENT][RX][x%x] UPLOAD EXPEDITED | x%x:x%x %v", client.nodeIdServer, client.index, client.subindex, response.raw)
					// Segmented
				} else {
					// Size indicated ?
					if (response.raw[0] & 0x01) != 0 {
						client.sizeIndicated = binary.LittleEndian.Uint32(response.raw[4:])
					}
					client.toggle = 0
					client.state = SDO_STATE_UPLOAD_SEGMENT_REQ
					log.Debugf("[CLIENT][RX][x%x] UPLOAD SEGMENT | x%x:x%x %v", client.nodeIdServer, client.index, client.subindex, response.raw)

				}

			case SDO_STATE_UPLOAD_SEGMENT_RSP:
				// Verify and alternate toggle bit
				log.Debugf("[CLIENT][RX][x%x] UPLOAD SEGMENT | x%x:x%x %v", client.nodeIdServer, client.index, client.subindex, response.raw)
				toggle := response.GetToggle()
				if toggle != client.toggle {
					abortCode = SDO_ABORT_TOGGLE_BIT
					client.state = SDO_STATE_ABORT
					break
				}
				client.toggle ^= 0x10
				count := 7 - (response.raw[0]>>1)&0x07
				countWr := client.fifo.Write(response.raw[1:1+count], nil)
				client.sizeTransferred += uint32(countWr)
				// Check enough space if fifo
				if countWr != int(count) {
					abortCode = SDO_ABORT_OUT_OF_MEM
					client.state = SDO_STATE_ABORT
					break
				}

				// Check size uploaded
				if client.sizeIndicated > 0 && client.sizeTransferred > client.sizeIndicated {
					abortCode = SDO_ABORT_DATA_LONG
					client.state = SDO_STATE_ABORT
					break
				}

				// No more segments ?
				if (response.raw[0] & 0x01) != 0 {
					// Check size uploaded
					if client.sizeIndicated > 0 && client.sizeTransferred < client.sizeIndicated {
						abortCode = SDO_ABORT_DATA_LONG
						client.state = SDO_STATE_ABORT
					} else {
						client.state = SDO_STATE_IDLE
						ret = SDO_SUCCESS
					}
				} else {
					client.state = SDO_STATE_UPLOAD_SEGMENT_REQ
				}

			case SDO_STATE_UPLOAD_BLK_INITIATE_RSP:

				index := response.GetIndex()
				subindex := response.GetSubindex()
				if index != client.index || subindex != client.subindex {
					abortCode = SDO_ABORT_PRAM_INCOMPAT
					client.state = SDO_STATE_ABORT
					break
				}
				// Block is supported
				if (response.raw[0] & 0xF9) == 0xC0 {
					client.blockCRCEnabled = response.IsCRCEnabled()
					if (response.raw[0] & 0x02) != 0 {
						client.sizeIndicated = uint32(response.GetBlockSize())
					}
					client.state = SDO_STATE_UPLOAD_BLK_INITIATE_REQ2
					log.Debugf("[CLIENT][RX][x%x] BLOCK UPLOAD INIT | x%x:x%x | crc enabled : %v expected size : %v | %v",
						client.nodeIdServer,
						client.index,
						client.subindex,
						response.IsCRCEnabled(),
						client.sizeIndicated,
						response.raw,
					)

					// Switch to normal transfer
				} else if (response.raw[0] & 0xF0) == 0x40 {
					if (response.raw[0] & 0x02) != 0 {
						// Expedited
						count := 4
						if (response.raw[0] & 0x01) != 0 {
							count -= (int(response.raw[0]>>2) & 0x03)
						}
						client.fifo.Write(response.raw[4:4+count], nil)
						client.sizeTransferred = uint32(count)
						client.state = SDO_STATE_IDLE
						ret = SDO_SUCCESS
						log.Debugf("[CLIENT][RX][x%x] BLOCK UPLOAD SWITCHING EXPEDITED | x%x:x%x %v", client.nodeIdServer, client.index, client.subindex, response.raw)

					} else {
						if (response.raw[0] & 0x01) != 0 {
							client.sizeIndicated = uint32(response.GetBlockSize())
						}
						client.toggle = 0x00
						client.state = SDO_STATE_UPLOAD_SEGMENT_REQ
						log.Debugf("[CLIENT][RX][x%x] BLOCK UPLOAD SWITCHING SEGMENTED | x%x:x%x %v", client.nodeIdServer, client.index, client.subindex, response.raw)
					}

				}
			case SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ:
				// Handled directly in Rx callback
				break

			case SDO_STATE_UPLOAD_BLK_END_SREQ:
				// Get number of data bytes in last segment, that do not
				// contain data. Then copy remaining data into fifo
				noData := (response.raw[0] >> 2) & 0x07
				client.fifo.Write(client.blockDataUploadLast[:7-noData], &client.blockCRC)
				client.sizeTransferred += uint32(7 - noData)

				if client.sizeIndicated > 0 && client.sizeTransferred > client.sizeIndicated {
					abortCode = SDO_ABORT_DATA_LONG
					client.state = SDO_STATE_ABORT
					break
				} else if client.sizeIndicated > 0 && client.sizeTransferred < client.sizeIndicated {
					abortCode = SDO_ABORT_DATA_SHORT
					client.state = SDO_STATE_ABORT
					break
				}
				if client.blockCRCEnabled {
					crcServer := crc.CRC16(binary.LittleEndian.Uint16(response.raw[1:3]))
					if crcServer != client.blockCRC {
						abortCode = SDO_ABORT_CRC
						client.state = SDO_STATE_ABORT
						break
					}
				}
				client.state = SDO_STATE_UPLOAD_BLK_END_CRSP
				log.Debugf("[CLIENT][RX][x%x] BLOCK UPLOAD END | x%x:x%x %v", client.nodeIdServer, client.index, client.subindex, response.raw)

			default:
				abortCode = SDO_ABORT_CMD
				client.state = SDO_STATE_ABORT
			}

		}
		client.timeoutTimer = 0
		timeDifferenceUs = 0
		client.rxNew = false
	} else if abort {
		abortCode = SDO_ABORT_DEVICE_INCOMPAT
		client.state = SDO_STATE_ABORT
	}

	if ret == SDO_WAITING_RESPONSE {
		if client.timeoutTimer < client.timeoutTimeUs {
			client.timeoutTimer += timeDifferenceUs
		}
		if client.timeoutTimer >= client.timeoutTimeUs {
			if client.state == SDO_STATE_UPLOAD_SEGMENT_REQ || client.state == SDO_STATE_UPLOAD_BLK_SUBBLOCK_CRSP {
				abortCode = SDO_ABORT_GENERAL
			} else {
				abortCode = SDO_ABORT_TIMEOUT
			}
			client.state = SDO_STATE_ABORT

		} else if timerNextUs != nil {
			diff := client.timeoutTimeUs - client.timeoutTimer
			if *timerNextUs > diff {
				*timerNextUs = diff
			}
		}
		// Timeout for subblocks
		if client.state == SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ {
			if client.timeoutTimerBlock < client.timeoutTimeBlockTransferUs {
				client.timeoutTimerBlock += timeDifferenceUs
			}
			if client.timeoutTimerBlock >= client.timeoutTimeBlockTransferUs {
				client.state = SDO_STATE_UPLOAD_BLK_SUBBLOCK_CRSP
				client.rxNew = false
			} else if timerNextUs != nil {
				diff := client.timeoutTimeBlockTransferUs - client.timeoutTimerBlock
				if *timerNextUs > diff {
					*timerNextUs = diff
				}
			}
		}
	}

	if ret == SDO_WAITING_RESPONSE {
		client.txBuffer.Data = [8]byte{0}
		switch client.state {
		case SDO_STATE_UPLOAD_INITIATE_REQ:
			client.txBuffer.Data[0] = 0x40
			client.txBuffer.Data[1] = byte(client.index)
			client.txBuffer.Data[2] = byte(client.index >> 8)
			client.txBuffer.Data[3] = client.subindex
			client.timeoutTimer = 0
			client.Send(client.txBuffer)
			client.state = SDO_STATE_UPLOAD_INITIATE_RSP
			log.Debugf("[CLIENT][TX][x%x] UPLOAD SEGMENT | x%x:x%x %v", client.nodeIdServer, client.index, client.subindex, client.txBuffer.Data)

		case SDO_STATE_UPLOAD_SEGMENT_REQ:
			if client.fifo.GetSpace() < 7 {
				ret = SDO_UPLOAD_DATA_FULL
				break
			}
			client.txBuffer.Data[0] = 0x60 | client.toggle
			client.timeoutTimer = 0
			client.Send(client.txBuffer)
			client.state = SDO_STATE_UPLOAD_SEGMENT_RSP
			log.Debugf("[CLIENT][TX][x%x] UPLOAD SEGMENT | x%x:x%x %v", client.nodeIdServer, client.index, client.subindex, client.txBuffer.Data)

		case SDO_STATE_UPLOAD_BLK_INITIATE_REQ:
			client.txBuffer.Data[0] = 0xA4
			client.txBuffer.Data[1] = byte(client.index)
			client.txBuffer.Data[2] = byte(client.index >> 8)
			client.txBuffer.Data[3] = client.subindex
			// Calculate number of block segments from free space
			count := client.fifo.GetSpace() / 7
			if count >= 127 {
				count = 127
			} else if count == 0 {
				abortCode = SDO_ABORT_OUT_OF_MEM
				client.state = SDO_STATE_ABORT
				break
			}
			client.blockSize = uint8(count)
			client.txBuffer.Data[4] = client.blockSize
			client.txBuffer.Data[5] = CO_CONFIG_SDO_CLI_PST
			client.timeoutTimer = 0
			client.Send(client.txBuffer)
			client.state = SDO_STATE_UPLOAD_BLK_INITIATE_RSP
			log.Debugf("[CLIENT][TX][x%x] BLOCK UPLOAD INITIATE | x%x:x%x %v blksize : %v", client.nodeIdServer, client.index, client.subindex, client.txBuffer.Data, client.blockSize)

		case SDO_STATE_UPLOAD_BLK_INITIATE_REQ2:
			client.txBuffer.Data[0] = 0xA3
			client.timeoutTimer = 0
			client.timeoutTimerBlock = 0
			client.blockSequenceNb = 0
			client.blockCRC = crc.CRC16(0)
			client.state = SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ
			client.rxNew = false
			client.Send(client.txBuffer)

		case SDO_STATE_UPLOAD_BLK_SUBBLOCK_CRSP:
			client.txBuffer.Data[0] = 0xA2
			client.txBuffer.Data[1] = client.blockSequenceNb
			transferShort := client.blockSequenceNb != client.blockSize
			seqnoStart := client.blockSequenceNb
			if client.finished {
				client.state = SDO_STATE_UPLOAD_BLK_END_SREQ
			} else {
				// Check size too large
				if client.sizeIndicated > 0 && client.sizeTransferred > client.sizeIndicated {
					abortCode = SDO_ABORT_DATA_LONG
					client.state = SDO_STATE_ABORT
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
				client.blockSize = uint8(count)
				client.blockSequenceNb = 0
				client.state = SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ
				client.rxNew = false
			}
			client.txBuffer.Data[2] = client.blockSize
			client.timeoutTimerBlock = 0
			client.Send(client.txBuffer)
			if transferShort && !client.finished {
				log.Warnf("sub-block restarted: seqnoPrev=%v, blksize=%v", seqnoStart, client.blockSize)
			}

		case SDO_STATE_UPLOAD_BLK_END_CRSP:
			client.txBuffer.Data[0] = 0xA1
			client.Send(client.txBuffer)
			client.state = SDO_STATE_IDLE
			ret = SDO_SUCCESS

		default:
			break
		}

	}

	if ret == SDO_WAITING_RESPONSE {
		switch client.state {
		case SDO_STATE_ABORT:
			client.abort(abortCode.(SDOAbortCode))
			err = abortCode
			client.state = SDO_STATE_IDLE
		case SDO_STATE_UPLOAD_BLK_SUBBLOCK_SREQ:
			ret = SDO_BLOCK_UPLOAD_IN_PROGRESS
		}
	}
	if sizeIndicated != nil {
		*sizeIndicated = client.sizeIndicated
	}

	if sizeTransferred != nil {
		*sizeTransferred = client.sizeTransferred
	}

	return ret, err

}

func NewSDOClient(
	bm *canopen.BusManager,
	odict *od.ObjectDictionary,
	nodeId uint8,
	timeoutMs uint32,
	entry1280 *od.Entry,
) (*SDOClient, error) {

	if bm == nil {
		return nil, canopen.ErrIllegalArgument
	}
	if entry1280 != nil && (entry1280.Index < 0x1280 || entry1280.Index > (0x1280+0x7F)) {
		log.Errorf("[SDO CLIENT] invalid index for sdo client : x%v", entry1280.Index)
		return nil, canopen.ErrIllegalArgument
	}
	client := &SDOClient{BusManager: bm}
	client.od = odict
	client.nodeId = nodeId
	client.timeoutTimeUs = 1000 * timeoutMs
	client.timeoutTimeBlockTransferUs = client.timeoutTimeUs
	client.streamer = &od.Streamer{}
	client.fifo = fifo.NewFifo(1000) // At least 127*7

	var nodeIdServer uint8
	var CobIdClientToServer, CobIdServerToClient uint32
	var err2, err3, err4 error
	if entry1280 != nil {
		maxSubindex, err1 := entry1280.Uint8(0)
		CobIdClientToServer, err2 = entry1280.Uint32(1)
		CobIdServerToClient, err3 = entry1280.Uint32(2)
		nodeIdServer, err4 = entry1280.Uint8(3)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || maxSubindex != 3 {
			log.Errorf("[SDO CLIENT] error when reading SDO client parameters in OD 0:%v,1:%v,2:%v,3:%v,max sub-index(should be 3) : %v", err1, err2, err3, err4, maxSubindex)
			return nil, canopen.ErrOdParameters
		}
	} else {
		nodeIdServer = 0
	}
	if entry1280 != nil {
		entry1280.AddExtension(client, od.ReadEntryDefault, writeEntry1280)
	}
	client.cobIdClientToServer = 0
	client.cobIdServerToClient = 0

	err := client.setupServer(CobIdClientToServer, CobIdServerToClient, nodeIdServer)
	if err != nil {
		return nil, canopen.ErrIllegalArgument
	}
	return client, nil
}

// Set read / write to local OD
// This is equivalent as reading with a node id set to 0
func (client *SDOClient) SetNoId() {
	client.nodeId = 0
}

// Read a given index/subindex from node into data
// Similar to io.Read
func (client *SDOClient) ReadRaw(nodeId uint8, index uint16, subindex uint8, data []byte) (int, error) {
	var timeDifferenceUs uint32 = 10000

	err := client.setupServer(
		uint32(CLIENT_ID)+uint32(nodeId),
		uint32(SERVER_ID)+uint32(nodeId),
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

// Read everything from a given index/subindex from node and return all bytes
// Similar to io.ReadAll
func (client *SDOClient) ReadAll(nodeId uint8, index uint16, subindex uint8) ([]byte, error) {
	var timeDifferenceUs uint32 = 10000
	err := client.setupServer(
		uint32(CLIENT_ID)+uint32(nodeId),
		uint32(SERVER_ID)+uint32(nodeId),
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

func (client *SDOClient) ReadUint8(nodeId uint8, index uint16, subindex uint8) (uint8, error) {
	buf := make([]byte, 1)
	n, err := client.ReadRaw(nodeId, index, subindex, buf)
	if err != nil {
		return 0, err
	} else if n != 1 {
		return 0, od.ODR_TYPE_MISMATCH
	}
	return buf[0], nil
}

func (client *SDOClient) ReadUint16(nodeId uint8, index uint16, subindex uint8) (uint16, error) {
	buf := make([]byte, 2)
	n, err := client.ReadRaw(nodeId, index, subindex, buf)
	if err != nil {
		return 0, err
	} else if n != 2 {
		return 0, od.ODR_TYPE_MISMATCH
	}
	return binary.LittleEndian.Uint16(buf), nil
}

func (client *SDOClient) ReadUint32(nodeId uint8, index uint16, subindex uint8) (uint32, error) {
	buf := make([]byte, 4)
	n, err := client.ReadRaw(nodeId, index, subindex, buf)
	if err != nil {
		return 0, err
	} else if n != 4 {
		return 0, od.ODR_TYPE_MISMATCH
	}
	return binary.LittleEndian.Uint32(buf), nil
}

func (client *SDOClient) ReadUint64(nodeId uint8, index uint16, subindex uint8) (uint64, error) {
	buf := make([]byte, 8)
	n, err := client.ReadRaw(nodeId, index, subindex, buf)
	if err != nil {
		return 0, err
	} else if n != 8 {
		return 0, od.ODR_TYPE_MISMATCH
	}
	return binary.LittleEndian.Uint64(buf), nil
}

// Write to a given index/subindex to node using raw data
// Similar to io.Write
func (client *SDOClient) WriteRaw(nodeId uint8, index uint16, subindex uint8, data any, forceSegmented bool) error {
	bufferPartial := false
	err := client.setupServer(
		uint32(CLIENT_ID)+uint32(nodeId),
		uint32(SERVER_ID)+uint32(nodeId),
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
		return od.ODR_TYPE_MISMATCH
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

// Set timeout for SDO non block transfers
func (client *SDOClient) SetTimeout(timeoutMs uint32) {
	client.timeoutTimeUs = timeoutMs * 1000
}

// Set timeout for SDO block transfers
func (client *SDOClient) SetTimeoutBlockTransfer(timeoutMs uint32) {
	client.timeoutTimeBlockTransferUs = timeoutMs * 1000
}
