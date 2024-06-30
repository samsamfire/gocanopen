package sdo

import (
	"encoding/binary"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/od"
)

// [SDO server] update server parameters
func writeEntry1201(stream *od.Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || countWritten == nil {
		return od.ErrDevIncompat
	}
	server, ok := stream.Object.(*SDOServer)
	if !ok {
		return od.ErrDevIncompat
	}
	switch stream.Subindex {
	case 0:
		return od.ErrReadonly
	// cob id client to server
	case 1:
		cobId := binary.LittleEndian.Uint32(data)
		canId := uint16(cobId & 0x7FF)
		canIdCurrent := uint16(server.cobIdClientToServer & 0x7FF)
		valid := (cobId & 0x80000000) == 0
		if (cobId&0x3FFFF800) != 0 ||
			(valid && server.valid && canId != canIdCurrent) ||
			(valid && canopen.IsIDRestricted(canId)) {
			return od.ErrInvalidValue
		}
		err := server.initRxTx(cobId, server.cobIdServerToClient)
		if err != nil {
			return od.ErrDevIncompat
		}
	// cob id server to client
	case 2:
		cobId := binary.LittleEndian.Uint32(data)
		canId := uint16(cobId & 0x7FF)
		canIdCurrent := uint16(server.cobIdServerToClient & 0x7FF)
		valid := (cobId & 0x80000000) == 0
		if (cobId&0x3FFFF800) != 0 ||
			(valid && server.valid && canId != canIdCurrent) ||
			(valid && canopen.IsIDRestricted(canId)) {
			return od.ErrInvalidValue
		}
		err := server.initRxTx(server.cobIdClientToServer, cobId)
		if err != nil {
			return od.ErrDevIncompat
		}
	// node id of server
	case 3:
		if len(data) != 1 {
			return od.ErrTypeMismatch
		}
		nodeId := data[0]
		if nodeId < 1 || nodeId > 127 {
			return od.ErrInvalidValue
		}
		server.nodeId = nodeId // ??

	default:
		return od.ErrSubNotExist

	}
	return od.WriteEntryDefault(stream, data, countWritten)
}

// [SDO Client] update parameters
func writeEntry1280(stream *od.Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || countWritten == nil {
		return od.ErrDevIncompat
	}
	client, ok := stream.Object.(*SDOClient)
	if !ok {
		return od.ErrDevIncompat
	}
	switch stream.Subindex {
	case 0:
		return od.ErrReadonly
	// cob id client to server
	case 1:
		cobId := binary.LittleEndian.Uint32(data)
		canId := uint16(cobId & 0x7FF)
		canIdCurrent := uint16(client.cobIdClientToServer & 0x7FF)
		valid := (cobId & 0x80000000) == 0
		if (cobId&0x3FFFF800) != 0 ||
			(valid && client.valid && canId != canIdCurrent) ||
			(valid && canopen.IsIDRestricted(canId)) {
			return od.ErrInvalidValue
		}
		err := client.setupServer(cobId, client.cobIdServerToClient, client.nodeIdServer)
		if err != nil {
			return od.ErrDevIncompat
		}
	// cob id server to client
	case 2:
		cobId := binary.LittleEndian.Uint32(data)
		canId := uint16(cobId & 0x7FF)
		canIdCurrent := uint16(client.cobIdServerToClient & 0x7FF)
		valid := (cobId & 0x80000000) == 0
		if (cobId&0x3FFFF800) != 0 ||
			(valid && client.valid && canId != canIdCurrent) ||
			(valid && canopen.IsIDRestricted(canId)) {
			return od.ErrInvalidValue
		}
		err := client.setupServer(cobId, client.cobIdClientToServer, client.nodeIdServer)
		if err != nil {
			return od.ErrDevIncompat
		}
	// node id of server
	case 3:
		if len(data) != 1 {
			return od.ErrTypeMismatch
		}
		nodeId := data[0]
		if nodeId > 127 {
			return od.ErrInvalidValue
		}
		client.nodeIdServer = nodeId

	default:
		return od.ErrSubNotExist

	}
	return od.WriteEntryDefault(stream, data, countWritten)
}
