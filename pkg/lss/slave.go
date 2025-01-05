package lss

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/od"
)

type LSSSlave struct {
	*canopen.BusManager
	logger          *slog.Logger
	address         LSSAddress
	addressSwitch   LSSAddress
	addressFastscan LSSAddress
	activeNodeId    uint8
	pendingNodeId   uint8
	rx              chan LSSMessage
	state           LSSState
}

// Handle [LSSSlave] related RX CAN frames
func (l *LSSSlave) Handle(frame canopen.Frame) {

	if frame.DLC != 8 {
		return
	}
	msg := LSSMessage{raw: frame.Data}
	l.logger.Info("received new command from master",
		"cmd", msg.Command(),
		"cmdHex", fmt.Sprintf("x%x", msg.Command()),
		"raw", msg.raw,
	)
	select {
	case l.rx <- msg:
	default:
		l.logger.Warn("dropped LSS master RX frame")
		// Drop frame
	}
}

// To be launched inside of a goroutine (replies to incoming messages)
func (l *LSSSlave) Process(ctx context.Context) {
	l.logger.Info("starting lss slave process", "address", l.address)
	for {
		select {
		case rx := <-l.rx:
			prevState := l.state
			l.processRequest(rx)
			currentState := l.state
			if prevState != currentState {
				l.logger.Info("slave moved from state", "previous", prevState.String(), "current", currentState.String())
			}
		case <-ctx.Done():
			l.logger.Info("exiting lss slave process")
			return
		}
	}
}

// Get current lss state
func (l *LSSSlave) GetState() LSSState {
	return l.state
}

// Process new request from master depending on the current LSS mode
// Available commands depend on the state.
func (l *LSSSlave) processRequest(rx LSSMessage) error {

	cmd := rx.Command()
	state := l.state

	switch {

	case (cmd >= CmdSwitchStateSelectiveVendor && cmd <= CmdSwitchStateSelectiveResult) || cmd == CmdSwitchStateGlobal:
		err := l.processSwitchStateService(rx)
		if err != nil {
			l.logger.Warn("error processing switch state service", "err", err)
		}

	case cmd >= CmdConfigureNodeId && cmd <= CmdConfigureStoreParameters:
		// Configuration service is only valid in configuration mode
		if state != StateConfiguration {
			return nil
		}
		err := l.processConfigurationService(rx)
		if err != nil {
			l.logger.Warn("error processing configuration service", "err", err)
		}

	case cmd >= CmdInquireVendor && cmd <= CmdInquireNodeId:
		// Inquire service is only valid in configuration mode
		if state != StateConfiguration {
			return nil
		}
		err := l.processInquiryService(cmd)
		if err != nil {
			l.logger.Warn("error processing inquiry service", "err", err)
		}
	}

	return nil
}

// Process switch state service message
func (l *LSSSlave) processSwitchStateService(msg LSSMessage) error {
	switch msg.Command() {

	case CmdSwitchStateGlobal:
		mode := LSSMode(msg.raw[1])
		switch mode {

		case ModeWaiting:
			// TODO : unclear whether it is the slave that should perform the reset
			// In case of reset comm, active node id should be taken from pending node id
			l.state = StateWaiting

		case ModeConfiguration:
			l.state = StateConfiguration
		default:
			// Not a standard command
			l.logger.Warn("switch mode unknown", "mode", mode)
		}

	case CmdSwitchStateSelectiveVendor:
		l.addressSwitch.VendorId = binary.LittleEndian.Uint32(msg.raw[1:5])
		l.logger.Debug("switch state selective", "vendor", l.addressSwitch.VendorId)

	case CmdSwitchStateSelectiveProduct:
		l.addressSwitch.ProductCode = binary.LittleEndian.Uint32(msg.raw[1:5])
		l.logger.Debug("switch state selective", "product", l.addressSwitch.ProductCode)

	case CmdSwitchStateSelectiveRevision:
		l.addressSwitch.RevisionNumber = binary.LittleEndian.Uint32(msg.raw[1:5])
		l.logger.Debug("switch state selective", "revision", l.addressSwitch.RevisionNumber)

	case CmdSwitchStateSelectiveSerialNb:
		// This is the last part of the switch state selective.
		// After this we can determine if we are the node that has been selected
		l.addressSwitch.SerialNumber = binary.LittleEndian.Uint32(msg.raw[1:5])
		l.logger.Debug("switch state selective", "serial number", l.addressSwitch.SerialNumber)
		if l.addressSwitch == l.address {
			l.state = StateConfiguration
			// Send successfull response
			return l.Send([8]byte{byte(CmdSwitchStateSelectiveResult)})
		} else {
			l.logger.Debug("switch state selective ignored", "requested", l.addressSwitch, "current", l.address)
		}
	}
	return nil
}

// Process inquiry service message, prepare TX buffer for sending
func (l *LSSSlave) processInquiryService(cmd LSSCommand) error {

	data := [8]byte{byte(cmd)}
	switch cmd {

	case CmdInquireVendor:
		binary.LittleEndian.PutUint32(data[1:], l.address.VendorId)

	case CmdInquireProduct:
		binary.LittleEndian.PutUint32(data[1:], l.address.ProductCode)

	case CmdInquireRevision:
		binary.LittleEndian.PutUint32(data[1:], l.address.RevisionNumber)

	case CmdInquireSerial:
		binary.LittleEndian.PutUint32(data[1:], l.address.SerialNumber)

	case CmdInquireNodeId:
		data[1] = l.activeNodeId

	default:
		return fmt.Errorf("unknown LSS command %v", cmd)
	}
	return l.Send(data)
}

// Process configuration service, prepare TX buffer for sending
func (l *LSSSlave) processConfigurationService(msg LSSMessage) error {

	switch msg.Command() {

	case CmdConfigureBitTiming, CmdConfigureActivateBitTiming, CmdConfigureStoreParameters:
		// Node id is the only supported configuration command for now
		l.logger.Warn("unsupported configuration command")

	case CmdConfigureNodeId:
		nodeId := msg.raw[1]
		if !(nodeId >= 1 && nodeId <= 0x7F || nodeId == 0xFF) {
			l.logger.Warn("requested nodeId is out of range", "id", nodeId)
			return l.Send([8]byte{byte(msg.Command()), ConfigNodeIdOutOfRange})
		}
		l.pendingNodeId = nodeId
		return l.Send([8]byte{byte(msg.Command()), ConfigNodeIdOk})

	default:
		return fmt.Errorf("unknown LSS command %v", msg.Command())

	}
	return nil
}

func (l *LSSSlave) Send(data [8]byte) error {
	frame := canopen.NewFrame(ServiceSlaveId, 0, 8)
	frame.Data = data
	return l.BusManager.Send(frame)
}

func NewLSSSlave(bm *canopen.BusManager, logger *slog.Logger, identity *od.Entry, nodeId uint8) (*LSSSlave, error) {

	var err error
	if logger == nil {
		logger = slog.Default()
	}
	lss := &LSSSlave{BusManager: bm, logger: logger}
	lss.logger = logger.With("service", "[LSSSlave]")
	lss.address.VendorId, err = identity.Uint32(1)
	if err != nil {
		return nil, err
	}
	lss.address.ProductCode, err = identity.Uint32(2)
	if err != nil {
		return nil, err
	}
	lss.address.RevisionNumber, err = identity.Uint32(3)
	if err != nil {
		return nil, err
	}
	lss.address.SerialNumber, err = identity.Uint32(4)
	if err != nil {
		return nil, err
	}
	lss.state = StateWaiting
	lss.rx = make(chan LSSMessage, 10)
	err = lss.Subscribe(ServiceMasterId, 0x7FF, false, lss)
	if err != nil {
		return nil, err
	}
	lss.activeNodeId = nodeId
	lss.pendingNodeId = nodeId
	return lss, nil
}
