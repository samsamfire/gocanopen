package all

import (
	_ "github.com/samsamfire/gocanopen/pkg/can/kvaser"
	_ "github.com/samsamfire/gocanopen/pkg/can/socketcanring"
	_ "github.com/samsamfire/gocanopen/pkg/can/socketcanv2"
	_ "github.com/samsamfire/gocanopen/pkg/can/socketcanv3"
	_ "github.com/samsamfire/gocanopen/pkg/can/virtual"
)
