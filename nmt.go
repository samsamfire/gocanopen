package canopen

type NMT_Control uint32

const (
	CO_NMT_ERR_REG_MASK            NMT_Control = 0x00FF
	CO_NMT_STARTUP_TO_OPERATIONAL  NMT_Control = 0x0100
	CO_NMT_ERR_ON_BUSOFF_HB        NMT_Control = 0x1000
	CO_NMT_ERR_ON_ERR_REG          NMT_Control = 0x2000
	CO_NMT_ERR_TO_STOPPED          NMT_Control = 0x4000
	CO_NMT_ERR_FREE_TO_OPERATIONAL NMT_Control = 0x8000
)

type NMT struct{}
