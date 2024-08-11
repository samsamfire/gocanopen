package can

import canopen "github.com/samsamfire/gocanopen"

type NewInterfaceFunc func(channel string) (canopen.Bus, error)

var AvailableInterfaces = make(map[string]NewInterfaceFunc)
var ImplementedInterfaces = []string{
	"socketcan",
	"socketcanv2",
	"virtualcan",
	"kvaser",
}

// Register a new CAN bus interface type
// This should be called inside an init() function of plugin
func RegisterInterface(interfaceType string, newInterface NewInterfaceFunc) {
	AvailableInterfaces[interfaceType] = newInterface
}
