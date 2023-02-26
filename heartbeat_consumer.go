package canopen

import (
	log "github.com/sirupsen/logrus"
)

const (
	HB_UNCONFIGURED = 0x00 /**< Consumer entry inactive */
	HB_UNKNOWN      = 0x01 /**< Consumer enabled, but no heartbeat received yet */
	HB_ACTIVE       = 0x02 /**< Heartbeat received within set time */
	HB_TIMEOUT      = 0x03 /**< No heatbeat received for set time */

)

// Node specific hearbeat consumer part
type HBConsumerNode struct {
	NodeId       uint8
	NMTState     int
	NMTStatePrev int
	HBState      uint8
	TimeoutTimer uint32
	TimeUs       uint32
	RxNew        bool
	RxBufferIdx  int
}

type HBConsumer struct {
	em                        *EM
	MonitoredNodes            []*HBConsumerNode
	NbMonitoredNodes          uint8
	AllMonitoredActive        bool
	AllMonitoredOperational   bool
	NMTisPreOrOperationalPrev bool
	busManager                *BusManager
	ExtensionEntry1016        Extension
}

// Handle hearbeat reception specific to a node
func (node_consumer *HBConsumerNode) Handle(frame Frame) {
	if frame.DLC != 8 {
		return
	}
	node_consumer.NMTState = int(frame.Data[0])
	node_consumer.RxNew = true

}

// Initialize hearbeat consumer
func (consumer *HBConsumer) Init(em *EM, monitoredNodes []*HBConsumerNode, entry1016 *Entry, busManager *BusManager) error {
	if monitoredNodes == nil || entry1016 == nil || busManager == nil {
		return CO_ERROR_ILLEGAL_ARGUMENT
	}
	consumer.em = em
	consumer.MonitoredNodes = monitoredNodes
	consumer.busManager = busManager

	// Get real number of monitored nodes
	nbSubEntries := entry1016.GetNbSubEntries()
	if nbSubEntries-1 < len(monitoredNodes) {
		consumer.NbMonitoredNodes = uint8(nbSubEntries) - 1
	} else {
		consumer.NbMonitoredNodes = uint8(len(monitoredNodes))
	}
	for index, monitoredNode := range consumer.MonitoredNodes {
		var hbConsValue uint32
		odRet := entry1016.GetUint32(uint8(index)+1, &hbConsValue)
		if odRet != nil {
			log.Errorf("Error accessing OD for HB consumer object because : %v", odRet)
			return CO_ERROR_OD_PARAMETERS
		}
		nodeId := (hbConsValue >> 16) & 0xFF
		time := uint16(hbConsValue & 0xFFFF)
		// Set the buffer index before initializing
		monitoredNode.RxBufferIdx = -1
		ret := consumer.InitEntry(uint8(index), uint8(nodeId), time)
		if ret != nil {
			// Exit only if more than a param problem
			if ret != CO_ERROR_OD_PARAMETERS {
				log.Errorf("Error when initializing HB consumer object for node x%x", nodeId)
				return ret
			}
			log.Warnf("Error (ignoring) when initializing HB consumer object for node x%x", nodeId)
		}

	}

	return nil

}

// Initialize a single node consumer
func (consumer *HBConsumer) InitEntry(index uint8, nodeId uint8, consumerTimeMs uint16) error {

	var ret error

	if index >= consumer.NbMonitoredNodes {
		return CO_ERROR_ILLEGAL_ARGUMENT
	}
	// Check duplicate entries
	if consumerTimeMs != 0 && nodeId != 0 {
		for i, consumer_node := range consumer.MonitoredNodes {
			if int(index) != i && consumer_node.TimeUs != 0 && consumer_node.NodeId == nodeId {
				return CO_ERROR_ILLEGAL_ARGUMENT
			}
		}
	}
	// Configure one monitored node
	monitoredNode := consumer.MonitoredNodes[index]
	monitoredNode.NodeId = nodeId
	monitoredNode.TimeUs = uint32(consumerTimeMs) * 1000
	monitoredNode.NMTState = CO_NMT_UNKNOWN
	monitoredNode.NMTStatePrev = CO_NMT_UNKNOWN
	monitoredNode.RxNew = false

	// Is it used ?
	var cobId uint16
	if monitoredNode.NodeId != 0 && monitoredNode.TimeUs != 0 {
		cobId = uint16(monitoredNode.NodeId) + HEARTBEAT_SERVICE_ID
	} else {
		cobId = 0
		monitoredNode.TimeUs = 0
		monitoredNode.HBState = HB_UNCONFIGURED
	}
	// Configure RX buffer for hearbeat reception
	if monitoredNode.HBState != HB_UNCONFIGURED {
		if monitoredNode.RxBufferIdx == -1 {
			// Never been configured
			monitoredNode.RxBufferIdx, ret = consumer.busManager.InsertRxBuffer(uint32(cobId), 0x7FF, false, monitoredNode)
		} else {
			//Only update
			ret = consumer.busManager.UpdateRxBuffer(monitoredNode.RxBufferIdx, uint32(cobId), 0x7FF, false, monitoredNode)
		}
	}
	return ret

}

// Process Hearbeat consuming
func (consumer *HBConsumer) Process(nmtIsPreOrOperational bool, timeDifferenceUs uint32, timerNextUs *uint32) {
	allMonitoredActiveCurrent := true
	allMonitoredOperationalCurrent := true
	if nmtIsPreOrOperational && consumer.NMTisPreOrOperationalPrev {
		for _, monitoredNode := range consumer.MonitoredNodes {
			timeDifferenceUsCopy := timeDifferenceUs
			// If unconfigured skip to next iteration
			if monitoredNode.HBState == HB_UNCONFIGURED {
				continue
			}

			if monitoredNode.RxNew {
				if monitoredNode.NMTState == CO_NMT_INITIALIZING {
					//Boot up message
					// if monitoredNode.HBState == HB_ACTIVE {
					// 	// TODO add emergency send
					// }
					monitoredNode.HBState = HB_UNKNOWN
				} else {
					// Heartbeat message
					monitoredNode.HBState = HB_ACTIVE
					// Reset timer
					monitoredNode.TimeoutTimer = 0
					timeDifferenceUsCopy = 0
				}
				monitoredNode.RxNew = false
			}
			// Check timeout
			if monitoredNode.HBState == HB_ACTIVE {
				monitoredNode.TimeoutTimer += timeDifferenceUsCopy
				if monitoredNode.TimeoutTimer >= monitoredNode.TimeUs {
					// Timeout is expired
					// TODO add emergency sending
					monitoredNode.NMTState = CO_NMT_UNKNOWN
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
			if monitoredNode.NMTState != CO_NMT_OPERATIONAL {
				allMonitoredOperationalCurrent = false
			}

			if monitoredNode.NMTState != monitoredNode.NMTStatePrev {
				monitoredNode.NMTStatePrev = monitoredNode.NMTState
				// TODO maybe add callbacks
			}
		}
	} else if nmtIsPreOrOperational || consumer.NMTisPreOrOperationalPrev {
		// pre or operational state changed, clear vars
		for _, monitoredNode := range consumer.MonitoredNodes {
			monitoredNode.NMTState = CO_NMT_UNKNOWN
			monitoredNode.NMTStatePrev = CO_NMT_UNKNOWN
			monitoredNode.RxNew = false
			if monitoredNode.HBState != HB_UNCONFIGURED {
				monitoredNode.HBState = HB_UNKNOWN
			}
		}
		allMonitoredActiveCurrent = false
		allMonitoredOperationalCurrent = false
	}

	// Clear emergencies when all monitored nodes become active
	// if !consumer.AllMonitoredActive && allMonitoredActiveCurrent {
	// 	// TODO send emergency frame
	// }
	consumer.AllMonitoredActive = allMonitoredActiveCurrent
	consumer.AllMonitoredOperational = allMonitoredOperationalCurrent
	consumer.NMTisPreOrOperationalPrev = nmtIsPreOrOperational
}
