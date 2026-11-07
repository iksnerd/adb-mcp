//go:build !windows

package gradle

import (
	"os"
	"syscall"
)

// ownedByCurrentUser reports whether info belongs to the user running the
// server. Where ownership cannot be read it errs on the side of accepting.
func ownedByCurrentUser(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return !ok || int(st.Uid) == os.Getuid()
}
