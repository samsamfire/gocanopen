package canopen

import (
	"encoding/binary"

	"github.com/brutella/can"
	log "github.com/sirupsen/logrus"
)

/* TODOs
- Maybe implement callbacks on change etc
- Has not been tested at all
- Missing nmt state on error transitions because don't have Emergency yet
- Finish CANModule sending

*/

const (
	CO_NMT_ERR_REG_MASK            uint16 = 0x00FF
	CO_NMT_STARTUP_TO_OPERATIONAL  uint16 = 0x0100
	CO_NMT_ERR_ON_BUSOFF_HB        uint16 = 0x1000
	CO_NMT_ERR_ON_ERR_REG          uint16 = 0x2000
	CO_NMT_ERR_TO_STOPPED          uint16 = 0x4000
	CO_NMT_ERR_FREE_TO_OPERATIONAL uint16 = 0x8000
)

const (
	CO_RESET_NOT  uint8 = 0
	CO_RESET_COMM uint8 = 1
	CO_RESET_APP  uint8 = 2
	CO_RESET_QUIT uint8 = 3
)

const (
	CO_NMT_UNKNOWN         = -1
	CO_NMT_INITIALIZING    = 0
	CO_NMT_PRE_OPERATIONAL = 127
	CO_NMT_OPERATIONAL     = 5
	CO_NMT_STOPPED         = 4
)

const (
	CO_NMT_NO_COMMAND            = 0
	CO_NMT_ENTER_OPERATIONAL     = 1
	CO_NMT_ENTER_STOPPED         = 2
	CO_NMT_ENTER_PRE_OPERATIONAL = 128
	CO_NMT_RESET_NODE            = 129
	CO_NMT_RESET_COMMUNICATION   = 130
)

// NMT object
type NMT struct {
	OperatingState         uint8
	OperatingStatePrev     uint8
	InternalCommand        uint8
	NodeId                 uint8
	Control                uint16
	HearbeatProducerTimeUs uint32
	HearbeatProducerTimer  uint32
	ExtensionEntry1017     Extension
	Emergency              *EM
	CANModule              *CANModule
	NMTTxBuff              *BufferTxFrame
	HBTxBuff               *BufferTxFrame
	Callback               func(nmtState uint8)
}

// NMT RX buffer handle (called when node receives an nmt message)
// Implements FrameHandler
func (nmt *NMT) Handle(frame can.Frame) {
	dlc := frame.Length
	data := frame.Data
	if dlc != 2 {
		log.Warnf("Received an NMT message but the length is not correct : %v", dlc)
	}
	command := data[0]
	nodeId := data[1]

	if dlc == 2 && (nodeId == 0 || nodeId == nmt.NodeId) {
		nmt.InternalCommand = command
	}
}

func (nmt *NMT) Init(
	EntryHbProducer *Entry,
	emergency *EM,
	node_id uint8,
	control uint16,
	first_hb_time_ms uint16,
	can_module *CANModule,
	can_id_nmt_tx uint16,
	can_id_nmt_rx uint16,
	can_id_hb_tx uint16,
) error {
	if EntryHbProducer == nil || can_module == nil {
		log.Errorf("Entry 1017, Emergency object or CANModule is nil")
		return CO_ERROR_ILLEGAL_ARGUMENT
	}

	nmt.OperatingState = CO_NMT_INITIALIZING
	nmt.OperatingStatePrev = nmt.OperatingState
	nmt.NodeId = node_id
	nmt.Control = control
	nmt.Emergency = emergency
	nmt.HearbeatProducerTimer = uint32(first_hb_time_ms * 1000)

	/* get and verify required "Producer heartbeat time" from Object Dict. */

	var HBprodTime_ms uint16
	err := EntryHbProducer.GetUint16(0, &HBprodTime_ms)
	if err != nil {
		log.Errorf("Error when reading entry for producer hearbeat at 0x1017 : %v", err)
		return CO_ERROR_OD_PARAMETERS
	}
	nmt.HearbeatProducerTimeUs = uint32(HBprodTime_ms) * 1000
	// Extension needs to be initialized
	nmt.ExtensionEntry1017.Object = nmt
	nmt.ExtensionEntry1017.Read = ReadEntryOriginal
	nmt.ExtensionEntry1017.Write = writeEntry1017
	// And added to the entry
	EntryHbProducer.AddExtension(&nmt.ExtensionEntry1017)

	if nmt.HearbeatProducerTimer > nmt.HearbeatProducerTimeUs {
		nmt.HearbeatProducerTimer = nmt.HearbeatProducerTimeUs
	}

	// Configure CAN TX/RX buffers
	nmt.CANModule = can_module
	// NMT RX buffer
	_, err = can_module.InsertRxBuffer(uint32(can_id_nmt_rx), 0x7FF, false, nmt)
	if err != nil {
		log.Error("Failed to Initialize NMT rx buffer")
		return err
	}
	// NMT TX buffer
	nmt.NMTTxBuff, _, err = can_module.InsertTxBuffer(uint32(can_id_nmt_tx), false, 2, false)
	if err != nil {
		log.Error("Failed to Initialize NMT tx buffer")
		return err
	}
	// NMT HB TX buffer
	nmt.HBTxBuff, _, err = can_module.InsertTxBuffer(uint32(can_id_hb_tx), false, 1, false)
	if err != nil {
		log.Error("Failed to Initialize HB tx buffer")
		return err
	}
	return nil

}

// Called cyclically
func (nmt *NMT) Process(internal_state *uint8, time_difference_us uint32, timer_next_us *uint32) uint8 {
	nmtStateCopy := nmt.OperatingState
	resetCommand := CO_RESET_NOT
	nmtInit := nmtStateCopy == CO_NMT_INITIALIZING
	if nmt.HearbeatProducerTimer > time_difference_us {
		nmt.HearbeatProducerTimer = nmt.HearbeatProducerTimer - time_difference_us
	} else {
		nmt.HearbeatProducerTimer = 0
	}
	/* Send Hearbeat if:
	* - First start (send bootup message)
	* - Hearbeat producer enabled and timer expired or the operating state has changed
	 */
	if nmtInit || (nmt.HearbeatProducerTimeUs != 0 && (nmt.HearbeatProducerTimer == 0 || nmtStateCopy != nmt.OperatingStatePrev)) {
		nmt.HBTxBuff.Data[0] = nmtStateCopy
		nmt.CANModule.Send(*nmt.HBTxBuff)
		if nmtStateCopy == CO_NMT_INITIALIZING {
			/* NMT slave is self starting */
			if nmt.Control&CO_NMT_STARTUP_TO_OPERATIONAL != 0 {
				nmtStateCopy = CO_NMT_OPERATIONAL
			} else {
				nmtStateCopy = CO_NMT_PRE_OPERATIONAL
			}
		} else {
			/* Start timer from the beginning. If OS is slow, time sliding may
			* occur. However, heartbeat is not for synchronization, it is for
			* health report. In case of initializing, timer is set in the
			* CO_NMT_init() function with pre-defined value. */
			nmt.HearbeatProducerTimer = nmt.HearbeatProducerTimeUs

		}
	}
	nmt.OperatingStatePrev = nmtStateCopy

	/* Process internal NMT commands either from RX buffer or nmt Send COmmand */
	if nmt.InternalCommand != CO_NMT_NO_COMMAND {
		switch nmt.InternalCommand {
		case CO_NMT_ENTER_OPERATIONAL:
			nmtStateCopy = CO_NMT_OPERATIONAL

		case CO_NMT_ENTER_STOPPED:
			nmtStateCopy = CO_NMT_STOPPED

		case CO_NMT_ENTER_PRE_OPERATIONAL:
			nmtStateCopy = CO_NMT_PRE_OPERATIONAL

		case CO_NMT_RESET_NODE:
			resetCommand = CO_RESET_APP

		case CO_NMT_RESET_COMMUNICATION:
			resetCommand = CO_RESET_COMM

		}
		nmt.InternalCommand = CO_NMT_NO_COMMAND
	}

	/* verify NMT transitions based on error register */
	// TODO, don't have Emergency implementation yet

	/* Callback on operating state change */
	if nmt.OperatingStatePrev != nmtStateCopy || nmtInit {
		if nmt.Callback != nil {
			nmt.Callback(nmtStateCopy)
		}
	}

	/* Calculate, when next Heartbeat needs to be send */
	if nmt.HearbeatProducerTimeUs != 0 && timer_next_us != nil {
		if nmt.OperatingStatePrev != nmtStateCopy {
			*timer_next_us = 0
		} else if *timer_next_us > nmt.HearbeatProducerTimer {
			*timer_next_us = nmt.HearbeatProducerTimer
		}
	}

	nmt.OperatingState = nmtStateCopy
	*internal_state = nmtStateCopy

	return resetCommand

}

func (nmt *NMT) GetInternalState() uint8 {
	if nmt == nil {
		return CO_NMT_INITIALIZING
	} else {
		return nmt.OperatingState
	}
}

// Send NMT command to self, don't send on network
func (nmt *NMT) SendInternalCommand(command uint8) {
	nmt.InternalCommand = command
}

// Send an NMT command to the network
func (nmt *NMT) SendCommand(command uint8, node_id uint8) error {

	if nmt == nil {
		return CO_ERROR_ILLEGAL_ARGUMENT
	}

	/* Apply NMT command also to this node, if set so. */
	if node_id == 0 || node_id == nmt.NodeId {
		nmt.InternalCommand = command
	}

	/* Send NMT master message. */
	nmt.NMTTxBuff.Data[0] = command
	nmt.NMTTxBuff.Data[1] = node_id

	nmt.CANModule.Send((*nmt.NMTTxBuff))
	return nil

}

// Extension for entry 0x1017
func writeEntry1017(stream *Stream, data []byte, countWritten *uint16) error {
	if stream.Subindex != 0 || data == nil || len(data) != 2 || countWritten == nil || stream == nil {
		return ODR_DEV_INCOMPAT
	}
	nmt, ok := stream.Object.(NMT)
	if !ok {
		log.Error("Invalid type for object 1017 : %v", nmt)
		return ODR_GENERAL
	}
	nmt.HearbeatProducerTimeUs = uint32(binary.LittleEndian.Uint16(data)) * 1000
	nmt.HearbeatProducerTimer = 0
	// Also update the original OD location
	return WriteEntryOriginal(stream, data, countWritten)
}
