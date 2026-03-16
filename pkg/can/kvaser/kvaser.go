//go:build windows

package kvaser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"syscall"
	"unsafe"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/can"
)

// Dynamically load Kvaser's canlib32.dll
var (
	canlib = syscall.NewLazyDLL("canlib32.dll")

	procInitializeLibrary   = canlib.NewProc("canInitializeLibrary")
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
)

const (
	defaultReadTimeoutMs  = 500
	defaultWriteTimeoutMs = defaultReadTimeoutMs
)

// Kvaser Constants (Mapped from canlib.h)
// As we are not using CGO, we need to manually set the equivalent
// canlib enums. A lot of these enums are missing here, only the ones
// being used have been copied.
const (
	StatusOk         = 0
	canERR_NOMSG     = -2
	canDRIVER_NORMAL = 4
	canMSG_STD       = 2

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

	Bitrate125k = -4
	Bitrate250k = -3
	Bitrate500k = -2
	Bitrate1M   = -1
)

var (
	ErrNoMsg = NewKvaserError(canERR_NOMSG)
	ErrArgs  = errors.New("error in arguments")
)

func init() {
	can.RegisterInterface("kvaser", NewKvaserBus)
}

type KvaserBus struct {
	handle       int
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

	if int(r1) < StatusOk {
		return fmt.Errorf("unable to get description for error code %v", code)
	}

	cleanMsg := string(bytes.TrimRight(msg[:], "\x00"))
	return &KvaserError{Code: code, Description: cleanMsg}
}

func NewKvaserBus(name string) (canopen.Bus, error) {
	bus := &KvaserBus{}
	bus.timeoutRead = defaultReadTimeoutMs
	bus.timeoutWrite = defaultWriteTimeoutMs
	bus.logger = slog.Default()

	procInitializeLibrary.Call()
	return bus, nil
}

// Open channel with specific flags
func (k *KvaserBus) Open(channel int, flags int) error {
	r1, _, _ := procOpenChannel.Call(uintptr(channel), uintptr(flags))
	err := NewKvaserError(int(r1))
	if err != nil {
		return err
	}
	k.handle = int(r1)
	return nil
}

func (k *KvaserBus) Connect(args ...any) error {
	if len(args) < 2 {
		return ErrArgs
	}
	channel, ok := args[0].(int)
	if !ok {
		return ErrArgs
	}
	flags, ok := args[1].(int)
	if !ok {
		return ErrArgs
	}

	err := k.Open(channel, flags)
	if err != nil {
		return err
	}

	bitrate := int32(Bitrate500k)

	r1, _, _ := procSetBusParams.Call(
		uintptr(k.handle),
		uintptr(uint32(bitrate)),
		0, 0, 0, 0, 0,
	)
	err = NewKvaserError(int(r1))
	if err != nil {
		return err
	}

	r1, _, _ = procSetBusOutputControl.Call(uintptr(k.handle), uintptr(canDRIVER_NORMAL))
	err = NewKvaserError(int(r1))
	if err != nil {
		return err
	}

	err = k.On()
	if err != nil {
		return err
	}

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
	k.Off()
	r1, _, _ := procClose.Call(uintptr(k.handle))
	return NewKvaserError(int(r1))
}

func (k *KvaserBus) Send(frame canopen.Frame) error {
	r1, _, _ := procWrite.Call(
		uintptr(k.handle),
		uintptr(frame.ID),
		uintptr(unsafe.Pointer(&frame.Data[0])),
		uintptr(frame.DLC),
		uintptr(canMSG_STD),
	)

	err := NewKvaserError(int(r1))
	if err != nil {
		return err
	}

	r1, _, _ = procWriteSync.Call(uintptr(k.handle), uintptr(k.timeoutWrite))
	return NewKvaserError(int(r1))
}

func (k *KvaserBus) Subscribe(callback canopen.FrameListener) error {
	k.rxCallback = callback
	return nil
}

func (k *KvaserBus) processIncoming(ctx context.Context) {
	k.logger.Info("handling reception")
	for {
		select {
		case <-ctx.Done():
			k.logger.Info("exiting CAN bus reception, closed")
			return
		default:
			frame, err := k.Recv()
			if err != nil {
				if errors.Is(err, ErrNoMsg) {
					continue
				}
				k.logger.Error("listening routine has closed because", "err", err)
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
		uintptr(k.handle),
		uintptr(unsafe.Pointer(&id)),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(unsafe.Pointer(&dlc)),
		uintptr(unsafe.Pointer(&flags)),
		uintptr(unsafe.Pointer(&time)),
		uintptr(k.timeoutRead),
	)

	err := NewKvaserError(int(r1))
	if err != nil {
		return canopen.Frame{}, err
	}

	frame := canopen.NewFrame(uint32(id), 0, uint8(dlc))
	frame.Data = data
	return frame, nil
}

// Turn bus On
func (k *KvaserBus) On() error {
	r1, _, _ := procBusOn.Call(uintptr(k.handle))
	return NewKvaserError(int(r1))
}

// Turn bus Off
func (k *KvaserBus) Off() error {
	r1, _, _ := procBusOff.Call(uintptr(k.handle))
	return NewKvaserError(int(r1))
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
