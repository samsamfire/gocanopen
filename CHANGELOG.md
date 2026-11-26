# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.2.0] - 2026-11-26

### Added

- Bus : `BusStateReader` to read the CAN controller state, implemented by the socketcan and Kvaser drivers.
- OD : `Parse` accepts an `io.Reader`, sections in any order, case insensitive keys, comments, BOM and CRLF.

### Changed

- OD : `Parse` and `ParseV2` are now a single parser, about 16x faster than the previous `Parse`.
- OD : a malformed EDS returns an error with its line number instead of panicking or being silently misread.
- OD : OCTET_STRING default values are read as hexadecimal.
- OD : signed values can be given in hexadecimal, REAL values as raw hexadecimal bits.
- OD : `gopkg.in/ini.v1` is no longer a dependency.
- NMT : reset communication re-initialises the node and sends a boot-up instead of being ignored.
- NMT : state changes are notified immediately instead of at the next heartbeat.
- Emergency : report and reset are only logged when an emergency is actually sent.
- Gateway : an invalid node id is rejected with error 107.

### Deprecated

- OD : `ParseV2`, use `Parse`.

### Removed

- OD : `NewVariableFromSection`.

### Fixed

- OD : a missing array sub-index returned a nil variable instead of `ErrSubNotExist`.
- OD : `ParseV2` ignored LowLimit and HighLimit.
- OD : EDS export wrote read-only entries as rw and dropped PDOMapping, limits and negative INTEGER8 values.
- OD : EDS export failed on a record with a single sub-index.
- OD : a DOMAIN entry without extension returned an unusable streamer.
- OD : `PutBytes` detached the variable from its streamers.
- OD : a file entry leaked a file descriptor when a transfer was aborted.
- OD : a file entry could not rewind after its end, aborting block upload retransmissions.
- SDO : the server never aborted a stalled block download.
- SDO : a lost sub-block acknowledgement duplicated data during a block upload.
- SDO : the server ignored errors in the middle of a block upload sub-block.
- SDO : block download counted re-transmitted segments twice.
- SDO : block download stalled when the first or last segment of a sub-block was lost.
- SDO : block upload of a reader ending on a sub-block boundary aborted.
- SDO : block upload size indication was truncated to one byte.
- SDO : segmented download bigger than the client fifo never finished.
- SDO : the server used the size indicated by a previous transfer.
- SDO : requests received in STOPPED were executed later.
- SDO : server crash on a custom writer error in an expedited download.
- SDO : wrong length checks and panic in local download.
- SDO : `WriteRaw` ignored `forceSegmented`.
- SDO : `ReadRaw` silently truncated data and left the transfer open.
- SDO : a second `Write` on a writer was silently dropped.
- SDO : the client ignored the timeout given at creation.
- SDO : client COB-IDs were swapped when writing 0x1280 sub 2.
- SDO : `Network.Scan` leaked the subscriptions of its clients.
- PDO : TPDO SYNC start value was ignored when written or after an NMT change.
- PDO : SYNC start value was applied to synchronous acyclic TPDOs.
- PDO : a correct RPDO cleared the length error of another RPDO.
- PDO : writing the base COB-ID gave a different COB-ID on read.
- PDO : `ClearMappings` failed on PDOs with less than 8 mapping entries.
- PDO : TPDOs were sent after `Stop`.
- SYNC : the producer never triggered its own synchronous PDOs.
- SYNC : a SYNC added with `AddSYNC` could not be enabled as producer.
- SYNC : RPDO and SYNC timeout errors were never cleared after an NMT change.
- SYNC : timeout was raised after `Stop`.
- TIME : the producer did not send the current UTC time.
- TIME : COB-ID changes written to 0x1012 were not applied.
- TIME : produced outside of PRE-OPERATIONAL and OPERATIONAL.
- NMT : `SendCommand` deadlocked with loopback and was not applied to the own node.
- NMT : deadlock when a state change callback used the NMT object.
- NMT : SYNC, TIME and heartbeat frames were sent after `Stop`.
- Node : node id 127 was rejected.
- Heartbeat : consumer callback had node id and index swapped.
- Heartbeat : a timeout raised the remote reset error instead of the timeout error.
- Heartbeat : writing 0x1016 or 0x1017 with a nil stream panicked.
- Emergency : error register was constant in EMCY frames and 0x1001 always read 0.
- Emergency : EMCY was sent while the producer was disabled in 0x1014.
- Emergency : remote EMCY were never received, the subscription mask was ignored.
- Bus : extended and RTR frames were routed to standard frame subscribers.
- Gateway : responses contained the body twice.
- Gateway : the sub-index bound check tested the wrong variable.
- Gateway : did not compile on 32 bit and could return negative sequence numbers.
- Drivers : socketcan drivers leaked or broke their socket on `Disconnect`, reconnecting did not work.
- Drivers : socketcan drivers reported a short write as success.
- Drivers : socketcanv3 delayed received frames by up to 100 ms.
- Drivers : socketcanring parsed TPACKET_V2 frames as TPACKET_V1, all ids were wrong.
- Drivers : socketcanring received its own frames.
- Drivers : socketcanring filters were applied to the wrong socket and never worked.
- Drivers : socketcan drivers did not compile on some architectures.
- Drivers : Kvaser ignored extended, RTR and error frame flags.
- Drivers : Kvaser status codes were not sign-extended.
- Drivers : Kvaser did not list adapters plugged in after start-up.
- Drivers : Kvaser never cleared the latched overrun flags once reported.

## [2.1.1] - 2026-10-07

### Changed

- SDO : the client reacts as soon as a response arrives instead of polling, which speeds up transfers.
- Kvaser : the third argument of Connect is now the bitrate.

### Fixed

- SDO : SetBlockMaxSize was ignored during block uploads, so devices that only support smaller blocks could not be read.
- SDO : a read could lose the end of the data when the caller's buffer was too small, for example with io.ReadAll on Go 1.26. 
- Kvaser : the bus always ran at 500 kbit/s whatever bitrate was requested; unsupported bitrates are now rejected.
- Kvaser : sending a frame could be delayed by up to the read timeout, causing SDO block transfers to time out.


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

[2.2.0]: https://github.com/samsamfire/gocanopen/compare/v2.1.1...v2.2.0
[2.1.1]: https://github.com/samsamfire/gocanopen/compare/v2.1.0...v2.1.1
[2.1.0]: https://github.com/samsamfire/gocanopen/compare/v2.0.1...v2.1.0
