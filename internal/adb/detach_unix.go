//go:build !windows

package adb

import (
	"os/exec"
	"syscall"
)

// detach puts the command in its own session so it survives the MCP server
// process exiting — an emulator we boot must outlive the tool call that
// launched it.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
