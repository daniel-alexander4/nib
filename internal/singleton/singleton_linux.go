//go:build linux

// Package singleton lets a launch replace any already-running Nib instance,
// so clicking the menu item kills the previous window's server and starts anew.
package singleton

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// ReplaceOthers sends SIGTERM to every other process running this same
// executable, leaving the current process. It returns the number signalled.
func ReplaceOthers() int {
	self, err := os.Executable()
	if err != nil {
		return 0
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	me := os.Getpid()

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	killed := 0
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == me {
			continue
		}
		exe, err := filepath.EvalSymlinks("/proc/" + e.Name() + "/exe")
		if err != nil || exe != self {
			continue
		}
		if syscall.Kill(pid, syscall.SIGTERM) == nil {
			killed++
		}
	}
	return killed
}
