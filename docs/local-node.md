# Local Node

A local node is a fully functional CANopen node as specified by CiA 301 standard.
A node can be created with the following commands:

```golang
import (
    "log"
    "github.com/samsamfire/gocanopen/pkg/network"
    "github.com/samsamfire/gocanopen/pkg/od"
)

// ...

// Create & start a local node with the default OD in the libraray
node, err := network.CreateLocalNode(0x10, od.Default())
if err != nil {
    log.Fatal(err)
}
```

## Custom OD parsing

The network can be configured to use a different OD parser
when creating local nodes.

```golang
import (
    "github.com/samsamfire/gocanopen/pkg/network"
    "github.com/samsamfire/gocanopen/pkg/od"
)

// ...

// Change default OD parser
network.SetParseV2(od.ParserV2)
```

## Custom node processing

Nodes can also be added to network and controlled locally
with a **NodeProcessor**. e.g.


```golang
import (
    "context"
    "log"
    "log/slog"
    "github.com/samsamfire/gocanopen/pkg/network"
    "github.com/samsamfire/gocanopen/pkg/nmt"
    "github.com/samsamfire/gocanopen/pkg/sdo"
)

// ...

// Create a local node
node, err := network.NewLocalNode(
		network.BusManager,
		slog.Default(),
		odNode, // OD object ==> Should be created
		nil, // Use definition from OD
		nil, // Use definition from OD
		nodeId,
		nmt.StartupToOperational,
		500,
		sdo.DefaultClientTimeout,
		sdo.DefaultServerTimeout,
		true,
		nil,
	)
if err != nil {
    log.Fatal(err)
}


// Add a custom node to network and control it independently
proc, err := network.AddNode(node)
if err != nil {
    log.Fatal(err)
}


// Start node processing
err = proc.Start(context.Background())
if err != nil {
    log.Fatal(err)
}
```

More information on local nodes [here](local.md)
