# SDO

The SDO protocol can be used to read or write values to a CANopen node.
All the SDO features of the CiA 301 are supported. This includes :
- expedited transfers
- segmented transfers
- block transfers

Object dictionary entries can be read or written to either by name or by index and sub-index.

```go
// Create a remote node, with id 6 and load the object dictionary from given file
node := network.AddRemoteNode(6, "/path/to/object_dictionary.eds")
node.Read(0x2001,0)
```