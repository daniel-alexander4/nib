//go:build !windows

package pdfops

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// startedPID reads the pid a test's shell wrote, which is the stimulus: without it the descendant never
// started and nothing below is about one.
func startedPID(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("setup: the descendant never recorded its pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		t.Fatalf("setup: the recorded pid %q is not one", b)
	}
	return pid
}

func processAlive(pid int) bool {
	if st, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat"); err == nil {
		// Linux: a zombie still answers signal 0 and is not running.
		if f := strings.Fields(string(st)); len(f) > 2 && f[2] == "Z" {
			return false
		}
	}
	return !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

// TestAConversionTimeoutKillsTheConvertersChildren — `/pending 503`. `soffice` launches `soffice.bin`, and
// the timeout killed only the launcher: the child kept running, held the stderr pipe, and `Run` waited on it.
// The shell here is the launcher and the backgrounded sleep is `soffice.bin`.
func TestAConversionTimeoutKillsTheConvertersChildren(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh to stand in for a converter's launcher")
	}
	pidFile := filepath.Join(t.TempDir(), "pid")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, sh, "-c", `sleep 8 & echo $! > "$1"; wait`, "sh", pidFile)
	start := time.Now()
	_, err = runConvert(ctx, cmd, filepath.Join(t.TempDir(), "never.pdf"), "Fake")
	elapsed := time.Since(start)
	pid := startedPID(t, pidFile)
	defer syscall.Kill(pid, syscall.SIGKILL)

	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("runConvert returned %v, want the timeout", err)
	}
	for deadline := time.Now().Add(2 * time.Second); processAlive(pid) && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
	}
	if processAlive(pid) {
		t.Errorf("the converter's child (pid %d) is still running %v after the timeout returned — only the launcher was killed", pid, elapsed)
	}
}

// TestAConversionTimeoutReturnsWhenADescendantEscapesTheGroup — `/pending 503`, the bound behind the group
// kill. A descendant in its own session is out of the group's reach and still holds stderr; the timeout
// must return anyway rather than wait for it.
func TestAConversionTimeoutReturnsWhenADescendantEscapesTheGroup(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh to stand in for a converter's launcher")
	}
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("no setsid to start a descendant outside the converter's process group")
	}
	pidFile := filepath.Join(t.TempDir(), "pid")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, sh, "-c", `setsid sleep 8 & echo $! > "$1"; wait`, "sh", pidFile)
	start := time.Now()
	_, err = runConvert(ctx, cmd, filepath.Join(t.TempDir(), "never.pdf"), "Fake")
	elapsed := time.Since(start)
	pid := startedPID(t, pidFile)
	defer syscall.Kill(pid, syscall.SIGKILL)

	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("runConvert returned %v, want the timeout", err)
	}
	if limit := convertWaitDelay + 3*time.Second; elapsed > limit {
		t.Errorf("runConvert returned after %v, past %v: it waited on a descendant holding the stderr pipe", elapsed, limit)
	}
}
