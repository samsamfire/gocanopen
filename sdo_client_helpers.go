package canopen

import (
	"encoding/binary"
	"math"
	"time"
)

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

func (client *SDOClient) ReadUint8(nodeId uint8, index uint16, subindex uint8) (uint8, error) {
	buf := make([]byte, 1)
	n, err := client.ReadRaw(nodeId, index, subindex, buf)
	if err != nil {
		return 0, err
	} else if n != 1 {
		return 0, ODR_TYPE_MISMATCH
	}
	return buf[0], nil
}

func (client *SDOClient) ReadUint16(nodeId uint8, index uint16, subindex uint8) (uint16, error) {
	buf := make([]byte, 2)
	n, err := client.ReadRaw(nodeId, index, subindex, buf)
	if err != nil {
		return 0, err
	} else if n != 2 {
		return 0, ODR_TYPE_MISMATCH
	}
	return binary.LittleEndian.Uint16(buf), nil
}

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

func (client *SDOClient) ReadUint64(nodeId uint8, index uint16, subindex uint8) (uint64, error) {
	buf := make([]byte, 8)
	n, err := client.ReadRaw(nodeId, index, subindex, buf)
	if err != nil {
		return 0, err
	} else if n != 8 {
		return 0, ODR_TYPE_MISMATCH
	}
	return binary.LittleEndian.Uint64(buf), nil
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
