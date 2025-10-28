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
func (s *SDOServer) Handle(frame canopen.Frame) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if frame.DLC != 8 {
		return
	}
	rx := SDOMessage{}
	rx.raw = frame.Data

	select {
	case s.rx <- rx:
	default:
		s.logger.Warn("dropped SDO server RX frame")
		// Drop frame
	}

}

// Process [SDOServer] state machine and TX CAN frames
// It returns the global server state and error if any
// This should be called periodically
func (s *SDOServer) Process(ctx context.Context) (state uint8, err error) {

	s.logger.Info("starting sdo server processing")
	timeout := time.Duration(s.timeoutTimeUs * uint32(time.Microsecond))

	for {
		s.mu.Lock()
		nmtIsPreOrOperationnal := s.nmt == nmt.StateOperational || s.nmt == nmt.StatePreOperational
		s.mu.Unlock()

		select {
		case <-ctx.Done():
			s.logger.Info("exiting sdo server process")
			return
		default:
			if !s.valid || !nmtIsPreOrOperationnal {
				s.state = stateIdle
				// Sleep to avoid huge CPU load when idling
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}

		select {
		case rx := <-s.rx:
			// New frame received, do what we need to do !
			err := s.processIncoming(rx)
			if err != nil && err != od.ErrPartial {
				// Abort straight away, nothing to send afterwards
				s.txAbort(err)
				break
			}
			// A response is expected
			err = s.processOutgoing()
			if err != nil {
				s.txAbort(err)
			}

		case <-time.After(timeout):
			if s.state != stateIdle {
				s.txAbort(AbortTimeout)
			}
		}
	}
}

func (s *SDOServer) initRxTx(cobIdClientToServer uint32, cobIdServerToClient uint32) error {

	// Only proceed if parameters change (i.e. different client)
	if cobIdServerToClient == s.cobIdServerToClient && cobIdClientToServer == s.cobIdClientToServer {
		return nil
	}
	s.cobIdServerToClient = cobIdServerToClient
	s.cobIdClientToServer = cobIdClientToServer

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
		s.valid = true
	} else {
		CanIdC2S = 0
		CanIdS2C = 0
		s.valid = false
	}
	// Configure buffers, if initializing then insert in buffer, otherwise, update
	err := s.Subscribe(uint32(CanIdC2S), 0x7FF, false, s)
	if err != nil {
		s.valid = false
		return err
	}
	s.txBuffer = canopen.NewFrame(uint32(CanIdS2C), 0, 8)
	return nil
}

func (s *SDOServer) writeObjectDictionary(crcOperation uint, crcClient crc.CRC16) error {

	added := 0

	// Check transfer size is not bigger than indicated
	if s.sizeIndicated > 0 && s.sizeTransferred > s.sizeIndicated {
		s.state = stateAbort
		return AbortDataLong
	}

	if s.finished {
		// Check transfer size is not smaller than indicated
		if s.sizeIndicated > 0 && s.sizeTransferred < s.sizeIndicated {
			s.state = stateAbort
			return AbortDataShort
		}
		// Golang does not have null termination characters so nothing particular to do
		// Stream data should be limited to the sent value
		varSizeInOd := s.streamer.DataLength
		if s.streamer.HasAttribute(od.AttributeStr) &&
			(varSizeInOd == 0 || s.sizeTransferred < varSizeInOd) &&
			s.buf.Available() >= 2 {
			s.buf.Write([]byte{0})
			s.sizeTransferred++
			added++
			if varSizeInOd == 0 || s.sizeTransferred < varSizeInOd {
				s.buf.Write([]byte{0})
				s.sizeTransferred++
				added++
			}
			s.streamer.DataLength = s.sizeTransferred
		} else if varSizeInOd == 0 {
			s.streamer.DataLength = s.sizeTransferred
		} else if s.sizeTransferred != varSizeInOd {
			if s.sizeTransferred > varSizeInOd {
				s.state = stateAbort
				return AbortDataLong
			} else if s.sizeTransferred < varSizeInOd {
				s.state = stateAbort
				return AbortDataShort
			}
		}

	}

	// Calculate CRC
	if s.blockCRCEnabled && crcOperation > 0 {
		s.blockCRC.Block(s.buf.Bytes()[:s.buf.Len()-added])
		if crcOperation == 2 && crcClient != s.blockCRC {
			s.state = stateAbort
			s.errorExtraInfo = fmt.Errorf("server was expecting %v but got %v", s.blockCRC, crcClient)
			return AbortCRC
		}
	}

	// Transfer from buffer to OD
	_, err := io.Copy(s.streamer, s.buf)
	if err != nil && err != od.ErrPartial {
		s.state = stateAbort
		odr, ok := err.(od.ODR)
		if !ok {
			s.logger.Warn("unexpected error in server on io.Copy", "error", err)
			odr = od.ErrGeneral
		}

		return ConvertOdToSdoAbort(odr)
	}

	if s.finished && err == od.ErrPartial {
		s.state = stateAbort
		return AbortDataShort
	}

	if !s.finished && err == nil {
		s.state = stateAbort
		return AbortDataLong
	}
	return nil
}

// Read from OD into buffer & calculate CRC if needed
// Depending on the transfer type, this might have to be called multiple times
// countMin : threshold to refill data in buffer
// countExact : number of bytes to read if != -1 exactly
func (s *SDOServer) readObjectDictionary(countMin uint32, countExact int, calculateCRC bool) error {

	// If we already have at least coutMin unread in the buffer
	// We don't need to refill
	if s.finished || (uint32(s.buf.Len()) >= countMin && countExact == -1) {
		return nil
	}

	// Read from OD into an intermediate buffer, we are limited by the remaining space inside the buffer
	// countExact can be used to control precisely how much bytes are read.
	nbToRead := 0
	if countExact == -1 {
		nbToRead = min(len(s.intermediateBuf), s.buf.Cap()-s.buf.Len())
	} else {
		nbToRead = min(len(s.intermediateBuf), countExact)
	}

	countRd, err := s.streamer.Read(s.intermediateBuf[:nbToRead])
	if countExact != -1 && countExact != countRd {
		return AbortOutOfMem
	}

	if err != nil && err != od.ErrPartial {
		s.state = stateAbort
		odr, ok := err.(od.ODR)
		if !ok {
			s.logger.Warn("unexpected error in server when reading", "err", err)
			odr = od.ErrGeneral
		}
		return ConvertOdToSdoAbort(odr)
	}

	// Stop sending at null termination if string
	if countRd > 0 && s.streamer.HasAttribute(od.AttributeStr) {
		countStr := int(s.streamer.DataLength)
		for i, v := range s.intermediateBuf {
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
			s.streamer.DataLength = s.sizeTransferred + uint32(countRd)
		}
	}
	countWritten, err2 := s.buf.Write(s.intermediateBuf[:countRd])
	if countWritten != countRd || err2 != nil {
		s.logger.Error("failed to write to buffer the same amount as read",
			"countWritten", countWritten,
			"countRead", countRd,
			"err", err2,
		)
		return AbortDeviceIncompat
	}

	if err == od.ErrPartial {
		s.finished = false
		if uint32(countRd) < countMin {
			s.state = stateAbort
			s.errorExtraInfo = fmt.Errorf("buffer unread %v is less than the minimum count %v", s.buf.Len(), countMin)
			return AbortDeviceIncompat
		}
	} else {
		s.finished = true
	}

	if s.blockCRCEnabled && calculateCRC {
		s.blockCRC.Block(s.intermediateBuf[:countRd])
	}

	return nil
}

// Update streamer object with new requested entry
func (s *SDOServer) updateStreamer(response SDOMessage) error {

	var err error
	s.index = response.GetIndex()
	s.subindex = response.GetSubindex()
	s.streamer, err = s.od.Streamer(s.index, s.subindex, false)
	s.errorExtraInfo = nil
	if err != nil {
		odr, ok := err.(od.ODR)
		if !ok {
			s.logger.Warn("unexpected error in server creating streamer", "error", err)
			odr = od.ErrGeneral
		}
		return ConvertOdToSdoAbort(odr)
	}
	if !s.streamer.HasAttribute(od.AttributeSdoRw) {
		return AbortUnsupportedAccess
	}
	upload := s.state == stateUploadBlkInitiateReq || s.state == stateUploadInitiateReq

	if upload && !s.streamer.HasAttribute(od.AttributeSdoR) {
		return AbortWriteOnly
	}
	if !upload && !s.streamer.HasAttribute(od.AttributeSdoW) {
		return AbortReadOnly
	}

	// In case of reading, we need to prepare data now
	if upload {
		return s.prepareRx()
	}
	return nil
}

// Prepare read transfer
func (server *SDOServer) prepareRx() error {

	server.buf.Reset()
	server.sizeTransferred = 0
	server.finished = false

	// Load data from OD now
	err := server.readObjectDictionary(BlockSeqSize, -1, false)
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
	server.buf = bytes.NewBuffer(make([]byte, 0, 2000))
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
