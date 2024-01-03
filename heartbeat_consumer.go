package canopen

import (
	log "github.com/sirupsen/logrus"
)

const (
	HB_UNCONFIGURED = 0x00 // Consumer entry inactive
	HB_UNKNOWN      = 0x01 // Consumer enabled, but no heartbeat received yet
	HB_ACTIVE       = 0x02 // Heartbeat received within set time
	HB_TIMEOUT      = 0x03 // No heatbeat received for set time
)

// Node specific hearbeat consumer part
type hbConsumerNode struct {
	nodeId       uint8
	cobId        uint16
	nmtState     uint8
	nmtStatePrev uint8
	hbState      uint8
	timeoutTimer uint32
	timeUs       uint32
	rxNew        bool
}

// Hearbeat consumer object for monitoring node hearbeats
type HBConsumer struct {
	*busManager
	emcy                      *EMCY
	monitoredNodes            []*hbConsumerNode
	nbMonitoredNodes          uint8
	allMonitoredActive        bool
	allMonitoredOperational   bool
	nmtIsPreOrOperationalPrev bool
}

// Handle hearbeat reception specific to a node
func (nodeConsumer *hbConsumerNode) Handle(frame Frame) {
	if frame.DLC != 1 {
		return
	}
	nodeConsumer.nmtState = frame.Data[0]
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
			if int(index) != i && consumerNode.timeUs != 0 && consumerNode.nodeId == nodeId {
				return ErrIllegalArgument
			}
		}
	}
	consumerNode := newHbConsumerNode(index, nodeId, consumerTimeMs)

	// Configure RX buffer for hearbeat reception
	if consumerNode.hbState != HB_UNCONFIGURED {
		log.Debugf("[HB CONSUMER] will monitor x%x | timeout %v us", consumerNode.nodeId, consumerNode.timeUs)
		consumer.Subscribe(uint32(consumerNode.cobId), 0x7FF, false, consumerNode)
	}
	consumer.monitoredNodes[index] = consumerNode
	return nil
}

// process Hearbeat consuming
func (consumer *HBConsumer) process(nmtIsPreOrOperational bool, timeDifferenceUs uint32, timerNextUs *uint32) {
	allMonitoredActiveCurrent := true
	allMonitoredOperationalCurrent := true
	if nmtIsPreOrOperational && consumer.nmtIsPreOrOperationalPrev {
		for i := range consumer.monitoredNodes {
			monitoredNode := consumer.monitoredNodes[i]
			timeDifferenceUsCopy := timeDifferenceUs
			// If unconfigured skip to next iteration
			if monitoredNode.hbState == HB_UNCONFIGURED {
				continue
			}
			if monitoredNode.rxNew {
				if monitoredNode.nmtState == NMT_INITIALIZING {
					// Boot up message is an error if previously received (means reboot)
					if monitoredNode.hbState == HB_ACTIVE {
						consumer.emcy.ErrorReport(emHBConsumerRemoteReset, emErrHeartbeat, uint32(i))
					}
					monitoredNode.hbState = HB_UNKNOWN
				} else {
					// Heartbeat message
					monitoredNode.hbState = HB_ACTIVE
					monitoredNode.timeoutTimer = 0
					timeDifferenceUsCopy = 0
				}
				monitoredNode.rxNew = false
			}
			// Check timeout
			if monitoredNode.hbState == HB_ACTIVE {
				monitoredNode.timeoutTimer += timeDifferenceUsCopy
				if monitoredNode.timeoutTimer >= monitoredNode.timeUs {
					// Timeout is expired
					consumer.emcy.ErrorReport(emHBConsumerRemoteReset, emErrHeartbeat, uint32(i))
					monitoredNode.nmtState = NMT_UNKNOWN
					monitoredNode.hbState = HB_TIMEOUT
				} else if timerNextUs != nil {
					// Calculate when to recheck
					diff := monitoredNode.timeUs - monitoredNode.timeoutTimer
					if *timerNextUs > diff {
						*timerNextUs = diff
					}
				}
			}
			if monitoredNode.hbState != HB_ACTIVE {
				allMonitoredActiveCurrent = false
			}
			if monitoredNode.nmtState != NMT_OPERATIONAL {
				allMonitoredOperationalCurrent = false
			}

			if monitoredNode.nmtState != monitoredNode.nmtStatePrev {
				monitoredNode.nmtStatePrev = monitoredNode.nmtState
			}
		}
	} else if nmtIsPreOrOperational || consumer.nmtIsPreOrOperationalPrev {
		// pre or operational state changed, clear vars
		for i := range consumer.monitoredNodes {
			monitoredNode := consumer.monitoredNodes[i]
			monitoredNode.nmtState = NMT_UNKNOWN
			monitoredNode.nmtStatePrev = NMT_UNKNOWN
			monitoredNode.rxNew = false
			if monitoredNode.hbState != HB_UNCONFIGURED {
				monitoredNode.hbState = HB_UNKNOWN
			}
		}
		allMonitoredActiveCurrent = false
		allMonitoredOperationalCurrent = false
	}

	// Clear emergencies when all monitored nodes become active
	if !consumer.allMonitoredActive && allMonitoredActiveCurrent {
		consumer.emcy.ErrorReset(emHeartbeatConsumer, 0)
		consumer.emcy.ErrorReset(emHBConsumerRemoteReset, 0)
	}
	consumer.allMonitoredActive = allMonitoredActiveCurrent
	consumer.allMonitoredOperational = allMonitoredOperationalCurrent
	consumer.nmtIsPreOrOperationalPrev = nmtIsPreOrOperational
}

// Initialize a single node consumer
func newHbConsumerNode(index uint8, nodeId uint8, consumerTimeMs uint16) *hbConsumerNode {

	monitoredNode := &hbConsumerNode{}
	monitoredNode.nodeId = nodeId
	monitoredNode.timeUs = uint32(consumerTimeMs) * 1000
	monitoredNode.nmtState = NMT_UNKNOWN
	monitoredNode.nmtStatePrev = NMT_UNKNOWN
	monitoredNode.rxNew = false

	if monitoredNode.nodeId != 0 && monitoredNode.timeUs != 0 {
		monitoredNode.cobId = uint16(monitoredNode.nodeId) + HEARTBEAT_SERVICE_ID
		monitoredNode.hbState = HB_UNKNOWN
	} else {
		monitoredNode.cobId = 0
		monitoredNode.timeUs = 0
		monitoredNode.hbState = HB_UNCONFIGURED
	}
	return monitoredNode
}

// Initialize hearbeat consumer
func NewHBConsumer(bm *busManager, em *EMCY, entry1016 *Entry) (*HBConsumer, error) {

	if entry1016 == nil || bm == nil || em == nil {
		return nil, ErrIllegalArgument
	}
	consumer := &HBConsumer{busManager: bm}
	consumer.emcy = em

	// Get real number of monitored nodes
	consumer.nbMonitoredNodes = uint8(entry1016.SubCount() - 1)
	log.Debugf("[HB CONSUMER] %v possible entries for nodes to monitor", consumer.nbMonitoredNodes)
	consumer.monitoredNodes = make([]*hbConsumerNode, consumer.nbMonitoredNodes)
	for index := 0; index < int(consumer.nbMonitoredNodes); index++ {
		hbConsValue, err := entry1016.Uint32(uint8(index) + 1)
		if err != nil {
			log.Errorf("[HB CONSUMER][%x|%x] reading %v failed : %v", entry1016.Index, index+1, entry1016.Name, err)
			return nil, ErrOdParameters
		}
		nodeId := uint8(hbConsValue >> 16)
		time := uint16(hbConsValue & 0xFFFF)
		// Set the buffer index before initializing
		err = consumer.addHearbeatConsumerNode(uint8(index), nodeId, time)
		if err != nil {
			return nil, err
		}
	}
	entry1016.AddExtension(consumer, ReadEntryDefault, writeEntry1016)
	return consumer, nil

}

type HBConfigurator struct {
	nodeId    uint8
	sdoClient *SDOClient
}

// Read current monitored nodes
// Returns a list of all the entries composed as the id of the monitored node
// And the expected period in ms
func (config *HBConfigurator) ReadMonitoredNodes() ([][]uint16, error) {
	nbMonitored, err := config.ReadMaxMonitorable()
	if err != nil {
		return nil, err
	}
	monitored := make([][]uint16, 0)
	for i := uint8(1); i <= nbMonitored; i++ {
		periodAndId, err := config.sdoClient.ReadUint32(config.nodeId, 0x1016, i)
		if err != nil {
			return monitored, err
		}
		nodeId := uint16((periodAndId >> 16) & 0xFF)
		period := uint16(periodAndId)
		monitored = append(monitored, []uint16{nodeId, period})
	}
	return monitored, nil
}

// Read max available entries for monitoring
func (config *HBConfigurator) ReadMaxMonitorable() (uint8, error) {
	nbMonitored, err := config.sdoClient.ReadUint8(config.nodeId, 0x1016, 0x0)
	if err != nil {
		return 0, err
	}
	return nbMonitored, nil
}

// Add or update a node to monitor with the expected heartbeat period
// Index needs to be between 1 & the max nodes that can be monitored
func (config *HBConfigurator) WriteMonitoredNode(index uint8, nodeId uint8, periodMs uint16) error {
	periodAndId := uint32(nodeId)<<16 + uint32(periodMs&0xFFFF)
	return config.sdoClient.WriteRaw(config.nodeId, 0x1016, index, periodAndId, false)
}

func NewHBConfigurator(nodeId uint8, sdoClient *SDOClient) *HBConfigurator {
	return &HBConfigurator{nodeId: nodeId, sdoClient: sdoClient}
}
