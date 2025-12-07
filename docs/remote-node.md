# Remote Node

A remote node can be used to control another node on the CAN bus.
To create a remote node, several options are possible :
- By providing an object dictionary (either EDS file, or ObjectDictionary object)
- By downloading the EDS from the remote node (SDO upload). In that case, two formats
are currently supported by default : ASCII and ZIP.


```golang
import (
    "log"
    "github.com/samsamfire/gocanopen/pkg/network"
)

// ...

// Add a remote node, with id 6 and load the object dictionary from given file
node, err := network.AddRemoteNode(6, "/path/to/object_dictionary.eds")
if err != nil {
    log.Fatal(err)
}


// First download EDS file. An optional callback can be provided for manufacturer specific parsing
// Then add remote node
odict, err := network.ReadEDS(6, nil)
if err != nil {
    log.Fatal(err)
}
node, err = network.AddRemoteNode(6, odict)
if err != nil {
    log.Fatal(err)
}

```
