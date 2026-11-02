//go:build !windows

package pdfops

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// reopenConverters undoes KillConverters for the next test.
func reopenConverters(t *testing.T) {
	t.Helper()
	converters.mu.Lock()
	converters.closed = false
	converters.mu.Unlock()
}

func pidAlive(pid int) bool {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	i := strings.LastIndexByte(string(b), ')')
	return i >= 0 && len(b) > i+2 && b[i+2] != 'Z'
}

// A converter running when Nib quits is killed BEFORE Nib leaves: Quit, idle exit and SIGTERM do not
// cancel the conversion's context, so the registry is the only thing that can.
func TestKillConvertersEndsARunningConverterWithNoCancel(t *testing.T) {
	defer reopenConverters(t)
	cmd := exec.CommandContext(context.Background(), "sh", "-c", "sleep 300 >/dev/null 2>&1 & wait")
	done := make(chan error, 1)
	go func() {
		_, err := runConvert(context.Background(), cmd, "/nonexistent/never.pdf", "Fake", nil)
		done <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for LiveConverters() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the running converter never registered")
		}
		time.Sleep(10 * time.Millisecond)
	}
	pid := cmd.Process.Pid
	if n := KillConverters(); n != 1 {
		t.Fatalf("KillConverters signalled %d groups, want 1", n)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a killed converter reported success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runConvert did not return after its group was killed")
	}
	if pidAlive(pid) {
		t.Fatal("the converter's leader survived KillConverters")
	}
	if LiveConverters() != 0 {
		t.Fatal("a finished converter is still registered")
	}
}

func TestNoConverterStartsAfterKillConverters(t *testing.T) {
	defer reopenConverters(t)
	KillConverters()
	marker := t.TempDir() + "/started"
	cmd := exec.CommandContext(context.Background(), "sh", "-c", "touch "+marker)
	_, err := runConvert(context.Background(), cmd, marker, "Fake", nil)
	if !errors.Is(err, ErrShuttingDown) {
		t.Fatalf("runConvert after KillConverters = %v, want ErrShuttingDown", err)
	}
	if _, serr := os.Stat(marker); serr == nil {
		t.Fatal("a converter was started on a Nib that is leaving")
	}
}

func TestAConverterOutputPastTheCapIsRefusedBeforeItIsRead(t *testing.T) {
	out := t.TempDir() + "/big.pdf"
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxConvertedBytes + 1); err != nil { // sparse: no 200 MiB on disk
		t.Fatal(err)
	}
	f.Close()
	_, err = runConvert(context.Background(), exec.CommandContext(context.Background(), "true"), out, "Fake", nil)
	if !errors.Is(err, ErrConvertedTooLarge) {
		t.Fatalf("an oversized output = %v, want ErrConvertedTooLarge", err)
	}
}
