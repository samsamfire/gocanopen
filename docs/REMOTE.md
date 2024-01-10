# Interacting with remote nodes

This package provides a lot of useful functions for interacting with remote nodes.
In particular the following is supported :
- SDO transfers (expedited, segmented) for reading / writing values.
- SDO block transfers, for reading or writing large amounts of data.
- RPDOs : if configured, RPDOs will be received by the RemoteNode and will update
an internal OD
- TPDOs : if configured, TPDOs can be sent using the internal OD.

### SDO expedited & segmented

TODO