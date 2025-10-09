package canopen

import (
	"log/slog"
	"sync"
)

// Bus manager is a wrapper around the CAN bus interface
// Used by the CANopen stack to control errors, callbacks for specific IDs, etc.
type BusManager struct {
	logger         *slog.Logger
	mu             sync.Mutex
	bus            Bus // Bus interface that can be adapted
	frameListeners map[uint32][]FrameListener
	canError       uint16
}

// Implements the FrameListener interface
// This handles all received CAN frames from Bus
// [listener.Handle] should not be blocking !
func (bm *BusManager) Handle(frame Frame) {
	bm.mu.Lock()
	listeners, ok := bm.frameListeners[frame.ID]
	bm.mu.Unlock()

	if !ok {
		return
	}

	for _, listener := range listeners {
		listener.Handle(frame)
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
func (bm *BusManager) Subscribe(ident uint32, mask uint32, rtr bool, callback FrameListener) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	ident = ident & CanSffMask
	if rtr {
		ident |= CanRtrFlag
	}
	_, ok := bm.frameListeners[ident]
	if !ok {
		bm.frameListeners[ident] = []FrameListener{callback}
		return nil
	}
	// Iterate over all callbacks and verify that we are not adding the same one twice
	for _, cb := range bm.frameListeners[ident] {
		if cb == callback {
			bm.logger.Warn("callback for frame already present", "id", ident)
			return nil
		}
	}
	bm.frameListeners[ident] = append(bm.frameListeners[ident], callback)
	return nil
}

// Get CAN error
func (bm *BusManager) Error() uint16 {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return bm.canError
}

func NewBusManager(bus Bus) *BusManager {
	bm := &BusManager{
		bus:            bus,
		logger:         slog.Default(),
		frameListeners: make(map[uint32][]FrameListener),
		canError:       0,
	}
	return bm
}
