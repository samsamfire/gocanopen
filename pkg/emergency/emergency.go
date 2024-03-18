package emergency

import (
	"encoding/binary"

	canopen "github.com/samsamfire/gocanopen"
	can "github.com/samsamfire/gocanopen/pkg/can"
	"github.com/samsamfire/gocanopen/pkg/od"
	log "github.com/sirupsen/logrus"
)

const CO_CONFIG_EM_ERR_STATUS_BITS_COUNT = 80
const SERVICE_ID = 0x80

// Error register values
const (
	ErrRegGeneric       = 0x01 // bit 0 - generic error
	ErrRegCurrent       = 0x02 // bit 1 - current
	ErrRegVoltage       = 0x04 // bit 2 - voltage
	ErrRegTemperature   = 0x08 // bit 3 - temperature
	ErrRegCommunication = 0x10 // bit 4 - communication error
	ErrRegDevProfile    = 0x20 // bit 5 - device profile specific
	ErrRegReserved      = 0x40 // bit 6 - reserved (always 0)
	ErrRegManufacturer  = 0x80 // bit 7 - manufacturer specific
)

// Error codes
const (
	ErrNoError          = 0x0000
	ErrGeneric          = 0x1000
	ErrCurrent          = 0x2000
	ErrCurrentInput     = 0x2100
	ErrCurrentInside    = 0x2200
	ErrCurrentOutput    = 0x2300
	ErrVoltage          = 0x3000
	ErrVoltageMains     = 0x3100
	ErrVoltageInside    = 0x3200
	ErrVoltageOutput    = 0x3300
	ErrTemperature      = 0x4000
	ErrTempAmbient      = 0x4100
	ErrTempDevice       = 0x4200
	ErrHardware         = 0x5000
	ErrSoftwareDevice   = 0x6000
	ErrSoftwareInternal = 0x6100
	ErrSoftwareUser     = 0x6200
	ErrDataSet          = 0x6300
	ErrAdditionalModul  = 0x7000
	ErrMonitoring       = 0x8000
	ErrCommunication    = 0x8100
	ErrCanOverrun       = 0x8110
	ErrCanPassive       = 0x8120
	ErrHeartbeat        = 0x8130
	ErrBusOffRecovered  = 0x8140
	ErrCanIdCollision   = 0x8150
	ErrProtocolError    = 0x8200
	ErrPdoLength        = 0x8210
	ErrPdoLengthExc     = 0x8220
	ErrDamMpdo          = 0x8230
	ErrSyncDataLength   = 0x8240
	ErrRpdoTimeout      = 0x8250
	ErrExternalError    = 0x9000
	ErrAdditionalFunc   = 0xF000
	ErrDeviceSpecific   = 0xFF00
	Err401OutCurHi      = 0x2310
	Err401OutShorted    = 0x2320
	Err401OutLoadDump   = 0x2330
	Err401InVoltHi      = 0x3110
	Err401InVoltLow     = 0x3120
	Err401InternVoltHi  = 0x3210
	Err401InternVoltLow = 0x3220
	Err401OutVoltHigh   = 0x3310
	Err401OutVoltLow    = 0x3320
)

var errorCodeDescriptionMap = map[int]string{
	ErrNoError:          "Reset or No Error",
	ErrGeneric:          "Generic Error",
	ErrCurrent:          "Current",
	ErrCurrentInput:     "Current, device input side",
	ErrCurrentInside:    "Current inside the device",
	ErrCurrentOutput:    "Current, device output side",
	ErrVoltage:          "Voltage",
	ErrVoltageMains:     "Mains Voltage",
	ErrVoltageInside:    "Voltage inside the device",
	ErrVoltageOutput:    "Output Voltage",
	ErrTemperature:      "Temperature",
	ErrTempAmbient:      "Ambient Temperature",
	ErrTempDevice:       "Device Temperature",
	ErrHardware:         "Device Hardware",
	ErrSoftwareDevice:   "Device Software",
	ErrSoftwareInternal: "Internal Software",
	ErrSoftwareUser:     "User Software",
	ErrDataSet:          "Data Set",
	ErrAdditionalModul:  "Additional Modules",
	ErrMonitoring:       "Monitoring",
	ErrCommunication:    "Communication",
	ErrCanOverrun:       "CAN Overrun (Objects lost)",
	ErrCanPassive:       "CAN in Error Passive Mode",
	ErrHeartbeat:        "Life Guard Error or Heartbeat Error",
	ErrBusOffRecovered:  "Recovered from bus off",
	ErrCanIdCollision:   "CAN-ID collision",
	ErrProtocolError:    "Protocol Error",
	ErrPdoLength:        "PDO not processed due to length error",
	ErrPdoLengthExc:     "PDO length exceeded",
	ErrDamMpdo:          "DAM MPDO not processed, destination object not available",
	ErrSyncDataLength:   "Unexpected SYNC data length",
	ErrRpdoTimeout:      "RPDO timeout",
	ErrExternalError:    "External Error",
	ErrAdditionalFunc:   "Additional Functions",
	ErrDeviceSpecific:   "Device specific",
	Err401OutCurHi:      "DS401, Current at outputs too high (overload)",
	Err401OutShorted:    "DS401, Short circuit at outputs",
	Err401OutLoadDump:   "DS401, Load dump at outputs",
	Err401InVoltHi:      "DS401, Input voltage too high",
	Err401InVoltLow:     "DS401, Input voltage too low",
	Err401InternVoltHi:  "DS401, Internal voltage too high",
	Err401InternVoltLow: "DS401, Internal voltage too low",
	Err401OutVoltHigh:   "DS401, Output voltage too high",
	Err401OutVoltLow:    "DS401, Output voltage too low",
}

// Error status bits
const (
	EmNoError                 = 0x00
	EmCanBusWarning           = 0x01
	EmRxMsgWrongLength        = 0x02
	EmRxMsgOverflow           = 0x03
	EmRPDOWrongLength         = 0x04
	EmRPDOOverflow            = 0x05
	EmCanRXBusPassive         = 0x06
	EmCanTXBusPassive         = 0x07
	EmNMTWrongCommand         = 0x08
	EmTimeTimeout             = 0x09
	Em0AUnused                = 0x0A
	Em0BUnused                = 0x0B
	Em0CUnused                = 0x0C
	Em0DUnused                = 0x0D
	Em0EUnused                = 0x0E
	Em0FUnused                = 0x0F
	Em10Unused                = 0x10
	Em11Unused                = 0x11
	EmCanTXBusOff             = 0x12
	EmCanRXBOverflow          = 0x13
	EmCanTXOverflow           = 0x14
	EmTPDOOutsideWindow       = 0x15
	Em16Unused                = 0x16
	EmRPDOTimeOut             = 0x17
	EmSyncTimeOut             = 0x18
	EmSyncLength              = 0x19
	EmPDOWrongMapping         = 0x1A
	EmHeartbeatConsumer       = 0x1B
	EmHBConsumerRemoteReset   = 0x1C
	Em1DUnused                = 0x1D
	Em1EUnused                = 0x1E
	Em1FUnused                = 0x1F
	EmEmergencyBufferFull     = 0x20
	Em21Unused                = 0x21
	EmMicrocontrollerReset    = 0x22
	Em23Unused                = 0x23
	Em24Unused                = 0x24
	Em25Unused                = 0x25
	Em26Unused                = 0x26
	EmNonVolatileAutoSave     = 0x27
	EmWrongErrorReport        = 0x28
	EmISRTimerOverflow        = 0x29
	EmMemoryAllocationError   = 0x2A
	EmGenericError            = 0x2B
	EmGenericSoftwareError    = 0x2C
	EmInconsistentObjectDict  = 0x2D
	EmCalculationOfParameters = 0x2E
	EmNonVolatileMemory       = 0x2F
	EmManufacturerStart       = 0x30
	EmManufacturerEnd         = CO_CONFIG_EM_ERR_STATUS_BITS_COUNT - 1
)

var errorStatusMap = map[uint8]string{
	EmNoError:                 "Error Reset or No Error",
	EmCanBusWarning:           "CAN bus warning limit reached",
	EmRxMsgWrongLength:        "Wrong data length of the received CAN message",
	EmRxMsgOverflow:           "Previous received CAN message wasn't processed yet",
	EmRPDOWrongLength:         "Wrong data length of received PDO",
	EmRPDOOverflow:            "Previous received PDO wasn't processed yet",
	EmCanRXBusPassive:         "CAN receive bus is passive",
	EmCanTXBusPassive:         "CAN transmit bus is passive",
	EmNMTWrongCommand:         "Wrong NMT command received",
	EmTimeTimeout:             "TIME message timeout",
	Em0AUnused:                "(unused)",
	Em0BUnused:                "(unused)",
	Em0CUnused:                "(unused)",
	Em0DUnused:                "(unused)",
	Em0EUnused:                "(unused)",
	Em0FUnused:                "(unused)",
	Em10Unused:                "(unused)",
	Em11Unused:                "(unused)",
	EmCanTXBusOff:             "CAN transmit bus is off",
	EmCanRXBOverflow:          "CAN module receive buffer has overflowed",
	EmCanTXOverflow:           "CAN transmit buffer has overflowed",
	EmTPDOOutsideWindow:       "TPDO is outside SYNC window",
	Em16Unused:                "(unused)",
	EmRPDOTimeOut:             "RPDO message timeout",
	EmSyncTimeOut:             "SYNC message timeout",
	EmSyncLength:              "Unexpected SYNC data length",
	EmPDOWrongMapping:         "Error with PDO mapping",
	EmHeartbeatConsumer:       "Heartbeat consumer timeout",
	EmHBConsumerRemoteReset:   "Heartbeat consumer detected remote node reset",
	Em1DUnused:                "(unused)",
	Em1EUnused:                "(unused)",
	Em1FUnused:                "(unused)",
	EmEmergencyBufferFull:     "Emergency buffer is full, Emergency message wasn't sent",
	Em21Unused:                "(unused)",
	EmMicrocontrollerReset:    "Microcontroller has just started",
	Em23Unused:                "(unused)",
	Em24Unused:                "(unused)",
	Em25Unused:                "(unused)",
	Em26Unused:                "(unused)",
	EmNonVolatileAutoSave:     "Automatic store to non-volatile memory failed",
	EmWrongErrorReport:        "Wrong parameters to ErrorReport function",
	EmISRTimerOverflow:        "Timer task has overflowed",
	EmMemoryAllocationError:   "Unable to allocate memory for objects",
	EmGenericError:            "Generic error, test usage",
	EmGenericSoftwareError:    "Software error",
	EmInconsistentObjectDict:  "Object dictionary does not match the software",
	EmCalculationOfParameters: "Error in calculation of device parameters",
	EmNonVolatileMemory:       "Error with access to non-volatile device memory",
}

func getErrorStatusDescription(errorStatus uint8) string {
	description, ok := errorStatusMap[errorStatus]
	if ok {
		return description
	} else if errorStatus >= EmManufacturerStart && errorStatus <= EmManufacturerEnd {
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
	*canopen.BusManager
	nodeId          byte
	errorStatusBits [CO_CONFIG_EM_ERR_STATUS_BITS_COUNT / 8]byte
	errorRegister   *byte
	canErrorOld     uint16
	txBuffer        can.Frame
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

func (emergency *EMCY) Handle(frame can.Frame) {
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

func (emergency *EMCY) Process(nmtIsPreOrOperational bool, timeDifferenceUs uint32, timerNextUs *uint32) {
	// Check errors from driver
	canErrStatus := emergency.BusManager.Error()
	if canErrStatus != emergency.canErrorOld {
		canErrStatusChanged := canErrStatus ^ emergency.canErrorOld
		emergency.canErrorOld = canErrStatus
		if (canErrStatusChanged & (can.CanErrorTxWarning | can.CanErrorRxWarning)) != 0 {
			emergency.Error(
				(canErrStatus&(can.CanErrorTxWarning|can.CanErrorRxWarning)) != 0,
				EmCanBusWarning,
				EmNoError,
				0,
			)
		}
		if (canErrStatusChanged & can.CanErrorTxPassive) != 0 {
			emergency.Error(
				(canErrStatus&can.CanErrorTxPassive) != 0,
				EmCanTXBusPassive,
				ErrCanPassive,
				0,
			)
		}

		if (canErrStatusChanged & can.CanErrorTxBusOff) != 0 {
			emergency.Error(
				(canErrStatus&can.CanErrorTxBusOff) != 0,
				EmCanTXBusOff,
				ErrBusOffRecovered,
				0)
		}

		if (canErrStatusChanged & can.CanErrorTxOverflow) != 0 {
			emergency.Error(
				(canErrStatus&can.CanErrorTxOverflow) != 0,
				EmCanTXOverflow,
				ErrCanOverrun,
				0)
		}

		if (canErrStatusChanged & can.CanErrorPdoLate) != 0 {
			emergency.Error(
				(canErrStatus&can.CanErrorPdoLate) != 0,
				EmTPDOOutsideWindow,
				ErrCommunication,
				0)
		}

		if (canErrStatusChanged & can.CanErrorRxPassive) != 0 {
			emergency.Error(
				(canErrStatus&can.CanErrorRxPassive) != 0,
				EmCanRXBusPassive,
				ErrCanPassive,
				0)
		}

		if (canErrStatusChanged & can.CanErrorRxOverflow) != 0 {
			emergency.Error(
				(canErrStatus&can.CanErrorRxOverflow) != 0,
				EmCanRXBOverflow,
				ErrCanOverrun,
				0)
		}
	}
	errorRegister := ErrRegGeneric |
		ErrRegCurrent |
		ErrRegVoltage |
		ErrRegTemperature |
		ErrRegCommunication |
		ErrRegDevProfile |
		ErrRegManufacturer

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
				emergency.ErrorReport(EmEmergencyBufferFull, ErrGeneric, 0)
			} else if emergency.fifoOverflow == 2 && fifoPpPtr == emergency.fifoWrPtr {
				emergency.fifoOverflow = 0
				emergency.ErrorReset(EmEmergencyBufferFull, 0)
			}
		} else if timerNextUs != nil && emergency.inhibitTimeUs < emergency.inhibitTimer {
			diff := emergency.inhibitTimeUs - emergency.inhibitTimer
			if *timerNextUs > diff {
				*timerNextUs = diff
			}
		}

	}
}

// Set or reset an Error condition
// Function adds a new Error to the history & Error will be processed by Process function
func (emergency *EMCY) Error(setError bool, errorBit byte, errorCode uint16, infoCode uint32) {

	index := errorBit >> 3
	bitMask := 1 << (errorBit & 0x7)

	// Unsupported errorBit
	if index >= CO_CONFIG_EM_ERR_STATUS_BITS_COUNT/8 {
		index = EmWrongErrorReport >> 3
		bitMask = 1 << (EmWrongErrorReport & 0x7)
		errorCode = ErrSoftwareInternal
		infoCode = uint32(errorBit)
	}
	errorStatusBits := &emergency.errorStatusBits[index]
	errorStatusBitMasked := *errorStatusBits & byte(bitMask)

	// If error is already set or not don't do anything
	if setError {
		if errorStatusBitMasked != 0 {
			return
		}
	} else {
		if errorStatusBitMasked == 0 {
			return
		}
		errorCode = ErrNoError
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
}

func (emergency *EMCY) ErrorReport(errorBit byte, errorCode uint16, infoCode uint32) {
	log.Warnf("[EMERGENCY][TX][ERROR] %v (x%x) | %v (x%x) | infoCode %v",
		getErrorCodeDescription(int(errorCode)),
		errorCode,
		getErrorStatusDescription(errorBit),
		errorBit,
		infoCode,
	)
	emergency.Error(true, errorBit, errorCode, infoCode)
}

func (emergency *EMCY) ErrorReset(errorBit byte, infoCode uint32) {
	log.Infof("[EMERGENCY][TX][RESET] reset emergency %v (x%x) | infoCode %v",
		getErrorStatusDescription(errorBit),
		errorBit,
		infoCode,
	)
	emergency.Error(false, errorBit, ErrNoError, infoCode)
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
	bm *canopen.BusManager,
	nodeId uint8,
	entry1001 *od.Entry,
	entry1014 *od.Entry,
	entry1015 *od.Entry,
	entry1003 *od.Entry,
	entryStatusBits *od.Entry,
) (*EMCY, error) {
	if entry1001 == nil || entry1014 == nil || bm == nil ||
		nodeId < 1 || nodeId > 127 ||
		entry1003 == nil {
		return nil, canopen.ErrIllegalArgument

	}
	emergency := &EMCY{BusManager: bm}
	// TODO handle error register ptr
	// emergency.errorRegister
	fifoSize := entry1003.SubCount()
	emergency.fifo = make([]emfifo, fifoSize)

	// Get cob id initial & verify
	cobIdEmergency, ret := entry1014.Uint32(0)
	if ret != nil || (cobIdEmergency&0x7FFFF800) != 0 {
		// Don't break if only value is wrong
		if ret != nil {
			return nil, canopen.ErrOdParameters
		}
	}
	producerCanId := cobIdEmergency & 0x7FF
	emergency.producerEnabled = (cobIdEmergency&0x80000000) == 0 && producerCanId != 0
	entry1014.AddExtension(emergency, readEntry1014, writeEntry1014)
	emergency.producerIdent = uint16(producerCanId)
	if producerCanId == uint32(SERVICE_ID) {
		producerCanId += uint32(nodeId)
	}
	emergency.nodeId = nodeId
	emergency.txBuffer = can.NewFrame(producerCanId, 0, 8)
	emergency.inhibitTimeUs = 0
	emergency.inhibitTimer = 0
	inhibitTime100us, ret := entry1015.Uint16(0)
	if ret == nil {
		emergency.inhibitTimeUs = uint32(inhibitTime100us) * 100
		entry1015.AddExtension(emergency, od.ReadEntryDefault, writeEntry1015)
	}
	entry1003.AddExtension(emergency, readEntry1003, writeEntry1003)
	if entryStatusBits != nil {
		entryStatusBits.AddExtension(emergency, readEntryStatusBits, writeEntryStatusBits)
	}

	err := emergency.Subscribe(uint32(SERVICE_ID), 0x780, false, emergency)
	if err != nil {
		return nil, err
	}
	return emergency, nil
}
