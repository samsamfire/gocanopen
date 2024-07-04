# ObjectDictionary

Every CANopen node has an **ObjectDictionary**. An ObjectDictionary consists of **Entries** with a given index
between 0 and 0xFFFF. An entry may also have a subindex for some specific CANopen types like RECORD or ARRAY types.
This subindex must be between 0 and 0xFF.
All of this information is stored inside of an EDS file as defined by CiA.
This library can parse a standard EDS file and create the corresponding CANopen objects (SDO, NMT, etc...).

## Basic Usage

To create an **ObjectDictionary** directly :

```go
import "github.com/samsamfire/gocanopen/pkg/od"

odict := od.Parse("../testdata/base.eds", 0x20) // parse EDS for node id 0x20
```
The node id is required but is only useful when the EDS uses the special variable $NODE_ID.
Usually, the ObjectDictionary is created when adding / creating a node on the network.
Accessing OD entries and subentries directly is possible either by name or by value.

```go
odict := od.Parse("../testdata/base.eds", 0x20)
odict.Index(0x201B) // get entry index by value
odict.Index("UNSIGNED64 value") // accessing with the actual name is also possible
odict.Index(0x201B).SubIndex(0) // returns the associated Variable object (for VAR types, subindex is always 0)
odict.Index(0x1018).SubIndex(1) // access sub-object of array
```

It is also possible to create new dictionary entries dynamically :

```go
odict.AddVariableType(index, indexName, od.DOMAIN, od.AttributeSdoRw, "") // add a DOMAIN entry, and returns it
odict.AddVariableType(0x2500, "a number", od.UNSIGNED32, od.AttributeSdoRw, "0x1000") // add an UNSIGNED32 entry, readable and writable
record := od.NewRecord()
record.AddSubObject(0, "sub0", od.UNSIGNED8, od.AttributeSdoRw, "0x11")
odict.AddVariableList(0x3030, "record", record) // add a RECORD entry
```

Some more complex objects can be created dynamically, currently only a few are supported :

```go
odict := od.Parse("../testdata/base.eds", 0x20)
odict.AddRPDO(1) // adds an rpdo object to EDS. i.e. new communication param at 0x1400 and mapping param at 0x1600
odict.AddTPDO(1) // adds a tpdo object to EDS. i.e. new communication param at 0x1800 and mapping param at 0x1A00
odict.AddSYNC() // adds sync object as well as extensions (1005,1006,1007,1019)
```
Note that currently adding these objects will not update the underlying EDS file on the system, meaning that
downloading EDS through object 0x1021 for example will still return the original EDS file.

A default CANopen EDS is embedded inside of this package. It can be useful for testing purposes, and can be used
like so :

```go
odict := od.Default() // this creates a default object dictionary with pre-configured values
```

## Special entries

CiA 301 defines a certain number of CANopen communication specific objects inside the object dictionary. 
These objects are inside of the **Communication Profile Area** and range between 0x1000 - 0x1FFF. 
Some of them are mandatory and others are optional.
The following table lists the available objects, and the ones that are currently implemented in this stack.

| Index | Name                          | Implemented |
|-------|-------------------------------|-------------|
| 1000  | device type                   | yes         |
| 1001  | error register                | yes         |
| 1003  | manufacturer status register  | yes         |
| 1005  | COB-ID SYNC                   | yes         |
| 1006  | communication cycle period    | yes         |
| 1007  | synchronous window length     | yes         |
| 1008  | manufacturer device name      | yes         |
| 1009  | manufacturer hardware version | yes         |
| 100A  | manufacturer software version | yes         |
| 100C  | gard time                     | no          |
| 100D  | life time factor              | no          |
| 1010  | store parameters              | no          |
| 1011  | restore default parameters    | no          |
| 1012  | COB-ID TIME                   | yes         |
| 1013  | high resolution time stamp    | yes         |
| 1014  | COB-ID EMCY                   | yes         |
| 1015  | Inhibit Time EMCY             | yes         |
| 1016  | Consumer heartbeat time       | yes         |
| 1017  | Producer heartbeat time       | yes         |
| 1018  | Identity Object               | yes         |
| 1021  | Store EDS                     | yes         |
| 1022  | Storage Format                | yes         |

These entries are different than regular OD entries as they use special extensions
that can perform various operations on the running CANopen node.
You can define your own CANopen extensions for this, you need to create two functions :

- An **od.StreamReader** that will be called on entry read access
- An **od.StreamWriter** that will be called on entry write access

The default implementations, i.e. for regular reading and writing are **od.ReadEntryDefault** and
**od.WriteEntryDefault**.

```go
odict := od.Parse("../testdata/base.eds", 0x20)
entry := odict.AddVariableType(index, indexName, od.DOMAIN, od.AttributeSdoRw, "")
entry.AddExtension(someObject,od.ReadEntryDefault,od.WriteEntryDefault)
```

Some pre-made extensions are available :
```go
odict := od.Parse("../testdata/base.eds", 0x20)
// this will create a file on disk that will be accessible by SDO block transfer
entry := odict.AddFile(0x3333, "File entry", "./path_to_file.txt", os.O_RDWR|os.O_CREATE, os.O_RDWR|os.O_CREATE)
```

Some helper methods are also available for reading or configuring these objects. The
following are non-exhaustive examples :

```go
config := net.Configurator(0x20) // create a NodeConfigurator object for node 0x20
config.HB.WriteHeartbeatPeriod(500) // update heartbeat period of node 0x20 to 500ms
config.SYNC.ProducerDisable() // disable sync transmission (if this node is the one sending the SYNC)
mappings, err := config.PDO.ReadMappings(1) // read pdo mapping parameter of 1st RPDO
config, err := config.PDO.ReadConfiguration(1) // read pdo configuration parameter of 1st RPDO
```