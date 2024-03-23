package sync

import (
	"encoding/binary"
	s "sync"

	canopen "github.com/samsamfire/gocanopen"
	can "github.com/samsamfire/gocanopen/pkg/can"
	"github.com/samsamfire/gocanopen/pkg/emergency"
	"github.com/samsamfire/gocanopen/pkg/od"
	log "github.com/sirupsen/logrus"
)

type SYNC struct {
	*canopen.BusManager
	mu                          s.Mutex
	emcy                        *emergency.EMCY
	rxNew                       bool
	receiveError                uint8
	rxToggle                    bool
	timeoutError                uint8
	counterOverflow             uint8
	counter                     uint8
	syncIsOutsideWindow         bool
	timer                       uint32
	rawCommunicationCyclePeriod []byte
	rawSynchronousWindowLength  []byte
	isProducer                  bool
	cobId                       uint32
	txBuffer                    can.Frame
}

const (
	EventNone         uint8 = 0 // No SYNC event in last cycle
	EventRxOrTx       uint8 = 1 // SYNC message was received or transmitted in last cycle
	EventPassedWindow uint8 = 2 // Time has just passed SYNC window in last cycle (0x1007)
)

func (sync *SYNC) Handle(frame can.Frame) {
	sync.mu.Lock()
	defer sync.mu.Unlock()

	syncReceived := false
	if sync.counterOverflow == 0 {
		if frame.DLC == 0 {
			syncReceived = true
		} else {
			sync.receiveError = frame.DLC | 0x40
		}
	} else {
		if frame.DLC == 1 {
			sync.counter = frame.Data[0]
			syncReceived = true
		} else {
			sync.receiveError = frame.DLC | 0x80
		}
	}
	if syncReceived {
		sync.rxToggle = !sync.rxToggle
		sync.rxNew = true
	}

}

func (sync *SYNC) send() {
	sync.counter += 1
	if sync.counter > sync.counterOverflow {
		sync.counter = 1
	}
	sync.timer = 0
	sync.rxToggle = !sync.rxToggle
	sync.txBuffer.Data[0] = sync.counter
	_ = sync.Send(sync.txBuffer)
}

func (sync *SYNC) Counter() uint8 {
	sync.mu.Lock()
	defer sync.mu.Unlock()

	return sync.counter
}

func (sync *SYNC) RxToggle() bool {
	sync.mu.Lock()
	defer sync.mu.Unlock()

	return sync.rxToggle
}

func (sync *SYNC) CounterOverflow() uint8 {
	sync.mu.Lock()
	defer sync.mu.Unlock()

	return sync.counterOverflow
}

func (sync *SYNC) Process(nmtIsPreOrOperational bool, timeDifferenceUs uint32, timerNextUs *uint32) uint8 {
	sync.mu.Lock()
	defer sync.mu.Unlock()

	status := EventNone
	if !nmtIsPreOrOperational {
		sync.rxNew = false
		sync.receiveError = 0
		sync.counter = 0
		sync.timer = 0
		return EventNone
	}

	timerNew := sync.timer + timeDifferenceUs
	if timerNew > sync.timer {
		sync.timer = timerNew
	}
	if sync.rxNew {
		sync.timer = 0
		sync.rxNew = false
	}
	communicationCyclePeriod := binary.LittleEndian.Uint32(sync.rawCommunicationCyclePeriod)
	if communicationCyclePeriod > 0 {
		if sync.isProducer {
			if sync.timer >= communicationCyclePeriod {
				status = EventRxOrTx
				sync.mu.Unlock()
				sync.send()
				sync.mu.Lock()
			}
			if timerNextUs != nil {
				diff := communicationCyclePeriod - sync.timer
				if *timerNextUs > diff {
					*timerNextUs = diff
				}
			}
		} else if sync.timeoutError == 1 {
			periodTimeout := communicationCyclePeriod + communicationCyclePeriod>>1
			if periodTimeout < communicationCyclePeriod {
				periodTimeout = 0xFFFFFFFF
			}
			if sync.timer > periodTimeout {
				sync.emcy.Error(true, emergency.EmSyncTimeOut, emergency.ErrCommunication, sync.timer)
				log.Warnf("[SYNC] time out error : %v", sync.timer)
				sync.timeoutError = 2
			} else if timerNextUs != nil {
				diff := periodTimeout - sync.timer
				if *timerNextUs > diff {
					*timerNextUs = diff
				}
			}
		}
	}
	synchronousWindowLength := binary.LittleEndian.Uint32(sync.rawSynchronousWindowLength)
	if synchronousWindowLength > 0 && sync.timer > synchronousWindowLength {
		if !sync.syncIsOutsideWindow {
			status = EventPassedWindow
		}
		sync.syncIsOutsideWindow = true
	} else {
		sync.syncIsOutsideWindow = false
	}

	// Check reception errors in handler
	if sync.receiveError != 0 {
		sync.emcy.Error(true, emergency.EmSyncLength, emergency.ErrSyncDataLength, sync.timer)
		log.Warnf("[SYNC] receive error : %v", sync.receiveError)
		sync.receiveError = 0
	}
	if status == EventRxOrTx {
		if sync.timeoutError == 2 {
			sync.emcy.Error(false, emergency.EmSyncTimeOut, 0, 0)
			log.Warnf("[SYNC] reset error")
		}
		sync.timeoutError = 1
	}
	return status
}

func NewSYNC(
	bm *canopen.BusManager,
	emergency *emergency.EMCY,
	entry1005 *od.Entry,
	entry1006 *od.Entry,
	entry1007 *od.Entry,
	entry1019 *od.Entry,
) (*SYNC, error) {

	sync := &SYNC{BusManager: bm}
	if entry1005 == nil {
		return nil, canopen.ErrIllegalArgument
	}
	cobIdSync, err := entry1005.Uint32(0)
	if err != nil {
		log.Errorf("[SYNC][%x] %v read error", entry1005.Index, entry1005.Name)
		return nil, canopen.ErrOdParameters
	}
	entry1005.AddExtension(sync, od.ReadEntryDefault, writeEntry1005)

	if entry1006 == nil {
		log.Errorf("[SYNC][1006] COMM CYCLE PERIOD not found")
		return nil, canopen.ErrOdParameters
	} else if entry1007 == nil {
		log.Errorf("[SYNC][1007] SYNCHRONOUS WINDOW LENGTH not found")
		return nil, canopen.ErrOdParameters
	}

	entry1006.AddExtension(sync, od.ReadEntryDefault, writeEntry1006)
	sync.rawCommunicationCyclePeriod, err = entry1006.GetRawData(0, 4)
	if err != nil {
		log.Errorf("[SYNC][%x] %v read error", entry1006.Index, entry1006.Name)
		return nil, canopen.ErrOdParameters
	}
	log.Infof("[SYNC][%x] %v : %v", entry1006.Index, entry1006.Name, binary.LittleEndian.Uint32(sync.rawCommunicationCyclePeriod))

	entry1007.AddExtension(sync, od.ReadEntryDefault, writeEntry1007)
	sync.rawSynchronousWindowLength, err = entry1007.GetRawData(0, 4)
	if err != nil {
		log.Errorf("[SYNC][%x] %v read error", entry1007.Index, entry1007.Name)
		return nil, canopen.ErrOdParameters
	}
	log.Infof("[SYNC][%x] %v : %v", entry1007.Index, entry1007.Name, binary.LittleEndian.Uint32(sync.rawSynchronousWindowLength))

	// This one is not mandatory
	var syncCounterOverflow uint8 = 0
	if entry1019 != nil {
		syncCounterOverflow, err = entry1019.Uint8(0)
		if err != nil {
			log.Errorf("[SYNC][%x] %v read error", entry1019.Index, entry1019.Name)
			return nil, canopen.ErrOdParameters
		}
		if syncCounterOverflow == 1 {
			syncCounterOverflow = 2
		} else if syncCounterOverflow > 240 {
			syncCounterOverflow = 240
		}
		entry1019.AddExtension(sync, od.ReadEntryDefault, writeEntry1019)
		log.Infof("[SYNC][%x] %v : %v", entry1019.Index, entry1019.Name, syncCounterOverflow)
	}
	sync.counterOverflow = syncCounterOverflow
	sync.emcy = emergency
	sync.isProducer = (cobIdSync & 0x40000000) != 0
	sync.cobId = cobIdSync & 0x7FF

	err = sync.Subscribe(sync.cobId, 0x7FF, false, sync)
	if err != nil {
		return nil, err
	}
	var frameSize uint8 = 0
	if syncCounterOverflow != 0 {
		frameSize = 1
	}
	sync.txBuffer = can.NewFrame(sync.cobId, 0, frameSize)
	log.Infof("[SYNC] Initialisation finished")
	return sync, nil
}
