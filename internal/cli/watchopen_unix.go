//go:build !windows

package cli

import (
	"os/exec"
	"syscall"
)

// detachLaunch puts a launched Nib in a process group of its own, out of the terminal's
// foreground group, so the Ctrl-C that stops `nib watch --do open` does not also close the Nib
// window it opened.
func detachLaunch(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
