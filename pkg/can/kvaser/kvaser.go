package kvaser

/*
#cgo LDFLAGS: -lcanlib

#include <canlib.h>
*/
import "C"
import (
	"errors"
	"fmt"
	"unsafe"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/can"

	log "github.com/sirupsen/logrus"
)

const (
	defaultReadTimeoutMs  = 500
	defaultWriteTimeoutMs = defaultReadTimeoutMs
)

const (
	OpenExclusive         int = C.canOPEN_EXCLUSIVE           // Exclusive access
	OpenRequireExtended   int = C.canOPEN_REQUIRE_EXTENDED    // Fail if can't use extended mode
	OpenAcceptVirtual     int = C.canOPEN_ACCEPT_VIRTUAL      // Allow use of virtual CAN
	OpenOverrideExclusive int = C.canOPEN_OVERRIDE_EXCLUSIVE  // Open, even if in exclusive access
	OpenRequireInitAccess int = C.canOPEN_REQUIRE_INIT_ACCESS // Init access to bus
	OpenNoInitAccess      int = C.canOPEN_NO_INIT_ACCESS
	OpenAcceptLargeDlc    int = C.canOPEN_ACCEPT_LARGE_DLC
	OpenCanFd             int = C.canOPEN_CAN_FD
	OpenCanFdNonIso       int = C.canOPEN_CAN_FD_NONISO
	OpenInternalL         int = C.canOPEN_INTERNAL_L
)

const (
	StatusOk int = C.canOK
)

var (
	ErrNoMsg error = NewKvaserError(C.canERR_NOMSG)
	ErrArgs  error = errors.New("error in arguments")
)

func init() {
	can.RegisterInterface("kvaser", NewKvaserBus)
}

type KvaserBus struct {
	handle       C.canHandle
	rxCallback   canopen.FrameListener
	timeoutRead  int
	timeoutWrite int
	exit         chan bool
}

type KvaserError struct {
	Code        int
	Description string
}

func (ke *KvaserError) Error() string {
	return fmt.Sprintf("%v (%v)", ke.Description, ke.Code)
}

func NewKvaserError(code int) error {
	if code >= StatusOk {
		return nil
	}
	msg := [64]C.char{}
	status := int(C.canGetErrorText(C.canStatus(code), &msg[0], C.uint(unsafe.Sizeof(msg))))
	if status < StatusOk {
		return fmt.Errorf("unable to get description for error code %v (%v)", code, status)
	}
	return &KvaserError{Code: code, Description: C.GoString(&msg[0])}
}

func NewKvaserBus(name string) (canopen.Bus, error) {
	bus := &KvaserBus{}
	bus.timeoutRead = defaultReadTimeoutMs
	bus.timeoutWrite = defaultWriteTimeoutMs
	bus.exit = make(chan bool)
	// Call lib init, any error here is silent
	// and will happen when trying to open port
	// calling this multiple times has no effect.
	C.canInitializeLibrary()
	return bus, nil
}

// Open channel with specific flags
func (k *KvaserBus) Open(channel int, flags int) error {
	handle := C.canOpenChannel(C.int(channel), C.int(flags))
	err := NewKvaserError(int(handle))
	if err != nil {
		return err
	}
	k.handle = handle
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
	open, ok := args[1].(int)
	if !ok {
		return ErrArgs
	}
	err := k.Open(channel, open)
	if err != nil {
		return err
	}
	status := C.canSetBusParams(k.handle, C.canBITRATE_500K, 0, 0, 0, 0, 0)
	err = NewKvaserError(int(status))
	if err != nil {
		return err
	}
	status = C.canSetBusOutputControl(k.handle, C.canDRIVER_NORMAL)
	err = NewKvaserError(int(status))
	if err != nil {
		return err
	}
	return k.On()
}

func (k *KvaserBus) Disconnect() error {
	if k.rxCallback != nil {
		k.exit <- true
	}
	k.Off()
	status := C.canClose(k.handle)
	return NewKvaserError(int(status))
}

func (k *KvaserBus) Send(frame canopen.Frame) error {
	id := C.long(frame.ID)
	status := C.canWrite(k.handle, id, unsafe.Pointer(&frame.Data[0]), C.uint(frame.DLC), C.canMSG_STD)
	err := NewKvaserError(int(status))
	if err != nil {
		return err
	}
	status = C.canWriteSync(k.handle, defaultWriteTimeoutMs)
	return NewKvaserError(int(status))
}

func (k *KvaserBus) Subscribe(callback canopen.FrameListener) error {
	k.rxCallback = callback
	go k.handleReception()
	return nil
}

func (k *KvaserBus) handleReception() {
	fmt.Println("handling reception")
	for {
		select {
		case <-k.exit:
			return
		default:
			frame, err := k.Recv()
			if err != nil && err.Error() != ErrNoMsg.Error() {
				log.Errorf("[KVASER DRIVER] listening routine has closed because : %v", err)
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
	id := C.long(0)
	var data [8]byte
	dlc := C.uint(0)
	flags := C.uint(0)
	time := C.ulong(0)
	timeout := C.ulong(k.timeoutRead)

	status := C.canReadWait(k.handle, &id, unsafe.Pointer(&data), &dlc, &flags, &time, timeout)
	err := NewKvaserError(int(status))
	if err != nil {
		return canopen.Frame{}, err
	}
	frame := canopen.NewFrame(uint32(id), 0, uint8(dlc))
	frame.Data = data
	return frame, nil

}

// Turn bus On
func (k *KvaserBus) On() error {
	status := int(C.canBusOn(k.handle))
	return NewKvaserError(status)
}

// Turn bus Off
func (k *KvaserBus) Off() error {
	status := int(C.canBusOff(k.handle))
	return NewKvaserError(status)
}

// Get canlib version as a string X.Y
func GerVersion() string {
	version := C.canGetVersion()
	low := version & 0xFF
	high := version >> 8
	return fmt.Sprintf("%v.%v", high, low)

}

// Get number of channels, also counts virtual channels
func GetNbChannels() int {
	nb := C.int(0)
	C.canGetNumberOfChannels(&nb)
	return int(nb)
}
