package canopen

import (
	"encoding/binary"

	"github.com/brutella/can"
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
	/** 0x00xx, error Reset or No Error */
	CO_EMC_NO_ERROR = 0x0000
	/** 0x10xx, Generic Error */
	CO_EMC_GENERIC = 0x1000
	/** 0x20xx, Current */
	CO_EMC_CURRENT = 0x2000
	/** 0x21xx, Current, device input side */
	CO_EMC_CURRENT_INPUT = 0x2100
	/** 0x22xx, Current inside the device */
	CO_EMC_CURRENT_INSIDE = 0x2200
	/** 0x23xx, Current, device output side */
	CO_EMC_CURRENT_OUTPUT = 0x2300
	/** 0x30xx, Voltage */
	CO_EMC_VOLTAGE = 0x3000
	/** 0x31xx, Mains Voltage */
	CO_EMC_VOLTAGE_MAINS = 0x3100
	/** 0x32xx, Voltage inside the device */
	CO_EMC_VOLTAGE_INSIDE = 0x3200
	/** 0x33xx, Output Voltage */
	CO_EMC_VOLTAGE_OUTPUT = 0x3300
	/** 0x40xx, Temperature */
	CO_EMC_TEMPERATURE = 0x4000
	/** 0x41xx, Ambient Temperature */
	CO_EMC_TEMP_AMBIENT = 0x4100
	/** 0x42xx, Device Temperature */
	CO_EMC_TEMP_DEVICE = 0x4200
	/** 0x50xx, Device Hardware */
	CO_EMC_HARDWARE = 0x5000
	/** 0x60xx, Device Software */
	CO_EMC_SOFTWARE_DEVICE = 0x6000
	/** 0x61xx, Internal Software */
	CO_EMC_SOFTWARE_INTERNAL = 0x6100
	/** 0x62xx, User Software */
	CO_EMC_SOFTWARE_USER = 0x6200
	/** 0x63xx, Data Set */
	CO_EMC_DATA_SET = 0x6300
	/** 0x70xx, Additional Modules */
	CO_EMC_ADDITIONAL_MODUL = 0x7000
	/** 0x80xx, Monitoring */
	CO_EMC_MONITORING = 0x8000
	/** 0x81xx, Communication */
	CO_EMC_COMMUNICATION = 0x8100
	/** 0x8110, CAN Overrun (Objects lost) */
	CO_EMC_CAN_OVERRUN = 0x8110
	/** 0x8120, CAN in Error Passive Mode */
	CO_EMC_CAN_PASSIVE = 0x8120
	/** 0x8130, Life Guard Error or Heartbeat Error */
	CO_EMC_HEARTBEAT = 0x8130
	/** 0x8140, recovered from bus off */
	CO_EMC_BUS_OFF_RECOVERED = 0x8140
	/** 0x8150, CAN-ID collision */
	CO_EMC_CAN_ID_COLLISION = 0x8150
	/** 0x82xx, Protocol Error */
	CO_EMC_PROTOCOL_ERROR = 0x8200
	/** 0x8210, PDO not processed due to length error */
	CO_EMC_PDO_LENGTH = 0x8210
	/** 0x8220, PDO length exceeded */
	CO_EMC_PDO_LENGTH_EXC = 0x8220
	/** 0x8230, DAM MPDO not processed, destination object not available */
	CO_EMC_DAM_MPDO = 0x8230
	/** 0x8240, Unexpected SYNC data length */
	CO_EMC_SYNC_DATA_LENGTH = 0x8240
	/** 0x8250, RPDO timeout */
	CO_EMC_RPDO_TIMEOUT = 0x8250
	/** 0x90xx, External Error */
	CO_EMC_EXTERNAL_ERROR = 0x9000
	/** 0xF0xx, Additional Functions */
	CO_EMC_ADDITIONAL_FUNC = 0xF000
	/** 0xFFxx, Device specific */
	CO_EMC_DEVICE_SPECIFIC = 0xFF00

	/** 0x2310, DS401, Current at outputs too high (overload) */
	CO_EMC401_OUT_CUR_HI = 0x2310
	/** 0x2320, DS401, Short circuit at outputs */
	CO_EMC401_OUT_SHORTED = 0x2320
	/** 0x2330, DS401, Load dump at outputs */
	CO_EMC401_OUT_LOAD_DUMP = 0x2330
	/** 0x3110, DS401, Input voltage too high */
	CO_EMC401_IN_VOLT_HI = 0x3110
	/** 0x3120, DS401, Input voltage too low */
	CO_EMC401_IN_VOLT_LOW = 0x3120
	/** 0x3210, DS401, Internal voltage too high */
	CO_EMC401_INTERN_VOLT_HI = 0x3210
	/** 0x3220, DS401, Internal voltage too low */
	CO_EMC401_INTERN_VOLT_LO = 0x3220
	/** 0x3310, DS401, Output voltage too high */
	CO_EMC401_OUT_VOLT_HIGH = 0x3310
	/** 0x3320, DS401, Output voltage too low */
	CO_EMC401_OUT_VOLT_LOW = 0x3320
)

// Error status bits
const (
	/** 0x00, Error Reset or No Error */
	CO_EM_NO_ERROR = 0x00
	/** 0x01, communication, info, CAN bus warning limit reached */
	CO_EM_CAN_BUS_WARNING = 0x01
	/** 0x02, communication, info, Wrong data length of the received CAN
	 * message */
	CO_EM_RXMSG_WRONG_LENGTH = 0x02
	/** 0x03, communication, info, Previous received CAN message wasn't
	 * processed yet */
	CO_EM_RXMSG_OVERFLOW = 0x03
	/** 0x04, communication, info, Wrong data length of received PDO */
	CO_EM_RPDO_WRONG_LENGTH = 0x04
	/** 0x05, communication, info, Previous received PDO wasn't processed yet */
	CO_EM_RPDO_OVERFLOW = 0x05
	/** 0x06, communication, info, CAN receive bus is passive */
	CO_EM_CAN_RX_BUS_PASSIVE = 0x06
	/** 0x07, communication, info, CAN transmit bus is passive */
	CO_EM_CAN_TX_BUS_PASSIVE = 0x07
	/** 0x08, communication, info, Wrong NMT command received */
	CO_EM_NMT_WRONG_COMMAND = 0x08
	/** 0x09, communication, info, TIME message timeout */
	CO_EM_TIME_TIMEOUT = 0x09
	/** 0x0A, communication, info, (unused) */
	CO_EM_0A_unused = 0x0A
	/** 0x0B, communication, info, (unused) */
	CO_EM_0B_unused = 0x0B
	/** 0x0C, communication, info, (unused) */
	CO_EM_0C_unused = 0x0C
	/** 0x0D, communication, info, (unused) */
	CO_EM_0D_unused = 0x0D
	/** 0x0E, communication, info, (unused) */
	CO_EM_0E_unused = 0x0E
	/** 0x0F, communication, info, (unused) */
	CO_EM_0F_unused = 0x0F

	/** 0x10, communication, critical, (unused) */
	CO_EM_10_unused = 0x10
	/** 0x11, communication, critical, (unused) */
	CO_EM_11_unused = 0x11
	/** 0x12, communication, critical, CAN transmit bus is off */
	CO_EM_CAN_TX_BUS_OFF = 0x12
	/** 0x13, communication, critical, CAN module receive buffer has
	 * overflowed */
	CO_EM_CAN_RXB_OVERFLOW = 0x13
	/** 0x14, communication, critical, CAN transmit buffer has overflowed */
	CO_EM_CAN_TX_OVERFLOW = 0x14
	/** 0x15, communication, critical, TPDO is outside SYNC window */
	CO_EM_TPDO_OUTSIDE_WINDOW = 0x15
	/** 0x16, communication, critical, (unused) */
	CO_EM_16_unused = 0x16
	/** 0x17, communication, critical, RPDO message timeout */
	CO_EM_RPDO_TIME_OUT = 0x17
	/** 0x18, communication, critical, SYNC message timeout */
	CO_EM_SYNC_TIME_OUT = 0x18
	/** 0x19, communication, critical, Unexpected SYNC data length */
	CO_EM_SYNC_LENGTH = 0x19
	/** 0x1A, communication, critical, Error with PDO mapping */
	CO_EM_PDO_WRONG_MAPPING = 0x1A
	/** 0x1B, communication, critical, Heartbeat consumer timeout */
	CO_EM_HEARTBEAT_CONSUMER = 0x1B
	/** 0x1C, communication, critical, Heartbeat consumer detected remote node
	 * reset */
	CO_EM_HB_CONSUMER_REMOTE_RESET = 0x1C
	/** 0x1D, communication, critical, (unused) */
	CO_EM_1D_unused = 0x1D
	/** 0x1E, communication, critical, (unused) */
	CO_EM_1E_unused = 0x1E
	/** 0x1F, communication, critical, (unused) */
	CO_EM_1F_unused = 0x1F

	/** 0x20, generic, info, Emergency buffer is full, Emergency message wasn't
	 * sent */
	CO_EM_EMERGENCY_BUFFER_FULL = 0x20
	/** 0x21, generic, info, (unused) */
	CO_EM_21_unused = 0x21
	/** 0x22, generic, info, Microcontroller has just started */
	CO_EM_MICROCONTROLLER_RESET = 0x22
	/** 0x23, generic, info, (unused) */
	CO_EM_23_unused = 0x23
	/** 0x24, generic, info, (unused) */
	CO_EM_24_unused = 0x24
	/** 0x25, generic, info, (unused) */
	CO_EM_25_unused = 0x25
	/** 0x26, generic, info, (unused) */
	CO_EM_26_unused = 0x26
	/** 0x27, generic, info, Automatic store to non-volatile memory failed */
	CO_EM_NON_VOLATILE_AUTO_SAVE = 0x27

	/** 0x28, generic, critical, Wrong parameters to CO_errorReport() function*/
	CO_EM_WRONG_ERROR_REPORT = 0x28
	/** 0x29, generic, critical, Timer task has overflowed */
	CO_EM_ISR_TIMER_OVERFLOW = 0x29
	/** 0x2A, generic, critical, Unable to allocate memory for objects */
	CO_EM_MEMORY_ALLOCATION_ERROR = 0x2A
	/** 0x2B, generic, critical, Generic error, test usage */
	CO_EM_GENERIC_ERROR = 0x2B
	/** 0x2C, generic, critical, Software error */
	CO_EM_GENERIC_SOFTWARE_ERROR = 0x2C
	/** 0x2D, generic, critical, Object dictionary does not match the software*/
	CO_EM_INCONSISTENT_OBJECT_DICT = 0x2D
	/** 0x2E, generic, critical, Error in calculation of device parameters */
	CO_EM_CALCULATION_OF_PARAMETERS = 0x2E
	/** 0x2F, generic, critical, Error with access to non volatile device memory
	 */
	CO_EM_NON_VOLATILE_MEMORY = 0x2F

	/** 0x30+, manufacturer, info or critical, Error status buts, free to use by
	 * manufacturer. By default bits 0x30..0x3F are set as informational and
	 * bits 0x40..0x4F are set as critical. Manufacturer critical bits sets the
	 * error register, as specified by @ref CO_CONFIG_ERR_CONDITION_MANUFACTURER
	 */
	CO_EM_MANUFACTURER_START = 0x30
	/** (@ref CO_CONFIG_EM_ERR_STATUS_BITS_COUNT - 1), largest value of the
	 * Error status bit. */
	CO_EM_MANUFACTURER_END = CO_CONFIG_EM_ERR_STATUS_BITS_COUNT - 1
)

type EMFifo struct {
	msg  uint32
	info uint32
}

type EM struct {
	errorStatusBits     [CO_CONFIG_EM_ERR_STATUS_BITS_COUNT / 8]byte
	errorRegister       *byte
	CANerrorStatusOld   uint16
	CANmodule           *CANModule
	Fifo                []EMFifo
	FifoSize            byte
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

func ReadEntry1014(stream *Stream, data []byte, countRead *uint16) error {
	if stream == nil || data == nil || countRead == nil || len(data) < 4 || stream.Subindex != 0 {
		return ODR_DEV_INCOMPAT
	}
	em, ok := stream.Object.(*EM)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	var canId uint16
	if em.ProducerIdent == EMERGENCY_SERVICE_ID {
		canId = EMERGENCY_SERVICE_ID + uint16(em.NodeId)
	} else {
		canId = em.ProducerIdent
	}
	var cobId uint32
	if em.ProducerEnabled {
		cobId = 0
	} else {
		cobId = 0x80000000
	}
	cobId |= uint32(canId)
	binary.LittleEndian.PutUint32(data, cobId)
	*countRead = 4
	return nil
}

func WriteEntry1014(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || countWritten == nil || len(data) != 4 || stream.Subindex != 0 {
		return ODR_DEV_INCOMPAT
	}
	em, ok := stream.Object.(*EM)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	// Check written value, cob id musn't change when enabled
	cobId := binary.LittleEndian.Uint32(data)
	newCanId := cobId & 0x7FF
	var currentCanId uint16
	if em.ProducerIdent == EMERGENCY_SERVICE_ID {
		currentCanId = EMERGENCY_SERVICE_ID + uint16(em.NodeId)
	} else {
		currentCanId = em.ProducerIdent
	}
	newEnabled := (cobId&uint32(currentCanId)) == 0 && newCanId != 0
	if cobId&0x7FFFF800 != 0 || isIDRestricted(uint16(newCanId)) ||
		(em.ProducerEnabled && newEnabled && newCanId != uint32(currentCanId)) {
		return ODR_INVALID_VALUE
	}
	em.ProducerEnabled = newEnabled
	if newCanId == uint32(EMERGENCY_SERVICE_ID+uint16(em.NodeId)) {
		em.ProducerIdent = EMERGENCY_SERVICE_ID
	} else {
		em.ProducerIdent = uint16(newCanId)
	}

	if newEnabled {
		var err error
		em.CANTxBuff, err = em.CANmodule.UpdateTxBuffer(
			int(em.TxBufferIdx),
			newCanId,
			false,
			8,
			false,
		)
		if em.CANTxBuff == nil || err != nil {
			return ODR_DEV_INCOMPAT
		}
	}
	return WriteEntryOriginal(stream, data, countWritten)

}

func WriteEntry1015(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || stream.Subindex != 0 || data == nil || len(data) != 2 || countWritten == nil {
		return ODR_DEV_INCOMPAT
	}
	em, ok := stream.Object.(*EM)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	em.InhibitEmTimeUs = uint32(binary.LittleEndian.Uint16(data)) * 100
	em.InhibitEmTimer = 0

	return WriteEntryOriginal(stream, data, countWritten)

}

func ReadEntry1003(stream *Stream, data []byte, countRead *uint16) error {
	if stream == nil || data == nil || countRead == nil ||
		(len(data) < 4 && stream.Subindex > 0) ||
		len(data) < 1 {
		return ODR_DEV_INCOMPAT
	}
	em, ok := stream.Object.(*EM)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	if em.FifoSize < 2 {
		return ODR_DEV_INCOMPAT
	}
	if stream.Subindex == 0 {
		data[0] = em.FifoCount
		*countRead = 1
		return nil
	}
	if stream.Subindex > em.FifoCount {
		return ODR_NO_DATA
	}
	// Most recent error is in subindex 1 and stored behind fifoWrPtr
	index := int(em.FifoWrPtr) - int(stream.Subindex)
	if index >= int(em.FifoSize) {
		return ODR_DEV_INCOMPAT
	}
	if index < 0 {
		index += int(em.FifoSize)
	}
	binary.LittleEndian.PutUint32(data, em.Fifo[index].msg)
	*countRead = 4
	return nil
}

func WriteEntry1003(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || stream.Subindex != 0 || data == nil || len(data) != 1 || countWritten == nil {
		return ODR_DEV_INCOMPAT
	}
	if data[0] != 0 {
		return ODR_INVALID_VALUE
	}
	em, ok := stream.Object.(*EM)
	if !ok {
		return ODR_DEV_INCOMPAT
	}
	// Clear error history
	em.FifoCount = 0
	*countWritten = 1
	return nil
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

func (emergency *EM) Handle(frame can.Frame) {
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
	canmodule *CANModule,
	entry1001 *Entry,
	fifo []EMFifo,
	entry1014 *Entry,
	entry1015 *Entry,
	entry1003 *Entry,
	entryStatusBits *Entry,
	nodeId uint8,
) error {
	if emergency == nil || entry1001 == nil ||
		fifo == nil ||
		entry1014 == nil || canmodule == nil ||
		nodeId < 1 || nodeId > 127 ||
		entry1003 == nil {
		return CO_ERROR_ILLEGAL_ARGUMENT

	}
	// Reset object
	emergency = &EM{}
	emergency.CANmodule = canmodule
	// TODO handle error register ptr
	*emergency.errorRegister = 0
	emergency.Fifo = fifo

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
	err := entry1014.AddExtension(&emergency.ExtensionEntry1014)
	if err != nil {
		return CO_ERROR_OD_PARAMETERS
	}
	emergency.ProducerIdent = uint16(producerCanId)
	if producerCanId == uint32(EMERGENCY_SERVICE_ID) {
		producerCanId += uint32(nodeId)
	}
	// Configure Tx buffer
	emergency.NodeId = nodeId
	emergency.CANTxBuff, emergency.TxBufferIdx, err = emergency.CANmodule.InsertTxBuffer(producerCanId, false, 8, false)
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

	emergency.ExtensionStatusBits.Object = emergency
	emergency.ExtensionStatusBits.Read = ReadEntryStatusBits
	emergency.ExtensionStatusBits.Write = WriteEntryStatusBits
	entryStatusBits.AddExtension(&emergency.ExtensionStatusBits)

	emergency.RxBufferIdx, err = canmodule.InsertRxBuffer(uint32(EMERGENCY_SERVICE_ID), 0x780, false, emergency)
	return err
}

func (emergency *EM) Process(nmtIsPreOrOperational bool, timeDifferenceUs uint32, timerNextUs *uint32) {
	// Check errors from driver
	canErrStatus := emergency.CANmodule.CANerrorstatus
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
	// TODO implement error register calculation
	errorRegister := 0
	if !nmtIsPreOrOperational {
		return
	}
	if emergency.FifoSize >= 2 {
		fifoPpPtr := emergency.FifoPpPtr
		if emergency.InhibitEmTimer < emergency.InhibitEmTimeUs {
			emergency.InhibitEmTimer += timeDifferenceUs
		}
		if fifoPpPtr != emergency.FifoWrPtr &&
			!emergency.CANTxBuff.BufferFull &&
			emergency.InhibitEmTimer >= emergency.InhibitEmTimeUs {
			emergency.InhibitEmTimer = 0
			emergency.Fifo[fifoPpPtr].msg |= uint32(errorRegister) << 16
			// Send emergency
			binary.LittleEndian.PutUint32(emergency.CANTxBuff.Data[:4], emergency.Fifo[fifoPpPtr].msg)
			emergency.CANmodule.Send(*emergency.CANTxBuff)
			// Also report own emergency message
			if emergency.EmergencyRxCallback != nil {
				errMsg := uint16(emergency.Fifo[fifoPpPtr].msg)
				emergency.EmergencyRxCallback(
					0,
					errMsg,
					byte(errorRegister),
					byte(errMsg)>>24,
					emergency.Fifo[fifoPpPtr].info,
				)
			}
			emergency.FifoPpPtr = fifoPpPtr
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
	if emergency.FifoSize >= 2 {
		fifoWrPtr := emergency.FifoWrPtr
		fifoWrPtrNext := fifoWrPtr + 1
		if fifoWrPtrNext >= emergency.FifoSize {
			fifoWrPtrNext = 0
		}
		if fifoWrPtrNext == emergency.FifoPpPtr {
			emergency.FifoOverflow = 1
		} else {
			emergency.Fifo[fifoWrPtr].msg = errMsg
			emergency.Fifo[fifoWrPtr].info = infoCode
			emergency.FifoWrPtr = fifoWrPtrNext
			if emergency.FifoCount < emergency.FifoSize-1 {
				emergency.FifoCount++
			}
		}
	}
}
