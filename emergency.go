package canopen

import (
	"encoding/binary"

	log "github.com/sirupsen/logrus"
)

/*
	TODOs:

- error register
*/
const CO_CONFIG_EM_ERR_STATUS_BITS_COUNT = 80

// Error register values
const (
	CO_ERR_REG_GENERIC_ERR   = 0x01 /**< bit 0, generic error */
	CO_ERR_REG_CURRENT       = 0x02 /**< bit 1, current */
	CO_ERR_REG_VOLTAGE       = 0x04 /**< bit 2, voltage */
	CO_ERR_REG_TEMPERATURE   = 0x08 /**< bit 3, temperature */
	CO_ERR_REG_COMMUNICATION = 0x10 /**< bit 4, communication error */
	CO_ERR_REG_DEV_PROFILE   = 0x20 /**< bit 5, device profile specific */
	CO_ERR_REG_RESERVED      = 0x40 /**< bit 6, reserved (always 0) */
	CO_ERR_REG_MANUFACTURER  = 0x80 /**< bit 7, manufacturer specific */
)

// Error codes
const (
	CO_EMC_NO_ERROR          = 0x0000
	CO_EMC_GENERIC           = 0x1000
	CO_EMC_CURRENT           = 0x2000
	CO_EMC_CURRENT_INPUT     = 0x2100
	CO_EMC_CURRENT_INSIDE    = 0x2200
	CO_EMC_CURRENT_OUTPUT    = 0x2300
	CO_EMC_VOLTAGE           = 0x3000
	CO_EMC_VOLTAGE_MAINS     = 0x3100
	CO_EMC_VOLTAGE_INSIDE    = 0x3200
	CO_EMC_VOLTAGE_OUTPUT    = 0x3300
	CO_EMC_TEMPERATURE       = 0x4000
	CO_EMC_TEMP_AMBIENT      = 0x4100
	CO_EMC_TEMP_DEVICE       = 0x4200
	CO_EMC_HARDWARE          = 0x5000
	CO_EMC_SOFTWARE_DEVICE   = 0x6000
	CO_EMC_SOFTWARE_INTERNAL = 0x6100
	CO_EMC_SOFTWARE_USER     = 0x6200
	CO_EMC_DATA_SET          = 0x6300
	CO_EMC_ADDITIONAL_MODUL  = 0x7000
	CO_EMC_MONITORING        = 0x8000
	CO_EMC_COMMUNICATION     = 0x8100
	CO_EMC_CAN_OVERRUN       = 0x8110
	CO_EMC_CAN_PASSIVE       = 0x8120
	CO_EMC_HEARTBEAT         = 0x8130
	CO_EMC_BUS_OFF_RECOVERED = 0x8140
	CO_EMC_CAN_ID_COLLISION  = 0x8150
	CO_EMC_PROTOCOL_ERROR    = 0x8200
	CO_EMC_PDO_LENGTH        = 0x8210
	CO_EMC_PDO_LENGTH_EXC    = 0x8220
	CO_EMC_DAM_MPDO          = 0x8230
	CO_EMC_SYNC_DATA_LENGTH  = 0x8240
	CO_EMC_RPDO_TIMEOUT      = 0x8250
	CO_EMC_EXTERNAL_ERROR    = 0x9000
	CO_EMC_ADDITIONAL_FUNC   = 0xF000
	CO_EMC_DEVICE_SPECIFIC   = 0xFF00
	CO_EMC401_OUT_CUR_HI     = 0x2310
	CO_EMC401_OUT_SHORTED    = 0x2320
	CO_EMC401_OUT_LOAD_DUMP  = 0x2330
	CO_EMC401_IN_VOLT_HI     = 0x3110
	CO_EMC401_IN_VOLT_LOW    = 0x3120
	CO_EMC401_INTERN_VOLT_HI = 0x3210
	CO_EMC401_INTERN_VOLT_LO = 0x3220
	CO_EMC401_OUT_VOLT_HIGH  = 0x3310
	CO_EMC401_OUT_VOLT_LOW   = 0x3320
)

var ERROR_CODE_MAP = map[int]string{
	CO_EMC_NO_ERROR:          "Reset or No Error",
	CO_EMC_GENERIC:           "Generic Error",
	CO_EMC_CURRENT:           "Current",
	CO_EMC_CURRENT_INPUT:     "Current, device input side",
	CO_EMC_CURRENT_INSIDE:    "Current inside the device",
	CO_EMC_CURRENT_OUTPUT:    "Current, device output side",
	CO_EMC_VOLTAGE:           "Voltage",
	CO_EMC_VOLTAGE_MAINS:     "Mains Voltage",
	CO_EMC_VOLTAGE_INSIDE:    "Voltage inside the device",
	CO_EMC_VOLTAGE_OUTPUT:    "Output Voltage",
	CO_EMC_TEMPERATURE:       "Temperature",
	CO_EMC_TEMP_AMBIENT:      "Ambient Temperature",
	CO_EMC_TEMP_DEVICE:       "Device Temperature",
	CO_EMC_HARDWARE:          "Device Hardware",
	CO_EMC_SOFTWARE_DEVICE:   "Device Software",
	CO_EMC_SOFTWARE_INTERNAL: "Internal Software",
	CO_EMC_SOFTWARE_USER:     "User Software",
	CO_EMC_DATA_SET:          "Data Set",
	CO_EMC_ADDITIONAL_MODUL:  "Additional Modules",
	CO_EMC_MONITORING:        "Monitoring",
	CO_EMC_COMMUNICATION:     "Communication",
	CO_EMC_CAN_OVERRUN:       "CAN Overrun (Objects lost)",
	CO_EMC_CAN_PASSIVE:       "CAN in Error Passive Mode",
	CO_EMC_HEARTBEAT:         "Life Guard Error or Heartbeat Error",
	CO_EMC_BUS_OFF_RECOVERED: "Recovered from bus off",
	CO_EMC_CAN_ID_COLLISION:  "CAN-ID collision",
	CO_EMC_PROTOCOL_ERROR:    "Protocol Error",
	CO_EMC_PDO_LENGTH:        "PDO not processed due to length error",
	CO_EMC_PDO_LENGTH_EXC:    "PDO length exceeded",
	CO_EMC_DAM_MPDO:          "DAM MPDO not processed, destination object not available",
	CO_EMC_SYNC_DATA_LENGTH:  "Unexpected SYNC data length",
	CO_EMC_RPDO_TIMEOUT:      "RPDO timeout",
	CO_EMC_EXTERNAL_ERROR:    "External Error",
	CO_EMC_ADDITIONAL_FUNC:   "Additional Functions",
	CO_EMC_DEVICE_SPECIFIC:   "Device specific",
	CO_EMC401_OUT_CUR_HI:     "DS401, Current at outputs too high (overload)",
	CO_EMC401_OUT_SHORTED:    "DS401, Short circuit at outputs",
	CO_EMC401_OUT_LOAD_DUMP:  "DS401, Load dump at outputs",
	CO_EMC401_IN_VOLT_HI:     "DS401, Input voltage too high",
	CO_EMC401_IN_VOLT_LOW:    "DS401, Input voltage too low",
	CO_EMC401_INTERN_VOLT_HI: "DS401, Internal voltage too high",
	CO_EMC401_INTERN_VOLT_LO: "DS401, Internal voltage too low",
	CO_EMC401_OUT_VOLT_HIGH:  "DS401, Output voltage too high",
	CO_EMC401_OUT_VOLT_LOW:   "DS401, Output voltage too low",
}

// Error status bits
const (
	CO_EM_NO_ERROR                  = 0x00
	CO_EM_CAN_BUS_WARNING           = 0x01
	CO_EM_RXMSG_WRONG_LENGTH        = 0x02
	CO_EM_RXMSG_OVERFLOW            = 0x03
	CO_EM_RPDO_WRONG_LENGTH         = 0x04
	CO_EM_RPDO_OVERFLOW             = 0x05
	CO_EM_CAN_RX_BUS_PASSIVE        = 0x06
	CO_EM_CAN_TX_BUS_PASSIVE        = 0x07
	CO_EM_NMT_WRONG_COMMAND         = 0x08
	CO_EM_TIME_TIMEOUT              = 0x09
	CO_EM_0A_unused                 = 0x0A
	CO_EM_0B_unused                 = 0x0B
	CO_EM_0C_unused                 = 0x0C
	CO_EM_0D_unused                 = 0x0D
	CO_EM_0E_unused                 = 0x0E
	CO_EM_0F_unused                 = 0x0F
	CO_EM_10_unused                 = 0x10
	CO_EM_11_unused                 = 0x11
	CO_EM_CAN_TX_BUS_OFF            = 0x12
	CO_EM_CAN_RXB_OVERFLOW          = 0x13
	CO_EM_CAN_TX_OVERFLOW           = 0x14
	CO_EM_TPDO_OUTSIDE_WINDOW       = 0x15
	CO_EM_16_unused                 = 0x16
	CO_EM_RPDO_TIME_OUT             = 0x17
	CO_EM_SYNC_TIME_OUT             = 0x18
	CO_EM_SYNC_LENGTH               = 0x19
	CO_EM_PDO_WRONG_MAPPING         = 0x1A
	CO_EM_HEARTBEAT_CONSUMER        = 0x1B
	CO_EM_HB_CONSUMER_REMOTE_RESET  = 0x1C
	CO_EM_1D_unused                 = 0x1D
	CO_EM_1E_unused                 = 0x1E
	CO_EM_1F_unused                 = 0x1F
	CO_EM_EMERGENCY_BUFFER_FULL     = 0x20
	CO_EM_21_unused                 = 0x21
	CO_EM_MICROCONTROLLER_RESET     = 0x22
	CO_EM_23_unused                 = 0x23
	CO_EM_24_unused                 = 0x24
	CO_EM_25_unused                 = 0x25
	CO_EM_26_unused                 = 0x26
	CO_EM_NON_VOLATILE_AUTO_SAVE    = 0x27
	CO_EM_WRONG_ERROR_REPORT        = 0x28
	CO_EM_ISR_TIMER_OVERFLOW        = 0x29
	CO_EM_MEMORY_ALLOCATION_ERROR   = 0x2A
	CO_EM_GENERIC_ERROR             = 0x2B
	CO_EM_GENERIC_SOFTWARE_ERROR    = 0x2C
	CO_EM_INCONSISTENT_OBJECT_DICT  = 0x2D
	CO_EM_CALCULATION_OF_PARAMETERS = 0x2E
	CO_EM_NON_VOLATILE_MEMORY       = 0x2F
	CO_EM_MANUFACTURER_START        = 0x30
	CO_EM_MANUFACTURER_END          = CO_CONFIG_EM_ERR_STATUS_BITS_COUNT - 1
)

var ERROR_STATUS_MAP = map[uint8]string{
	CO_EM_NO_ERROR:                  "Error Reset or No Error",
	CO_EM_CAN_BUS_WARNING:           "CAN bus warning limit reached",
	CO_EM_RXMSG_WRONG_LENGTH:        "Wrong data length of the received CAN message",
	CO_EM_RXMSG_OVERFLOW:            "Previous received CAN message wasn't processed yet",
	CO_EM_RPDO_WRONG_LENGTH:         "Wrong data length of received PDO",
	CO_EM_RPDO_OVERFLOW:             "Previous received PDO wasn't processed yet",
	CO_EM_CAN_RX_BUS_PASSIVE:        "CAN receive bus is passive",
	CO_EM_CAN_TX_BUS_PASSIVE:        "CAN transmit bus is passive",
	CO_EM_NMT_WRONG_COMMAND:         "Wrong NMT command received",
	CO_EM_TIME_TIMEOUT:              "TIME message timeout",
	CO_EM_0A_unused:                 "(unused)",
	CO_EM_0B_unused:                 "(unused)",
	CO_EM_0C_unused:                 "(unused)",
	CO_EM_0D_unused:                 "(unused)",
	CO_EM_0E_unused:                 "(unused)",
	CO_EM_0F_unused:                 "(unused)",
	CO_EM_10_unused:                 "(unused)",
	CO_EM_11_unused:                 "(unused)",
	CO_EM_CAN_TX_BUS_OFF:            "CAN transmit bus is off",
	CO_EM_CAN_RXB_OVERFLOW:          "CAN module receive buffer has overflowed",
	CO_EM_CAN_TX_OVERFLOW:           "CAN transmit buffer has overflowed",
	CO_EM_TPDO_OUTSIDE_WINDOW:       "TPDO is outside SYNC window",
	CO_EM_16_unused:                 "(unused)",
	CO_EM_RPDO_TIME_OUT:             "RPDO message timeout",
	CO_EM_SYNC_TIME_OUT:             "SYNC message timeout",
	CO_EM_SYNC_LENGTH:               "Unexpected SYNC data length",
	CO_EM_PDO_WRONG_MAPPING:         "Error with PDO mapping",
	CO_EM_HEARTBEAT_CONSUMER:        "Heartbeat consumer timeout",
	CO_EM_HB_CONSUMER_REMOTE_RESET:  "Heartbeat consumer detected remote node reset",
	CO_EM_1D_unused:                 "(unused)",
	CO_EM_1E_unused:                 "(unused)",
	CO_EM_1F_unused:                 "(unused)",
	CO_EM_EMERGENCY_BUFFER_FULL:     "Emergency buffer is full, Emergency message wasn't sent",
	CO_EM_21_unused:                 "(unused)",
	CO_EM_MICROCONTROLLER_RESET:     "Microcontroller has just started",
	CO_EM_23_unused:                 "(unused)",
	CO_EM_24_unused:                 "(unused)",
	CO_EM_25_unused:                 "(unused)",
	CO_EM_26_unused:                 "(unused)",
	CO_EM_NON_VOLATILE_AUTO_SAVE:    "Automatic store to non-volatile memory failed",
	CO_EM_WRONG_ERROR_REPORT:        "Wrong parameters to ErrorReport function",
	CO_EM_ISR_TIMER_OVERFLOW:        "Timer task has overflowed",
	CO_EM_MEMORY_ALLOCATION_ERROR:   "Unable to allocate memory for objects",
	CO_EM_GENERIC_ERROR:             "Generic error, test usage",
	CO_EM_GENERIC_SOFTWARE_ERROR:    "Software error",
	CO_EM_INCONSISTENT_OBJECT_DICT:  "Object dictionary does not match the software",
	CO_EM_CALCULATION_OF_PARAMETERS: "Error in calculation of device parameters",
	CO_EM_NON_VOLATILE_MEMORY:       "Error with access to non-volatile device memory",
}

func getErrorStatusDescription(errorStatus uint8) string {
	description, ok := ERROR_STATUS_MAP[errorStatus]
	if ok {
		return description
	} else if errorStatus >= CO_EM_MANUFACTURER_START && errorStatus <= CO_EM_MANUFACTURER_END {
		return "Manufacturer error"
	} else {
		return "Invalid or not implemented error status"
	}
}

func getErrorCodeDescription(errorCode int) string {
	description, ok := ERROR_CODE_MAP[errorCode]
	if ok {
		return description
	} else {
		return "Invalid or not implemented error code"
	}
}

type EMFifo struct {
	msg  uint32
	info uint32
}

type EM struct {
	errorStatusBits     [CO_CONFIG_EM_ERR_STATUS_BITS_COUNT / 8]byte
	errorRegister       *byte
	CANerrorStatusOld   uint16
	busManager          *BusManager
	Fifo                []EMFifo
	FifoWrPtr           byte
	FifoPpPtr           byte
	FifoOverflow        byte
	FifoCount           byte
	ProducerEnabled     bool
	NodeId              byte
	CANTxBuff           *BufferTxFrame
	TxBufferIdx         int
	RxBufferIdx         int
	ProducerIdent       uint16
	InhibitEmTimeUs     uint32
	InhibitEmTimer      uint32
	ExtensionEntry1014  Extension
	ExtensionEntry1015  Extension
	ExtensionEntry1003  Extension
	ExtensionStatusBits Extension
	EmergencyRxCallback func(ident uint16, errorCode uint16, errorRegister byte, errorBit byte, infoCode uint32)
}

func ReadEntryStatusBits(stream *Stream, data []byte, countRead *uint16) error {
	if stream == nil || stream.Subindex != 0 || data == nil || countRead == nil {
		return ODR_DEV_INCOMPAT
	}
	em, ok := stream.Object.(*EM)
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

func WriteEntryStatusBits(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || stream.Subindex != 0 || countWritten == nil || data == nil {
		return ODR_DEV_INCOMPAT
	}
	em, ok := stream.Object.(*EM)
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

func (emergency *EM) Handle(frame Frame) {
	// Ignore sync messages and only accept 8 bytes size
	if emergency == nil || emergency.EmergencyRxCallback == nil ||
		frame.ID == 0x80 ||
		len(frame.Data) != 8 {
		return
	}
	errorCode := binary.LittleEndian.Uint16(frame.Data[0:2])
	infoCode := binary.LittleEndian.Uint32(frame.Data[4:8])
	emergency.EmergencyRxCallback(
		uint16(frame.ID),
		errorCode,
		frame.Data[2],
		frame.Data[3],
		infoCode)

}
func (emergency *EM) Init(
	busManager *BusManager,
	entry1001 *Entry,
	entry1014 *Entry,
	entry1015 *Entry,
	entry1003 *Entry,
	entryStatusBits *Entry,
	nodeId uint8,
) error {
	if emergency == nil || entry1001 == nil ||
		entry1014 == nil || busManager == nil ||
		nodeId < 1 || nodeId > 127 ||
		entry1003 == nil {
		log.Debugf("%v", emergency)
		return CO_ERROR_ILLEGAL_ARGUMENT

	}
	var err error
	emergency.busManager = busManager
	// TODO handle error register ptr
	//emergency.errorRegister
	fifoSize := entry1003.SubEntriesCount()
	emergency.Fifo = make([]EMFifo, fifoSize)

	// Get cob id initial & verify
	cobIdEmergency := uint32(0)
	ret := entry1014.GetUint32(0, &cobIdEmergency)
	if ret != nil || (cobIdEmergency&0x7FFFF800) != 0 {
		// Don't break if only value is wrong
		if ret != nil {
			return CO_ERROR_OD_PARAMETERS
		}
	}
	producerCanId := cobIdEmergency & 0x7FF
	emergency.ProducerEnabled = (cobIdEmergency&0x80000000) == 0 && producerCanId != 0
	emergency.ExtensionEntry1014.Object = emergency
	emergency.ExtensionEntry1014.Read = ReadEntry1014
	emergency.ExtensionEntry1014.Write = WriteEntry1014
	err = entry1014.AddExtension(&emergency.ExtensionEntry1014)
	if err != nil {
		return CO_ERROR_OD_PARAMETERS
	}
	emergency.ProducerIdent = uint16(producerCanId)
	if producerCanId == uint32(EMERGENCY_SERVICE_ID) {
		producerCanId += uint32(nodeId)
	}
	// Configure Tx buffer
	emergency.NodeId = nodeId
	emergency.CANTxBuff, emergency.TxBufferIdx, err = emergency.busManager.InsertTxBuffer(producerCanId, false, 8, false)
	if emergency.CANTxBuff == nil || err != nil {
		return CO_ERROR_ILLEGAL_ARGUMENT
	}
	emergency.InhibitEmTimeUs = 0
	emergency.InhibitEmTimer = 0
	inhibitTime100us := uint16(0)
	ret = entry1015.GetUint16(0, &inhibitTime100us)
	if ret == nil {
		emergency.InhibitEmTimeUs = uint32(inhibitTime100us) * 100
		emergency.ExtensionEntry1015.Object = emergency
		emergency.ExtensionEntry1015.Write = WriteEntry1015
		emergency.ExtensionEntry1015.Read = ReadEntryOriginal
		entry1015.AddExtension(&emergency.ExtensionEntry1015)
	}
	emergency.ExtensionEntry1003.Object = emergency
	emergency.ExtensionEntry1003.Read = ReadEntry1003
	emergency.ExtensionEntry1003.Write = WriteEntry1003
	entry1003.AddExtension(&emergency.ExtensionEntry1003)
	if entryStatusBits != nil {
		emergency.ExtensionStatusBits.Object = emergency
		emergency.ExtensionStatusBits.Read = ReadEntryStatusBits
		emergency.ExtensionStatusBits.Write = WriteEntryStatusBits
		entryStatusBits.AddExtension(&emergency.ExtensionStatusBits)
	}

	emergency.RxBufferIdx, err = busManager.InsertRxBuffer(uint32(EMERGENCY_SERVICE_ID), 0x780, false, emergency)
	return err
}

func (emergency *EM) Process(nmtIsPreOrOperational bool, timeDifferenceUs uint32, timerNextUs *uint32) {
	// Check errors from driver
	canErrStatus := emergency.busManager.CANerrorstatus
	if canErrStatus != emergency.CANerrorStatusOld {
		canErrStatusChanged := canErrStatus ^ emergency.CANerrorStatusOld
		emergency.CANerrorStatusOld = canErrStatus
		if (canErrStatusChanged & (CAN_ERRTX_WARNING | CAN_ERRRX_WARNING)) != 0 {
			emergency.Error(
				(canErrStatus&(CAN_ERRTX_WARNING|CAN_ERRRX_WARNING)) != 0,
				CO_EM_CAN_BUS_WARNING,
				CO_EMC_NO_ERROR,
				0,
			)
		}
		if (canErrStatusChanged & CAN_ERRTX_PASSIVE) != 0 {
			emergency.Error(
				(canErrStatus&CAN_ERRTX_PASSIVE) != 0,
				CO_EM_CAN_TX_BUS_PASSIVE,
				CO_EMC_CAN_PASSIVE,
				0,
			)
		}

		if (canErrStatusChanged & CAN_ERRTX_BUS_OFF) != 0 {
			emergency.Error(
				(canErrStatus&CAN_ERRTX_BUS_OFF) != 0,
				CO_EM_CAN_TX_BUS_OFF,
				CO_EMC_BUS_OFF_RECOVERED,
				0)
		}

		if (canErrStatusChanged & CAN_ERRTX_OVERFLOW) != 0 {
			emergency.Error(
				(canErrStatus&CAN_ERRTX_OVERFLOW) != 0,
				CO_EM_CAN_TX_OVERFLOW,
				CO_EMC_CAN_OVERRUN,
				0)
		}

		if (canErrStatusChanged & CAN_ERRTX_PDO_LATE) != 0 {
			emergency.Error(
				(canErrStatus&CAN_ERRTX_PDO_LATE) != 0,
				CO_EM_TPDO_OUTSIDE_WINDOW,
				CO_EMC_COMMUNICATION,
				0)
		}

		if (canErrStatusChanged & CAN_ERRRX_PASSIVE) != 0 {
			emergency.Error(
				(canErrStatus&CAN_ERRRX_PASSIVE) != 0,
				CO_EM_CAN_RX_BUS_PASSIVE,
				CO_EMC_CAN_PASSIVE,
				0)
		}

		if (canErrStatusChanged & CAN_ERRRX_OVERFLOW) != 0 {
			emergency.Error(
				(canErrStatus&CAN_ERRRX_OVERFLOW) != 0,
				CO_EM_CAN_RXB_OVERFLOW,
				CO_EM_CAN_RXB_OVERFLOW,
				0)
		}
	}
	errorRegister := CO_ERR_REG_GENERIC_ERR |
		CO_ERR_REG_CURRENT |
		CO_ERR_REG_VOLTAGE |
		CO_ERR_REG_TEMPERATURE |
		CO_ERR_REG_COMMUNICATION |
		CO_ERR_REG_DEV_PROFILE |
		CO_ERR_REG_MANUFACTURER

	if !nmtIsPreOrOperational {
		return
	}
	if len(emergency.Fifo) >= 2 {
		fifoPpPtr := emergency.FifoPpPtr
		if emergency.InhibitEmTimer < emergency.InhibitEmTimeUs {
			emergency.InhibitEmTimer += timeDifferenceUs
		}
		if fifoPpPtr != emergency.FifoWrPtr &&
			!emergency.CANTxBuff.BufferFull &&
			emergency.InhibitEmTimer >= emergency.InhibitEmTimeUs {
			emergency.InhibitEmTimer = 0

			emergency.Fifo[fifoPpPtr].msg |= uint32(errorRegister) << 16
			binary.LittleEndian.PutUint32(emergency.CANTxBuff.Data[:4], emergency.Fifo[fifoPpPtr].msg)
			emergency.busManager.Send(*emergency.CANTxBuff)
			// Also report own emergency message
			if emergency.EmergencyRxCallback != nil {
				errMsg := uint32(emergency.Fifo[fifoPpPtr].msg)
				emergency.EmergencyRxCallback(
					0,
					uint16(errMsg),
					byte(errorRegister),
					byte(errMsg>>24),
					emergency.Fifo[fifoPpPtr].info,
				)
			}
			fifoPpPtr += 1
			if int(fifoPpPtr) < len(emergency.Fifo) {
				emergency.FifoPpPtr = fifoPpPtr
			} else {
				emergency.FifoPpPtr = 0
			}
			if emergency.FifoOverflow == 1 {
				emergency.FifoOverflow = 2
				emergency.ErrorReport(CO_EM_EMERGENCY_BUFFER_FULL, CO_EMC_GENERIC, 0)
			} else if emergency.FifoOverflow == 2 && fifoPpPtr == emergency.FifoWrPtr {
				emergency.FifoOverflow = 0
				emergency.ErrorReset(CO_EM_EMERGENCY_BUFFER_FULL, 0)
			}
		} else if timerNextUs != nil && emergency.InhibitEmTimeUs < emergency.InhibitEmTimer {
			diff := emergency.InhibitEmTimeUs - emergency.InhibitEmTimer
			if *timerNextUs > diff {
				*timerNextUs = diff
			}
		}

	}
}

// Set or reset an error condition
// Function adds a new error to the history & error will be processed by Process function
func (emergency *EM) Error(setError bool, errorBit byte, errorCode uint16, infoCode uint32) error {
	if emergency == nil {
		return nil
	}
	index := errorBit >> 3
	bitMask := 1 << (errorBit & 0x7)

	// Unsupported errorBit
	if index >= CO_CONFIG_EM_ERR_STATUS_BITS_COUNT/8 {
		index = CO_EM_WRONG_ERROR_REPORT >> 3
		bitMask = 1 << (CO_EM_WRONG_ERROR_REPORT & 0x7)
		errorCode = CO_EMC_SOFTWARE_INTERNAL
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
		errorCode = CO_EMC_NO_ERROR
	}
	errMsg := (uint32(errorBit) << 24) | uint32(errorCode)
	if len(emergency.Fifo) >= 2 {
		fifoWrPtr := emergency.FifoWrPtr
		fifoWrPtrNext := fifoWrPtr + 1
		if int(fifoWrPtrNext) >= len(emergency.Fifo) {
			fifoWrPtrNext = 0
		}
		if fifoWrPtrNext == emergency.FifoPpPtr {
			emergency.FifoOverflow = 1
		} else {
			emergency.Fifo[fifoWrPtr].msg = errMsg
			emergency.Fifo[fifoWrPtr].info = infoCode
			emergency.FifoWrPtr = fifoWrPtrNext
			if int(emergency.FifoCount) < len(emergency.Fifo)-1 {
				emergency.FifoCount++
			}
		}
	}
	return nil
}

func (emergency *EM) ErrorReport(errorBit byte, errorCode uint16, infoCode uint32) error {
	log.Warnf("[EMERGENCY][TX][ERROR] %v (x%x) | %v (x%x) | infoCode %v",
		getErrorCodeDescription(int(errorCode)),
		errorCode,
		getErrorStatusDescription(errorBit),
		errorBit,
		infoCode,
	)
	return emergency.Error(true, errorBit, errorCode, infoCode)
}

func (emergency *EM) ErrorReset(errorBit byte, infoCode uint32) error {
	log.Warnf("[EMERGENCY][TX][RESET] reset emergency %v (x%x) | infoCode %v",
		getErrorStatusDescription(errorBit),
		errorBit,
		infoCode,
	)
	return emergency.Error(false, errorBit, CO_EMC_NO_ERROR, infoCode)
}

func (emergency *EM) IsError(errorBit byte) bool {
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

func (emergency *EM) GetErrorRegister() byte {
	if emergency == nil || emergency.errorRegister == nil {
		return 0
	}
	return *emergency.errorRegister
}
