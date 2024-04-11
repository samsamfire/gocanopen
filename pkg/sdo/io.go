package sdo

import (
	"encoding/binary"
	"io"
	"time"

	"github.com/samsamfire/gocanopen/pkg/od"
)

type sdoRawReadWriter struct {
	client       *SDOClient
	nodeId       uint8
	index        uint16
	subindex     uint8
	blockEnabled bool
	size         uint32
}

var defaultClientProcessPeriodUs = uint32(10000)

func (client *SDOClient) newRawReadWriter(nodeId uint8, index uint16, subindex uint8, blockEnabled bool, size uint32,
) (*sdoRawReadWriter, error) {
	rw := &sdoRawReadWriter{
		client:       client,
		nodeId:       nodeId,
		index:        index,
		subindex:     subindex,
		blockEnabled: blockEnabled,
	}
	// Setup client for a new transfer
	err := client.setupServer(
		uint32(CLIENT_ID)+uint32(nodeId),
		uint32(SERVER_ID)+uint32(nodeId),
		nodeId,
	)
	return rw, err
}

// Create a new raw SDO reader
// This does not need an object dictionary but no checks will be made for the expected data
// If blockEnabled is set to true, reading attempted using block transfer
// If counterpart does not support block transfer or if transfer size is too small, this should
// default to expedited / segmented transfer
func (client *SDOClient) NewRawReader(nodeId uint8, index uint16, subindex uint8, blockEnabled bool, size uint32,
) (io.Reader, error) {
	rw, err := client.newRawReadWriter(nodeId, index, subindex, blockEnabled, size)
	if err != nil {
		return nil, err
	}
	// Setup client for reading
	err = client.uploadSetup(index, subindex, blockEnabled)
	return rw, err
}

// Create a new raw SDO writer
// This does not need an object dictionary but no checks will be made for the expected data
// If blockEnabled is set to true, writing attempted using block transfer
// If counterpart does not support block transfer or if transfer size is too small, this should
// default to expedited / segmented transfer
func (client *SDOClient) NewRawWriter(nodeId uint8, index uint16, subindex uint8, blockEnabled bool, size uint32,
) (io.Writer, error) {
	rw, err := client.newRawReadWriter(nodeId, index, subindex, blockEnabled, size)
	if err != nil {
		return nil, err
	}
	// Setup client for writing
	err = client.downloadSetup(index, subindex, size, true)
	return rw, err
}

// Implements io.Reader interface
// Read bytes from remote node using sdo client
func (rw *sdoRawReadWriter) Read(b []byte) (n int, err error) {
	client := rw.client
	n = 0

	for {
		ret, err := client.upload(defaultClientProcessPeriodUs, false, nil, nil, nil)
		switch {
		case err != nil:
			return n, err
		case ret == SDO_UPLOAD_DATA_FULL:
			// Fifo needs emptying
			n += client.fifo.Read(b[n:], nil)
		case ret == SDO_SUCCESS:
			// Read finished successfully, empty fifo one last time and return EOF
			n += client.fifo.Read(b[n:], nil)
			return n, io.EOF
		}
		// If no more space in buffer return
		if n >= len(b) {
			return n, err
		}
		time.Sleep(time.Duration(defaultClientProcessPeriodUs) * time.Microsecond)
	}
}

// Read a given index/subindex from node into data
// This is blocking
func (client *SDOClient) ReadRaw(nodeId uint8, index uint16, subindex uint8, data []byte) (int, error) {
	r, err := client.NewRawReader(nodeId, index, subindex, true, 0) // size not specified
	if err != nil {
		return 0, err
	}
	return r.Read(data)
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
	for {
		ret, err := client.downloadMain(
			defaultClientProcessPeriodUs,
			bufferPartial,
			true,
			&nUint32,
			nil,
			false,
		)
		switch {
		case err != nil:
			return int(nUint32), err
		case ret == SDO_BLOCK_DOWNLOAD_IN_PROGRESS && bufferPartial:
			// Fill buffer whilst block download in progress
			n += client.fifo.Write(b[n:], nil)
			if n == len(b) {
				bufferPartial = false
			}
		case ret == SDO_SUCCESS:
			return int(nUint32), err
		}
		time.Sleep(time.Duration(defaultClientProcessPeriodUs) * time.Microsecond)
	}
}

// Write a given index/subindex from node into data
// This is blocking
func (client *SDOClient) WriteRaw(nodeId uint8, index uint16, subindex uint8, data any, forceSegmented bool) error {
	_ = forceSegmented
	encoded, err := od.EncodeFromGeneric(data)
	if err != nil {
		return err
	}
	w, err := client.NewRawWriter(nodeId, index, subindex, true, uint32(len(encoded))) // size not specified
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
		return 0, od.ODR_TYPE_MISMATCH
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
		return 0, od.ODR_TYPE_MISMATCH
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
		return 0, od.ODR_TYPE_MISMATCH
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
		return 0, od.ODR_TYPE_MISMATCH
	}
	return binary.LittleEndian.Uint64(buf), nil
}
