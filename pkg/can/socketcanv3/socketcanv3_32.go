//go:build 386 || arm || mips || mipsle || ppc

package socketcanv3

import "golang.org/x/sys/unix"

// Mmsghdr is a Go representation of the C struct mmsghdr (does not exist in golang.org/x/sys/unix)
// Hdr = 28 bytes
// Len = 4 bytes
// 0 padding to reach 32 bytes alignment
type Mmsghdr struct {
	Hdr unix.Msghdr
	Len uint32
	pad [4]byte
}
