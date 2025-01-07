package sdo

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/internal/crc"
	"github.com/samsamfire/gocanopen/pkg/nmt"
	"github.com/samsamfire/gocanopen/pkg/od"
)

type SDOServer struct {
	*canopen.BusManager
	logger              *slog.Logger
	mu                  sync.Mutex
	od                  *od.ObjectDictionary
	nodeId              uint8
	rx                  chan SDOMessage
	streamer            *od.Streamer
	txBuffer            canopen.Frame
	cobIdClientToServer uint32
	cobIdServerToClient uint32
	valid               bool
	running             bool
	buf                 *bytes.Buffer
	intermediateBuf     []byte
	index               uint16
	subindex            uint8
	sizeIndicated       uint32
	sizeTransferred     uint32
	toggle              uint8
	finished            bool
	state               internalState
	timeoutTimeUs       uint32
	// Block transfers
	blockSequenceNb uint8
	blockSize       uint8
	blockNoData     uint8
	blockCRCEnabled bool
	blockCRC        crc.CRC16
	blockTimeout    uint32
	errorExtraInfo  error

	nmt uint8
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
	select {
	case server.rx <- rx:
	default:
		server.logger.Warn("dropped SDO server RX frame")
		// Drop frame
	}

}

// Process [SDOServer] state machine and TX CAN frames
// It returns the global server state and error if any
// This should be called periodically
func (server *SDOServer) Process(ctx context.Context) (state uint8, err error) {

	server.logger.Info("starting sdo server processing")
	timeout := time.Duration(server.timeoutTimeUs * uint32(time.Microsecond))

	for {
		server.mu.Lock()
		nmtIsPreOrOperationnal := server.nmt == nmt.StateOperational || server.nmt == nmt.StatePreOperational
		server.mu.Unlock()

		select {
		case <-ctx.Done():
			server.logger.Info("exiting sdo server process")
			return
		default:
			if !server.valid || !nmtIsPreOrOperationnal {
				server.state = stateIdle
				// Sleep to avoid huge CPU load when idling
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}

		select {
		case rx := <-server.rx:
			// New frame received, do what we need to do !
			err := server.processIncoming(rx)
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
	err := server.Subscribe(uint32(CanIdC2S), 0x7FF, false, server)
	if err != nil {
		server.valid = false
		return err
	}
	server.txBuffer = canopen.NewFrame(uint32(CanIdS2C), 0, 8)
	return nil
}

func (server *SDOServer) writeObjectDictionary(crcOperation uint, crcClient crc.CRC16) error {

	added := 0

	// Check transfer size is not bigger than indicated
	if server.sizeIndicated > 0 && server.sizeTransferred > server.sizeIndicated {
		server.state = stateAbort
		return AbortDataLong
	}

	if server.finished {
		// Check transfer size is not smaller than indicated
		if server.sizeIndicated > 0 && server.sizeTransferred < server.sizeIndicated {
			server.state = stateAbort
			return AbortDataShort
		}
		// Golang does not have null termination characters so nothing particular to do
		// Stream data should be limited to the sent value
		varSizeInOd := server.streamer.DataLength
		if server.streamer.HasAttribute(od.AttributeStr) &&
			(varSizeInOd == 0 || server.sizeTransferred < varSizeInOd) &&
			server.buf.Available() >= 2 {
			server.buf.Write([]byte{0})
			server.sizeTransferred++
			added++
			if varSizeInOd == 0 || server.sizeTransferred < varSizeInOd {
				server.buf.Write([]byte{0})
				server.sizeTransferred++
				added++
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

	}

	// Calculate CRC
	if server.blockCRCEnabled && crcOperation > 0 {
		server.blockCRC.Block(server.buf.Bytes()[:server.buf.Len()-added])
		if crcOperation == 2 && crcClient != server.blockCRC {
			server.state = stateAbort
			server.errorExtraInfo = fmt.Errorf("server was expecting %v but got %v", server.blockCRC, crcClient)
			return AbortCRC
		}
	}

	// Transfer from buffer to OD
	_, err := io.Copy(server.streamer, server.buf)
	if err != nil && err != od.ErrPartial {
		server.state = stateAbort
		odr, ok := err.(od.ODR)
		if !ok {
			server.logger.Warn("unexpected error in server on io.Copy", "error", err)
			odr = od.ErrGeneral
		}

		return ConvertOdToSdoAbort(odr)
	}

	if server.finished && err == od.ErrPartial {
		server.state = stateAbort
		return AbortDataShort
	}

	if !server.finished && err == nil {
		server.state = stateAbort
		return AbortDataLong
	}
	return nil
}

// Read from OD into buffer & calculate CRC if needed
// Depending on the transfer type, this might have to be called multiple times
func (server *SDOServer) readObjectDictionary(countMinimum uint32, size int, calculateCRC bool) error {

	unread := server.buf.Len()
	if server.finished || unread >= int(countMinimum) {
		return nil
	}

	// Read from OD into the buffer
	countRd, err := server.streamer.Read(server.intermediateBuf)
	if err != nil && err != od.ErrPartial {
		server.state = stateAbort
		odr, ok := err.(od.ODR)
		if !ok {
			server.logger.Warn("unexpected error in server when reading", "error", err)
			odr = od.ErrGeneral
		}
		return ConvertOdToSdoAbort(odr)
	}

	// Stop sending at null termination if string
	if countRd > 0 && server.streamer.HasAttribute(od.AttributeStr) {
		countStr := int(server.streamer.DataLength)
		for i, v := range server.intermediateBuf {
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
	// Calculate CRC for the read data
	if size > 0 {
		countRd = size
	}
	server.buf.Write(server.intermediateBuf[:countRd])

	if err == od.ErrPartial {
		server.finished = false
		if uint32(countRd) < countMinimum {
			server.state = stateAbort
			server.errorExtraInfo = fmt.Errorf("buffer unread %v is less than the minimum count %v", server.buf.Len(), countMinimum)
			return AbortDeviceIncompat
		}
	} else {
		server.finished = true
	}
	if calculateCRC && server.blockCRCEnabled {
		server.blockCRC.Block(server.intermediateBuf[:countRd])
	}
	return nil
}

// Update streamer object with new requested entry
func (server *SDOServer) updateStreamer(response SDOMessage) error {
	var err error
	server.index = response.GetIndex()
	server.subindex = response.GetSubindex()
	server.streamer, err = server.od.Streamer(server.index, server.subindex, false)
	if err != nil {
		odr, ok := err.(od.ODR)
		if !ok {
			server.logger.Warn("unexpected error in server creating streamer", "error", err)
			odr = od.ErrGeneral
		}
		return ConvertOdToSdoAbort(odr)
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
	server.buf.Reset()
	server.sizeTransferred = 0
	server.finished = false

	// Load data from OD now
	err := server.readObjectDictionary(BlockSeqSize, 0, false)
	if err != nil && err != od.ErrPartial {
		return err
	}

	// For small transfers (e.g. expedited), we might finish straight away
	if server.finished {
		server.sizeIndicated = server.streamer.DataLength
		if server.sizeIndicated == 0 {
			server.sizeIndicated = uint32(server.buf.Len())
		} else if server.sizeIndicated != uint32(server.buf.Len()) {
			// Because we have finished, we should have exactly sizeIndicated bytes in buffer
			server.errorExtraInfo = fmt.Errorf("size indicated %v != to buffer write offset %v", server.sizeIndicated, server.buf.Len())
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
	server.logger.Warn("[TX] server abort",
		"index", fmt.Sprintf("x%x", server.index),
		"subindex", fmt.Sprintf("x%x", server.subindex),
		"code", code,
		"description", abortCode,
		"extraInfo", server.errorExtraInfo,
	)
}

// Set internal nmt state
func (server *SDOServer) SetNMTState(state uint8) {
	server.mu.Lock()
	defer server.mu.Unlock()
	server.nmt = state
}

func NewSDOServer(
	bm *canopen.BusManager,
	logger *slog.Logger,
	odict *od.ObjectDictionary,
	nodeId uint8,
	timeoutMs uint32,
	entry12xx *od.Entry,
) (*SDOServer, error) {
	server := &SDOServer{BusManager: bm}
	if odict == nil || bm == nil || entry12xx == nil {
		return nil, canopen.ErrIllegalArgument
	}
	if logger == nil {
		logger = slog.Default()
	}
	server.logger = logger.With("service", "[SERVER]")
	server.od = odict
	server.streamer = &od.Streamer{}
	server.nodeId = nodeId
	server.timeoutTimeUs = timeoutMs * 1000
	server.blockTimeout = timeoutMs * 700
	server.rx = make(chan SDOMessage, 127)
	server.buf = bytes.NewBuffer(make([]byte, 0, 1000))
	server.intermediateBuf = make([]byte, 1000)
	var canIdClientToServer uint16
	var canIdServerToClient uint16
	if entry12xx.Index == 0x1200 {
		// Default channels
		if nodeId < 1 || nodeId > BlockMaxSize {
			server.logger.Error("node id is not valid", "nodeId", nodeId)
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
			server.logger.Error("error getting server params",
				"err0", err0,
				"err1", err1,
				"err2", err2,
				"maxSubindex", maxSubIndex)
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

// Check consistency between indicated size & transferred size
func (s *SDOServer) checkSizeConsitency() error {
	if s.sizeIndicated == 0 {
		return nil
	}

	if s.sizeTransferred > s.sizeIndicated {
		s.state = stateAbort
		return AbortDataLong
	}

	if s.state == stateIdle && s.sizeTransferred < s.sizeIndicated {
		s.state = stateAbort
		return AbortDataShort
	}

	return nil
}
