//go:build !windows

package cli

import (
	"os"
	"syscall"
	"testing"
	"time"
)

// A command that may run an outside converter is interruptible: SIGINT cancels its context (which kills the
// converter's group and lets the directory removal run, ADR-185) instead of ending the process.
func TestInterruptContextIsCancelledBySIGINT(t *testing.T) {
	ctx, stop := interruptContext()
	defer stop()
	if ctx.Err() != nil {
		t.Fatal("cancelled before any signal")
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Skipf("cannot signal self: %v", err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("SIGINT did not cancel the command's context")
	}
}
