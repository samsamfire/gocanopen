package pdo

import (
	"testing"

	canopen "github.com/samsamfire/gocanopen"
	"github.com/samsamfire/gocanopen/pkg/can/virtual"
	"github.com/samsamfire/gocanopen/pkg/emergency"
	"github.com/samsamfire/gocanopen/pkg/od"
	"github.com/stretchr/testify/assert"
)

func BenchmarkXxx(b *testing.B) {
	b.StopTimer()
	bus, err := virtual.NewVirtualCanBus("localhost:18888")
	bus.Connect()
	assert.Nil(b, err)
	bm := canopen.NewBusManager(bus)
	od := od.Default()
	tpdo, err := NewTPDO(bm, od, &emergency.EMCY{}, nil, od.Index(0x1801), od.Index(0x1A01), 0)
	assert.Nil(b, err)
	b.StartTimer()
	for n := 0; n < b.N; n++ {
		err := tpdo.send()
		assert.Nil(b, err)
	}

}
