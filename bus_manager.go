package canopen

import (
	can "github.com/samsamfire/gocanopen/pkg/can"
	log "github.com/sirupsen/logrus"
)

// Bus manager is a wrapper around the CAN bus interface
// Used by the CANopen stack to control errors, callbacks for specific IDs, etc.
type busManager struct {
	bus            can.Bus // Bus interface that can be adapted
	frameListeners map[uint32][]can.FrameListener
	canError       uint16
}

// Implements the FrameListener interface
// This handles all received CAN frames from Bus
func (bm *busManager) Handle(frame can.Frame) {
	listeners, ok := bm.frameListeners[frame.ID]
	if !ok {
		return
	}
	for _, listener := range listeners {
		listener.Handle(frame)
	}
}

// Send a CAN message
// Limited error handling
func (bm *busManager) Send(frame can.Frame) error {
	err := bm.bus.Send(frame)
	if err != nil {
		log.Warnf("[CAN ERROR] %v", err)
	}
	return err
}

// This should be called cyclically to update errors
func (bm *busManager) process() error {
	// TODO get bus state error
	bm.canError = 0
	return nil
}

// Subscribe to a specific CAN ID
func (bm *busManager) Subscribe(ident uint32, mask uint32, rtr bool, callback can.FrameListener) error {
	ident = ident & can.CanSffMask
	if rtr {
		ident |= can.CanRtrFlag
	}
	_, ok := bm.frameListeners[ident]
	if !ok {
		bm.frameListeners[ident] = []can.FrameListener{callback}
		return nil
	}
	// TODO add error if callback exists already
	bm.frameListeners[ident] = append(bm.frameListeners[ident], callback)
	return nil
}

func NewBusManager(bus can.Bus) *busManager {
	bm := &busManager{
		bus:            bus,
		frameListeners: make(map[uint32][]can.FrameListener),
		canError:       0,
	}
	return bm
}
