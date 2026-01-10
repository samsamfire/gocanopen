# CAN driver

In order to be able to connect to the CAN network, a specific driver is needed.
Currently, this library comes with 3 supported devices :

- socketcan (including vcan)
- kvaser
- virtualcan [here](https://github.com/windelbouwman/virtualcan)

> Note : In order to use kvaser, kvaser canlib should be downloaded & installed.
> Specific compile flags are required :
> - CFLAGS: -g -Wall -I/path_to_kvaser/canlib/include
> - LDFLAGS: -L/path_to_kvaser/canlib

# SocketCAN

Multiple versions of the socketcan driver exists, in particular :

- socketcan (standard socket recv/write)
- socketcanv3 (standard socket write, reads are done using recvmmsg)
- socketcanring (standard socket write, reads are done using an rx buffer ring)

In particular the **socketcanring** implementation can drastically boost RX performance on smaller embedded systems
if latency is not an issue.

## Creating a custom driver

More transceivers can be added by creating your own driver and implementing the following
interface :

```go
type Bus interface {
	Connect(...any) error                   // Connect to the actual bus
	Disconnect() error                      // Disconnect from bus
	Send(frame Frame) error                 // Send a frame on the bus
	Subscribe(callback FrameListener) error // Subscribe to all can frames
}
```
Feel free to contribute to add specific drivers, we will find a way to integrate them in this repo.