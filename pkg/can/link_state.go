package can

import (
	"errors"
	"fmt"
	"net"

	canopen "github.com/samsamfire/gocanopen/v2"
)

// ErrLinkDown is returned by LinkState for an interface that is down.
var ErrLinkDown = errors.New("can interface is down")

// LinkState returns the state of a network CAN interface from its flags only:
//   - down or missing (e.g. an unplugged USB adapter): disconnected
//   - up without carrier: bus-off, the kernel drops the carrier of a CAN
//     interface when its controller leaves the bus
//   - up with carrier: error active, whatever the controller error counters,
//     so error warning and error passive are not reported
func LinkState(ifname string) (canopen.BusState, error) {
	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		return canopen.BusStateDisconnected, fmt.Errorf("find can interface %s: %w", ifname, err)
	}
	if iface.Flags&net.FlagUp == 0 {
		return canopen.BusStateDisconnected, ErrLinkDown
	}
	if iface.Flags&net.FlagRunning == 0 {
		return canopen.BusStateOff, nil
	}
	return canopen.BusStateErrorActive, nil
}
