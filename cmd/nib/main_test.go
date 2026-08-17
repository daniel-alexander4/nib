package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestLoopbackBind(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:0", true},
		{"127.0.0.1:8791", true},
		{"localhost:8791", true},
		{"[::1]:8791", true},
		{"0.0.0.0:8791", false},
		{":8791", false},
		{"192.168.1.10:8791", false},
		{"example.com:8791", false},
		{"127.0.0.1", false}, // missing port
		{"", false},
	}
	for _, c := range cases {
		if got := loopbackBind(c.addr); got != c.want {
			t.Errorf("loopbackBind(%q) = %v, want %v", c.addr, got, c.want)
		}
	}
}

// TestReplaceIsAcceptedAndHarmless — the flag must keep parsing, forever.
//
// P07 retired replace-and-kill, and the tempting last step is to delete the flag along
// with the behaviour. That step is the defect: an older installed `build/nib.desktop`
// still passes `--replace`, Go's flag package exits non-zero on an unknown flag, and a
// double-click has no terminal to show the error in. The user's app simply stops
// starting, for a reason nothing surfaces.
//
// Driven against a REAL BINARY rather than by reading main.go, because what is asserted
// is what the flag package does with an argument — which a source scan cannot tell you.
//
// The observable is the instance record: reaching the point where it is published means
// the process parsed its flags, bound a port, and started serving. An unknown flag exits
// 2 long before that.
//
// A first draft ran `nib --replace version`, expecting `version` to be a headless
// subcommand that prints and exits. `cli.Run` reads argv[0] only — "--replace" is not a
// subcommand — so the launch fell through to the desktop boot and the test hung on a
// server that never exits. Recorded because the hang read as a build problem and was a
// test bug.
func TestReplaceIsAcceptedAndHarmless(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a binary")
	}
	bin := filepath.Join(t.TempDir(), "nib-flagcheck")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("building nib: %v\n%s", err, out)
	}

	start := func(args ...string) (*exec.Cmd, string) {
		t.Helper()
		home, cfg := t.TempDir(), t.TempDir()
		c := exec.Command(bin, args...)
		c.Env = append(os.Environ(),
			"HOME="+home, "XDG_CONFIG_HOME="+cfg, "NIB_NO_BROWSER=1")
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = c.Process.Signal(syscall.SIGTERM)
			_, _ = c.Process.Wait()
		})
		return c, filepath.Join(cfg, "nib", "instance.json")
	}
	served := func(rec string) bool {
		for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); {
			if _, err := os.Stat(rec); err == nil {
				return true
			}
			time.Sleep(50 * time.Millisecond)
		}
		return false
	}

	// The stimulus: a plain launch must reach serving, or "the flagged one did" says
	// nothing about the flag.
	if _, rec := start(); !served(rec) {
		t.Fatal("setup: a plain launch never published its instance record, so this test cannot isolate the flag")
	}

	if _, rec := start("--replace"); !served(rec) {
		t.Fatal("`nib --replace` never started serving — an older installed desktop entry passes this flag, and a launch that cannot parse it stops starting with an error a double-click never shows")
	}
}
