package sdo

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"sync"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/internal/crc"
	"github.com/samsamfire/gocanopen/internal/fifo"
	"github.com/samsamfire/gocanopen/pkg/od"
)

const (
	waitingLocalTransfer    uint8 = 6 // Waiting in client local transfer.
	uploadDataFull          uint8 = 5 // Data buffer is full.SDO client: data must be read before next upload cycle begins.
	transmitBufferFull      uint8 = 4 // CAN transmit buffer is full. Waiting.
	blockDownloadInProgress uint8 = 3 // Block download is in progress. Sending train of messages.
	blockUploadInProgress   uint8 = 2 // Block upload is in progress. Receiving train of messages.SDO client: Data must not be read in this state.
	waitingResponse         uint8 = 1 // Waiting server or client response.
	success                 uint8 = 0 // Success, end of communication. SDO client: uploaded data must be read.
)

type SDOClient struct {
	*canopen.BusManager
	logger                     *slog.Logger
	mu                         sync.Mutex
	od                         *od.ObjectDictionary
	streamer                   *od.Streamer
	rw                         *sdoRawReadWriter
	localBuffer                []byte
	nodeId                     uint8
	txBuffer                   canopen.Frame
	cobIdClientToServer        uint32
	cobIdServerToClient        uint32
	nodeIdServer               uint8
	valid                      bool
	index                      uint16
	subindex                   uint8
	finished                   bool
	sizeIndicated              uint32
	sizeTransferred            uint32
	state                      internalState
	processingPeriodUs         int
	fifo                       *fifo.Fifo
	rxNew                      bool
	response                   SDOMessage
	toggle                     uint8
	timeoutTimeUs              uint32
	timeoutTimer               uint32
	timeoutTimeBlockTransferUs uint32
	timeoutTimerBlock          uint32
	blockSequenceNb            uint8
	blockSize                  uint8
	blockMaxSize               int
	blockNoData                uint8
	blockCRCEnabled            bool
	blockDataUploadLast        [BlockSeqSize]byte
	blockCRC                   crc.CRC16
}

// Handle [SDOClient] related RX CAN frames
func (c *SDOClient) Handle(frame canopen.Frame) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != stateIdle && frame.DLC == 8 && (!c.rxNew || frame.Data[0] == 0x80) {
		if frame.Data[0] == 0x80 || (c.state != stateUploadBlkSubblockSreq && c.state != stateUploadBlkSubblockCrsp) {
			// Copy data in response
			c.response.raw = frame.Data
			c.rxNew = true
		} else if c.state == stateUploadBlkSubblockSreq {
			state := stateUploadBlkSubblockSreq
			seqno := frame.Data[0] & 0x7F
			c.timeoutTimer = 0
			c.timeoutTimerBlock = 0
			// Checks on the Sequence number
			switch {
			case seqno <= c.blockSize && seqno == (c.blockSequenceNb+1):
				c.blockSequenceNb = seqno
				// Is it last segment
				if (frame.Data[0] & 0x80) != 0 {
					copy(c.blockDataUploadLast[:], frame.Data[1:])
					c.finished = true
					state = stateUploadBlkSubblockCrsp
				} else {
					c.fifo.Write(frame.Data[1:], &c.blockCRC)
					c.sizeTransferred += BlockSeqSize
					if seqno == c.blockSize {
						c.logger.Debug("[RX] block upload end segment",
							"server", fmt.Sprintf("x%x", c.nodeIdServer),
							"index", fmt.Sprintf("x%x", c.index),
							"subindex", fmt.Sprintf("x%x", c.subindex),
							"data", frame.Data,
						)
						state = stateUploadBlkSubblockCrsp
					}
				}
			case seqno != c.blockSequenceNb && c.blockSequenceNb != 0:
				state = stateUploadBlkSubblockCrsp
				c.logger.Warn("wrong sequence number in rx sub-block", "seqno", seqno, "prevSeqno", c.blockSequenceNb)
			default:
				c.logger.Warn("wrong sequence number in rx sub-block,ignored", "seqno", seqno, "expected", c.blockSequenceNb+1)
			}
			if state != stateUploadBlkSubblockSreq {
				c.rxNew = false
				c.state = state
			}
		}
	}

}

// Setup the client for communication with an SDO server
func (c *SDOClient) setupServer(cobIdClientToServer uint32, cobIdServerToClient uint32, nodeIdServer uint8) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = stateIdle
	c.rxNew = false
	c.nodeIdServer = nodeIdServer
	// If server is the same don't re-initialize the buffers
	if c.cobIdClientToServer == cobIdClientToServer && c.cobIdServerToClient == cobIdServerToClient {
		return nil
	}
	c.cobIdClientToServer = cobIdClientToServer
	c.cobIdServerToClient = cobIdServerToClient
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
		c.valid = true
	} else {
		CanIdC2S = 0
		CanIdS2C = 0
		c.valid = false
	}
	err := c.Subscribe(uint32(CanIdS2C), 0x7FF, false, c)
	if err != nil {
		return err
	}
	c.txBuffer = canopen.NewFrame(uint32(CanIdC2S), 0, 8)
	return nil
}

// Start a new download sequence
func (c *SDOClient) downloadSetup(index uint16, subindex uint8, sizeIndicated uint32, blockEnabled bool) error {
	if !c.valid {
		return ErrInvalidArgs
	}
	c.index = index
	c.subindex = subindex
	c.sizeIndicated = sizeIndicated
	c.sizeTransferred = 0
	c.finished = false
	c.timeoutTimer = 0
	c.fifo.Reset()

	// Select transfer type
	switch {
	case c.od != nil && c.nodeIdServer == c.nodeId:
		c.streamer.SetWriter(nil)
		// Local transfer
		c.state = stateDownloadLocalTransfer
	case blockEnabled && (sizeIndicated == 0 || sizeIndicated > ClientProtocolSwitchThreshold):
		// Block download
		c.state = stateDownloadBlkInitiateReq
	default:
		// Segmented / expedited download
		c.state = stateDownloadInitiateReq
	}
	c.rxNew = false
	return nil
}

func (c *SDOClient) downloadMain(
	timeDifferenceUs uint32,
	abort bool,
	bufferPartial bool,
	sizeTransferred *uint32,
	timerNextUs *uint32,
	forceSegmented bool,
) (uint8, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ret := waitingResponse
	var err error
	var abortCode error

	if !c.valid {
		abortCode = AbortDeviceIncompat
		err = ErrInvalidArgs
	} else if c.state == stateIdle {
		ret = success
	} else if c.state == stateDownloadLocalTransfer && !abort {
		ret, err = c.downloadLocal(bufferPartial)
		if ret != waitingLocalTransfer {
			c.state = stateIdle
		} else if timerNextUs != nil {
			*timerNextUs = 0
		}
	} else if c.rxNew {
		response := c.response
		if response.IsAbort() {
			abortCode = response.GetAbortCode()
			c.logger.Info("[RX] server abort",
				"server", fmt.Sprintf("x%x", c.nodeIdServer),
				"index", fmt.Sprintf("x%x", c.index),
				"subindex", fmt.Sprintf("x%x", c.subindex),
				"code", uint32(response.GetAbortCode()),
				"description", abortCode,
			)
			c.state = stateIdle
			err = abortCode
			// Abort from the client
		} else if abort {
			abortCode = AbortDeviceIncompat
			c.state = stateAbort

		} else if !response.isResponseCommandValid(c.state) {
			c.logger.Warn("[RX] unexpected response code from server", "code", response.raw[0])
			c.state = stateAbort
			abortCode = AbortCmd

		} else {
			switch c.state {
			case stateDownloadInitiateRsp:

				index := response.GetIndex()
				subIndex := response.GetSubindex()
				if index != c.index || subIndex != c.subindex {
					abortCode = AbortParamIncompat
					c.state = stateAbort
					break
				}
				// Expedited transfer
				if c.finished {
					c.state = stateIdle
					ret = success
					c.logger.Debug("[RX] download expedited",
						"server", fmt.Sprintf("x%x", c.nodeIdServer),
						"index", fmt.Sprintf("x%x", c.index),
						"subindex", fmt.Sprintf("x%x", c.subindex),
						"raw", response.raw,
					)
					// Segmented transfer
				} else {
					c.toggle = 0x00
					c.state = stateDownloadSegmentReq
					c.logger.Debug("[RX] download segment",
						"server", fmt.Sprintf("x%x", c.nodeIdServer),
						"index", fmt.Sprintf("x%x", c.index),
						"subindex", fmt.Sprintf("x%x", c.subindex),
						"raw", response.raw,
					)
				}

			case stateDownloadSegmentRsp:

				// Verify and alternate toggle bit
				toggle := response.GetToggle()
				if toggle != c.toggle {
					abortCode = AbortToggleBit
					c.state = stateAbort
					break
				}
				c.toggle ^= 0x10
				if c.finished {
					c.state = stateIdle
					ret = success
				} else {
					c.state = stateDownloadSegmentReq
				}
				c.logger.Debug("[RX] download segment",
					"server", fmt.Sprintf("x%x", c.nodeIdServer),
					"index", fmt.Sprintf("x%x", c.index),
					"subindex", fmt.Sprintf("x%x", c.subindex),
					"raw", response.raw,
				)

			case stateDownloadBlkInitiateRsp:

				index := response.GetIndex()
				subIndex := response.GetSubindex()
				if index != c.index || subIndex != c.subindex {
					abortCode = AbortParamIncompat
					c.state = stateAbort
					break
				}
				c.blockCRC = crc.CRC16(0)
				c.blockSize = response.GetBlockSize()
				if c.blockSize < 1 || c.blockSize > BlockMaxSize {
					c.blockSize = BlockMaxSize
				}
				c.blockSequenceNb = 0
				c.fifo.AltBegin(0)
				c.state = stateDownloadBlkSubblockReq
				c.logger.Debug("[RX] download block",
					"server", fmt.Sprintf("x%x", c.nodeIdServer),
					"index", fmt.Sprintf("x%x", c.index),
					"subindex", fmt.Sprintf("x%x", c.subindex),
					"blksize", c.blockSize,
					"raw", response.raw,
				)

			case stateDownloadBlkSubblockReq, stateDownloadBlkSubblockRsp:

				if response.GetNumberOfSegments() < c.blockSequenceNb {
					c.logger.Error("not all segments transferred successfully")
					c.fifo.AltBegin(int(response.raw[1]) * BlockSeqSize)
					c.finished = false

				} else if response.GetNumberOfSegments() > c.blockSequenceNb {
					abortCode = AbortCmd
					c.state = stateAbort
					break
				}
				c.fifo.AltFinish(&c.blockCRC)
				if c.finished {
					c.state = stateDownloadBlkEndReq
				} else {
					c.blockSize = response.raw[2]
					c.blockSequenceNb = 0
					c.fifo.AltBegin(0)
					c.state = stateDownloadBlkSubblockReq
				}

			case stateDownloadBlkEndRsp:

				c.state = stateIdle
				ret = success

			}

			c.timeoutTimer = 0
			timeDifferenceUs = 0
			c.rxNew = false

		}

	} else if abort {
		abortCode = AbortDeviceIncompat
		c.state = stateAbort
	}

	if ret == waitingResponse {
		if c.timeoutTimer < c.timeoutTimeUs {
			c.timeoutTimer += timeDifferenceUs
		}
		if c.timeoutTimer >= c.timeoutTimeUs {
			abortCode = AbortTimeout
			c.state = stateAbort
		} else if timerNextUs != nil {
			diff := c.timeoutTimeUs - c.timeoutTimer
			if *timerNextUs > diff {
				*timerNextUs = diff
			}
		}
	}

	if ret == waitingResponse {
		c.txBuffer.Data = [8]byte{0}
		switch c.state {
		case stateDownloadInitiateReq:
			abortCode = c.downloadInitiate(forceSegmented)
			if abortCode != nil {
				c.state = stateIdle
				err = abortCode
				break
			}
			c.state = stateDownloadInitiateRsp

		case stateDownloadSegmentReq:
			abortCode = c.downloadSegment(bufferPartial)
			if abortCode != nil {
				c.state = stateAbort
				err = abortCode
				break
			}
			c.state = stateDownloadSegmentRsp

		case stateDownloadBlkInitiateReq:
			_ = c.downloadBlockInitiate()
			c.state = stateDownloadBlkInitiateRsp

		case stateDownloadBlkSubblockReq:
			abortCode = c.downloadBlock(bufferPartial, timerNextUs)
			if abortCode != nil {
				c.state = stateAbort
			}

		case stateDownloadBlkEndReq:
			c.downloadBlockEnd()
			c.state = stateDownloadBlkEndRsp

		default:
			break

		}

	}

	if ret == waitingResponse {

		switch c.state {
		case stateAbort:
			c.abort(abortCode.(Abort))
			err = abortCode
			c.state = stateIdle
		case stateDownloadBlkSubblockReq:
			ret = blockDownloadInProgress
		}
	}

	if sizeTransferred != nil {
		*sizeTransferred = c.sizeTransferred
	}
	return ret, err
}

// Helper function for starting download
// Valid for expedited or segmented transfer
func (c *SDOClient) downloadInitiate(forceSegmented bool) error {

	c.txBuffer.Data[0] = 0x20
	c.txBuffer.Data[1] = byte(c.index)
	c.txBuffer.Data[2] = byte(c.index >> 8)
	c.txBuffer.Data[3] = c.subindex

	count := uint32(c.fifo.GetOccupied())
	if (c.sizeIndicated == 0 && count <= 4) || (c.sizeIndicated > 0 && c.sizeIndicated <= 4) && !forceSegmented {
		c.txBuffer.Data[0] |= 0x02
		// Check length
		if count == 0 || (c.sizeIndicated > 0 && c.sizeIndicated != count) {
			c.state = stateIdle
			return AbortTypeMismatch
		}
		if c.sizeIndicated > 0 {
			c.txBuffer.Data[0] |= byte(0x01 | ((4 - count) << 2))
		}
		// Copy the data in queue and add the count
		count = uint32(c.fifo.Read(c.txBuffer.Data[4:], nil))
		c.sizeTransferred = count
		c.finished = true
		c.logger.Debug("[TX] download expedited",
			"server", fmt.Sprintf("x%x", c.nodeIdServer),
			"index", fmt.Sprintf("x%x", c.index),
			"subindex", fmt.Sprintf("x%x", c.subindex),
			"raw", c.txBuffer.Data,
		)
	} else {
		/* segmented transfer, indicate data size */
		if c.sizeIndicated > 0 {
			size := c.sizeIndicated
			c.txBuffer.Data[0] |= 0x01
			binary.LittleEndian.PutUint32(c.txBuffer.Data[4:], size)
		}
		c.logger.Debug("[TX] download segment",
			"server", fmt.Sprintf("x%x", c.nodeIdServer),
			"index", fmt.Sprintf("x%x", c.index),
			"subindex", fmt.Sprintf("x%x", c.subindex),
			"raw", c.txBuffer.Data,
		)
	}
	c.timeoutTimer = 0
	return c.Send(c.txBuffer)
}

// Write value to OD locally
func (c *SDOClient) downloadLocal(bufferPartial bool) (ret uint8, abortCode error) {
	var err error

	if c.streamer.Writer() == nil {
		c.logger.Debug("[TX] local transfer write",
			"nodeId", fmt.Sprintf("x%x", c.nodeId),
			"index", fmt.Sprintf("x%x", c.index),
			"subindex", fmt.Sprintf("x%x", c.subindex),
		)
		streamer, err := c.od.Streamer(c.index, c.subindex, false)
		if err == nil {
			c.streamer = streamer
		}

		odErr, ok := err.(od.ODR)
		if err != nil {
			if !ok {
				return 0, AbortGeneral
			}
			return 0, ConvertOdToSdoAbort(odErr)
		} else if !c.streamer.HasAttribute(od.AttributeSdoRw) {
			return 0, AbortUnsupportedAccess
		} else if !c.streamer.HasAttribute(od.AttributeSdoW) {
			return 0, AbortReadOnly
		} else if c.streamer.Writer() == nil {
			return 0, AbortDeviceIncompat
		}
	}
	// If still nil, return
	if c.streamer.Writer() == nil {
		return
	}
	count := c.fifo.Read(c.localBuffer, nil)
	c.sizeTransferred += uint32(count)
	// No data error
	if count == 0 {
		abortCode = AbortDeviceIncompat
		// Size transferred is too large
	} else if c.sizeIndicated > 0 && c.sizeTransferred > c.sizeIndicated {
		c.sizeTransferred -= uint32(count)
		abortCode = AbortDataLong
		// Size transferred is too small (check on last call)
	} else if !bufferPartial && c.sizeIndicated > 0 && c.sizeTransferred < c.sizeIndicated {
		abortCode = AbortDataShort
		// Last part of data !
	} else if !bufferPartial {
		odVarSize := c.streamer.DataLength
		// Special case for strings where the downloaded data may be shorter (nul character can be omitted)
		if c.streamer.HasAttribute(od.AttributeStr) && odVarSize == 0 || c.sizeTransferred < uint32(odVarSize) {
			count += 1
			c.localBuffer[count] = 0
			c.sizeTransferred += 1
			if odVarSize == 0 || odVarSize > c.sizeTransferred {
				count += 1
				c.localBuffer[count] = 0
				c.sizeTransferred += 1
			}
			c.streamer.DataLength = c.sizeTransferred
		} else if odVarSize == 0 {
			c.streamer.DataLength = c.sizeTransferred
		} else if c.sizeTransferred > uint32(odVarSize) {
			abortCode = AbortDataLong
		} else if c.sizeTransferred < uint32(odVarSize) {
			abortCode = AbortDataShort
		}
	}
	if abortCode == nil {
		_, err = c.streamer.Write(c.localBuffer[:count])
		odErr, ok := err.(od.ODR)
		if err != nil && odErr != od.ErrPartial {
			if !ok {
				return 0, AbortGeneral
			}
			return 0, ConvertOdToSdoAbort(odErr)
		} else if bufferPartial && err == nil {
			return 0, AbortDataLong
		} else if !bufferPartial {
			// Error if not written completely but download end
			if odErr == od.ErrPartial {
				return 0, AbortDataShort
			} else {
				return success, nil
			}
		} else {
			return waitingLocalTransfer, nil
		}
	}

	return 0, abortCode
}

// Helper function for downloading a segement of segmented transfer
func (c *SDOClient) downloadSegment(bufferPartial bool) error {
	// Fill data part
	count := uint32(c.fifo.Read(c.txBuffer.Data[1:], nil))
	c.sizeTransferred += count
	if c.sizeIndicated > 0 && c.sizeTransferred > c.sizeIndicated {
		c.sizeTransferred -= count
		return AbortDataLong
	}

	// Command specifier
	c.txBuffer.Data[0] = uint8(uint32(c.toggle) | ((BlockSeqSize - count) << 1))
	if c.fifo.GetOccupied() == 0 && !bufferPartial {
		if c.sizeIndicated > 0 && c.sizeTransferred < c.sizeIndicated {
			return AbortDataShort
		}
		c.txBuffer.Data[0] |= 0x01
		c.finished = true
	}

	c.timeoutTimer = 0
	c.logger.Debug("[TX] download segment",
		"server", fmt.Sprintf("x%x", c.nodeIdServer),
		"index", fmt.Sprintf("x%x", c.index),
		"subindex", fmt.Sprintf("x%x", c.subindex),
		"raw", c.txBuffer.Data,
	)
	return c.Send(c.txBuffer)
}

// Helper function for initiating a block download
func (c *SDOClient) downloadBlockInitiate() error {
	c.txBuffer.Data[0] = 0xC4
	c.txBuffer.Data[1] = byte(c.index)
	c.txBuffer.Data[2] = byte(c.index >> 8)
	c.txBuffer.Data[3] = c.subindex
	if c.sizeIndicated > 0 {
		c.txBuffer.Data[0] |= 0x02
		binary.LittleEndian.PutUint32(c.txBuffer.Data[4:], c.sizeIndicated)
	}
	c.timeoutTimer = 0
	return c.Send(c.txBuffer)
}

// Helper function for downloading a sub-block
func (c *SDOClient) downloadBlock(bufferPartial bool, timerNext *uint32) error {
	if c.fifo.AltGetOccupied() < BlockSeqSize && bufferPartial {
		// No data yet
		return nil
	}
	c.blockSequenceNb++
	c.txBuffer.Data[0] = c.blockSequenceNb
	count := uint32(c.fifo.AltRead(c.txBuffer.Data[1:]))
	c.blockNoData = uint8(BlockSeqSize - count)
	c.sizeTransferred += count
	if c.sizeIndicated > 0 && c.sizeTransferred > c.sizeIndicated {
		c.sizeTransferred -= count
		return AbortDataLong
	}
	if c.fifo.AltGetOccupied() == 0 && !bufferPartial {
		if c.sizeIndicated > 0 && c.sizeTransferred < c.sizeIndicated {
			return AbortDataShort
		}
		c.txBuffer.Data[0] |= 0x80
		c.finished = true
		c.state = stateDownloadBlkSubblockRsp
	} else if c.blockSequenceNb >= c.blockSize {
		c.state = stateDownloadBlkSubblockRsp
	} else if timerNext != nil {
		*timerNext = 0
	}
	c.timeoutTimer = 0
	return c.Send(c.txBuffer)
}

// Helper function for end of block
func (c *SDOClient) downloadBlockEnd() {
	c.txBuffer.Data[0] = 0xC1 | (c.blockNoData << 2)
	c.txBuffer.Data[1] = byte(c.blockCRC)
	c.txBuffer.Data[2] = byte(c.blockCRC >> 8)
	c.timeoutTimer = 0
	_ = c.Send(c.txBuffer)
}

// Create & send abort on bus
func (c *SDOClient) abort(abortCode Abort) {
	code := uint32(abortCode)
	c.txBuffer.Data[0] = 0x80
	c.txBuffer.Data[1] = uint8(c.index)
	c.txBuffer.Data[2] = uint8(c.index >> 8)
	c.txBuffer.Data[3] = c.subindex
	binary.LittleEndian.PutUint32(c.txBuffer.Data[4:], code)
	c.logger.Warn("[TX] client abort",
		"server", fmt.Sprintf("x%x", c.nodeIdServer),
		"index", fmt.Sprintf("x%x", c.index),
		"subindex", fmt.Sprintf("x%x", c.subindex),
		"code", code,
		"description", abortCode,
	)
	_ = c.Send(c.txBuffer)
}

/////////////////////////////////////
////////////SDO UPLOAD///////////////
/////////////////////////////////////

func (c *SDOClient) uploadSetup(index uint16, subindex uint8, blockEnabled bool) error {
	if !c.valid {
		return ErrInvalidArgs
	}
	c.index = index
	c.subindex = subindex
	c.sizeIndicated = 0
	c.sizeTransferred = 0
	c.finished = false
	c.fifo.Reset()
	if c.od != nil && c.nodeIdServer == c.nodeId {
		c.streamer.SetReader(nil)
		c.state = stateUploadLocalTransfer
	} else if blockEnabled {
		c.state = stateUploadBlkInitiateReq
	} else {
		c.state = stateUploadInitiateReq
	}
	c.rxNew = false
	return nil
}

func (c *SDOClient) uploadLocal() (ret uint8, err error) {

	if c.streamer.Reader() == nil {
		c.logger.Debug("[RX] local transfer read",
			"nodeId", fmt.Sprintf("x%x", c.nodeId),
			"index", fmt.Sprintf("x%x", c.index),
			"subindex", fmt.Sprintf("x%x", c.subindex),
		)
		streamer, err := c.od.Streamer(c.index, c.subindex, false)
		if err == nil {
			c.streamer = streamer
		}

		odErr, ok := err.(od.ODR)
		if err != nil {
			if !ok {
				return 0, AbortGeneral
			}
			return 0, ConvertOdToSdoAbort(odErr)
		} else if !c.streamer.HasAttribute(od.AttributeSdoRw) {
			return 0, AbortUnsupportedAccess
		} else if !c.streamer.HasAttribute(od.AttributeSdoR) {
			return 0, AbortWriteOnly
		} else if c.streamer.Reader() == nil {
			return 0, AbortDeviceIncompat
		}
	}
	countFifo := c.fifo.GetSpace()
	if countFifo == 0 {
		ret = uploadDataFull
	} else if c.streamer.Reader() != nil {
		countData := c.streamer.DataLength
		countBuffer := uint32(0)
		countRead := 0
		if countData > 0 && countData <= uint32(countFifo) {
			countBuffer = countData
		} else {
			countBuffer = uint32(countFifo)
		}
		countRead, err = c.streamer.Read(c.localBuffer[:countBuffer])
		odErr, ok := err.(od.ODR)
		if err != nil && err != od.ErrPartial {
			if !ok {
				return 0, AbortGeneral
			}
			return 0, ConvertOdToSdoAbort(odErr)
		} else {
			if countRead > 0 && c.streamer.HasAttribute(od.AttributeStr) {
				c.localBuffer[countRead] = 0
				countStr := 0
				for i, v := range c.localBuffer {
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
					odErr = od.ErrNo
					c.streamer.DataLength = c.sizeTransferred + uint32(countRead)
				}
			}
			c.fifo.Write(c.localBuffer[:countRead], nil)
			c.sizeTransferred += uint32(countRead)
			c.sizeIndicated = c.streamer.DataLength
			if c.sizeIndicated > 0 && c.sizeTransferred > c.sizeIndicated {
				err = AbortDataLong
			} else if odErr == od.ErrNo {
				if c.sizeIndicated > 0 && c.sizeTransferred < c.sizeIndicated {
					err = AbortDataShort
				}
			} else {
				ret = waitingLocalTransfer
			}
		}

	}
	return ret, err
}

// Main state machine
func (c *SDOClient) upload(
	timeDifferenceUs uint32,
	abort bool,
	sizeIndicated *uint32,
	sizeTransferred *uint32,
	timerNextUs *uint32,
) (uint8, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ret := waitingResponse
	var err error
	var abortCode error

	if !c.valid {
		abortCode = AbortDeviceIncompat
		err = ErrInvalidArgs
	} else if c.state == stateIdle {
		ret = success
	} else if c.state == stateUploadLocalTransfer && !abort {
		ret, err = c.uploadLocal()
		if ret != uploadDataFull && ret != waitingLocalTransfer {
			c.state = stateIdle
		} else if timerNextUs != nil {
			*timerNextUs = 0
		}
	} else if c.rxNew {
		response := c.response
		if response.IsAbort() {
			abortCode = response.GetAbortCode()
			c.logger.Info("[RX] server abort",
				"server", fmt.Sprintf("x%x", c.nodeIdServer),
				"index", fmt.Sprintf("x%x", c.index),
				"subindex", fmt.Sprintf("x%x", c.subindex),
				"code", uint32(response.GetAbortCode()),
				"description", abortCode,
			)
			c.state = stateIdle
			err = abortCode

		} else if abort {
			abortCode = AbortDeviceIncompat
			c.state = stateAbort

		} else if !response.isResponseCommandValid(c.state) {
			c.logger.Warn("unexpected response code from server", "code", response.raw[0])
			c.state = stateAbort
			abortCode = AbortCmd

		} else {
			switch c.state {
			case stateUploadInitiateRsp:
				index := response.GetIndex()
				subIndex := response.GetSubindex()
				if index != c.index || subIndex != c.subindex {
					abortCode = AbortParamIncompat
					c.state = stateAbort
					break
				}
				if (response.raw[0] & 0x02) != 0 {
					// Expedited
					var count uint32 = 4
					// Size indicated ?
					if (response.raw[0] & 0x01) != 0 {
						count -= uint32((response.raw[0] >> 2) & 0x03)
					}
					c.fifo.Write(response.raw[4:4+count], nil)
					c.sizeTransferred = count
					c.state = stateIdle
					ret = success
					c.logger.Debug("[RX] upload expedited",
						"server", c.nodeIdServer,
						"index", fmt.Sprintf("x%x", c.index),
						"subindex", fmt.Sprintf("x%x", c.subindex),
						"raw", response.raw,
					)
					// Segmented
				} else {
					// Size indicated ?
					if (response.raw[0] & 0x01) != 0 {
						c.sizeIndicated = binary.LittleEndian.Uint32(response.raw[4:])
					}
					c.toggle = 0
					c.state = stateUploadSegmentReq
					c.logger.Debug("[RX] upload segment",
						"server", c.nodeIdServer,
						"index", fmt.Sprintf("x%x", c.index),
						"subindex", fmt.Sprintf("x%x", c.subindex),
						"raw", response.raw,
					)
				}

			case stateUploadSegmentRsp:
				// Verify and alternate toggle bit
				c.logger.Debug("[RX] upload segment",
					"server", c.nodeIdServer,
					"index", fmt.Sprintf("x%x", c.index),
					"subindex", fmt.Sprintf("x%x", c.subindex),
					"raw", response.raw,
				)
				toggle := response.GetToggle()
				if toggle != c.toggle {
					abortCode = AbortToggleBit
					c.state = stateAbort
					break
				}
				c.toggle ^= 0x10
				count := BlockSeqSize - (response.raw[0]>>1)&0x07
				countWr := c.fifo.Write(response.raw[1:1+count], nil)
				c.sizeTransferred += uint32(countWr)
				// Check enough space if fifo
				if countWr != int(count) {
					abortCode = AbortOutOfMem
					c.state = stateAbort
					break
				}

				// Check size uploaded
				if c.sizeIndicated > 0 && c.sizeTransferred > c.sizeIndicated {
					abortCode = AbortDataLong
					c.state = stateAbort
					break
				}

				// No more segments ?
				if (response.raw[0] & 0x01) != 0 {
					// Check size uploaded
					if c.sizeIndicated > 0 && c.sizeTransferred < c.sizeIndicated {
						abortCode = AbortDataLong
						c.state = stateAbort
					} else {
						c.state = stateIdle
						ret = success
					}
				} else {
					c.state = stateUploadSegmentReq
				}

			case stateUploadBlkInitiateRsp:

				index := response.GetIndex()
				subindex := response.GetSubindex()
				if index != c.index || subindex != c.subindex {
					abortCode = AbortParamIncompat
					c.state = stateAbort
					break
				}
				// Block is supported
				if (response.raw[0] & 0xF9) == 0xC0 {
					c.blockCRCEnabled = response.IsCRCEnabled()
					if (response.raw[0] & 0x02) != 0 {
						c.sizeIndicated = uint32(response.GetBlockSize())
					}
					c.state = stateUploadBlkInitiateReq2
					c.logger.Debug("[RX] block upload init",
						"server", c.nodeIdServer,
						"index", fmt.Sprintf("x%x", c.index),
						"subindex", fmt.Sprintf("x%x", c.subindex),
						"crc", response.IsCRCEnabled(),
						"sizeIndicated", c.sizeIndicated,
						"raw", response.raw,
					)

					// Switch to normal transfer
				} else if (response.raw[0] & 0xF0) == 0x40 {
					if (response.raw[0] & 0x02) != 0 {
						// Expedited
						count := 4
						if (response.raw[0] & 0x01) != 0 {
							count -= (int(response.raw[0]>>2) & 0x03)
						}
						c.fifo.Write(response.raw[4:4+count], nil)
						c.sizeTransferred = uint32(count)
						c.state = stateIdle
						ret = success
						c.logger.Debug("[RX] block upload switching expedited",
							"server", c.nodeIdServer,
							"index", fmt.Sprintf("x%x", c.index),
							"subindex", fmt.Sprintf("x%x", c.subindex),
							"raw", response.raw,
						)
					} else {
						if (response.raw[0] & 0x01) != 0 {
							c.sizeIndicated = uint32(response.GetBlockSize())
						}
						c.toggle = 0x00
						c.state = stateUploadSegmentReq
						c.logger.Debug("[RX] block upload switching segmented",
							"server", c.nodeIdServer,
							"index", fmt.Sprintf("x%x", c.index),
							"subindex", fmt.Sprintf("x%x", c.subindex),
							"raw", response.raw,
						)
					}
				}
			case stateUploadBlkSubblockSreq:
				// Handled directly in Rx callback
				break

			case stateUploadBlkEndSreq:
				// Get number of data bytes in last segment, that do not
				// contain data. Then copy remaining data into fifo
				noData := (response.raw[0] >> 2) & 0x07
				c.fifo.Write(c.blockDataUploadLast[:BlockSeqSize-noData], &c.blockCRC)
				c.sizeTransferred += uint32(BlockSeqSize - noData)

				if c.sizeIndicated > 0 && c.sizeTransferred > c.sizeIndicated {
					abortCode = AbortDataLong
					c.state = stateAbort
					break
				} else if c.sizeIndicated > 0 && c.sizeTransferred < c.sizeIndicated {
					abortCode = AbortDataShort
					c.state = stateAbort
					break
				}
				if c.blockCRCEnabled {
					crcServer := crc.CRC16(binary.LittleEndian.Uint16(response.raw[1:3]))
					if crcServer != c.blockCRC {
						abortCode = AbortCRC
						c.state = stateAbort
						break
					}
				}
				c.state = stateUploadBlkEndCrsp
				c.logger.Debug("[RX] block upload end",
					"server", c.nodeIdServer,
					"index", fmt.Sprintf("x%x", c.index),
					"subindex", fmt.Sprintf("x%x", c.subindex),
					"raw", response.raw,
				)

			default:
				abortCode = AbortCmd
				c.state = stateAbort
			}

		}
		c.timeoutTimer = 0
		timeDifferenceUs = 0
		c.rxNew = false
	} else if abort {
		abortCode = AbortDeviceIncompat
		c.state = stateAbort
	}

	if ret == waitingResponse {
		if c.timeoutTimer < c.timeoutTimeUs {
			c.timeoutTimer += timeDifferenceUs
		}
		if c.timeoutTimer >= c.timeoutTimeUs {
			if c.state == stateUploadSegmentReq || c.state == stateUploadBlkSubblockCrsp {
				abortCode = AbortGeneral
			} else {
				abortCode = AbortTimeout
			}
			c.state = stateAbort

		} else if timerNextUs != nil {
			diff := c.timeoutTimeUs - c.timeoutTimer
			if *timerNextUs > diff {
				*timerNextUs = diff
			}
		}
		// Timeout for subblocks
		if c.state == stateUploadBlkSubblockSreq {
			if c.timeoutTimerBlock < c.timeoutTimeBlockTransferUs {
				c.timeoutTimerBlock += timeDifferenceUs
			}
			if c.timeoutTimerBlock >= c.timeoutTimeBlockTransferUs {
				c.state = stateUploadBlkSubblockCrsp
				c.rxNew = false
			} else if timerNextUs != nil {
				diff := c.timeoutTimeBlockTransferUs - c.timeoutTimerBlock
				if *timerNextUs > diff {
					*timerNextUs = diff
				}
			}
		}
	}

	if ret == waitingResponse {
		c.txBuffer.Data = [8]byte{0}
		switch c.state {
		case stateUploadInitiateReq:
			c.txBuffer.Data[0] = 0x40
			c.txBuffer.Data[1] = byte(c.index)
			c.txBuffer.Data[2] = byte(c.index >> 8)
			c.txBuffer.Data[3] = c.subindex
			c.timeoutTimer = 0
			_ = c.Send(c.txBuffer)
			c.state = stateUploadInitiateRsp
			c.logger.Debug("[TX] upload segment",
				"server", c.nodeIdServer,
				"index", fmt.Sprintf("x%x", c.index),
				"subindex", fmt.Sprintf("x%x", c.subindex),
				"raw", c.txBuffer.Data,
			)

		case stateUploadSegmentReq:
			if c.fifo.GetSpace() < BlockSeqSize {
				ret = uploadDataFull
				break
			}
			c.txBuffer.Data[0] = 0x60 | c.toggle
			c.timeoutTimer = 0
			_ = c.Send(c.txBuffer)
			c.state = stateUploadSegmentRsp
			c.logger.Debug("[TX] upload segment",
				"server", c.nodeIdServer,
				"index", fmt.Sprintf("x%x", c.index),
				"subindex", fmt.Sprintf("x%x", c.subindex),
				"raw", c.txBuffer.Data,
			)

		case stateUploadBlkInitiateReq:
			c.txBuffer.Data[0] = 0xA4
			c.txBuffer.Data[1] = byte(c.index)
			c.txBuffer.Data[2] = byte(c.index >> 8)
			c.txBuffer.Data[3] = c.subindex
			// Calculate number of block segments from free space
			count := c.fifo.GetSpace() / BlockSeqSize
			if count >= BlockMaxSize {
				count = BlockMaxSize
			} else if count == 0 {
				abortCode = AbortOutOfMem
				c.state = stateAbort
				break
			}
			c.blockSize = uint8(count)
			c.txBuffer.Data[4] = c.blockSize
			c.txBuffer.Data[5] = ClientProtocolSwitchThreshold
			c.timeoutTimer = 0
			_ = c.Send(c.txBuffer)
			c.state = stateUploadBlkInitiateRsp
			c.logger.Debug("[TX] block upload initiate",
				"server", c.nodeIdServer,
				"index", fmt.Sprintf("x%x", c.index),
				"subindex", fmt.Sprintf("x%x", c.subindex),
				"blksize", c.blockSize,
				"raw", c.txBuffer.Data,
			)

		case stateUploadBlkInitiateReq2:
			c.txBuffer.Data[0] = 0xA3
			c.timeoutTimer = 0
			c.timeoutTimerBlock = 0
			c.blockSequenceNb = 0
			c.blockCRC = crc.CRC16(0)
			c.state = stateUploadBlkSubblockSreq
			c.rxNew = false
			_ = c.Send(c.txBuffer)

		case stateUploadBlkSubblockCrsp:
			c.txBuffer.Data[0] = 0xA2
			c.txBuffer.Data[1] = c.blockSequenceNb
			transferShort := c.blockSequenceNb != c.blockSize
			seqnoStart := c.blockSequenceNb
			if c.finished {
				c.state = stateUploadBlkEndSreq
			} else {
				// Check size too large
				if c.sizeIndicated > 0 && c.sizeTransferred > c.sizeIndicated {
					abortCode = AbortDataLong
					c.state = stateAbort
					break
				}
				// Calculate number of block segments from remaining space
				count := c.fifo.GetSpace() / BlockSeqSize
				if count >= BlockMaxSize {
					count = BlockMaxSize
				} else if c.fifo.GetOccupied() > 0 {
					ret = uploadDataFull
					if transferShort {
						c.logger.Warn("upload data is full", "seqno", seqnoStart)
					}
					if timerNextUs != nil {
						*timerNextUs = 0
					}
					break
				}
				c.blockSize = uint8(count)
				c.blockSequenceNb = 0
				c.state = stateUploadBlkSubblockSreq
				c.rxNew = false
			}
			c.txBuffer.Data[2] = c.blockSize
			c.timeoutTimerBlock = 0
			_ = c.Send(c.txBuffer)
			if transferShort && !c.finished {
				c.logger.Warn("sub-block restarted", "seqnoPrev", seqnoStart, "blksize", c.blockSize)
			}

		case stateUploadBlkEndCrsp:
			c.txBuffer.Data[0] = 0xA1
			_ = c.Send(c.txBuffer)
			c.state = stateIdle
			ret = success

		default:
			break
		}

	}

	if ret == waitingResponse {
		switch c.state {
		case stateAbort:
			c.abort(abortCode.(Abort))
			err = abortCode
			c.state = stateIdle
		case stateUploadBlkSubblockSreq:
			ret = blockUploadInProgress
		}
	}
	if sizeIndicated != nil {
		*sizeIndicated = c.sizeIndicated
	}

	if sizeTransferred != nil {
		*sizeTransferred = c.sizeTransferred
	}

	return ret, err

}

func NewSDOClient(
	bm *canopen.BusManager,
	logger *slog.Logger,
	odict *od.ObjectDictionary,
	nodeId uint8,
	timeoutMs uint32,
	entry1280 *od.Entry,
) (*SDOClient, error) {

	if bm == nil {
		return nil, canopen.ErrIllegalArgument
	}
	if logger == nil {
		logger = slog.Default()
	}

	logger = logger.With("service", "[CLIENT]")

	if entry1280 != nil && (entry1280.Index < 0x1280 || entry1280.Index > (0x1280+0x7F)) {
		logger.Error("invalid index for sdo client", "index", fmt.Sprintf("x%x", entry1280.Index))
		return nil, canopen.ErrIllegalArgument
	}

	c := &SDOClient{BusManager: bm, logger: logger}
	c.od = odict
	c.nodeId = nodeId
	c.streamer = &od.Streamer{}
	c.fifo = fifo.NewFifo(BlockMaxSize * BlockSeqSize)
	c.localBuffer = make([]byte, DefaultClientBufferSize+2)
	c.SetTimeout(DefaultClientTimeout)
	c.SetTimeoutBlockTransfer(DefaultClientTimeout)
	c.SetBlockMaxSize(BlockMaxSize)
	c.SetProcessingPeriod(DefaultClientProcessPeriodUs)
	rw := &sdoRawReadWriter{
		client: c,
	}
	c.rw = rw

	var nodeIdServer uint8
	var CobIdClientToServer, CobIdServerToClient uint32
	var err2, err3, err4 error
	if entry1280 != nil {
		maxSubindex, err1 := entry1280.Uint8(0)
		CobIdClientToServer, err2 = entry1280.Uint32(1)
		CobIdServerToClient, err3 = entry1280.Uint32(2)
		nodeIdServer, err4 = entry1280.Uint8(3)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || maxSubindex != 3 {
			logger.Error("error reading SDO client params")
			return nil, canopen.ErrOdParameters
		}
	} else {
		nodeIdServer = 0
	}
	if entry1280 != nil {
		entry1280.AddExtension(c, od.ReadEntryDefault, writeEntry1280)
	}
	c.cobIdClientToServer = 0
	c.cobIdServerToClient = 0

	err := c.setupServer(CobIdClientToServer, CobIdServerToClient, nodeIdServer)
	if err != nil {
		return nil, canopen.ErrIllegalArgument
	}
	return c, nil
}

// Set read / write to local OD
// This is equivalent as reading with a node id set to 0
func (c *SDOClient) SetNoId() {
	c.nodeId = 0
}

// Set timeout for SDO non block transfers
func (c *SDOClient) SetTimeout(timeoutMs uint32) {
	c.timeoutTimeUs = timeoutMs * 1000
}

// Set timeout for SDO block transfers
func (c *SDOClient) SetTimeoutBlockTransfer(timeoutMs uint32) {
	c.timeoutTimeBlockTransferUs = timeoutMs * 1000
}

// Set the processing period for SDO client
// lower number can increase transfer speeds at the cost
// of more CPU usage
func (c *SDOClient) SetProcessingPeriod(periodUs int) {
	c.processingPeriodUs = periodUs
}

// Set maximum block size to use during block transfers
// Some devices may not support big block sizes as it can use a lot of RAM.
func (c *SDOClient) SetBlockMaxSize(size int) {
	c.blockMaxSize = max(min(size, BlockMaxSize), BlockMinSize)
}
