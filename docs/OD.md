# ObjectDictionary

Every CANopen node has an **ObjectDictionary**. An ObjectDictionary consists of **Entries** with a given index
between 0 and 0xFFFF. An entry may also have a subindex for some specific CANopen types like RECORD or ARRAY types.
This subindex must be between 0 and 0xFF.
All of this information is stored inside of an EDS file as defined by CiA.
This library can parse a standard EDS file and create the corresponding CANopen objects (SDO, NMT, etc...).

## Usage

To create an **ObjectDictionary** directly :

```golang
od := ParseEDSFromFile("../testdata/base.eds")
```

Usually, the ObjectDictionary is created when adding / creating a node on the network.
Accessing OD entries directly is possible :

```golang
od := ParseEDSFromFile("../testdata/base.eds")
od.Index(0x201B) // returns the associated OD Entry
od.Index("UNSIGNED64 value") // accessing with the actual name is also possible
od.Index(0x201B).SubIndex(0) // returns the associated Variable (for VAR types, subindex is always 0)
```

It is also possible to create new dictionary entries dynamically. This is currently not documented, refer to go doc.