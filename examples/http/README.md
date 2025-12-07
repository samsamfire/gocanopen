# HTTP Gateway Example

This example demonstrates how to set up an HTTP gateway for the CANopen network.
The gateway exposes a REST API to interact with the CANopen nodes.

## How to Run

Make sure you have a running CAN interface (e.g., `can0` or `vcan0`).

To run the example with the default settings (vcan0, port 8090):
```bash
go run main.go
```

You can specify a different CAN interface using the `-i` flag:
```bash
go run main.go -i can0
```
