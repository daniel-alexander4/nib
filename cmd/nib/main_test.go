package main

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"nib/internal/instance"
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

// TestAStaleRecordIsTakenOverWithoutUserAction is P07's second exit criterion, and
// before this it was driven by nothing.
//
// "A stale lock left by a killed process recovers without user action" was argued in
// `handedOff`'s comments and covered obliquely by `TestProbeAnswersForALiveInstanceAnd\
// NotForAStaleOne`, which proves only that `Probe` answers false for a closed port. The
// step that matters to the user — the launch REMOVES the dead record, publishes its own,
// and serves — had no test at any tier. That gap is the phase's own recurring shape: a
// criterion whose instrument was never built, found at the gate.
//
// It matters because of how the failure would present. A launch that treated an
// unreachable record as authoritative would exit silently, having handed off to nobody,
// and Nib would simply stop starting — for every user whose machine lost power with Nib
// open, with no error and no file to delete because nobody would know to look.
func TestAStaleRecordIsTakenOverWithoutUserAction(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a binary")
	}
	bin := filepath.Join(t.TempDir(), "nib-stalecheck")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("building nib: %v\n%s", err, out)
	}

	// A port that is genuinely nobody's: bind it, learn its number, release it. This is
	// the killed-process case exactly — the record names an address the kernel has since
	// handed back.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadAddr := l.Addr().String()
	l.Close()

	cfg := t.TempDir()
	nibDir := filepath.Join(cfg, "nib")
	if err := os.MkdirAll(nibDir, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := instance.Record{Addr: deadAddr, Token: "stale-token", Handoff: "stale-secret", Version: "0.0.0-dead"}
	if err := instance.Create(nibDir, stale); err != nil {
		t.Fatal(err)
	}

	// Two stimuli, because the assertion below is meaningless without either. The record
	// must really be there — otherwise this is just a cold start — and it must really be
	// unreachable, or "the launch took over" would be indistinguishable from "the launch
	// found a live instance and handed off to it".
	if got, err := instance.Read(nibDir); err != nil || got.Addr != deadAddr {
		t.Fatalf("setup: the stale record did not land (%+v, %v)", got, err)
	}
	if instance.Probe(stale) {
		t.Fatal("setup: the supposedly dead address answered a probe; the port was reused between closing it and now")
	}

	c := exec.Command(bin)
	c.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+cfg, "NIB_NO_BROWSER=1")
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = c.Process.Signal(syscall.SIGTERM)
		_, _ = c.Process.Wait()
	})

	// No user action of any kind between the launch and the assertion — no deleted file,
	// no second attempt, no prompt answered. That absence IS the criterion.
	var live instance.Record
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); {
		rec, err := instance.Read(nibDir)
		if err == nil && rec.Addr != deadAddr {
			live = rec
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if live.Addr == "" {
		t.Fatal("the launch never replaced the stale record — it either exited believing it had handed off to a dead instance, or it served without publishing, leaving the next launch to find the same dead address")
	}
	if !instance.Probe(live) {
		t.Errorf("the record now names %s, but nothing answers a probe there — the rendezvous points at a process that is not serving", live.Addr)
	}

	// And the JSON on disk is a whole record, not the dead one with a field edited: a
	// take-over that reused the previous token would leave the old secret authorising
	// probes against the new process.
	data, err := os.ReadFile(instance.Path(nibDir))
	if err != nil {
		t.Fatal(err)
	}
	var onDisk instance.Record
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatalf("the published record is not valid JSON: %v", err)
	}
	if onDisk.Token == stale.Token || onDisk.Handoff == stale.Handoff {
		t.Error("the take-over kept the dead instance's secrets; anything that read the old record still holds a working token")
	}
}
