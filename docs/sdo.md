# SDO

The SDO protocol can be used to read or write values to a CANopen node.
All the SDO features of the CiA 301 are supported. This includes :
- expedited transfers
- segmented transfers
- block transfers

Object dictionary entries can be read or written to either by name or by index and sub-index.

Here is a more complete example:

```go
import (
    "fmt"
    "log"
    "github.com/samsamfire/gocanopen/pkg/network"
)

// ...

// Create a remote node, with id 6 and load the object dictionary from given file
node, err := network.AddRemoteNode(6, "/path/to/object_dictionary.eds")
if err != nil {
    log.Fatal(err)
}

// Read an UNSIGNED32 value from the object dictionary
val, err := node.ReadUint("UNSIGNED32 value", "")
if err != nil {
    log.Printf("failed to read value: %v", err)
} else {
    fmt.Printf("Read value: %d\n", val)
}

// Write a string value to the object dictionary
err = node.WriteAnyExact("STRING value", "", "hello world")
if err != nil {
    log.Printf("failed to write value: %v", err)
}

// You can also use index and sub-index directly
// Read a value from index 0x2001, sub-index 0
rawData, err := node.Read(0x2001, 0)
if err != nil {
    log.Printf("failed to read raw value: %v", err)
} else {
    fmt.Printf("Read raw value: %x\n", rawData)
}

```