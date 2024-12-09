package http

import (
	"strconv"
)

const TOKEN_NONE = -3
const TOKEN_DEFAULT = -2
const TOKEN_ALL = -1

// Gets SDO command as list of strings and processes it
func parseSdoCommand(command []string) (index uint64, subindex uint64, err error) {
	if len(command) != 3 {
		return 0, 0, ErrGwSyntaxError
	}
	indexStr := command[1]
	subIndexStr := command[2]
	// Unclear if this is "supported" not really specified in 309-5
	if indexStr == "all" {
		return 0, 0, ErrGwRequestNotSupported
	}
	index, e := strconv.ParseUint(indexStr, 0, 64)
	if e != nil {
		return 0, 0, ErrGwSyntaxError
	}
	subIndex, e := strconv.ParseUint(subIndexStr, 0, 64)
	if e != nil {
		return 0, 0, ErrGwSyntaxError
	}
	if index > 0xFFFF || subindex > 0xFF {
		return 0, 0, ErrGwSyntaxError
	}
	return index, subIndex, nil
}

// Parse raw network / node string param
func parseNodeOrNetworkParam(param string) (int, error) {
	// Check if any of the string values
	switch param {
	case "default":
		return TOKEN_DEFAULT, nil
	case "none":
		return TOKEN_NONE, nil
	case "all":
		return TOKEN_ALL, nil
	}
	// Else try a specific id
	// This automatically treats 0x,0X,... correctly
	// which is allowed in the spec
	paramUint, err := strconv.ParseUint(param, 0, 64)
	if err != nil {
		return 0, err
	}
	return int(paramUint), nil
}
