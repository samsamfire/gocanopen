package lss

import (
	"errors"

	"github.com/samsamfire/gocanopen/pkg/config"
)

const (
	ServiceSlaveId     = 0x7E4
	ServiceMasterId    = 0x7E5
	NodeIdUnconfigured = 0xFF
	NodeIdMin          = 0x1
	NodeIdMax          = 0x7F
)

var (
	ErrTimeout       = errors.New("no answer received")
	ErrInvalidNodeId = errors.New("invalid node id")
)

type LSSMode uint8

const (
	ModeWaiting       LSSMode = 0
	ModeConfiguration LSSMode = 1
)

const (

	// Switch mode services, used to connect master & slave for configuration
	CmdSwitchStateGlobal            LSSCommand = 4
	CmdSwitchStateSelectiveVendor   LSSCommand = 64
	CmdSwitchStateSelectiveProduct  LSSCommand = 65
	CmdSwitchStateSelectiveRevision LSSCommand = 66
	CmdSwitchStateSelectiveSerialNb LSSCommand = 67
	CmdSwitchStateSelectiveResult   LSSCommand = 68

	// Configuration services, only available in configuration mode
	CmdConfigureNodeId            LSSCommand = 17
	CmdConfigureBitTiming         LSSCommand = 19
	CmdConfigureActivateBitTiming LSSCommand = 21
	CmdConfigureStoreParameters   LSSCommand = 23

	// Inquiry services, only available in configuration mode
	CmdInquireVendor   LSSCommand = 90
	CmdInquireProduct  LSSCommand = 91
	CmdInquireRevision LSSCommand = 92
	CmdInquireSerial   LSSCommand = 93
	CmdInquireNodeId   LSSCommand = 94

	// Identification services, available in operational & configuration mode

)

const (
	ConfigNodeIdOk           = 0
	ConfigNodeIdOutOfRange   = 1
	ConfigNodeIdManufacturer = 0xFF
)

// The LSS address is used to uniquely identify each node on the CANopen network.
// It corresponds to the concatenated values of the identity object (0x1018)
type LSSAddress struct {
	config.Identity
}

type LSSMessage struct {
	raw [8]byte
}

type LSSCommand uint8

func (m *LSSMessage) Command() LSSCommand {
	return LSSCommand(m.raw[0])
}

type LSSState uint8

func (state LSSState) String() string {
	switch state {
	case StateWaiting:
		return "WAITING"
	case StateConfiguration:
		return "CONFIGURATION"
	default:
		return "UNKNOWN"
	}
}

// LSS states as defined by CiA 305
const (
	// LSS waiting: In this state, the LSS slave devices may be identified. Otherwise the LSS
	// slave device waits for a request to enter LSS configuration state.
	// The LSS slave is operating on its active bit rate.
	// The virtual node-ID and bit rate variables are not changeable by means of LSS in this
	// state.
	StateWaiting LSSState = 1
	// LSS configuration: In this state the virtual node-ID and bit rate variables may be
	// configured at the LSS slave. Device can be configured in this state.
	StateConfiguration LSSState = 2
)
