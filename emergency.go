package canopen

import (
	"encoding/binary"

	log "github.com/sirupsen/logrus"
)

const CO_CONFIG_EM_ERR_STATUS_BITS_COUNT = 80

// Error register values
const (
	emErrRegGeneric       = 0x01 // bit 0 - generic error
	emErrRegCurrent       = 0x02 // bit 1 - current
	emErrRegVoltage       = 0x04 // bit 2 - voltage
	emErrRegTemperature   = 0x08 // bit 3 - temperature
	emErrRegCommunication = 0x10 // bit 4 - communication error
	emErrRegDevProfile    = 0x20 // bit 5 - device profile specific
	emErrRegReserved      = 0x40 // bit 6 - reserved (always 0)
	emErrRegManufacturer  = 0x80 // bit 7 - manufacturer specific
)

// Error codes
const (
	emErrNoError          = 0x0000
	emErrGeneric          = 0x1000
	emErrCurrent          = 0x2000
	emErrCurrentInput     = 0x2100
	emErrCurrentInside    = 0x2200
	emErrCurrentOutput    = 0x2300
	emErrVoltage          = 0x3000
	emErrVoltageMains     = 0x3100
	emErrVoltageInside    = 0x3200
	emErrVoltageOutput    = 0x3300
	emErrTemperature      = 0x4000
	emErrTempAmbient      = 0x4100
	emErrTempDevice       = 0x4200
	emErrHardware         = 0x5000
	emErrSoftwareDevice   = 0x6000
	emErrSoftwareInternal = 0x6100
	emErrSoftwareUser     = 0x6200
	emErrDataSet          = 0x6300
	emErrAdditionalModul  = 0x7000
	emErrMonitoring       = 0x8000
	emErrCommunication    = 0x8100
	emErrCanOverrun       = 0x8110
	emErrCanPassive       = 0x8120
	emErrHeartbeat        = 0x8130
	emErrBusOffRecovered  = 0x8140
	emErrCanIdCollision   = 0x8150
	emErrProtocolError    = 0x8200
	emErrPdoLength        = 0x8210
	emErrPdoLengthExc     = 0x8220
	emErrDamMpdo          = 0x8230
	emErrSyncDataLength   = 0x8240
	emErrRpdoTimeout      = 0x8250
	emErrExternalError    = 0x9000
	emErrAdditionalFunc   = 0xF000
	emErrDeviceSpecific   = 0xFF00
	emErr401OutCurHi      = 0x2310
	emErr401OutShorted    = 0x2320
	emErr401OutLoadDump   = 0x2330
	emErr401InVoltHi      = 0x3110
	emErr401InVoltLow     = 0x3120
	emErr401InternVoltHi  = 0x3210
	emErr401InternVoltLow = 0x3220
	emErr401OutVoltHigh   = 0x3310
	emErr401OutVoltLow    = 0x3320
)

var errorCodeDescriptionMap = map[int]string{
	emErrNoError:          "Reset or No Error",
	emErrGeneric:          "Generic Error",
	emErrCurrent:          "Current",
	emErrCurrentInput:     "Current, device input side",
	emErrCurrentInside:    "Current inside the device",
	emErrCurrentOutput:    "Current, device output side",
	emErrVoltage:          "Voltage",
	emErrVoltageMains:     "Mains Voltage",
	emErrVoltageInside:    "Voltage inside the device",
	emErrVoltageOutput:    "Output Voltage",
	emErrTemperature:      "Temperature",
	emErrTempAmbient:      "Ambient Temperature",
	emErrTempDevice:       "Device Temperature",
	emErrHardware:         "Device Hardware",
	emErrSoftwareDevice:   "Device Software",
	emErrSoftwareInternal: "Internal Software",
	emErrSoftwareUser:     "User Software",
	emErrDataSet:          "Data Set",
	emErrAdditionalModul:  "Additional Modules",
	emErrMonitoring:       "Monitoring",
	emErrCommunication:    "Communication",
	emErrCanOverrun:       "CAN Overrun (Objects lost)",
	emErrCanPassive:       "CAN in Error Passive Mode",
	emErrHeartbeat:        "Life Guard Error or Heartbeat Error",
	emErrBusOffRecovered:  "Recovered from bus off",
	emErrCanIdCollision:   "CAN-ID collision",
	emErrProtocolError:    "Protocol Error",
	emErrPdoLength:        "PDO not processed due to length error",
	emErrPdoLengthExc:     "PDO length exceeded",
	emErrDamMpdo:          "DAM MPDO not processed, destination object not available",
	emErrSyncDataLength:   "Unexpected SYNC data length",
	emErrRpdoTimeout:      "RPDO timeout",
	emErrExternalError:    "External Error",
	emErrAdditionalFunc:   "Additional Functions",
	emErrDeviceSpecific:   "Device specific",
	emErr401OutCurHi:      "DS401, Current at outputs too high (overload)",
	emErr401OutShorted:    "DS401, Short circuit at outputs",
	emErr401OutLoadDump:   "DS401, Load dump at outputs",
	emErr401InVoltHi:      "DS401, Input voltage too high",
	emErr401InVoltLow:     "DS401, Input voltage too low",
	emErr401InternVoltHi:  "DS401, Internal voltage too high",
	emErr401InternVoltLow: "DS401, Internal voltage too low",
	emErr401OutVoltHigh:   "DS401, Output voltage too high",
	emErr401OutVoltLow:    "DS401, Output voltage too low",
}

// Error status bits
const (
	emNoError                 = 0x00
	emCanBusWarning           = 0x01
	emRxMsgWrongLength        = 0x02
	emRxMsgOverflow           = 0x03
	emRPDOWrongLength         = 0x04
	emRPDOOverflow            = 0x05
	emCanRXBusPassive         = 0x06
	emCanTXBusPassive         = 0x07
	emNMTWrongCommand         = 0x08
	emTimeTimeout             = 0x09
	em0AUnused                = 0x0A
	em0BUnused                = 0x0B
	em0CUnused                = 0x0C
	em0DUnused                = 0x0D
	em0EUnused                = 0x0E
	em0FUnused                = 0x0F
	em10Unused                = 0x10
	em11Unused                = 0x11
	emCanTXBusOff             = 0x12
	emCanRXBOverflow          = 0x13
	emCanTXOverflow           = 0x14
	emTPDOOutsideWindow       = 0x15
	em16Unused                = 0x16
	emRPDOTimeOut             = 0x17
	emSyncTimeOut             = 0x18
	emSyncLength              = 0x19
	emPDOWrongMapping         = 0x1A
	emHeartbeatConsumer       = 0x1B
	emHBConsumerRemoteReset   = 0x1C
	em1DUnused                = 0x1D
	em1EUnused                = 0x1E
	em1FUnused                = 0x1F
	emEmergencyBufferFull     = 0x20
	em21Unused                = 0x21
	emMicrocontrollerReset    = 0x22
	em23Unused                = 0x23
	em24Unused                = 0x24
	em25Unused                = 0x25
	em26Unused                = 0x26
	emNonVolatileAutoSave     = 0x27
	emWrongErrorReport        = 0x28
	emISRTimerOverflow        = 0x29
	emMemoryAllocationError   = 0x2A
	emGenericError            = 0x2B
	emGenericSoftwareError    = 0x2C
	emInconsistentObjectDict  = 0x2D
	emCalculationOfParameters = 0x2E
	emNonVolatileMemory       = 0x2F
	emManufacturerStart       = 0x30
	emManufacturerEnd         = CO_CONFIG_EM_ERR_STATUS_BITS_COUNT - 1
)

var errorStatusMap = map[uint8]string{
	emNoError:                 "Error Reset or No Error",
	emCanBusWarning:           "CAN bus warning limit reached",
	emRxMsgWrongLength:        "Wrong data length of the received CAN message",
	emRxMsgOverflow:           "Previous received CAN message wasn't processed yet",
	emRPDOWrongLength:         "Wrong data length of received PDO",
	emRPDOOverflow:            "Previous received PDO wasn't processed yet",
	emCanRXBusPassive:         "CAN receive bus is passive",
	emCanTXBusPassive:         "CAN transmit bus is passive",
	emNMTWrongCommand:         "Wrong NMT command received",
	emTimeTimeout:             "TIME message timeout",
	em0AUnused:                "(unused)",
	em0BUnused:                "(unused)",
	em0CUnused:                "(unused)",
	em0DUnused:                "(unused)",
	em0EUnused:                "(unused)",
	em0FUnused:                "(unused)",
	em10Unused:                "(unused)",
	em11Unused:                "(unused)",
	emCanTXBusOff:             "CAN transmit bus is off",
	emCanRXBOverflow:          "CAN module receive buffer has overflowed",
	emCanTXOverflow:           "CAN transmit buffer has overflowed",
	emTPDOOutsideWindow:       "TPDO is outside SYNC window",
	em16Unused:                "(unused)",
	emRPDOTimeOut:             "RPDO message timeout",
	emSyncTimeOut:             "SYNC message timeout",
	emSyncLength:              "Unexpected SYNC data length",
	emPDOWrongMapping:         "Error with PDO mapping",
	emHeartbeatConsumer:       "Heartbeat consumer timeout",
	emHBConsumerRemoteReset:   "Heartbeat consumer detected remote node reset",
	em1DUnused:                "(unused)",
	em1EUnused:                "(unused)",
	em1FUnused:                "(unused)",
	emEmergencyBufferFull:     "Emergency buffer is full, Emergency message wasn't sent",
	em21Unused:                "(unused)",
	emMicrocontrollerReset:    "Microcontroller has just started",
	em23Unused:                "(unused)",
	em24Unused:                "(unused)",
	em25Unused:                "(unused)",
	em26Unused:                "(unused)",
	emNonVolatileAutoSave:     "Automatic store to non-volatile memory failed",
	emWrongErrorReport:        "Wrong parameters to ErrorReport function",
	emISRTimerOverflow:        "Timer task has overflowed",
	emMemoryAllocationError:   "Unable to allocate memory for objects",
	emGenericError:            "Generic error, test usage",
	emGenericSoftwareError:    "Software error",
	emInconsistentObjectDict:  "Object dictionary does not match the software",
	emCalculationOfParameters: "Error in calculation of device parameters",
	emNonVolatileMemory:       "Error with access to non-volatile device memory",
}

func getErrorStatusDescription(errorStatus uint8) string {
	description, ok := errorStatusMap[errorStatus]
	if ok {
		return description
	} else if errorStatus >= emManufacturerStart && errorStatus <= emManufacturerEnd {
		return "Manufacturer error"
	} else {
		return "Invalid or not implemented error status"
	}
}

func getErrorCodeDescription(errorCode int) string {
	description, ok := errorCodeDescriptionMap[errorCode]
	if ok {
		return description
	} else {
		return "Invalid or not implemented error code"
	}
}

// Fifo for emergency
type emfifo struct {
	msg  uint32
	info uint32
}

// Emergency object callback on message reception, including own
type EMCYRxCallback func(ident uint16, errorCode uint16, errorRegister byte, errorBit byte, infoCode uint32)

// Emergency object for receiving & transmitting emergencies
type EMCY struct {
	*busManager
	nodeId          byte
	errorStatusBits [CO_CONFIG_EM_ERR_STATUS_BITS_COUNT / 8]byte
	errorRegister   *byte
	canErrorOld     uint16
	txBuffer        Frame
	fifo            []emfifo
	fifoWrPtr       byte
	fifoPpPtr       byte
	fifoOverflow    byte
	fifoCount       byte
	producerEnabled bool
	producerIdent   uint16
	inhibitTimeUs   uint32 // Changed by writing to object 0x1015
	inhibitTimer    uint32
	rxCallback      EMCYRxCallback
}

func readEntryStatusBits(stream *Stream, data []byte, countRead *uint16) error {
	if stream == nil || stream.Subindex != 0 || data == nil || countRead == nil {
		return ODR_DEV_INCOMPAT
	}
	em, ok := stream.Object.(*EMCY)
	if !ok {
		return ODR_DEV_INCOMPAT
	}

	countReadLocal := CO_CONFIG_EM_ERR_STATUS_BITS_COUNT / 8
	if countReadLocal > len(data) {
		countReadLocal = len(data)
	}
	if len(stream.Data) != 0 && countReadLocal > len(stream.Data) {
		countReadLocal = len(stream.Data)
	} // Unclear why we change datalength
	copy(data, em.errorStatusBits[:countReadLocal])
	*countRead = uint16(countReadLocal)
	return nil
}

func writeEntryStatusBits(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || stream.Subindex != 0 || countWritten == nil || data == nil {
		return ODR_DEV_INCOMPAT
	}
	em, ok := stream.Object.(*EMCY)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	countWriteLocal := CO_CONFIG_EM_ERR_STATUS_BITS_COUNT / 8
	if countWriteLocal > len(data) {
		countWriteLocal = len(data)
	}
	if len(stream.Data) != 0 && countWriteLocal > len(stream.Data) {
		countWriteLocal = len(stream.Data)
	} // Unclear why we change datalength
	copy(em.errorStatusBits[:], data[:countWriteLocal])
	*countWritten = uint16(countWriteLocal)
	return nil
}

func (emergency *EMCY) Handle(frame Frame) {
	// Ignore sync messages and only accept 8 bytes size
	if emergency == nil || emergency.rxCallback == nil ||
		frame.ID == 0x80 ||
		len(frame.Data) != 8 {
		return
	}
	errorCode := binary.LittleEndian.Uint16(frame.Data[0:2])
	infoCode := binary.LittleEndian.Uint32(frame.Data[4:8])
	emergency.rxCallback(
		uint16(frame.ID),
		errorCode,
		frame.Data[2],
		frame.Data[3],
		infoCode)

}

func (emergency *EMCY) process(nmtIsPreOrOperational bool, timeDifferenceUs uint32, timerNextUs *uint32) {
	// Check errors from driver
	canErrStatus := emergency.busManager.canError
	if canErrStatus != emergency.canErrorOld {
		canErrStatusChanged := canErrStatus ^ emergency.canErrorOld
		emergency.canErrorOld = canErrStatus
		if (canErrStatusChanged & (canErrorTxWarning | canErrorRxWarning)) != 0 {
			emergency.error(
				(canErrStatus&(canErrorTxWarning|canErrorRxWarning)) != 0,
				emCanBusWarning,
				emNoError,
				0,
			)
		}
		if (canErrStatusChanged & canErrorTxPassive) != 0 {
			emergency.error(
				(canErrStatus&canErrorTxPassive) != 0,
				emCanTXBusPassive,
				emErrCanPassive,
				0,
			)
		}

		if (canErrStatusChanged & canErrorTxBusOff) != 0 {
			emergency.error(
				(canErrStatus&canErrorTxBusOff) != 0,
				emCanTXBusOff,
				emErrBusOffRecovered,
				0)
		}

		if (canErrStatusChanged & canErrorTxOverflow) != 0 {
			emergency.error(
				(canErrStatus&canErrorTxOverflow) != 0,
				emCanTXOverflow,
				emErrCanOverrun,
				0)
		}

		if (canErrStatusChanged & canErrorPdoLate) != 0 {
			emergency.error(
				(canErrStatus&canErrorPdoLate) != 0,
				emTPDOOutsideWindow,
				emErrCommunication,
				0)
		}

		if (canErrStatusChanged & canErrorRxPassive) != 0 {
			emergency.error(
				(canErrStatus&canErrorRxPassive) != 0,
				emCanRXBusPassive,
				emErrCanPassive,
				0)
		}

		if (canErrStatusChanged & canErrorRxOverflow) != 0 {
			emergency.error(
				(canErrStatus&canErrorRxOverflow) != 0,
				emCanRXBOverflow,
				emErrCanOverrun,
				0)
		}
	}
	errorRegister := emErrRegGeneric |
		emErrRegCurrent |
		emErrRegVoltage |
		emErrRegTemperature |
		emErrRegCommunication |
		emErrRegDevProfile |
		emErrRegManufacturer

	if !nmtIsPreOrOperational {
		return
	}
	if len(emergency.fifo) >= 2 {
		fifoPpPtr := emergency.fifoPpPtr
		if emergency.inhibitTimer < emergency.inhibitTimeUs {
			emergency.inhibitTimer += timeDifferenceUs
		}
		if fifoPpPtr != emergency.fifoWrPtr &&
			emergency.inhibitTimer >= emergency.inhibitTimeUs {
			emergency.inhibitTimer = 0

			emergency.fifo[fifoPpPtr].msg |= uint32(errorRegister) << 16
			binary.LittleEndian.PutUint32(emergency.txBuffer.Data[:4], emergency.fifo[fifoPpPtr].msg)
			emergency.Send(emergency.txBuffer)
			// Also report own emergency message
			if emergency.rxCallback != nil {
				errMsg := uint32(emergency.fifo[fifoPpPtr].msg)
				emergency.rxCallback(
					0,
					uint16(errMsg),
					byte(errorRegister),
					byte(errMsg>>24),
					emergency.fifo[fifoPpPtr].info,
				)
			}
			fifoPpPtr += 1
			if int(fifoPpPtr) < len(emergency.fifo) {
				emergency.fifoPpPtr = fifoPpPtr
			} else {
				emergency.fifoPpPtr = 0
			}
			if emergency.fifoOverflow == 1 {
				emergency.fifoOverflow = 2
				emergency.ErrorReport(emEmergencyBufferFull, emErrGeneric, 0)
			} else if emergency.fifoOverflow == 2 && fifoPpPtr == emergency.fifoWrPtr {
				emergency.fifoOverflow = 0
				emergency.ErrorReset(emEmergencyBufferFull, 0)
			}
		} else if timerNextUs != nil && emergency.inhibitTimeUs < emergency.inhibitTimer {
			diff := emergency.inhibitTimeUs - emergency.inhibitTimer
			if *timerNextUs > diff {
				*timerNextUs = diff
			}
		}

	}
}

// Set or reset an error condition
// Function adds a new error to the history & error will be processed by Process function
func (emergency *EMCY) error(setError bool, errorBit byte, errorCode uint16, infoCode uint32) error {
	if emergency == nil {
		return nil
	}
	index := errorBit >> 3
	bitMask := 1 << (errorBit & 0x7)

	// Unsupported errorBit
	if index >= CO_CONFIG_EM_ERR_STATUS_BITS_COUNT/8 {
		index = emWrongErrorReport >> 3
		bitMask = 1 << (emWrongErrorReport & 0x7)
		errorCode = emErrSoftwareInternal
		infoCode = uint32(errorBit)
	}
	errorStatusBits := &emergency.errorStatusBits[index]
	errorStatusBitMasked := *errorStatusBits & byte(bitMask)

	// If error is already set or not don't do anything
	if setError {
		if errorStatusBitMasked != 0 {
			return nil
		}
	} else {
		if errorStatusBitMasked == 0 {
			return nil
		}
		errorCode = emErrNoError
	}
	errMsg := (uint32(errorBit) << 24) | uint32(errorCode)
	if len(emergency.fifo) >= 2 {
		fifoWrPtr := emergency.fifoWrPtr
		fifoWrPtrNext := fifoWrPtr + 1
		if int(fifoWrPtrNext) >= len(emergency.fifo) {
			fifoWrPtrNext = 0
		}
		if fifoWrPtrNext == emergency.fifoPpPtr {
			emergency.fifoOverflow = 1
		} else {
			emergency.fifo[fifoWrPtr].msg = errMsg
			emergency.fifo[fifoWrPtr].info = infoCode
			emergency.fifoWrPtr = fifoWrPtrNext
			if int(emergency.fifoCount) < len(emergency.fifo)-1 {
				emergency.fifoCount++
			}
		}
	}
	return nil
}

func (emergency *EMCY) ErrorReport(errorBit byte, errorCode uint16, infoCode uint32) error {
	log.Warnf("[EMERGENCY][TX][ERROR] %v (x%x) | %v (x%x) | infoCode %v",
		getErrorCodeDescription(int(errorCode)),
		errorCode,
		getErrorStatusDescription(errorBit),
		errorBit,
		infoCode,
	)
	return emergency.error(true, errorBit, errorCode, infoCode)
}

func (emergency *EMCY) ErrorReset(errorBit byte, infoCode uint32) error {
	log.Infof("[EMERGENCY][TX][RESET] reset emergency %v (x%x) | infoCode %v",
		getErrorStatusDescription(errorBit),
		errorBit,
		infoCode,
	)
	return emergency.error(false, errorBit, emErrNoError, infoCode)
}

func (emergency *EMCY) IsError(errorBit byte) bool {
	if emergency == nil {
		return true
	}
	byteIndex := errorBit >> 3
	bitMask := uint8(1) << (errorBit & 0x7)
	if byteIndex >= (CO_CONFIG_EM_ERR_STATUS_BITS_COUNT / 8) {
		return true
	}
	return (emergency.errorStatusBits[byteIndex] & bitMask) != 0
}

func (emergency *EMCY) GetErrorRegister() byte {
	if emergency == nil || emergency.errorRegister == nil {
		return 0
	}
	return *emergency.errorRegister
}

func (emergency *EMCY) ProducerEnabled() bool {
	return emergency.producerEnabled
}

func (emergency *EMCY) SetCallback(callback EMCYRxCallback) {
	emergency.rxCallback = callback
}

func NewEM(
	bm *busManager,
	nodeId uint8,
	entry1001 *Entry,
	entry1014 *Entry,
	entry1015 *Entry,
	entry1003 *Entry,
	entryStatusBits *Entry,
) (*EMCY, error) {
	if entry1001 == nil || entry1014 == nil || bm == nil ||
		nodeId < 1 || nodeId > 127 ||
		entry1003 == nil {
		return nil, ErrIllegalArgument

	}
	emergency := &EMCY{busManager: bm}
	// TODO handle error register ptr
	//emergency.errorRegister
	fifoSize := entry1003.SubCount()
	emergency.fifo = make([]emfifo, fifoSize)

	// Get cob id initial & verify
	cobIdEmergency, ret := entry1014.Uint32(0)
	if ret != nil || (cobIdEmergency&0x7FFFF800) != 0 {
		// Don't break if only value is wrong
		if ret != nil {
			return nil, ErrOdParameters
		}
	}
	producerCanId := cobIdEmergency & 0x7FF
	emergency.producerEnabled = (cobIdEmergency&0x80000000) == 0 && producerCanId != 0
	entry1014.AddExtension(emergency, readEntry1014, writeEntry1014)
	emergency.producerIdent = uint16(producerCanId)
	if producerCanId == uint32(EMERGENCY_SERVICE_ID) {
		producerCanId += uint32(nodeId)
	}
	emergency.nodeId = nodeId
	emergency.txBuffer = NewFrame(producerCanId, 0, 8)
	emergency.inhibitTimeUs = 0
	emergency.inhibitTimer = 0
	inhibitTime100us, ret := entry1015.Uint16(0)
	if ret == nil {
		emergency.inhibitTimeUs = uint32(inhibitTime100us) * 100
		entry1015.AddExtension(emergency, ReadEntryDefault, writeEntry1015)
	}
	entry1003.AddExtension(emergency, readEntry1003, writeEntry1003)
	if entryStatusBits != nil {
		entryStatusBits.AddExtension(emergency, readEntryStatusBits, writeEntryStatusBits)
	}

	err := emergency.Subscribe(uint32(EMERGENCY_SERVICE_ID), 0x780, false, emergency)
	if err != nil {
		return nil, err
	}
	return emergency, nil
}
