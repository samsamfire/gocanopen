package canopen

import (
	"errors"
	"sync"
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
	mu  sync.Mutex
	bus Bus
	// CAN id indexed subscribers
	listeners [LookupArraySize][]subscriber
	nextSubId uint64
	canError  uint16
}

// Implements the FrameListener interface
// This handles all received CAN frames from Bus
// [listener.Handle] should not be blocking !
func (bm *BusManager) Handle(frame Frame) {

	canId := frame.ID & MaxCanId
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

// Subscribe to a specific CAN ID, or to all the CAN IDs matching ident
// on the bits set in mask (e.g. 0x80 with mask 0x780 matches 0x80 to 0xFF)
// Returns a cancel func to remove subscription
func (bm *BusManager) Subscribe(ident uint32, mask uint32, rtr bool, callback FrameListener) (cancel func(), err error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if ident > MaxCanId {
		return nil, errors.New("array-based manager only supports Standard 11-bit IDs")
	}

	// Offset by 2048 for RTR frames
	offset := uint32(0)
	if rtr {
		offset = MaxCanId + 1
	}
	mask &= MaxCanId
	indexes := []uint32{}
	for id := uint32(0); id <= MaxCanId; id++ {
		if id&mask == ident&mask {
			indexes = append(indexes, id+offset)
		}
	}

	bm.nextSubId++
	subId := bm.nextSubId
	for _, idx := range indexes {
		bm.listeners[idx] = append(bm.listeners[idx], subscriber{
			id:       subId,
			callback: callback,
		})
	}

	cancel = func() {
		bm.mu.Lock()
		defer bm.mu.Unlock()

		for _, idx := range indexes {
			subs := bm.listeners[idx]
			for i, sub := range subs {
				if sub.id == subId {
					bm.listeners[idx] = append(subs[:i], subs[i+1:]...)
					break
				}
			}
		}
	}

	return cancel, nil
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
		listeners: [LookupArraySize][]subscriber{},
		canError:  0,
	}

	return bm
}
