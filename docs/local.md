# Creating a LocalNode

A **LocalNode** is the CiA 301 implementation of a CANopen node in golang.
It has it's own CANopen stack, which is run within a goroutine.
When creating a local node, the library parses the EDS file and creates the relevant
CANopen objects. This is different from other implementations that usually require a pre-build step 
that generates .c/.h file that will then be used inside of the application.

# Nodes

This document is deprecated. Please see:
- [Local Node](local-node.md)
- [Remote Node](remote-node.md)


