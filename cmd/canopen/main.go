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

var NODE_ID uint8 = 0x20

func main() {

	log.SetLevel(logrus.DebugLevel)

	bus, e := can.NewBusForInterfaceWithName("vcan0")
	if e != nil {
		fmt.Println(e)
		return
	}

	socketcanbus := canopen.SocketCANBus{Bus: bus}

	canmodule := &canopen.CANModule{}
	canmodule.Init(&socketcanbus)

	// CanModule is the message broker
	bus.Subscribe(canmodule)

	go bus.ConnectAndPublish()

	// First load the EDS
	od, err := canopen.ParseEDS("../../base.eds", NODE_ID)
	if err != nil {
		log.Panicf("Error encountered when loading EDS : %v", err)
	}

	node := canopen.Node{Config: nil, CANModule: canmodule, NMT: nil}
	err = node.Init(nil, nil, od, nil, canopen.CO_NMT_STARTUP_TO_OPERATIONAL, 500, 0, 0, true, NODE_ID)

	if err != nil {
		log.Panicf("Failed Initializing Node because %v", err)
	}

	var time_difference_us uint32

	start := time.Now()
	var timer_next_us uint32 = 1000 // i.e 1 ms

	// client := node.SDOclients[0]

	go func() {
		var tmrNextUs uint32 = 1000
		for {
			elapsed := time.Since(start)
			start = time.Now()
			time_difference_us = uint32(elapsed.Microseconds())
			node.ProcessSYNC(time_difference_us, &tmrNextUs)
			//fmt.Printf("Timer next %v ; Elapsed %v", timer_next_us, time_difference_us)
			time.Sleep(time.Duration(timer_next_us) * time.Microsecond)
		}
	}()

	counter := 0
	for {
		counter += 1
		elapsed := time.Since(start)
		start = time.Now()
		time_difference_us = uint32(elapsed.Microseconds())
		node.Process(false, time_difference_us, &timer_next_us)
		//fmt.Printf("Timer next %v ; Elapsed %v", timer_next_us, time_difference_us)
		time.Sleep(time.Duration(timer_next_us) * time.Microsecond)

		//_ = client.WriteRaw(0x10, 0x2000, 0x0, data, true)
		// reader := canopen.NewBlockReader(0x10, 0x1021, 0x0, &client)
		// data, _ := reader.ReadAll()
		// // _, err = client.ReadRaw(0x10, 0x1021, 0x0, data)
		// log.Infof("Read back %v", data)

		// // _, err = f.Write(data)
		// // if err != nil {
		// // 	fmt.Print("Error occurred when writing to file")
		// // }

		// // Write to a file
		// buf := bytes.NewReader(data)
		// w, _ := zip.NewReader(buf, int64(len(data)))
		// f, err := w.File[0].Open()
		// if err != nil {
		// 	panic(err)
		// }
		// unzipped_data, _ := io.ReadAll(f)
		// os.WriteFile("dictionnary.eds", unzipped_data, 0644)

		// if err != nil {
		// 	panic(err)
		// }

		// return
		// if err != nil {
		// 	log.Errorf("Error reading %v", err)
		// }
		//once = false

		// if counter >= 2000 && sent == false {
		// 	node.NMT.SendInternalCommand(canopen.CO_NMT_ENTER_PRE_OPERATIONAL)
		// 	counter = 0
		// 	sent = true
		// }

	}
}
