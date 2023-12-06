package canopen

import "errors"

var (
	ErrIllegalArgument       = errors.New("error in function arguments")
	ErrOutOfMemory           = errors.New("memory allocation failed")
	ErrTimeout               = errors.New("function timeout")
	ErrIllegalBaudrate       = errors.New("illegal baudrate passed to function")
	ErrRxOverflow            = errors.New("previous message was not processed yet")
	ErrRxPdoOverflow         = errors.New("previous PDO was not processed yet")
	ErrRxMsgLength           = errors.New("wrong receive message length")
	ErrRxPdoLength           = errors.New("wrong receive PDO length")
	ErrTxOverflow            = errors.New("previous message is still waiting, buffer full")
	ErrTxPdoWindow           = errors.New("synchronous TPDO is outside window")
	ErrTxUnconfigured        = errors.New("transmit buffer was not configured properly")
	ErrOdParameters          = errors.New("error in Object Dictionary parameters")
	ErrDataCorrupt           = errors.New("stored data are corrupt")
	ErrCRC                   = errors.New("crc does not match")
	ErrTxBusy                = errors.New("sending rejected because driver is busy. Try again")
	ErrWrongNMTState         = errors.New("command can't be processed in the current state")
	ErrSyscall               = errors.New("syscall failed")
	ErrInvalidState          = errors.New("driver not ready")
	ErrNodeIdUnconfiguredLSS = errors.New("node-id is in LSS unconfigured state. If objects are handled properly, this may not be an error")
)
