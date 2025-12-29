package canopen

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"golang.org/x/sys/unix"
)

const (
	// Max Standard CAN ID is 0x7FF (2047).
	MaxCanId = 0x7FF

	// The array must hold standard frames + RTR frames (so 2x size)
	LookupArraySize = (MaxCanId + 1) * 2
)

type subscriber struct {
	id       uint64
	callback FrameListener
}

// Bus manager is a wrapper around the CAN bus interface
// Used by the CANopen stack to control errors, callbacks for specific IDs, etc.
type BusManager struct {
	logger *slog.Logger
	mu     sync.Mutex
	bus    Bus
	// CAN id indexed subscribers
	listeners [LookupArraySize][]subscriber
	nextSubId uint64
	canError  uint16
}

// Implements the FrameListener interface
// This handles all received CAN frames from Bus
// [listener.Handle] should not be blocking !
func (bm *BusManager) Handle(frame Frame) {

	canId := frame.ID & unix.CAN_SFF_MASK
	if canId >= LookupArraySize {
		return
	}

	bm.mu.Lock()
	listeners := bm.listeners[canId]
	bm.mu.Unlock()

	for _, sub := range listeners {
		sub.callback.Handle(frame)
	}
}

// Set bus
func (bm *BusManager) SetBus(bus Bus) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.bus = bus
}

func (bm *BusManager) Bus() Bus {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return bm.bus
}

// Send a CAN message
// Limited error handling
func (bm *BusManager) Send(frame Frame) error {
	err := bm.bus.Send(frame)
	if err != nil {
		bm.logger.Warn("error sending frame", "err", err)
	}
	return err
}

// This should be called cyclically to update errors
func (bm *BusManager) Process() error {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	// TODO get bus state error
	bm.canError = 0
	return nil
}

// Subscribe to a specific CAN ID
// Returns a cancel func to remove subscription
func (bm *BusManager) Subscribe(ident uint32, mask uint32, rtr bool, callback FrameListener) (cancel func(), err error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if int(ident) >= len(bm.listeners) {
		return nil, errors.New("array-based manager only supports Standard 11-bit IDs")
	}

	idx := ident
	if rtr {
		// Offset by 2048 for RTR frames
		idx += MaxCanId + 1
	}

	bm.nextSubId++
	subId := bm.nextSubId
	bm.listeners[idx] = append(bm.listeners[idx], subscriber{
		id:       subId,
		callback: callback,
	})

	cancel = func() {
		bm.mu.Lock()
		defer bm.mu.Unlock()

		subs := bm.listeners[idx]
		for i, sub := range subs {
			if sub.id == subId {
				bm.listeners[idx] = append(subs[:i], subs[i+1:]...)
				return
			}
		}
	}

	return cancel, nil
}

// Unsubscribe from a specific CAN ID
func (bm *BusManager) Unsubscribe(ident uint32, mask uint32, rtr bool, callback FrameListener) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	ident = ident & CanSffMask
	if rtr {
		ident |= CanRtrFlag
	}
	_, ok := bm.frameListeners[ident]
	if !ok {
		return fmt.Errorf("no registerd callbacks for id %v", ident)
	}
	// Iterate over callbacks and remove corresponding one
	callbacks := bm.frameListeners[ident]

	for i, cb := range callbacks {
		if cb == callback {
			bm.frameListeners[ident] = append(callbacks[:i], callbacks[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("callback not found for id %v", ident)
}

// Get CAN error
func (bm *BusManager) Error() uint16 {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return bm.canError
}

func NewBusManager(bus Bus) *BusManager {
	bm := &BusManager{
		bus:       bus,
		logger:    slog.Default(),
		listeners: [LookupArraySize][]subscriber{},
		canError:  0,
	}
	return bm
}
