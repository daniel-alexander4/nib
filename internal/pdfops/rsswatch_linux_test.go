//go:build linux

package pdfops

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// TestMain's sibling: when asked, this binary allocates and TOUCHES the given number of MiB and sleeps —
// the stand-in for a converter that balloons.
func init() {
	if v := os.Getenv("NIB_TEST_HOG_MIB"); v != "" {
		mib, _ := strconv.Atoi(v)
		b := make([]byte, mib<<20)
		for i := 0; i < len(b); i += 4096 {
			b[i] = 1
		}
		time.Sleep(time.Minute)
		_ = b
		os.Exit(0)
	}
}

func hog(t *testing.T, mib int) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=NONE")
	cmd.Env = append(os.Environ(), "NIB_TEST_HOG_MIB="+strconv.Itoa(mib))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); _, _ = cmd.Process.Wait() })
	return cmd
}

func TestTheWatchdogKillsAGroupThatPassesItsCeiling(t *testing.T) {
	cmd := hog(t, 400)
	stop := watchMemory(cmd.Process.Pid, 150<<20, 50*time.Millisecond)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("a process 250 MiB past its ceiling was not stopped")
	}
	killed, peak := stop()
	if !killed || peak <= 150<<20 {
		t.Fatalf("killed=%v peak=%d MiB, want killed past the 150 MiB ceiling", killed, peak>>20)
	}
}

func TestTheWatchdogLeavesAGroupUnderItsCeilingAlone(t *testing.T) {
	cmd := hog(t, 60)
	stop := watchMemory(cmd.Process.Pid, 1<<30, 50*time.Millisecond)
	time.Sleep(1500 * time.Millisecond)
	killed, peak := stop()
	if killed {
		t.Fatal("a 60 MiB process under a 1 GiB ceiling was killed")
	}
	if peak < 40<<20 {
		t.Fatalf("the watchdog saw a peak of %d MiB for a process holding 60 MiB; it is not reading the group", peak>>20)
	}
	if err := syscall.Kill(cmd.Process.Pid, 0); err != nil {
		t.Fatalf("the process is gone: %v", err)
	}
}

func TestStoppingTheWatchdogTwiceIsSafe(t *testing.T) {
	cmd := hog(t, 10)
	stop := watchMemory(cmd.Process.Pid, 1<<30, 20*time.Millisecond)
	stop()
	stop()
}

func TestTheCeilingSiblingsAreHonest(t *testing.T) {
	if !CeilingEnforced || CeilingUnenforcedNote != "" {
		t.Fatal("on Linux the ceiling is enforced and carries no note")
	}
}
