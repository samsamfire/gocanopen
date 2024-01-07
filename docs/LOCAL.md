# Creating a LocalNode

A **LocalNode** is the CiA 301 implementation of a CANopen node in golang.
It has it's own CANopen stack, which is run within a goroutine.
When creating a local node, the library parses the EDS file and creates the relevant
CANopen objects.

### Usage

To create a CANopen node :

```go
network := canopen.NewNetwork(nil)
network.Connect("socketcan","can0",500000)
defer network.Disconnect() // properly disconnect

// Create a node with id 0x10 using the example EDS
node := canopen.CreateLocalNode(0x10,"../testdata/base.eds")

```
Thats it ! The node will go automatically to NMT state **OPERATIONAL** if there are no errors.
A callback can be added that will be called by the node's goroutine. It must be **non blocking**.

```golang

// Called every stack tick, i.e. ~10ms
func NodeMainCallback(node canopen.Node) {
	od := node.GetOD()
	// Get manufacturer device name
	deviceName, _ := od.Index(0x1008).GetRawData(0, 0)
	fmt.Println("My name is : ", string(deviceName))
}

node.SetMainCallback(NodeMainCallback)

```

Note that there are several ways of accessing data in the object dictionnary. Some standard functions
are provided for direct memory access to the OD. However, all nodes come with a local sdo client for 
reading the internal OD. Here are some examples:

```golang
od := node.GetOD()
od.Index("UNSIGNED32 value").Uint32(0) // Get od value as a uint32, if length is incorrect, it will error

localNode := node.(*canopen.LocalNode) // Get actual type : *canopen.LocalNode
localNode.ReadUint("UNSIGNED32","") // Returns it as uint64
localNode.ReadUint("UNSIGNED8","") // Returns it as uint64 also
deviceName,_ := localNode.ReadString(0x1008, 0x0) // This corresponds to device name

```

See **BaseNode** go doc for more information on the available methods.


