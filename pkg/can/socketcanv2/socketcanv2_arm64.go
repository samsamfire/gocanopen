//go:build arm64 && linux

package socketcanv2

import (
	"golang.org/x/sys/unix"
)

var DefaultTimeVal = unix.Timeval{
	Sec:  int64(0),
	Usec: int64(100_000),
}
