package kvaser

import canopen "github.com/samsamfire/gocanopen/v2"

// canReadStatus flags, from canlib.h
const (
	canSTAT_ERROR_PASSIVE = 0x00000001
	canSTAT_BUS_OFF       = 0x00000002
	canSTAT_ERROR_WARNING = 0x00000004
	canSTAT_ERROR_ACTIVE  = 0x00000008
	canSTAT_HW_OVERRUN    = 0x00000200
	canSTAT_SW_OVERRUN    = 0x00000400
)

// statusToState maps canReadStatus flags to a bus state. Several flags can be
// set at once (e.g. error active with frames pending): the most severe wins.
func statusToState(flags uint32) canopen.BusState {
	switch {
	case flags&canSTAT_BUS_OFF != 0:
		return canopen.BusStateOff
	case flags&canSTAT_ERROR_PASSIVE != 0:
		return canopen.BusStateErrorPassive
	case flags&canSTAT_ERROR_WARNING != 0:
		return canopen.BusStateErrorWarning
	case flags&(canSTAT_HW_OVERRUN|canSTAT_SW_OVERRUN) != 0:
		return canopen.BusStateHwOverflow
	case flags&canSTAT_ERROR_ACTIVE != 0:
		return canopen.BusStateErrorActive
	default:
		return canopen.BusStateUnknown
	}
}
