package main

import (
	"fmt"

	"github.com/brutella/can"
)

func main() {

	bus, e := can.NewBusForInterfaceWithName("vcan0")
	if e != nil {
		fmt.Println(e)
		return
	}

	fmt.Println("OK")
	//bus.ConnectAndPublish()

	frm := can.Frame{
		ID:     0x701,
		Length: 1,
		Flags:  0,
		Res0:   0,
		Res1:   0,
		Data:   [8]uint8{0x05},
	}
	bus.Publish(frm)

	fmt.Println("Hello there!")
}
