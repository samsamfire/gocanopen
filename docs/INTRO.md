# Introduction

This package was written because I needed the ability to create local CANopen nodes
on embedded linux systems for simulation and to do various maintenance related tasks
on nodes already present on the bus : hearbeat monitoring, updates via block transfer, etc.
Because this had to be run on an embedded system, I also had some constraints on the efficiency,
and lastly.
Golang was a great option because modern tooling and efficient.
This package has been inspired by two existing projects : [CANopenNode](https://github.com/CANopenNode/CANopenNode)
and [canopen](https://github.com/christiansandberg/canopen) to combine the best of both worlds.
This documentation does not aim to be a tutorial of how CANopen works, a lot of information is freely available online.

## Network

The **Network** object is used for managing the CANopen stack. It holds CANopen **Nodes** which can be of two types :
Either a **LocalNode**, which represents a real CANopen, CiA 301 compliant node or a **RemoteNode** which is the 
local representation of a remote CANopen node on the CAN bus.

### Usage

First, connect to a CAN bus.
``` golang
network := canopen.NewNetwork(nil)
network.Connect("socketcan","can0",500000)
defer network.Disconnect() // properly disconnect

```
The network object itself can be used to perform high level commands.
The following uses **NMT** commands :

``` golang
network.Command(0,canopen.NMT_RESET_NODE) // resets all nodes
network.Command(12,canopen.NMT_RESET_NODE) // resets node with id 12
```


