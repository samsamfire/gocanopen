package canopen

import "errors"

var (
	ErrIllegalArgument       = errors.New("Error in function arguments")
	ErrOutOfMemory           = errors.New("Memory allocation failed")
	ErrTimeout               = errors.New("Function timeout")
	ErrIllegalBaudrate       = errors.New("Illegal baudrate passed to function")
	ErrRxOverflow            = errors.New("Previous message was not processed yet")
	ErrRxPdoOverflow         = errors.New("Previous PDO was not processed yet")
	ErrRxMsgLength           = errors.New("Wrong receive message length")
	ErrRxPdoLength           = errors.New("Wrong receive PDO length")
	ErrTxOverflow            = errors.New("Previous message is still waiting, buffer full")
	ErrTxPdoWindow           = errors.New("Synchronous TPDO is outside window")
	ErrTxUnconfigured        = errors.New("Transmit buffer was not configured properly")
	ErrOdParameters          = errors.New("Error in Object Dictionary parameters")
	ErrDataCorrupt           = errors.New("Stored data are corrupt")
	ErrCRC                   = errors.New("CRC does not match")
	ErrTxBusy                = errors.New("Sending rejected because driver is busy. Try again")
	ErrWrongNMTState         = errors.New("Command can't be processed in the current state")
	ErrSyscall               = errors.New("Syscall failed")
	ErrInvalidState          = errors.New("Driver not ready")
	ErrNodeIdUnconfiguredLSS = errors.New("Node-id is in LSS unconfigured state. If objects are handled properly, this may not be an error.")
)
