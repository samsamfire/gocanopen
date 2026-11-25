//go:build linux

package socketcanv3

import "golang.org/x/sys/unix"

// Mmsghdr is a Go representation of the C struct mmsghdr (does not exist in golang.org/x/sys/unix)
// Go pads the struct to the alignment of Hdr, like C does : 32 bytes on 32 bit
// architectures, 64 bytes on 64 bit architectures
type Mmsghdr struct {
	Hdr unix.Msghdr
	Len uint32
}
