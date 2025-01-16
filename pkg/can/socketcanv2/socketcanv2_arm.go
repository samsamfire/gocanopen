//go:build arm

package socketcanv2

import (
	"golang.org/x/sys/unix"
)

var DefaultTimeVal = unix.Timeval{
	Sec:  int32(0),
	Usec: int32(100_000),
}
