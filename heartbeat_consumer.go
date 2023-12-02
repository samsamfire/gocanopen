package canopen

import (
	log "github.com/sirupsen/logrus"
)

const (
	HB_UNCONFIGURED       = 0x00 /**< Consumer entry inactive */
	HB_UNKNOWN            = 0x01 /**< Consumer enabled, but no heartbeat received yet */
	HB_ACTIVE             = 0x02 /**< Heartbeat received within set time */
	HB_TIMEOUT            = 0x03 /**< No heatbeat received for set time */
	NMT_UNKNOWN     int16 = -1
)

// Node specific hearbeat consumer part
type HBConsumerNode struct {
	NodeId       uint8
	cobId        uint16
	NMTState     int16
	NMTStatePrev int16
	HBState      uint8
	TimeoutTimer uint32
	TimeUs       uint32
	rxNew        bool
}

type HBConsumer struct {
	em                        *EM
	monitoredNodes            []HBConsumerNode
	nbMonitoredNodes          uint8
	AllMonitoredActive        bool
	AllMonitoredOperational   bool
	NMTisPreOrOperationalPrev bool
	busManager                *BusManager
}

// Handle hearbeat reception specific to a node
func (nodeConsumer *HBConsumerNode) Handle(frame Frame) {
	if frame.DLC != 1 {
		return
	}
	nodeConsumer.NMTState = int16(frame.Data[0])
	nodeConsumer.rxNew = true

}

// Add a consumer node
func (consumer *HBConsumer) addHearbeatConsumerNode(index uint8, nodeId uint8, consumerTimeMs uint16) error {
	if index >= consumer.nbMonitoredNodes {
		return ErrIllegalArgument
	}
	// Check duplicate entries
	if consumerTimeMs != 0 && nodeId != 0 {
		for i, consumerNode := range consumer.monitoredNodes {
			if int(index) != i && consumerNode.TimeUs != 0 && consumerNode.NodeId == nodeId {
				return ErrIllegalArgument
			}
		}
	}
	consumerNode, err := NewHBConsumerNode(index, nodeId, consumerTimeMs)
	if err != nil {
		return err
	}
	// Configure RX buffer for hearbeat reception
	if consumerNode.HBState != HB_UNCONFIGURED {
		log.Debugf("[HB CONSUMER] adding consumer for id %v | timeout %v us", consumerNode.NodeId, consumerNode.TimeUs)
		err = consumer.busManager.Subscribe(uint32(consumerNode.cobId), 0x7FF, false, consumerNode)
	}
	if err != nil {
		return err
	}
	log.Debugf("[HB CONSUMER] added x%x to list of monitored nodes | timeout %v", nodeId, consumerTimeMs)
	if err != nil && err != ErrOdParameters {
		// Unknown error
		log.Errorf("[HB CONSUMER] initializing HB consumer object %v failed : %v", index, err)
		return err
	} else if err == ErrOdParameters {
		log.Warnf("[HB CONSUMER] initializing HB consumer object %v failed, ignoring : %v", index, err)
	} else {
		consumer.monitoredNodes[index] = *consumerNode
	}
	return err
}

// process Hearbeat consuming
func (consumer *HBConsumer) process(nmtIsPreOrOperational bool, timeDifferenceUs uint32, timerNextUs *uint32) {
	allMonitoredActiveCurrent := true
	allMonitoredOperationalCurrent := true
	if nmtIsPreOrOperational && consumer.NMTisPreOrOperationalPrev {
		for i := range consumer.monitoredNodes {
			monitoredNode := &consumer.monitoredNodes[i]
			timeDifferenceUsCopy := timeDifferenceUs
			// If unconfigured skip to next iteration
			if monitoredNode.HBState == HB_UNCONFIGURED {
				continue
			}
			if monitoredNode.rxNew {
				if monitoredNode.NMTState == int16(NMT_INITIALIZING) {
					// Boot up message is an error if previously received (means reboot)
					if monitoredNode.HBState == HB_ACTIVE {
						consumer.em.ErrorReport(CO_EM_HB_CONSUMER_REMOTE_RESET, CO_EMC_HEARTBEAT, uint32(i))
					}
					monitoredNode.HBState = HB_UNKNOWN
				} else {
					// Heartbeat message
					monitoredNode.HBState = HB_ACTIVE
					monitoredNode.TimeoutTimer = 0
					timeDifferenceUsCopy = 0
				}
				monitoredNode.rxNew = false
			}
			// Check timeout
			if monitoredNode.HBState == HB_ACTIVE {
				monitoredNode.TimeoutTimer += timeDifferenceUsCopy
				if monitoredNode.TimeoutTimer >= monitoredNode.TimeUs {
					// Timeout is expired
					consumer.em.ErrorReport(CO_EM_HEARTBEAT_CONSUMER, CO_EMC_HEARTBEAT, uint32(i))
					monitoredNode.NMTState = NMT_UNKNOWN
					monitoredNode.HBState = HB_TIMEOUT
				} else if timerNextUs != nil {
					// Calculate when to recheck
					diff := monitoredNode.TimeUs - monitoredNode.TimeoutTimer
					if *timerNextUs > diff {
						*timerNextUs = diff
					}
				}
			}
			if monitoredNode.HBState != HB_ACTIVE {
				allMonitoredActiveCurrent = false
			}
			if monitoredNode.NMTState != int16(NMT_OPERATIONAL) {
				allMonitoredOperationalCurrent = false
			}

			if monitoredNode.NMTState != monitoredNode.NMTStatePrev {
				monitoredNode.NMTStatePrev = monitoredNode.NMTState
			}
		}
	} else if nmtIsPreOrOperational || consumer.NMTisPreOrOperationalPrev {
		// pre or operational state changed, clear vars
		for i := range consumer.monitoredNodes {
			monitoredNode := &consumer.monitoredNodes[i]
			monitoredNode.NMTState = NMT_UNKNOWN
			monitoredNode.NMTStatePrev = NMT_UNKNOWN
			monitoredNode.rxNew = false
			if monitoredNode.HBState != HB_UNCONFIGURED {
				monitoredNode.HBState = HB_UNKNOWN
			}
		}
		allMonitoredActiveCurrent = false
		allMonitoredOperationalCurrent = false
	}

	// Clear emergencies when all monitored nodes become active
	if !consumer.AllMonitoredActive && allMonitoredActiveCurrent {
		consumer.em.ErrorReset(CO_EM_HEARTBEAT_CONSUMER, 0)
		consumer.em.ErrorReset(CO_EM_HB_CONSUMER_REMOTE_RESET, 0)
	}
	consumer.AllMonitoredActive = allMonitoredActiveCurrent
	consumer.AllMonitoredOperational = allMonitoredOperationalCurrent
	consumer.NMTisPreOrOperationalPrev = nmtIsPreOrOperational
}

// Initialize a single node consumer
func NewHBConsumerNode(index uint8, nodeId uint8, consumerTimeMs uint16) (*HBConsumerNode, error) {

	monitoredNode := &HBConsumerNode{}
	monitoredNode.NodeId = nodeId
	monitoredNode.TimeUs = uint32(consumerTimeMs) * 1000
	monitoredNode.NMTState = NMT_UNKNOWN
	monitoredNode.NMTStatePrev = NMT_UNKNOWN
	monitoredNode.rxNew = false

	if monitoredNode.NodeId != 0 && monitoredNode.TimeUs != 0 {
		monitoredNode.cobId = uint16(monitoredNode.NodeId) + HEARTBEAT_SERVICE_ID
		monitoredNode.HBState = HB_UNKNOWN
	} else {
		monitoredNode.cobId = 0
		monitoredNode.TimeUs = 0
		monitoredNode.HBState = HB_UNCONFIGURED
	}
	return monitoredNode, nil

}

// Initialize hearbeat consumer
func NewHBConsumer(busManager *BusManager, em *EM, entry1016 *Entry) (*HBConsumer, error) {

	if entry1016 == nil || busManager == nil || em == nil {
		return nil, ErrIllegalArgument
	}
	consumer := &HBConsumer{}
	consumer.em = em
	consumer.busManager = busManager

	// Get real number of monitored nodes
	consumer.nbMonitoredNodes = uint8(entry1016.SubCount() - 1)
	log.Debugf("[HB CONSUMER] %v possible entries for nodes to monitor", consumer.nbMonitoredNodes)
	consumer.monitoredNodes = make([]HBConsumerNode, consumer.nbMonitoredNodes)
	for index := 0; index < int(consumer.nbMonitoredNodes); index++ {
		var hbConsValue uint32
		err := entry1016.Uint32(uint8(index)+1, &hbConsValue)
		if err != nil {
			log.Errorf("[HB CONSUMER][%x|%x] reading %v failed : %v", entry1016.Index, index+1, entry1016.Name, err)
			return nil, ErrOdParameters
		}
		nodeId := (hbConsValue >> 16) & 0xFF
		time := uint16(hbConsValue & 0xFFFF)
		// Set the buffer index before initializing
		err = consumer.addHearbeatConsumerNode(uint8(index), uint8(nodeId), time)
		if err != nil {
			return nil, err
		}
	}
	entry1016.AddExtension(consumer, ReadEntryDefault, WriteEntry1016)
	return consumer, nil

}
