//go:build windows

package gradle

import "os"

// ownedByCurrentUser is not checked on Windows, where the temp dir is
// per-user already.
func ownedByCurrentUser(os.FileInfo) bool { return true }
