# Master Example

This example demonstrates how to use the library as a CANopen master to:
1. Connect to a CAN bus.
2. Add a remote node.
3. Start PDOs for the remote node.
4. Read and write values to the remote node's object dictionary using SDO.

## How to Run

Make sure you have a running CAN interface (e.g., `can0` or `vcan0`) and a slave node with ID `0x20` on the bus.

```bash
go run main.go
```
