package canopen

import (
	"encoding/binary"

	log "github.com/sirupsen/logrus"
)

type SYNC struct {
	*busManager
	emergency                   *EM
	RxNew                       bool
	ReceiveError                uint8
	RxToggle                    bool
	TimeoutError                uint8
	CounterOverflowValue        uint8
	Counter                     uint8
	SyncIsOutsideWindow         bool
	Timer                       uint32
	rawCommunicationCyclePeriod []byte
	rawSynchronousWindowLength  []byte
	IsProducer                  bool
	cobId                       uint32
	txBuffer                    Frame
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

func (sync *SYNC) sendSync() {
	sync.Counter += 1
	if sync.Counter > sync.CounterOverflowValue {
		sync.Counter = 1
	}
	sync.Timer = 0
	sync.RxToggle = !sync.RxToggle
	sync.txBuffer.Data[0] = sync.Counter
	sync.Send(sync.txBuffer)
}

func (sync *SYNC) process(nmtIsPreOrOperational bool, timeDifferenceUs uint32, timerNextUs *uint32) uint8 {
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
	communicationCyclePeriod := binary.LittleEndian.Uint32(sync.rawCommunicationCyclePeriod)
	if communicationCyclePeriod > 0 {
		if sync.IsProducer {
			if sync.Timer >= communicationCyclePeriod {
				status = CO_SYNC_RX_TX
				sync.sendSync()
			}
			if timerNextUs != nil {
				diff := communicationCyclePeriod - sync.Timer
				if *timerNextUs > diff {
					*timerNextUs = diff
				}
			}
		} else if sync.TimeoutError == 1 {
			periodTimeout := communicationCyclePeriod + communicationCyclePeriod>>1
			if periodTimeout < communicationCyclePeriod {
				periodTimeout = 0xFFFFFFFF
			}
			if sync.Timer > periodTimeout {
				sync.emergency.Error(true, emSyncTimeOut, emErrCommunication, sync.Timer)
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
	synchronousWindowLength := binary.LittleEndian.Uint32(sync.rawSynchronousWindowLength)
	if synchronousWindowLength > 0 && sync.Timer > synchronousWindowLength {
		if !sync.SyncIsOutsideWindow {
			status = CO_SYNC_PASSED_WINDOW
		}
		sync.SyncIsOutsideWindow = true
	} else {
		sync.SyncIsOutsideWindow = false
	}

	// Check reception errors in handler
	if sync.ReceiveError != 0 {
		sync.emergency.Error(true, emSyncLength, emErrSyncDataLength, sync.Timer)
		log.Warnf("[SYNC] receive error : %v", sync.ReceiveError)
		sync.ReceiveError = 0

	}

	if status == CO_SYNC_RX_TX {
		if sync.TimeoutError == 2 {
			sync.emergency.Error(false, emSyncTimeOut, 0, 0)
			log.Warnf("[SYNC] reset error")
		}
		sync.TimeoutError = 1
	}

	return status
}

func NewSYNC(
	bm *busManager,
	emergency *EM,
	entry1005 *Entry,
	entry1006 *Entry,
	entry1007 *Entry,
	entry1019 *Entry,
) (*SYNC, error) {

	sync := &SYNC{busManager: bm}
	if entry1005 == nil {
		return nil, ErrIllegalArgument
	}
	cobIdSync, err := entry1005.Uint32(0)
	if err != nil {
		log.Errorf("[SYNC][%x] %v read error", entry1005.Index, entry1005.Name)
		return nil, ErrOdParameters
	}
	entry1005.AddExtension(sync, ReadEntryDefault, WriteEntry1005)

	if entry1006 == nil {
		log.Errorf("[SYNC][1006] COMM CYCLE PERIOD not found")
		return nil, ErrOdParameters
	} else if entry1007 == nil {
		log.Errorf("[SYNC][1007] SYNCHRONOUS WINDOW LENGTH not found")
		return nil, ErrOdParameters
	}

	entry1006.AddExtension(sync, ReadEntryDefault, WriteEntry1006)
	sync.rawCommunicationCyclePeriod, err = entry1006.GetRawData(0, 4)
	if err != nil {
		log.Errorf("[SYNC][%x] %v read error", entry1006.Index, entry1006.Name)
		return nil, ErrOdParameters
	}
	log.Infof("[SYNC][%x] %v : %v", entry1006.Index, entry1006.Name, binary.LittleEndian.Uint32(sync.rawCommunicationCyclePeriod))

	entry1007.AddExtension(sync, ReadEntryDefault, WriteEntry1007)
	sync.rawSynchronousWindowLength, err = entry1007.GetRawData(0, 4)
	if err != nil {
		log.Errorf("[SYNC][%x] %v read error", entry1007.Index, entry1007.Name)
		return nil, ErrOdParameters
	}
	log.Infof("[SYNC][%x] %v : %v", entry1007.Index, entry1007.Name, binary.LittleEndian.Uint32(sync.rawSynchronousWindowLength))

	// This one is not mandatory
	var syncCounterOverflow uint8 = 0
	if entry1019 != nil {
		syncCounterOverflow, err = entry1019.Uint8(0)
		if err != nil {
			log.Errorf("[SYNC][%x] %v read error", entry1019.Index, entry1019.Name)
			return nil, ErrOdParameters
		}
		if syncCounterOverflow == 1 {
			syncCounterOverflow = 2
		} else if syncCounterOverflow > 240 {
			syncCounterOverflow = 240
		}
		entry1019.AddExtension(sync, ReadEntryDefault, WriteEntry1019)
		log.Infof("[SYNC][%x] %v : %v", entry1019.Index, entry1019.Name, syncCounterOverflow)
	}
	sync.CounterOverflowValue = syncCounterOverflow
	sync.emergency = emergency
	sync.IsProducer = (cobIdSync & 0x40000000) != 0
	sync.cobId = cobIdSync & 0x7FF

	err = sync.Subscribe(sync.cobId, 0x7FF, false, sync)
	if err != nil {
		return nil, err
	}
	var frameSize uint8 = 0
	if syncCounterOverflow != 0 {
		frameSize = 1
	}
	sync.txBuffer = NewFrame(sync.cobId, 0, frameSize)
	log.Infof("[SYNC] Initialisation finished")
	return sync, nil
}
