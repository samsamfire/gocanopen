# Network

The **Network** object is used for managing the CAN network & stack. It holds CANopen **Nodes** which can be of two types :
Either a **LocalNode**, which represents a real CANopen CiA 301 compliant node or a **RemoteNode** which is the 
local representation of a remote CANopen node on the CAN bus used for master control.

The network already embeds some CANopen functionalities such as :
- SDO Client
- NMT Master

SDO client can be used by running :

```golang
// Read node id 0x10, index 0x2000, subindex 0x0 inside buffer
// No decoding is done
nbRead,err := network.ReadRaw(0x10,0x2000,0,buffer)

// Write node id 0x10, index 0x2000, subindex 0x0
// value can be any CANopen standard type
value := uint16(600)
nbRead,err := network.WriteRaw(0x10,0x2000,0,value,false)
```

NMT Master :

```golang
// RESET all nodes on the network
network.Command(0,nmt.CommandResetNode)

// RESET node id 0x10
network.Command(0x10,nmt.CommandResetNode)
```

A network scan of all the available devices can be performed. This will send 
an SDO request to all the nodes and wait for a reply. This expects the identity
object to exist on the remote node (in conformance with CiA standard).

```golang
// Scan for existing nodes on the bus, if no response is received after 1 second
// It is considered as non existent
timeoutMs := 1000
devices,err := network.Scan(1000)
```

# Remote node

A remote node can be used to control another node on the CAN bus.
To create a remote node, several options are possible :
- By providing an object dictionary (either EDS file, or ObjectDictionary object)
- By downloading the EDS from the remote node (SDO upload). In that case, two formats
are currently supported by default : ASCII and ZIP.


```golang
// Add a remote node, with id 6 and load the object dictionary from given file
node := network.AddRemoteNode(6, "/path/to/object_dictionary.eds")

// First download EDS file. An optional callback can be provided for manufacturer specific parsing
// Then add remote node
odict,_ := network.ReadEDS(6, nil)
node := network.AddRemoteNode(6, odict)

```

# Local node

A local node is a fully functional CANopen node as specified by CiA 301 standard.
A node can be created with the following commands:

```golang
// Create & start a local node with the default OD in the libraray
node,err := network.CreateLocalNode(0x10,od.Default())
```

More information on local nodes [here](local.md)