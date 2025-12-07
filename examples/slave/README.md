# Slave Example

This example demonstrates how to create a minimal CANopen slave node.
The slave node will be created with a default object dictionary and will be visible on the CAN bus.

## How to Run

Make sure you have a running CAN interface (e.g., `can0` or `vcan0`).

```bash
go run main.go
```

You can then use other tools (like a CANopen master or another example from this library) to interact with this slave node.
For example, you can use the `basic` example to scan the network and see this slave node.
