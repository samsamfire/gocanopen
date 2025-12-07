# Network

The **Network** object is used for managing the CAN network & stack. It holds CANopen **Nodes** which can be of two types:
a **[LocalNode](local-node.md)**, which represents a real CANopen CiA 301 compliant node, or a **[RemoteNode](remote-node.md)**, which is the 
local representation of a remote CANopen node on the CAN bus used for master control.

The network already embeds some CANopen functionalities such as :
- SDO Client
- NMT Master

SDO client can be used by running :

```golang
import (
    "log"
    "github.com/samsamfire/gocanopen/pkg/network"
)

// ...

// Read node id 0x10, index 0x2000, subindex 0x0 inside buffer
// No decoding is done
nbRead, err := network.ReadRaw(0x10, 0x2000, 0, buffer)
if err != nil {
    log.Fatal(err)
}


// Write node id 0x10, index 0x2000, subindex 0x0
// value can be any CANopen standard type
value := uint16(600)
nbRead, err = network.WriteRaw(0x10, 0x2000, 0, value, false)
if err != nil {
    log.Fatal(err)
}
```

NMT Master :

```golang
import (
    "github.com/samsamfire/gocanopen/pkg/nmt"
    "github.com/samsamfire/gocanopen/pkg/network"
)

// ...

// RESET all nodes on the network
network.Command(0, nmt.CommandResetNode)

// RESET node id 0x10
network.Command(0x10, nmt.CommandResetNode)
```

A network scan of all the available devices can be performed. This will send 
an SDO request to all the nodes and wait for a reply. This expects the identity
object to exist on the remote node (in conformance with CiA standard).

```golang
import (
    "log"
    "github.com/samsamfire/gocanopen/pkg/network"
)

// ...

// Scan for existing nodes on the bus, if no response is received after 1 second
// It is considered as non existent
timeoutMs := 1000
devices, err := network.Scan(1000)
if err != nil {
    log.Fatal(err)
}
```

For more information, see:
- [Local Node](local-node.md)
- [Remote Node](remote-node.md)