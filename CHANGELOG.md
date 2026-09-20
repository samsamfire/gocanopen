# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).


## [2.1.0] - 2026-09-20

### Added

- SDO : `SDOServer.Stop()` to release a server's CAN id subscription without running its processing loop.

### Changed

- PDO : an RPDO longer than the mapped length is processed, the surplus bytes being ignored.
- PDO : `TPDO.SendAsync()` inside the inhibit window is delayed to the end of the window.
- PDO : a communication or mapping parameter written with a mismatched size is rejected.
- PDO : a number of mapped objects above what the object dictionary declares is rejected.
- Emergency : an error condition is reported once instead of on every occurrence.
- Emergency : resolving an error condition is reported with a no error emergency.

### Fixed

- SDO : several issues in sub-block reception during a block upload.
- SDO : an entry read in one go could not be retransmitted during a block upload.
- SDO : segmented upload was truncated at a buffer boundary.
- SDO : wrong size indication in upload segments.
- SDO : panic on an expedited download into a string entry.
- SDO : timeout timer was not reset correctly.
- SDO : block transfers now use their own timeout.
- OD : reading an entry in several chunks returned the chunks out of order.
- OD : reading an entry in several chunks could read out of bounds.
- OD : writing an entry in several chunks always failed.
- OD : wrong data offset when reading an entry backed by a reader.
- PDO : a partially mapped object was written with the length of the object.
- PDO : a TPDO panicked when a mapped object was bigger than the space left in the frame.
- PDO : an out of range number of mapped objects left mapping slots unconfigured, and panicked on reception.
- PDO : mapping could not be cleared when dummy entries were used.
- PDO : TPDOs were transmitted outside of OPERATIONAL.
- PDO : the event timer of a TPDO was not re-armed after a disable / enable.
- PDO : the event timer was applied to synchronous transmission types.
- PDO : the SYNC subscription did not follow a transmission type change.
- PDO : deadline monitoring of an RPDO kept running once the RPDO was disabled.
- PDO : disabling an RPDO subscribed it to CAN id 0 instead of cancelling its subscription.
- PDO : the length emergency reported the expected length instead of the offending one.
- SYNC : the producer kept transmitting after the communication cycle period was set to 0.
- SYNC : the consumer timeout was not disarmed when SYNC was stopped or disabled.
- SYNC : the counter was updated even when no counter overflow is configured.
- SYNC : a length error was never cleared by a valid SYNC.
- SYNC : sync events could be dropped, making a synchronous PDO miss a cycle.
- SYNC : a sync event could be handled with a stale counter.
- SYNC : a remote node built its SYNC object without an emergency object.
- Emergency : reporting an error from a service without an emergency object panicked.
- Emergency : the error status bits were never updated.
- Node : a node rejected at creation kept transmitting for the lifetime of the program.
- Node : a remote node did not stop its PDOs.
- Node : a heartbeat producer time of 0 did not start the node nor send the boot-up message.
- Drivers : kvaser Windows build.
- Drivers : creating a kvaser bus panicked on failure, and now takes a channel name.
- Drivers : virtual bus reception of segmented and coalesced frames.

### Internal

- Non regression tests for chunked object dictionary access, PDO mapping, SYNC and lossy SDO transfers.

[2.1.0]: https://github.com/samsamfire/gocanopen/compare/v2.0.1...v2.1.0
