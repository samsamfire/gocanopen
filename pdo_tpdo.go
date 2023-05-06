package canopen

type TPDO struct {
	PDO              PDOCommon
	TxBuffer         *BufferTxFrame
	TransmissionType uint8
	SendRequest      bool
	Sync             *SYNC
	SyncStartValue   uint8
	SyncCounter      uint8
	InhibitTimeUs    uint32
	EventTimeUs      uint32
	InhibitTimer     uint32
	EventTimer       uint32
}
