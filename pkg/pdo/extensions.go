package pdo

import (
	"encoding/binary"
	"fmt"
	"slices"
	"sync"
	"time"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/od"
)

const (
	CobIdValidBit  = 0x80000000
	CobIdCanIdMask = 0x000007FF
	// Mask for bits that MUST be zero (bits 11-29 excluding specific flags)
	CobIdReservedMask           = 0x1FFFF800
	CobIdCanIdWithoutNodeIdMask = 0xFFFFFF80
	CanIdWithoutNodeIdMask      = 0xFF80
	// CobId validity mask
	// bits 11 to 29 should be 0, pdo should be disabled and can id 0 is prohibited
	CobIdValidityMask = 0x3FFFF800
)

// [RPDO] update communication parameter
// refer to chapter 7.5.2.35
func writeEntry14xx(stream *od.Stream, data []byte) (uint16, error) {
	if stream == nil || data == nil || len(data) > 4 {
		return 0, od.ErrDevIncompat
	}
	rpdo, ok := stream.Object.(*RPDO)
	if !ok {
		return 0, od.ErrDevIncompat
	}
	rpdo.mu.Lock()
	defer rpdo.mu.Unlock()

	pdo := rpdo.pdo
	dataCopy := slices.Clone(data)

	switch stream.Subindex {

	case od.SubPdoCobId:
		cobId := binary.LittleEndian.Uint32(data)
		canId := cobId & CobIdCanIdMask
		valid := (cobId & CobIdValidBit) == 0

		rpdo.pdo.logger.Debug("updating cob-id",
			"valid", valid,
			"canId", fmt.Sprintf("x%x", canId),
			"cobId", fmt.Sprintf("x%x", cobId),
		)

		if (cobId&CobIdValidityMask) != 0 ||
			valid && pdo.Valid && canId != uint32(pdo.configuredId) ||
			valid && canopen.IsIDRestricted(uint16(canId)) ||
			valid && pdo.nbMapped == 0 {
			return 0, od.ErrInvalidValue
		}

		if valid != pdo.Valid || canId != uint32(pdo.configuredId) {
			// If the default od value (predefined id) is the same
			// then we do not keep the node id
			if canId == uint32(pdo.predefinedId) {
				binary.LittleEndian.PutUint32(dataCopy, cobId&CobIdCanIdWithoutNodeIdMask)
			}
			if !valid {
				canId = 0
			}
			if rpdo.rxCancel != nil {
				rpdo.rxCancel()
			}
			rxCancel, err := rpdo.Subscribe(canId, 0x7FF, false, rpdo)
			rpdo.rxCancel = rxCancel
			if valid && err == nil {
				pdo.Valid = true
				pdo.configuredId = uint16(canId)
				rpdo.pdo.logger.Debug("updated cob-id",
					"valid", valid,
					"canId", fmt.Sprintf("x%x", pdo.configuredId&0x7FF),
				)
			} else {
				pdo.Valid = false
				rpdo.rxNew = false
				if err != nil {
					return 0, od.ErrDevIncompat
				}
			}
		}

	case od.SubPdoTransmissionType:
		transType := data[0]
		if transType > TransmissionTypeSync240 && transType < TransmissionTypeSyncEventLo {
			return 0, od.ErrInvalidValue
		}
		synchronous := transType <= TransmissionTypeSync240
		if rpdo.synchronous != synchronous {
			rpdo.rxNew = false
			if synchronous {
				if rpdo.sync != nil && rpdo.syncCh == nil {
					rpdo.syncCh = rpdo.sync.Subscribe()
					go rpdo.syncHandler()
				}
			} else {
				if rpdo.sync != nil && rpdo.syncCh != nil {
					rpdo.sync.Unsubscribe(rpdo.syncCh)
					rpdo.syncCh = nil
				}
			}
		}
		rpdo.pdo.logger.Debug("updated transmission type (synchronous)", "prev", rpdo.synchronous, "new", synchronous)
		rpdo.synchronous = synchronous

	case od.SubPdoInhibitTime:
		// No particular processing
		break

	case od.SubPdoReserved:
		return 0, od.ErrSubNotExist

	case od.SubPdoEventTimer:
		eventTimer := binary.LittleEndian.Uint16(data)
		rpdo.timeoutRx = time.Duration(eventTimer) * time.Millisecond
		if rpdo.timer != nil {
			rpdo.timer.Stop()
		}
		rpdo.pdo.logger.Debug("updated event timer", "eventTimer", rpdo.timeoutRx)

	case od.SubPdoSyncStart:
		return 0, od.ErrSubNotExist
	}

	return od.WriteEntryDefault(stream, dataCopy)
}

// [TPDO] update communication parameter
// refer to chapter 7.5.2.37
func writeEntry18xx(stream *od.Stream, data []byte) (uint16, error) {
	if stream == nil || data == nil || len(data) > 4 {
		return 0, od.ErrDevIncompat
	}
	tpdo, ok := stream.Object.(*TPDO)
	if !ok {
		return 0, od.ErrDevIncompat
	}
	tpdo.mu.Lock()
	defer tpdo.mu.Unlock()

	pdo := tpdo.pdo
	dataCopy := slices.Clone(data)

	switch stream.Subindex {

	case od.SubPdoCobId:
		cobId := binary.LittleEndian.Uint32(data)
		canId := cobId & CobIdCanIdMask
		valid := (cobId & CobIdValidBit) == 0

		tpdo.pdo.logger.Debug("updating cob-id",
			"valid", valid,
			"canId", fmt.Sprintf("x%x", canId),
			"cobId", fmt.Sprintf("x%x", cobId),
		)

		if (cobId&CobIdValidityMask) != 0 ||
			valid && pdo.Valid && canId != uint32(pdo.configuredId) ||
			valid && canopen.IsIDRestricted(uint16(canId)) ||
			valid && pdo.nbMapped == 0 {
			return 0, od.ErrInvalidValue
		}

		if valid != pdo.Valid || canId != uint32(pdo.configuredId) {
			// If the default od value (predefined id) is the same
			// then we do not keep the node id
			if canId == uint32(pdo.predefinedId) {
				binary.LittleEndian.PutUint32(dataCopy, cobId&CobIdCanIdWithoutNodeIdMask)
			}
			if !valid {
				canId = 0
			}
			tpdo.txBuffer = canopen.NewFrame(canId, 0, uint8(pdo.dataLength))
			pdo.Valid = valid
			pdo.configuredId = uint16(canId)
		}

	case od.SubPdoTransmissionType:
		transType := data[0]
		if transType > TransmissionTypeSync240 && transType < TransmissionTypeSyncEventLo {
			return 0, od.ErrInvalidValue
		}
		tpdo.syncCounter = SyncCounterReset
		tpdo.transmissionType = transType
		tpdo.sendRequest = true
		tpdo.inhibitTimer = 0
		tpdo.eventTimer = 0

	case od.SubPdoInhibitTime:
		if pdo.Valid {
			return 0, od.ErrInvalidValue
		}
		inhibitTime := binary.LittleEndian.Uint16(data)
		tpdo.inhibitTimeUs = uint32(inhibitTime) * 100
		tpdo.inhibitTimer = 0

	case od.SubPdoReserved:
		return 0, od.ErrSubNotExist

	case od.SubPdoEventTimer:
		eventTime := binary.LittleEndian.Uint16(data)
		tpdo.eventTimeUs = uint32(eventTime) * 1000
		tpdo.eventTimer = 0
		tpdo.pdo.logger.Debug("updated event timer", "eventTimeUs", tpdo.eventTimeUs)

	case od.SubPdoSyncStart:
		syncStart := data[0]
		if pdo.Valid || syncStart > TransmissionTypeSync240 {
			return 0, od.ErrInvalidValue
		}
		tpdo.syncStartValue = syncStart

	}
	return od.WriteEntryDefault(stream, dataCopy)
}

// [RPDO][TPDO] get communication parameter
// refer to chapter 7.5.2.35 & 7.5.2.37
func readEntry14xxOr18xx(stream *od.Stream, data []byte) (uint16, error) {

	// Reading communication parameters does not require
	// special post-processing except for cob id sub entry
	n, err := od.ReadEntryDefault(stream, data)
	if err != nil {
		return n, err
	}

	// Get the corresponding object, either TPDO or RPDO
	var (
		pdo  *PDOCommon
		lock sync.Locker
	)
	switch v := stream.Object.(type) {
	case *RPDO:
		pdo, lock = v.pdo, &v.mu
		if stream.Subindex == od.SubPdoSyncStart {
			return 0, od.ErrSubNotExist
		}
	case *TPDO:
		pdo, lock = v.pdo, &v.mu
	default:
		return n, od.ErrDevIncompat
	}

	if stream.Subindex != od.SubPdoCobId {
		return n, err
	}

	if n != 4 {
		// This should never happen
		return n, od.ErrTypeMismatch
	}

	lock.Lock()
	defer lock.Unlock()

	cobId := binary.LittleEndian.Uint32(data)
	canId := uint16(cobId & CobIdCanIdMask)
	baseId := pdo.predefinedId & CanIdWithoutNodeIdMask

	// Add node id if not contained
	if canId != 0 && canId == baseId {
		// Clear lower 16 bits (CanId) and set the full predefined ID
		cobId = (cobId & 0xFFFF0000) | uint32(pdo.predefinedId)
	}
	// Apply valid bit
	if !pdo.Valid {
		cobId |= CobIdValidBit
	}
	binary.LittleEndian.PutUint32(data, cobId)

	return n, err
}

// [RPDO][TPDO] update mapping parameter
// refer to chapter 7.5.2.36 & 7.5.2.38
func writeEntry16xxOr1Axx(stream *od.Stream, data []byte) (uint16, error) {
	if stream == nil || data == nil || stream.Subindex > od.MaxMappedEntriesPdo {
		return 0, od.ErrDevIncompat
	}
	// Get the corresponding object, either TPDO or RPDO
	var (
		pdo  *PDOCommon
		lock sync.Locker
	)
	switch v := stream.Object.(type) {
	case *RPDO:
		pdo, lock = v.pdo, &v.mu
	case *TPDO:
		pdo, lock = v.pdo, &v.mu
	default:
		return 0, od.ErrDevIncompat
	}
	lock.Lock()
	defer lock.Unlock()

	// PDO must be disabled in order to allow mapping
	if pdo.Valid || pdo.nbMapped != 0 && stream.Subindex > 0 {
		return 0, od.ErrUnsuppAccess
	}

	// Change of a mapping parameter
	if stream.Subindex != od.SubPdoNbMappings {
		err := pdo.configureMap(binary.LittleEndian.Uint32(data), uint32(stream.Subindex)-1, pdo.IsRPDO)
		if err != nil {
			return 0, err
		}
		return od.WriteEntryDefault(stream, data)
	}

	// Change in number of mapped objects
	nbMapped := data[0]
	pdoDataLength := uint32(0)

	// Don't allow number greater than possible mapped objects
	if nbMapped > od.MaxMappedEntriesPdo {
		return 0, od.ErrMapLen
	}

	for i := range nbMapped {
		streamer := pdo.streamers[i]
		dataLength := streamer.DataLength
		mappedLength := streamer.DataOffset
		if mappedLength > dataLength {
			return 0, od.ErrNoMap
		}
		pdoDataLength += mappedLength
	}

	// Total length should not exceed max PDO length
	if pdoDataLength > uint32(MaxPdoLength) {
		return 0, od.ErrMapLen
	}

	// Total length cannot be zero if there is at least one mapped object
	if pdoDataLength == 0 && nbMapped > 0 {
		return 0, od.ErrInvalidValue
	}

	pdo.dataLength = pdoDataLength
	pdo.nbMapped = nbMapped
	pdo.logger.Debug("updated number of mapped objects to", "count", nbMapped, "lengthBytes", pdo.dataLength)

	return od.WriteEntryDefault(stream, data)
}

// [RPDO][TPDO] write method that fakes writing an OD variable
func WriteDummy(stream *od.Stream, data []byte) (uint16, error) {
	return uint16(len(data)), nil
}

// [RPDO][TPDO] read method that fakes reading an OD variable
func ReadDummy(stream *od.Stream, data []byte) (uint16, error) {
	if data == nil || stream == nil {
		return 0, od.ErrDevIncompat
	}
	if len(data) > len(stream.Data) {
		return uint16(len(stream.Data)), nil
	}
	return uint16(len(data)), nil
}
