# Configurator
This library comes with a **NodeConfigurator** that can be used to easily
 **get / update** CANopen behaviour using CiA defined entries.

The following is a non-exhaustive example :

```go
config := net.Configurator(0x20) // create a NodeConfigurator object for node 0x20

// Configure RPDO 1
var pdoNb uint8 = 0

// Read / write event timer
conf.WriteEventTimer(pdoNb, 1111)
eventTimer, _ := conf.ReadEventTimer(pdoNb)
fmt.Prinln("event timer",eventTimer)

// Read / write transmission type
conf.WriteTransmissionType(pdoNb,11)
transType, _ := conf.ReadTransmissionType(pdoNb)
fmt.Prinln("transmission type",transType)

// Clear current mapping
conf.ClearMappings(pdoNb)

// Write / read PDO mapping
var Mapping = []config.PDOMappingParameter{
	{Index: 0x2001, Subindex: 0x0, LengthBits: 8},
	{Index: 0x2002, Subindex: 0x0, LengthBits: 8},
	{Index: 0x2003, Subindex: 0x0, LengthBits: 16},
	{Index: 0x2004, Subindex: 0x0, LengthBits: 32},
}
_ = conf.WriteMappings(pdoNb, Mapping)

mapping, _ := conf.ReadMappings(pdoNb)
fmt.Println("mapping",mapping)

// Update COB-ID
conf.WriteCanIdPDO(pdoNb, 0x211)

// Read hole configuration
config, _ := conf.ReadConfigurationPDO(pdoNb)
fmt.Println("pdo config",config)
```

Other configuration APIs exist for SDO, HB, SYNC, TIME, NMT, ...