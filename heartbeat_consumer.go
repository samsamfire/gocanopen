package canopen

import (
	"github.com/brutella/can"
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
}

type HBConsumer struct {
	em                        *EM
	MonitoredNodes            []*HBConsumerNode
	NbMonitoredNodes          uint8
	AllMonitoredActive        bool
	AllMonitoredOperational   bool
	NMTisPreOrOperationalPrev bool
	canmodule                 *CANModule
	ExtensionEntry1016        Extension
}

// Handle hearbeat reception specific to a node
func (node_consumer *HBConsumerNode) Handle(frame can.Frame) {
	if frame.Length != 8 {
		return
	}
	node_consumer.NMTState = int(frame.Data[0])
	node_consumer.RxNew = true

}

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

}

// /* configure Heartbeat consumer (or disable) CAN reception */
// ret = CO_CANrxBufferInit(HBcons->CANdevRx,
// HBcons->CANdevRxIdxStart + idx,
// COB_ID,
// 0x7FF,
// 0,
// (void*)&HBcons->monitoredNodes[idx],
// CO_HBcons_receive);
// }
// return ret;
// }
