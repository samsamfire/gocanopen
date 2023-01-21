package main

import (
	"canopen"
	"fmt"
	"time"

	"github.com/brutella/can"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
)

func handleCANFrame(frame can.Frame) {
	fmt.Println(frame)
}

func main() {

	log.SetLevel(logrus.DebugLevel)

	bus, e := can.NewBusForInterfaceWithName("vcan0")
	if e != nil {
		fmt.Println(e)
		return
	}

	socketcanbus := canopen.SocketCANBus{Bus: bus}

	canmodule := &canopen.CANModule{}
	rxBuffer := make([]canopen.BufferRxFrame, 100)
	txBuffer := make([]canopen.BufferTxFrame, 100)
	canmodule.Init(&socketcanbus, rxBuffer, txBuffer)

	// CanModule is the message broker
	bus.Subscribe(canmodule)

	//go bus.ConnectAndPublish()

	// First load the EDS
	od, err := canopen.ParseEDS("../../base.eds", 0x10)
	if err != nil {
		log.Panicf("Error encountered when loading EDS : %v", err)
	}

	node := canopen.Node{Config: nil, CANModule: canmodule, NMT: nil}
	err = node.Init(nil, nil, od, nil, canopen.CO_NMT_STARTUP_TO_OPERATIONAL, 500, 0, 0, true, 0x10)

	if err != nil {
		log.Panicf("Failed Initializing Node because %v", err)
	}

	var time_difference_us uint32

	start := time.Now()
	var timer_next_us uint32 = 1000 // i.e 1 ms

	counter := 0
	sent := false
	for {
		counter += 1
		elapsed := time.Since(start)
		start = time.Now()
		time_difference_us = uint32(elapsed.Microseconds())
		node.Process(false, time_difference_us, &timer_next_us)
		//fmt.Printf("Timer next %v ; Elapsed %v", timer_next_us, time_difference_us)
		time.Sleep(time.Duration(timer_next_us) * time.Microsecond)

		if counter >= 2000 && sent == false {
			node.NMT.SendInternalCommand(canopen.CO_NMT_ENTER_PRE_OPERATIONAL)
			counter = 0
			sent = true
		}

	}
}
