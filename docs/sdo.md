# SDO (Service Data Object)

The SDO protocol is used for direct access to a device's Object Dictionary (OD).
It supports reading (Upload) and writing (Download) of values.
All CiA 301 SDO transfer modes are supported:

- Expedited transfers
- Segmented transfers
- Block transfers

## SDO Client (Accessing Remote Nodes)

To communicate with another node on the network (acting as an SDO Client), you typically use a `RemoteNode` or the `Network` raw API.

### Using RemoteNode

If you have a `RemoteNode` instance (e.g., from `network.AddRemoteNode`), you can use its helper methods to read entries.

```go
// Create a remote node, with id 6 and load the object dictionary from given file
node := network.AddRemoteNode(6, "/path/to/object_dictionary.eds")

// Read a value (Index 0x2001, SubIndex 0)
data, err := node.Read(0x2001, 0)
if err != nil {
    // Handle SDO Abort or Timeout
    log.Printf("SDO Read failed: %v", err)
}
```

### Using Network Raw API

You can also send SDO requests directly via the Network object without explicitly managing a RemoteNode.

```go
// Read Raw: Node 0x10, Index 0x2000, SubIndex 0
buffer := make([]byte, 8)
nbRead, err := network.ReadRaw(0x10, 0x2000, 0, buffer)

// Write Raw: Node 0x10, Index 0x2000, SubIndex 0
value := uint16(600)
// false = disable block transfer (use expedited/segmented)
nbWritten, err := network.WriteRaw(0x10, 0x2000, 0, value, false) 
```

## SDO Server (Local Node)

If you are running a `LocalNode`, it automatically acts as an SDO Server.
To interact with the Local Node's OD (which is what SDO clients access), see the **[Local Node Documentation](local.md)**.
