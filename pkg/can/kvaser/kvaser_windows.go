//go:build windows

package kvaser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unsafe"

	canopen "github.com/samsamfire/gocanopen/v2"
	can "github.com/samsamfire/gocanopen/v2/pkg/can"
)

// Dynamically load Kvaser's canlib32.dll
var (
	canlib = syscall.NewLazyDLL("canlib32.dll")

	procInitializeLibrary   = canlib.NewProc("canInitializeLibrary")
	procEnumHardwareEx      = canlib.NewProc("canEnumHardwareEx")
	procGetErrorText        = canlib.NewProc("canGetErrorText")
	procOpenChannel         = canlib.NewProc("canOpenChannel")
	procSetBusParams        = canlib.NewProc("canSetBusParams")
	procSetBusOutputControl = canlib.NewProc("canSetBusOutputControl")
	procBusOn               = canlib.NewProc("canBusOn")
	procBusOff              = canlib.NewProc("canBusOff")
	procClose               = canlib.NewProc("canClose")
	procWrite               = canlib.NewProc("canWrite")
	procWriteSync           = canlib.NewProc("canWriteSync")
	procReadWait            = canlib.NewProc("canReadWait")
	procGetVersion          = canlib.NewProc("canGetVersion")
	procGetNumberOfChannels = canlib.NewProc("canGetNumberOfChannels")
	procIoCtl               = canlib.NewProc("canIoCtl")
	procReadStatus          = canlib.NewProc("canReadStatus")
	procRequestChipStatus   = canlib.NewProc("canRequestChipStatus")
)

const (
	defaultReadTimeoutMs  = 500
	defaultWriteTimeoutMs = defaultReadTimeoutMs
	// statePollPeriod is the minimum time between two reads of the channel
	// state by the reception goroutine. A quiet bus reads it at most every
	// defaultReadTimeoutMs, when canReadWait times out.
	statePollPeriod = 500 * time.Millisecond
)

// Kvaser Constants (Mapped from canlib.h)
// As we are not using CGO, we need to manually set the equivalent
// canlib enums. A lot of these enums are missing here, only the ones
// being used have been copied.
const (
	StatusOk         = 0
	canERR_NOMSG     = -2
	canDRIVER_NORMAL = 4

	canIOCTL_CLEAR_ERROR_COUNTERS = 5
	canIOCTL_SET_LOCAL_TXECHO     = 32

	OpenExclusive         = 0x0008
	OpenRequireExtended   = 0x0010
	OpenAcceptVirtual     = 0x0020
	OpenOverrideExclusive = 0x0040
	OpenRequireInitAccess = 0x0080
	OpenNoInitAccess      = 0x0100
	OpenAcceptLargeDlc    = 0x0200
	OpenCanFd             = 0x0400
	OpenCanFdNonIso       = 0x0800
	OpenInternalL         = 0x1000
)

var (
	ErrNoMsg = &KvaserError{Code: canERR_NOMSG, Description: "No message"}
	ErrArgs  = errors.New("error in arguments")
	// ErrNotConnected is returned when the bus holds no canlib handle.
	ErrNotConnected = errors.New("kvaser bus not connected")
)

func init() {
	can.RegisterInterface("kvaser", NewKvaserBus)
}

// KvaserBus drives one Kvaser channel through two CANlib handles: one that
// the reception goroutine blocks on, and one that Send writes on. A CANlib
// handle must not be used from several threads at once, and with a single
// handle a Send could wait behind the blocking read for up to its timeout,
// delaying e.g. an SDO block acknowledgement past the server's timeout.
// The reception goroutine also reads the channel state on its handle, between
// two reads, and State returns the last one.
type KvaserBus struct {
	rxHandle     int        // reception and channel state, owns the bus parameters; -1 when not open
	txHandle     int        // transmission; -1 when not open
	txMu         sync.Mutex // serializes use of txHandle: a CANlib handle must not be used from several threads
	stateMu      sync.Mutex // guards state and stateErr
	state        canopen.BusState
	stateErr     error
	logger       *slog.Logger
	rxCallback   canopen.FrameListener
	timeoutRead  int
	timeoutWrite int
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

type KvaserError struct {
	Code        int
	Description string
}

func (ke *KvaserError) Error() string {
	return fmt.Sprintf("%v (%v)", ke.Description, ke.Code)
}

func (ke *KvaserError) Is(target error) bool {
	t, ok := target.(*KvaserError)
	if !ok {
		return false
	}
	return ke.Code == t.Code
}

func NewKvaserError(code int) error {
	if code >= StatusOk {
		return nil
	}
	var msg [64]byte
	r1, _, _ := procGetErrorText.Call(
		uintptr(code),
		uintptr(unsafe.Pointer(&msg[0])),
		uintptr(len(msg)),
	)

	if int(int32(r1)) < StatusOk {
		return fmt.Errorf("unable to get description for error code %v", code)
	}

	cleanMsg := string(bytes.TrimRight(msg[:], "\x00"))
	return &KvaserError{Code: code, Description: cleanMsg}
}

// NewKvaserBus asks canlib to list the channels again, so that an adapter
// plugged in since canlib was initialized can be opened. The open channels
// are not affected, but the channel numbers may change.
func NewKvaserBus(name string) (bus canopen.Bus, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("kvaser: failed to initialize canlib32.dll: %v", r)
		}
	}()

	b := &KvaserBus{rxHandle: -1, txHandle: -1}
	b.setState(canopen.BusStateDisconnected, ErrNotConnected)
	b.timeoutRead = defaultReadTimeoutMs
	b.timeoutWrite = defaultWriteTimeoutMs
	b.logger = slog.Default()

	// canInitializeLibrary does nothing after the first call: re-enumerate
	// the channels so that adapters plugged in since are listed. Unloading
	// the library would do it too, but would close every open channel.
	procInitializeLibrary.Call()
	if err := procEnumHardwareEx.Find(); err != nil {
		b.logger.Warn("kvaser: canEnumHardwareEx not available, adapters plugged in after start-up are not listed; update canlib", "err", err)
		return b, nil
	}
	var nbChannels int32
	r1, _, _ := procEnumHardwareEx.Call(uintptr(unsafe.Pointer(&nbChannels)))
	if err := NewKvaserError(int(int32(r1))); err != nil {
		b.logger.Warn("kvaser: unable to re-enumerate the channels", "err", err)
	}
	return b, nil
}

// Open the reception and transmission handles on a channel, with specific flags.
// The handles share the channel, so flags must not include OpenExclusive.
func (k *KvaserBus) Open(channel int, flags int) error {
	// Send uses txHandle concurrently.
	k.txMu.Lock()
	defer k.txMu.Unlock()

	return k.open(channel, flags)
}

// open is Open for callers holding txMu.
func (k *KvaserBus) open(channel int, flags int) error {
	r1, _, _ := procOpenChannel.Call(uintptr(channel), uintptr(flags))
	err := NewKvaserError(int(int32(r1)))
	if err != nil {
		return err
	}
	k.rxHandle = int(int32(r1))

	r1, _, _ = procOpenChannel.Call(uintptr(channel), uintptr(flags))
	err = NewKvaserError(int(int32(r1)))
	if err != nil {
		procClose.Call(uintptr(k.rxHandle))
		k.rxHandle = -1
		return err
	}
	k.txHandle = int(int32(r1))
	return nil
}

func (k *KvaserBus) Connect(args ...any) error {
	if len(args) < 3 {
		return ErrArgs
	}
	channel, ok := args[1].(string)
	if !ok {
		return ErrArgs
	}
	bitrateHz, ok := args[2].(int)
	if !ok {
		return ErrArgs
	}
	bitrate, err := canlibBitrate(bitrateHz)
	if err != nil {
		return err
	}
	// Open flags are optional, after the bitrate
	flags := 0
	if len(args) > 3 {
		flags, ok = args[3].(int)
		if !ok {
			return ErrArgs
		}
	}
	channelNum, err := strconv.Atoi(channel)
	if err != nil {
		return ErrArgs
	}
	// Send must not use txHandle while it is being opened, configured, or
	// closed after a failed configuration.
	k.txMu.Lock()
	defer k.txMu.Unlock()

	if err := k.open(channelNum, flags); err != nil {
		return fmt.Errorf("open channel %d: %w", channelNum, err)
	}
	// Once opened, a failed configuration must release the handles, or the
	// channel stays claimed.
	connected := false
	defer func() {
		if !connected {
			procClose.Call(uintptr(k.txHandle))
			procClose.Call(uintptr(k.rxHandle))
			k.txHandle = -1
			k.rxHandle = -1
		}
	}()

	r1, _, _ := procSetBusParams.Call(
		uintptr(k.rxHandle),
		uintptr(uint32(bitrate)),
		0, 0, 0, 0, 0,
	)
	if err := NewKvaserError(int(int32(r1))); err != nil {
		return fmt.Errorf("set bus params (%d bit/s): %w", bitrateHz, err)
	}

	// Frames sent on the transmission handle would otherwise be echoed to the
	// reception handle, and handled as if another node had sent them.
	echo := byte(0)
	r1, _, _ = procIoCtl.Call(
		uintptr(k.rxHandle),
		uintptr(canIOCTL_SET_LOCAL_TXECHO),
		uintptr(unsafe.Pointer(&echo)),
		1,
	)
	if err := NewKvaserError(int(int32(r1))); err != nil {
		return fmt.Errorf("disable local tx echo: %w", err)
	}

	r1, _, _ = procSetBusOutputControl.Call(uintptr(k.txHandle), uintptr(canDRIVER_NORMAL))
	if err := NewKvaserError(int(int32(r1))); err != nil {
		return fmt.Errorf("set bus output control: %w", err)
	}

	if err := k.On(); err != nil {
		return fmt.Errorf("bus on: %w", err)
	}
	connected = true
	// The reception goroutine is not started yet: rxHandle is free.
	k.pollState()

	var ctx context.Context
	ctx, k.cancel = context.WithCancel(context.Background())
	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		k.processIncoming(ctx)
	}()

	return nil
}

func (k *KvaserBus) Disconnect() error {
	if k.cancel != nil {
		k.cancel()
		k.wg.Wait()
	}
	k.setState(canopen.BusStateDisconnected, ErrNotConnected)
	k.txMu.Lock()
	defer k.txMu.Unlock()
	if k.txHandle < 0 && k.rxHandle < 0 {
		return nil
	}
	k.Off()
	r1, _, _ := procClose.Call(uintptr(k.txHandle))
	txErr := NewKvaserError(int(int32(r1)))
	r1, _, _ = procClose.Call(uintptr(k.rxHandle))
	k.txHandle = -1
	k.rxHandle = -1
	return errors.Join(NewKvaserError(int(int32(r1))), txErr)
}

func (k *KvaserBus) Send(frame canopen.Frame) error {
	k.txMu.Lock()
	defer k.txMu.Unlock()
	if k.txHandle < 0 {
		return ErrNotConnected
	}

	id, flags := toCanlibId(frame.ID)
	r1, _, _ := procWrite.Call(
		uintptr(k.txHandle),
		uintptr(id),
		uintptr(unsafe.Pointer(&frame.Data[0])),
		uintptr(frame.DLC),
		uintptr(flags),
	)

	err := NewKvaserError(int(int32(r1)))
	if err != nil {
		return err
	}

	r1, _, _ = procWriteSync.Call(uintptr(k.txHandle), uintptr(k.timeoutWrite))
	return NewKvaserError(int(int32(r1)))
}

func (k *KvaserBus) Subscribe(callback canopen.FrameListener) error {
	k.rxCallback = callback
	return nil
}

// processIncoming receives the frames on rxHandle, and reads the channel state
// between two reads, at most every statePollPeriod.
func (k *KvaserBus) processIncoming(ctx context.Context) {
	k.logger.Info("handling reception")
	lastPoll := time.Now()
	for {
		select {
		case <-ctx.Done():
			k.logger.Info("exiting CAN bus reception, closed")
			return
		default:
			if time.Since(lastPoll) >= statePollPeriod {
				k.pollState()
				lastPoll = time.Now()
			}
			frame, err := k.Recv()
			if err != nil {
				if errors.Is(err, ErrNoMsg) {
					continue
				}
				k.logger.Error("listening routine has closed because", "err", err)
				// Nothing reads the state anymore, e.g. the device was unplugged.
				k.setState(canopen.BusStateDisconnected, err)
				return
			}
			if k.rxCallback != nil {
				k.rxCallback.Handle(frame)
			}
		}
	}
}

// Read a single CAN frame with a timeout
func (k *KvaserBus) Recv() (canopen.Frame, error) {
	var id int32
	var data [8]byte
	var dlc uint32
	var flags uint32
	var time uint32

	r1, _, _ := procReadWait.Call(
		uintptr(k.rxHandle),
		uintptr(unsafe.Pointer(&id)),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(unsafe.Pointer(&dlc)),
		uintptr(unsafe.Pointer(&flags)),
		uintptr(unsafe.Pointer(&time)),
		uintptr(k.timeoutRead),
	)

	err := NewKvaserError(int(int32(r1)))
	if err != nil {
		return canopen.Frame{}, err
	}

	frame := canopen.NewFrame(fromCanlibId(uint32(id), flags), 0, uint8(dlc))
	frame.Data = data
	return frame, nil
}

// Turn bus On, on all handles
func (k *KvaserBus) On() error {
	r1, _, _ := procBusOn.Call(uintptr(k.rxHandle))
	if err := NewKvaserError(int(int32(r1))); err != nil {
		return err
	}
	r1, _, _ = procBusOn.Call(uintptr(k.txHandle))
	return NewKvaserError(int(int32(r1)))
}

// Turn bus Off, on all handles
func (k *KvaserBus) Off() error {
	r1, _, _ := procBusOff.Call(uintptr(k.txHandle))
	txErr := NewKvaserError(int(int32(r1)))
	r1, _, _ = procBusOff.Call(uintptr(k.rxHandle))
	return errors.Join(NewKvaserError(int(int32(r1))), txErr)
}

// Get canlib version as a string X.Y
func GetVersion() string {
	r1, _, _ := procGetVersion.Call()
	version := int(r1)
	low := version & 0xFF
	high := version >> 8
	return fmt.Sprintf("%v.%v", high, low)
}

// Get number of channels, also counts virtual channels
func GetNbChannels() int {
	var nb int32
	procGetNumberOfChannels.Call(uintptr(unsafe.Pointer(&nb)))
	return int(nb)
}

// pollState reads the channel state on rxHandle: only the reception goroutine,
// or Connect before starting it, may call it. canReadStatus returns the last
// state the device reported, and keeps answering once it is unplugged:
// canRequestChipStatus asks the device for a new report, read on the next
// poll, and fails once it is unplugged.
func (k *KvaserBus) pollState() {
	r1, _, _ := procRequestChipStatus.Call(uintptr(k.rxHandle))
	if err := NewKvaserError(int(int32(r1))); err != nil {
		k.setState(canopen.BusStateDisconnected, fmt.Errorf("request chip status: %w", err))
		return
	}
	var flags uint32
	r1, _, _ = procReadStatus.Call(uintptr(k.rxHandle), uintptr(unsafe.Pointer(&flags)))
	if err := NewKvaserError(int(int32(r1))); err != nil {
		k.setState(canopen.BusStateUnknown, fmt.Errorf("read status: %w", err))
		return
	}
	k.logger.Debug("canReadStatus", "flags", fmt.Sprintf("0x%08x", flags))
	state := statusToState(flags)
	k.setState(state, nil)
	if state == canopen.BusStateHwOverflow {
		// The overrun flags stay latched until cleared: clear them so that the
		// next poll reports error active again unless a new overrun occurred.
		// The error counters are cleared too, but they are below the warning
		// limit here, or the state would be more severe than an overflow.
		r1, _, _ = procIoCtl.Call(uintptr(k.rxHandle), uintptr(canIOCTL_CLEAR_ERROR_COUNTERS), 0, 0)
		if err := NewKvaserError(int(int32(r1))); err != nil {
			k.logger.Warn("unable to clear the overrun flags", "err", err)
		}
	}
}

func (k *KvaserBus) setState(state canopen.BusState, err error) {
	k.stateMu.Lock()
	defer k.stateMu.Unlock()
	k.state = state
	k.stateErr = err
}

// State returns the state of the channel last read by the reception goroutine,
// at most statePollPeriod or a read timeout old. It is BusStateDisconnected
// once the device is unplugged or the bus disconnected.
func (k *KvaserBus) State() (canopen.BusState, error) {
	k.stateMu.Lock()
	defer k.stateMu.Unlock()
	return k.state, k.stateErr
}
