package can

import canopen "github.com/samsamfire/gocanopen"

type NewInterfaceFunc func(channel string) (canopen.Bus, error)

var CanInterfaces = make(map[string]NewInterfaceFunc)

// Register a new CAN bus interface type
// This should be called inside an init() function of plugin
func RegisterInterface(interfaceType string, newInterface NewInterfaceFunc) {
	CanInterfaces[interfaceType] = newInterface
}
