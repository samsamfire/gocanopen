package sdo

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"

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
	buffer                     []byte
	bufWriteOffset             uint32
	bufReadOffset              uint32
	toggle                     uint8
	timeoutTimeBlockTransferUs uint32
	blockSequenceNb            uint8
	blockSize                  uint8
	blockNoData                uint8
	blockCRCEnabled            bool
	blockCRC                   crc.CRC16
	errorExtraInfo             error

	rx chan SDOMessage
}

// Handle [SDOServer] related RX CAN frames
func (server *SDOServer) Handle(frame canopen.Frame) {
	server.mu.Lock()
	defer server.mu.Unlock()
	if frame.DLC != 8 {
		return
	}
	rx := SDOMessage{}
	rx.raw = frame.Data
	server.rx <- rx
}

// Process [SDOServer] state machine and TX CAN frames
// It returns the global server state and error if any
// This should be called periodically
func (server *SDOServer) Process(
	nmtIsPreOrOperationnal bool,
	timeDifferenceUs uint32,
	timerNextUs *uint32,
) (state uint8, err error) {

	timeout := time.Duration(server.timeoutTimeUs * uint32(time.Microsecond))

	for {
		select {
		case frame := <-server.rx:
			// New frame received, do what we need to do !
			if !nmtIsPreOrOperationnal || !server.valid {
				server.state = stateIdle
				return success, nil
			}
			err := server.processIncoming(frame)
			if err != nil && err != od.ErrPartial {
				// Abort straight away, nothing to send afterwards
				server.txAbort(err)
				break
			}
			// A response is expected
			err = server.processOutgoing()
			if err != nil {
				server.txAbort(err)
			}

		case <-time.After(timeout):
			if server.state != stateIdle {
				server.txAbort(AbortTimeout)
			}
		}
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

// Update streamer object with new requested entry
func (server *SDOServer) updateStreamer(response SDOMessage) error {
	var err error
	server.index = response.GetIndex()
	server.subindex = response.GetSubindex()
	server.streamer, err = od.NewStreamer(server.od.Index(server.index), server.subindex, false)
	if err != nil {
		return ConvertOdToSdoAbort(err.(od.ODR))
	}
	if !server.streamer.HasAttribute(od.AttributeSdoRw) {
		return AbortUnsupportedAccess
	}
	upload := server.state == stateUploadBlkInitiateReq || server.state == stateUploadInitiateReq

	if upload && !server.streamer.HasAttribute(od.AttributeSdoR) {
		return AbortWriteOnly
	}
	if !upload && !server.streamer.HasAttribute(od.AttributeSdoW) {
		return AbortReadOnly
	}

	// In case of reading, we need to prepare data now
	if upload {
		return server.prepareRx()
	}
	return nil
}

// Prepare read transfer
func (server *SDOServer) prepareRx() error {
	server.bufReadOffset = 0
	server.bufWriteOffset = 0
	server.sizeTransferred = 0
	server.finished = false
	err := server.readObjectDictionary(BlockSeqSize, false)
	if err != nil && err != od.ErrPartial {
		return err
	}

	if server.finished {
		server.sizeIndicated = server.streamer.DataLength
		if server.sizeIndicated == 0 {
			server.sizeIndicated = server.bufWriteOffset
		} else if server.sizeIndicated != server.bufWriteOffset {
			server.errorExtraInfo = fmt.Errorf("size indicated %v != to buffer write offset %v", server.sizeIndicated, server.bufWriteOffset)
			return AbortDeviceIncompat
		}
		return nil
	}

	if !server.streamer.HasAttribute(od.AttributeStr) {
		server.sizeIndicated = server.streamer.DataLength
		return nil
	}
	server.sizeIndicated = 0
	return nil
}

// Create & send abort on bus
func (server *SDOServer) SendAbort(abortCode Abort) {
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
	server.rx = make(chan SDOMessage, 1)
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
	server.cobIdClientToServer = 0
	server.cobIdServerToClient = 0
	return server, server.initRxTx(uint32(canIdClientToServer), uint32(canIdServerToClient))

}
