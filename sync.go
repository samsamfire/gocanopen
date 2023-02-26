package canopen

import (
	"encoding/binary"

	log "github.com/sirupsen/logrus"
)

/* TODOs
- BUG ! : don't recreate buffers on client setup, only update them
- Add dynamic od entries
- Test sync reception
- Test also with PDOs
- Add emergency frames
*/

type SYNC struct {
	emergency            *EM
	RxNew                bool
	ReceiveError         uint8
	RxToggle             bool
	TimeoutError         uint8
	CounterOverflowValue uint8
	Counter              uint8
	SyncIsOutsideWindow  bool
	Timer                uint32
	OD1006Period         *[]byte
	OD1007Window         *[]byte
	IsProducer           bool
	CANTxBuff            *BufferTxFrame
	CANTxBuffIndex       int
	CANRxBuffIndex       int
	BusManager           *BusManager
	Ident                uint16
	ExtensionEntry1005   Extension
	ExtensionEntry1019   Extension
}

const (
	CO_SYNC_NONE          uint8 = 0 /** No SYNC event in last cycle */
	CO_SYNC_RX_TX         uint8 = 1 /** SYNC message was received or transmitted in last cycle */
	CO_SYNC_PASSED_WINDOW uint8 = 2 /** Time has just passed SYNC window (OD_1007) in last cycle */
)

func (sync *SYNC) Handle(frame Frame) {
	syncReceived := false
	if sync.CounterOverflowValue == 0 {
		if frame.DLC == 0 {
			syncReceived = true
		} else {
			sync.ReceiveError = frame.DLC | 0x40
		}
	} else {
		if frame.DLC == 1 {
			sync.Counter = frame.Data[0]
			syncReceived = true
		} else {
			sync.ReceiveError = frame.DLC | 0x80
		}
	}
	if syncReceived {
		sync.RxToggle = !sync.RxToggle
		sync.RxNew = true
	}

}

func (sync *SYNC) Init(emergency *EM, entry1005 *Entry, entry1006 *Entry, entry1007 *Entry, entry1019 *Entry, busManager *BusManager) error {
	if emergency == nil || entry1005 == nil {
		return CO_ERROR_ILLEGAL_ARGUMENT
	}
	var cobIdSync uint32 = 0
	res := entry1005.GetUint32(0, &cobIdSync)
	if res != nil {
		log.Errorf("Error reading entry 1005 (Sync ID): %v", res)
		return CO_ERROR_OD_PARAMETERS
	}

	sync.ExtensionEntry1005.Object = sync
	sync.ExtensionEntry1005.Read = ReadEntryOriginal
	sync.ExtensionEntry1005.Write = WriteEntry1005
	entry1005.AddExtension(&sync.ExtensionEntry1005)

	var err error

	if entry1006 == nil {
		log.Warnf("Failed to read entry 1006 (Comm cycle period) because empty")
	} else {
		sync.OD1006Period, err = entry1006.GetPtr(0, 4)
		if err != nil {
			log.Errorf("Error reading entry 1006 (Comm cycle period): %v", res)
			return CO_ERROR_OD_PARAMETERS
		}
		log.Infof("[SYNC] comm cycle period %v ms", binary.LittleEndian.Uint32(*sync.OD1006Period))
	}

	if entry1007 == nil {
		log.Warnf("Failed to read entry 1007 (Synchronous window len) because empty")
	} else {
		sync.OD1007Window, err = entry1007.GetPtr(0, 4)
		if err != nil {
			log.Errorf("Error reading entry 1007 (Synchronous window len): %v", res)
			return CO_ERROR_OD_PARAMETERS
		}
		log.Infof("[SYNC] synchronous window %v ms", binary.LittleEndian.Uint32(*sync.OD1007Window))
	}

	var syncCounterOverflow uint8 = 0
	if entry1019 != nil {
		err = entry1019.GetUint8(0, &syncCounterOverflow)
		if err != nil {
			log.Errorf("Error reading entry 1019 (Synchronous counter overflow): %v", res)
			return CO_ERROR_OD_PARAMETERS
		}
		if syncCounterOverflow == 1 {
			syncCounterOverflow = 2
		} else if syncCounterOverflow > 240 {
			syncCounterOverflow = 240
		}
		sync.ExtensionEntry1019.Object = sync
		sync.ExtensionEntry1019.Read = ReadEntryOriginal
		sync.ExtensionEntry1019.Write = WriteEntry1019
		log.Infof("[SYNC] sync counter overflow %v", syncCounterOverflow)
	}
	sync.CounterOverflowValue = syncCounterOverflow
	sync.emergency = emergency
	sync.IsProducer = (cobIdSync & 0x40000000) != 0
	sync.Ident = uint16(cobIdSync) & 0x7FF
	sync.BusManager = busManager

	var err1 error
	sync.CANRxBuffIndex, err1 = sync.BusManager.InsertRxBuffer(uint32(sync.Ident), 0x7FF, false, sync)
	if err1 != nil {
		log.Errorf("Error initializing RX buffer for SDO client %v", err1)
		return err1
	}
	var err2 error
	var frameSize uint8 = 0
	if syncCounterOverflow != 0 {
		frameSize = 1
	}
	sync.CANTxBuff, sync.CANTxBuffIndex, err2 = sync.BusManager.InsertTxBuffer(uint32(sync.Ident), false, frameSize, false)
	if err2 != nil {
		log.Errorf("Error initializing TX buffer for SDO client %v", err2)
		return err2
	}
	if sync.CANTxBuff == nil {
		return CO_ERROR_ILLEGAL_ARGUMENT
	}
	log.Infof("[SYNC] initialized sync | producer : %v ",
		sync.IsProducer,
		syncCounterOverflow,
	)
	return nil
}

func (sync *SYNC) sendSync() {
	sync.Counter += 1
	if sync.Counter > sync.CounterOverflowValue {
		sync.Counter = 1
	}
	sync.Timer = 0
	sync.RxToggle = !sync.RxToggle
	sync.CANTxBuff.Data[0] = sync.Counter
	sync.BusManager.Send(*sync.CANTxBuff)
}

func (sync *SYNC) Process(nmtIsPreOrOperational bool, timeDifferenceUs uint32, timerNextUs *uint32) uint8 {
	status := CO_SYNC_NONE
	if !nmtIsPreOrOperational {
		sync.RxNew = false
		sync.ReceiveError = 0
		sync.Counter = 0
		sync.Timer = 0
		return CO_SYNC_NONE
	}

	timerNew := sync.Timer + timeDifferenceUs
	if timerNew > sync.Timer {
		sync.Timer = timerNew
	}
	if sync.RxNew {
		sync.Timer = 0
		sync.RxNew = false
	}
	entry1006Period := binary.LittleEndian.Uint32(*sync.OD1006Period)
	if entry1006Period > 0 {
		if sync.IsProducer {
			if sync.Timer >= entry1006Period {
				status = CO_SYNC_RX_TX
				sync.sendSync()
			}
			if timerNextUs != nil {
				diff := entry1006Period - sync.Timer
				if *timerNextUs > diff {
					*timerNextUs = diff
				}
			}
		} else if sync.TimeoutError == 1 {
			periodTimeout := entry1006Period + entry1006Period>>1
			if periodTimeout < entry1006Period {
				periodTimeout = 0xFFFFFFFF
			}
			if sync.Timer > periodTimeout {
				sync.emergency.Error(true, CO_EM_SYNC_TIME_OUT, CO_EMC_COMMUNICATION, sync.Timer)
				log.Warnf("[SYNC] time out error : %v", sync.Timer)
				sync.TimeoutError = 2
			} else if timerNextUs != nil {
				diff := periodTimeout - sync.Timer
				if *timerNextUs > diff {
					*timerNextUs = diff
				}
			}
		}
	}
	if sync.OD1007Window != nil {
		entry1007Window := binary.LittleEndian.Uint32(*sync.OD1007Window)
		if entry1007Window > 0 && sync.Timer > entry1007Window {
			if !sync.SyncIsOutsideWindow {
				status = CO_SYNC_PASSED_WINDOW
			}
		}
		sync.SyncIsOutsideWindow = true
	} else {
		sync.SyncIsOutsideWindow = false
	}

	// Check reception errors in handler
	if sync.ReceiveError != 0 {
		sync.emergency.Error(true, CO_EM_SYNC_LENGTH, CO_EMC_SYNC_DATA_LENGTH, sync.Timer)
		log.Warnf("[SYNC] receive error : %v", sync.ReceiveError)
		sync.ReceiveError = 0

	}

	if status == CO_SYNC_RX_TX {
		if sync.TimeoutError == 2 {
			sync.emergency.Error(false, CO_EM_SYNC_TIME_OUT, 0, 0)
			log.Warnf("[SYNC] reset error")
		}
		sync.TimeoutError = 1
	}

	return status
}
