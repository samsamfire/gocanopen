//go:build amd64 || arm64 || mips64 || mips64le || ppc64 || ppc64le || riscv64 || s390x

package socketcanv3

import (
	"golang.org/x/sys/unix"
)

// Mmsghdr is a Go representation of the C struct mmsghdr (does not exist in golang.org/x/sys/unix)
// Hdr = 56 bytes
// Len = 4 bytes
// 4 bytes padding to reach 64 bytes alignment
type Mmsghdr struct {
	Hdr unix.Msghdr
	Len uint32
	pad [4]byte
}
