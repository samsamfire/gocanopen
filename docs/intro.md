# Introduction

This package was written because I wanted an easy to use and efficient CANopen stack CiA 301
capable of running on embedded devices. golang is a great option thanks to its modern tooling.
This project has been inspired by two other existing projects:
- [CANopenNode](https://github.com/CANopenNode/CANopenNode) a C implementation slave side.
- [canopen](https://github.com/christiansandberg/canopen) a python implementation mostly for master control.

This project takes the best of both worlds and tries to implement the slave & the master side in a simple yet
efficient API.

This documentation does not aim to be a tutorial on how CANopen works, a lot of information is freely available online.

## Network

The **Network** object is used for managing the CANopen stack. It holds CANopen **Nodes** which can be of two types :
Either a **LocalNode**, which represents a real CANopen, CiA 301 compliant node or a **RemoteNode** which is the 
local representation of a remote CANopen node on the CAN bus used for master control.

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
network.Command(0,nmt.CommandResetNode) // resets all nodes
network.Command(12,nmt.CommandResetNode) // resets node with id 12
```


