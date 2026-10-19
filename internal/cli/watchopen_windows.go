//go:build windows

package cli

import (
	"os/exec"
	"syscall"
)

// detachLaunch starts a launched Nib in a process group of its own, so the console's Ctrl-C that
// stops `nib watch --do open` is not delivered to it. Compiled here, never run (declared):
// TestEveryPlatformCompiles builds it, and nothing on this machine starts it.
func detachLaunch(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
