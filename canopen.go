package canopen

import log "github.com/sirupsen/logrus"

type Configuration struct{}

const (
	NMT_SERVICE_ID       uint16 = 0
	HEARTBEAT_SERVICE_ID uint16 = 0x700
)

// Node regroups all the different canopen objects and is responsible for processing each one of them
type Node struct {
	Config    *Configuration
	CANModule *CANModule
	NMT       *NMT
}

/* This file contains the basic high level API */

/* Create a new canopen management object */
func NewNode(configuration *Configuration) *Node {
	return &Node{Config: configuration}
}

/* Process SRDO part */
func (Node *Node) ProcessSRDO(time_difference_us uint32) (timer_next_us uint32) {
	// Process SRDO object
	return 0
}

/* Process TPDO */
func (Node *Node) ProcessTPDO(sync_was bool, time_difference_us uint32) (timer_next_us uint32) {
	// Process TPDO object
	return 0
}

/* Process RPDO */
func (Node *Node) ProcessRPDO(sync_was bool, time_difference_us uint32) (timer_next_us uint32) {
	// Process RPDO object
	return 0
}

/* Process SYNC */
func (Node *Node) ProcessSYNC(time_difference_us uint32) (sync_was bool, timer_next_us uint32) {
	// Process SYNC object
	return false, 0
}

/* Process all objects */
func (Node *Node) Process(enable_gateway bool, time_difference_us uint32, timer_next_us *uint32) uint8 {
	// Process all objects
	reset := CO_RESET_NOT
	NMTState := Node.NMT.GetInternalState()
	//NMTisPreOrOperational := (NMTState == CO_NMT_PRE_OPERATIONAL) || (NMTState == CO_NMT_OPERATIONAL)

	// CAN stuff to process
	Node.CANModule.Process()

	// For now, only process NMT heartbeat part
	reset = Node.NMT.Process(&NMTState, time_difference_us, timer_next_us)
	// Update NMTisPreOrOperational
	//NMTisPreOrOperational = (NMTState == CO_NMT_PRE_OPERATIONAL) || (NMTState == CO_NMT_OPERATIONAL)

	return reset

}

/*Initialize all PDOs*/
func (Node *Node) InitPDO(emergency *EM, od *ObjectDictionary, node_id uint8) (result COResult) {
	//  TODO
	return 0
}

/*Initialize CANopen stack */
func (Node *Node) Init(
	nmt *NMT,
	emergency *EM,
	od *ObjectDictionary,
	status_bits *Entry,
	nmt_control uint16,
	first_hb_time_ms uint16,
	sdo_server_timeout_ms uint16,
	sdo_client_timeout_ms uint16,
	block_transfer bool,
	node_id uint8,

) error {
	if nmt == nil {
		Node.NMT = &NMT{}
	} else {
		Node.NMT = nmt
	}
	// For now just NMT init
	// Get NMT obj 1017 :
	Entry1017 := od.Find(0x1017)
	if Entry1017 == nil {
		return CO_ERROR_OD_PARAMETERS
	}
	err := Node.NMT.Init(Entry1017, nil, node_id, nmt_control, first_hb_time_ms, Node.CANModule, NMT_SERVICE_ID, NMT_SERVICE_ID, HEARTBEAT_SERVICE_ID+uint16(node_id))
	if err != nil {
		log.Errorf("Error when initializing NMT object %v", err)
		return err
	}
	return nil
}
