package sdo

import (
	"encoding/binary"
	"io"
	"time"

	"github.com/samsamfire/gocanopen/v2/pkg/od"
)

// Delay before re-sending a block segment that failed because the CAN
// tx queue was full
const txBusyRetryDelay = 500 * time.Microsecond

type sdoRawReadWriter struct {
	client *SDOClient
	// Upload is finished, remaining data is only in the fifo
	uploadDone bool
}

// Create a new raw SDO reader
// This does not need an object dictionary but no checks will be made for the expected data
// If blockEnabled is set to true, reading attempted using block transfer
// If counterpart does not support block transfer or if transfer size is too small, this should
// default to expedited / segmented transfer
func (client *SDOClient) NewRawReader(nodeId uint8, index uint16, subindex uint8, blockEnabled bool, size uint32,
) (io.Reader, error) {
	// Setup client for a new transfer
	err := client.setupServer(
		uint32(ClientServiceId)+uint32(nodeId),
		uint32(ServerServiceId)+uint32(nodeId),
		nodeId,
	)
	if err != nil {
		return nil, err
	}
	// Setup client for reading
	err = client.uploadSetup(index, subindex, blockEnabled)
	return client.rw, err
}

// Create a new raw SDO writer
// This does not need an object dictionary but no checks will be made for the expected data
// If blockEnabled is set to true, writing attempted using block transfer
// If counterpart does not support block transfer or if transfer size is too small, this should
// default to expedited / segmented transfer
func (client *SDOClient) NewRawWriter(nodeId uint8, index uint16, subindex uint8, blockEnabled bool, size uint32,
) (io.Writer, error) {
	// Setup client for a new transfer
	err := client.setupServer(
		uint32(ClientServiceId)+uint32(nodeId),
		uint32(ServerServiceId)+uint32(nodeId),
		nodeId,
	)
	if err != nil {
		return nil, err
	}
	// Setup client for writing
	err = client.downloadSetup(index, subindex, size, blockEnabled)
	return client.rw, err
}

// Implements io.Reader interface
// Read bytes from remote node using sdo client
func (rw *sdoRawReadWriter) Read(b []byte) (n int, err error) {
	client := rw.client
	if rw.uploadDone {
		return rw.readRemaining(b)
	}
	n = 0
	last := time.Now()

	for {
		elapsed := time.Since(last)
		last = time.Now()

		ret, err := client.upload(uint32(elapsed.Microseconds()), false, nil, nil)
		switch {
		case err != nil:
			return n, err
		case ret == success:
			// Read finished successfully, empty fifo
			rw.uploadDone = true
			m, err := rw.readRemaining(b[n:])
			return n + m, err
		case ret == uploadDataFull:
			// Fifo needs emptying, then process again straight away
			n += client.fifo.Read(b[n:], nil)
			if n >= len(b) {
				return n, nil
			}
		case ret == waitingLocalTransfer:
			// Local transfer, nothing to wait for
		default:
			// Waiting for the server
			client.waitEvent(ret)
		}
	}
}

// Empty the fifo once the upload is finished. b may be too small to hold
// everything left, so EOF is only returned once the fifo is empty.
func (rw *sdoRawReadWriter) readRemaining(b []byte) (int, error) {
	n := rw.client.fifo.Read(b, nil)
	if rw.client.fifo.GetOccupied() > 0 {
		return n, nil
	}
	return n, io.EOF
}

// Read a given index/subindex from node into data
// This is blocking
func (client *SDOClient) ReadRaw(nodeId uint8, index uint16, subindex uint8, data []byte) (int, error) {
	r, err := client.NewRawReader(nodeId, index, subindex, false, 0) // size not specified
	if err != nil {
		return 0, err
	}
	n, err := r.Read(data)
	if err != nil && err != io.EOF {
		return n, err
	}
	return n, nil
}

// Read everything from a given index/subindex from node and return all bytes
// Similar to io.ReadAll
func (client *SDOClient) ReadAll(nodeId uint8, index uint16, subindex uint8) ([]byte, error) {
	r, err := client.NewRawReader(nodeId, index, subindex, true, 0) // size not specified
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

// Implements io.Writer interface
// Write bytes from remote node using sdo client
// Writing in several iterations is only possible in block transfers
// as in regular small transfers, client state machine starts processing
// internal fifo as soon a we call downloadMain. This means that for small
// transfers, exact size should be written.
func (rw *sdoRawReadWriter) Write(b []byte) (n int, err error) {
	client := rw.client

	// Fill fifo buffer
	nUint32 := uint32(0)
	bufferPartial := false // whether we will need to re-fill fifo
	n += client.fifo.Write(b, nil)
	if n < len(b) {
		bufferPartial = true
	}
	last := time.Now()
	for {
		elapsed := time.Since(last)
		last = time.Now()

		ret, err := client.downloadMain(
			uint32(elapsed.Microseconds()),
			false,
			bufferPartial,
			&nUint32,
			false,
		)
		switch {
		case err != nil:
			return int(nUint32), err
		case ret == success:
			return int(nUint32), err
		case ret == blockDownloadInProgress:
			// Fill buffer whilst block download in progress
			if bufferPartial {
				n += client.fifo.Write(b[n:], nil)
				if n == len(b) {
					bufferPartial = false
				}
			}
			if client.processingPeriod > 0 {
				time.Sleep(client.processingPeriod)
			}
		case ret == transmitBufferFull:
			// CAN tx queue is full, give it some time to empty
			time.Sleep(max(client.processingPeriod, txBusyRetryDelay))
		case ret == waitingLocalTransfer:
			// Local transfer, nothing to wait for
		default:
			// Waiting for the server
			client.waitEvent(ret)
		}
	}
}

// Write a given index/subindex from node into data
// This is blocking
func (client *SDOClient) WriteRaw(nodeId uint8, index uint16, subindex uint8, data any, forceSegmented bool) error {
	_ = forceSegmented
	encoded, err := od.EncodeFromTypeExact(data)
	if err != nil {
		return err
	}
	w, err := client.NewRawWriter(nodeId, index, subindex, false, uint32(len(encoded)))
	if err != nil {
		return err
	}
	_, err = w.Write(encoded)
	return err
}

// Helper function for reading directly a uint8
func (client *SDOClient) ReadUint8(nodeId uint8, index uint16, subindex uint8) (uint8, error) {
	buf := make([]byte, 1)
	n, err := client.ReadRaw(nodeId, index, subindex, buf)
	if err != nil {
		return 0, err
	} else if n != 1 {
		return 0, od.ErrTypeMismatch
	}
	return buf[0], nil
}

// Helper function for reading directly a uint16
func (client *SDOClient) ReadUint16(nodeId uint8, index uint16, subindex uint8) (uint16, error) {
	buf := make([]byte, 2)
	n, err := client.ReadRaw(nodeId, index, subindex, buf)
	if err != nil {
		return 0, err
	} else if n != 2 {
		return 0, od.ErrTypeMismatch
	}
	return binary.LittleEndian.Uint16(buf), nil
}

// Helper function for reading directly a uint32
func (client *SDOClient) ReadUint32(nodeId uint8, index uint16, subindex uint8) (uint32, error) {
	buf := make([]byte, 4)
	n, err := client.ReadRaw(nodeId, index, subindex, buf)
	if err != nil {
		return 0, err
	} else if n != 4 {
		return 0, od.ErrTypeMismatch
	}
	return binary.LittleEndian.Uint32(buf), nil
}

// Helper function for reading directly a uint64
func (client *SDOClient) ReadUint64(nodeId uint8, index uint16, subindex uint8) (uint64, error) {
	buf := make([]byte, 8)
	n, err := client.ReadRaw(nodeId, index, subindex, buf)
	if err != nil {
		return 0, err
	} else if n != 8 {
		return 0, od.ErrTypeMismatch
	}
	return binary.LittleEndian.Uint64(buf), nil
}
