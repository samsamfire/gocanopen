package lss

import (
	"encoding/binary"
	"log/slog"
	"sync"
	"time"

	canopen "github.com/samsamfire/gocanopen"
)

var DefaultTimeout = 1000 * time.Millisecond

type LSSMaster struct {
	*canopen.BusManager
	logger  *slog.Logger
	mu      sync.Mutex
	rx      chan LSSMessage
	timeout time.Duration
}

// Handle [LSSMaster] related RX CAN frames
func (l *LSSMaster) Handle(frame canopen.Frame) {
	if frame.DLC != 8 {
		return
	}
	msg := LSSMessage{raw: frame.Data}
	select {
	case l.rx <- msg:
	default:
		l.logger.Warn("dropped LSS slave RX frame")
		// Drop frame
	}
}

// Wait for an answer from slave with a given command
// Any other command is ignored until timeout is elapsed
func (l *LSSMaster) WaitForResponse(cmd LSSCommand) (LSSMessage, error) {

	begin := time.Now()

	for {
		elapsed := time.Since(begin)
		if elapsed >= l.timeout {
			return LSSMessage{}, ErrTimeout
		}

		timeout := l.timeout - elapsed

		select {
		case resp := <-l.rx:
			if cmd == resp.Command() {
				return resp, nil
			} else {
				// Unexpected response, ignore
				l.logger.Warn("received unexpected response, ignoring", "response", resp)
			}
		case <-time.After(timeout):
			l.logger.Warn("no response received from slave, expecting", "command", cmd)
			return LSSMessage{}, ErrTimeout
		}
	}
}

// Send a switch state global command to all nodes
// i.e. waiting or configuration
// No answer is expected
func (l *LSSMaster) SwitchStateGlobal(mode LSSMode) error {
	frame := canopen.NewFrame(ServiceMasterId, 0, 8)
	frame.Data[0] = byte(CmdSwitchStateGlobal)
	frame.Data[1] = byte(mode)
	return l.Send(frame)
}

// Send a switch state selective command to the desired node
// based on the LSS address.
// If no answer is received, command will timeout
func (l *LSSMaster) SwitchStateSelective(address LSSAddress) error {

	frame := canopen.NewFrame(ServiceMasterId, 0, 8)
	frame.Data[0] = byte(CmdSwitchStateSelectiveVendor)
	binary.LittleEndian.PutUint32(frame.Data[1:], address.VendorId)
	l.Send(frame)

	frame.Data[0] = byte(CmdSwitchStateSelectiveProduct)
	binary.LittleEndian.PutUint32(frame.Data[1:], address.ProductCode)
	l.Send(frame)

	frame.Data[0] = byte(CmdSwitchStateSelectiveRevision)
	binary.LittleEndian.PutUint32(frame.Data[1:], address.RevisionNumber)
	l.Send(frame)

	frame.Data[0] = byte(CmdSwitchStateSelectiveSerialNb)
	binary.LittleEndian.PutUint32(frame.Data[1:], address.SerialNumber)
	l.Send(frame)

	_, err := l.WaitForResponse(CmdSwitchStateSelectiveResult)
	return err
}

// Update timeout for answer from slave nodes
func (l *LSSMaster) SetTimeout(timeout time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.timeout = timeout
}

func NewLSSMaster(bm *canopen.BusManager, logger *slog.Logger, timeout time.Duration) (*LSSMaster, error) {

	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("service", "[LSSMaster]")
	lss := &LSSMaster{BusManager: bm, logger: logger}
	lss.rx = make(chan LSSMessage, 2)
	lss.SetTimeout(timeout)
	err := lss.Subscribe(ServiceSlaveId, 0x7FF, false, lss)
	if err != nil {
		return nil, err
	}

	return lss, nil
}
