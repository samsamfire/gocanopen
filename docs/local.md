# Creating a LocalNode (slave node)

A **LocalNode** is the CiA 301 implementation of a CANopen node in golang.
It has its own CANopen stack, which is run within a goroutine.
When creating a local node, the library parses the EDS file and creates the relevant
CANopen objects (including the Object Dictionary). This differs from other implementations that usually require a pre-build step that generates .c/.h files.

## Usage

To create and start a CANopen node:

```go
package main

import (
	"log"
	"github.com/samsamfire/gocanopen/pkg/network"
	"github.com/samsamfire/gocanopen/pkg/od"
)

func main() {
	// 1. Initialize Network

net := network.NewNetwork(nil)
	if err := net.Connect("socketcan", "can0", 500000); err != nil {
		log.Fatal(err)
	}
	defer net.Disconnect()

	// 2. Create a LocalNode (Slave) with ID 0x10
	// Using default embedded EDS
	node, err := net.CreateLocalNode(0x10, od.Default())
	if err != nil {
		log.Fatal(err)
	}

	// Alternatively, load from an EDS file:
	// node2, err := net.CreateLocalNode(0x11, "path/to/slave.eds")
	
	// The node automatically enters NMT state OPERATIONAL
	select {} // Keep running
}
```

## Local OD Access (SDO Interface)

The `LocalNode` provides a direct API to interact with its internal Object Dictionary (OD). This is equivalent to performing SDO operations locally.
You can **Read** and **Write** entries using their **Index/SubIndex** or their **Name** (as defined in the EDS).

### Reading Values

The API provides typed helpers to read values. These helpers handle type casting where possible to simplify usage.

**Read String**

```go
// Read by Name
deviceName, err := node.ReadString("Manufacturer device name", 0)

// Read by Index/SubIndex (0x1008:00 is Device Name)
deviceName, err := node.ReadString(0x1008, 0x0)
```

**Read Integers (Uint/Int)**
The `ReadUint` and `ReadInt` methods automatically cast the underlying OD value to `uint64` or `int64`.

```go
// Read an UNSIGNED8 entry (0x1001 Error Register)
// Returns uint64
errReg, err := node.ReadUint(0x1001, 0) 

// Read using Entry Name (assuming "SomeParam" exists)
// Returns int64
val, err := node.ReadInt("SomeParam", 0) 
```

**Read Floats**
The `ReadFloat`, `ReadFloat32`, and `ReadFloat64` methods are available for REAL types.

```go
// Read a REAL32 (float32) entry
// Returns float64 (cast from float32)
val, err := node.ReadFloat(0x2001, 0)

// Read REAL32 specifically as float32
f32Val, err := node.ReadFloat32("MyFloat32Param", 0)

// Read REAL64 specifically as float64
f64Val, err := node.ReadFloat64("MyFloat64Param", 0)
```

### Writing Values

To write values to the OD, you can use `WriteAnyExact`. This method writes the provided value to the specified entry.
The value's type must match the OD entry's expected type (or be compatible).

```go
// Write a uint32 value to Index 0x2000, SubIndex 0
err := node.WriteAnyExact(0x2000, 0, uint32(12345))

// Write a string
err := node.WriteAnyExact(0x1008, 0, "My Custom Device")

// Write using Name
err := node.WriteAnyExact("Producer heartbeat time", 0, uint16(1000))

// Write a float32
err := node.WriteAnyExact(0x2001, 0, float32(12.34))

// Write a float64
err := node.WriteAnyExact(0x2002, 0, float64(56.78))
```

*Note: It is recommended to cast your Go values to the specific type expected by the OD entry (e.g. `uint16`, `int8`, `uint32`) to ensure successful type checking.*

### Writing Raw Bytes

You can write raw bytes directly to an OD entry using `WriteBytes`. This is useful for `DOMAIN` types or when you want to bypass type checking and write the raw binary representation directly.

```go
// Write raw bytes to a DOMAIN entry or similar
data := []byte{0x01, 0x02, 0x03, 0x04}
err := node.WriteBytes(0x3000, 0, data)
```

### Error Handling

The API returns specific errors from the `od` package when operations fail. Handling these allows for robust application logic.

**Common Errors:**

- `od.ErrIdxNotExist`: The requested Index does not exist in the OD.
- `od.ErrSubNotExist`: The Index exists, but the SubIndex does not.
- `od.ErrTypeMismatch`: The requested read type or provided write value does not match the OD entry's type.
- `od.ErrObjectNotWritable`: Trying to write to a read-only entry.

**Example: Robust Read/Write with Error Checking**

```go
func updateParameter(node *network.LocalNode) {
	// Try to read a value
	val, err := node.ReadUint(0x2000, 0)
	if err != nil {
		switch err {
		case od.ErrIdxNotExist:
			log.Printf("Parameter 0x2000 not found in EDS")
		case od.ErrTypeMismatch:
			log.Printf("Parameter 0x2000 is not a Uint")
		default:
			log.Printf("Read failed: %v", err)
		}
		return
	}

	fmt.Printf("Current Value: %d\n", val)

	// Try to write a new value
	newValue := uint32(val + 1)
	err = node.WriteAnyExact(0x2000, 0, newValue)
	if err != nil {
		log.Printf("Failed to write: %v", err)
	} else {
		log.Println("Value updated successfully")
	}
}
```

## Special OD Entries & Node Behavior



Certain Object Dictionary entries are linked to the internal behavior of the `LocalNode`. When these entries are written to (either locally or via a remote SDO client), the node automatically updates its configuration or behavior.



The supported special objects are listed in the **Special entries** table in [od.md](od.md). These typically include entries in the **Communication Profile Area** (0x1000 - 0x1FFF), such as:



- **0x1017: Producer Heartbeat Time**: Updating this value changes how often the node sends its heartbeat message.

- **0x1016: Consumer Heartbeat Time**: Updating this allows the node to start or stop monitoring heartbeats from other nodes.

- **Dynamic PDO Reconfiguration**: The `LocalNode` supports dynamic PDO mapping and communication parameter updates (0x1400-0x15FF/0x1800-0x19FF for communication and 0x1600-0x17FF/0x1A00-0x1BFF for mapping).



### Example: Master Updating Slave Heartbeat

A common use case is a Master node reconfiguring a Slave node's heartbeat frequency over the network.

**Slave Side (LocalNode):**
The slave node simply needs to be running. It has 0x1017 defined in its EDS.

**Master Side:**
The master can use a `NodeConfigurator` to update the slave's heartbeat time.

```go
// Create a configurator for the slave node (ID 0x10) (this is basically a wrapper around an SDO client)
config := net.Configurator(0x10)

// Update slave's heartbeat producer time to 500ms
// This will perform an SDO write to index 0x1017:00 on the slave
err := config.WriteProducerHeartbeatTime(500)
```

**What happens internally:**
1. The Master sends an SDO Download request to the Slave for entry `0x1017:00`.
2. The Slave's `LocalNode` receives the SDO request.
3. Because `0x1017` has a special **Extension** registered, the new value is not just stored in the OD; it also triggers a configuration update in the Slave's NMT service.
4. The Slave immediately starts sending heartbeats every 500ms.

## SDO Server Behavior

The `LocalNode` automatically acts as an **SDO Server**. It handles SDO requests from other nodes (SDO Clients) on the network implicitly.

- **Reads (SDO Upload):** Remote nodes can read values from this node's OD.
- **Writes (SDO Download):** Remote nodes can write values to this node's OD.

The values read/written via remote SDO are the same ones accessed via the `Read...` and `Write...` local API documented above. The `LocalNode` ensures atomicity and consistency.
