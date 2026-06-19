//go:build unix

package realfs

import "golang.org/x/sys/unix"

const sysOpenNoFollow = unix.O_NOFOLLOW
