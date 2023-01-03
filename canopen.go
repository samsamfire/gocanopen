package canopen

type Configuration struct{}

type Manager struct {
	Config *Configuration
}

/* This file contains the basic high level API */

/* Create a new canopen management object */
func NewManager(configuration *Configuration) *Manager {
	return &Manager{Config: configuration}
}

/* Process SRDO part */
func (manager *Manager) ProcessSRDO(time_difference_us uint32) (timer_next_us uint32) {
	// Process SRDO object
	return 0
}

/* Process TPDO */
func (manager *Manager) ProcessTPDO(sync_was bool, time_difference_us uint32) (timer_next_us uint32) {
	// Process TPDO object
	return 0
}

/* Process RPDO */
func (manager *Manager) ProcessRPDO(sync_was bool, time_difference_us uint32) (timer_next_us uint32) {
	// Process RPDO object
	return 0
}

/* Process SYNC */
func (manager *Manager) ProcessSYNC(time_difference_us uint32) (sync_was bool, timer_next_us uint32) {
	// Process SYNC object
	return false, 0
}

/* Process all objects */
func (manager *Manager) Process(enable_gateway bool, time_difference_us uint32) (timer_next_us uint32) {
	// Process all objects
	return 0
}

/*Initialize all PDOs*/
func (manager *Manager) InitPDO(emergency *EM, od *ObjectDictionary, node_id uint8) (result COResult) {
	//  TODO
	return 0
}

/*Initialize CANopen stack */
func (manager *Manager) Init(
	nmt *NMT,
	emergency *EM, od *ObjectDictionary,
	status_bits *Entry,
	nmt_control NMT_Control,
	first_hb_time_ms uint16,
	sdo_server_timeout_ms uint16,
	sdo_client_timeout_ms uint16,
	block_transfer bool,
	node_id uint8,

) COResult {
	// TODO
	return 0
}
