package server

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestTheIdleExitArmsOnlyForAProcessThatLaunchedAWindow — P01.S03, D2.
//
// The flag decides, at P01.S04, whether this process ever exits for want of a window. Getting it
// wrong in one direction leaves the orphan the plan exists to remove; in the other it exits a
// harness — or a user whose browser failed to start — mid-work.
func TestTheIdleExitArmsOnlyForAProcessThatLaunchedAWindow(t *testing.T) {
	s := &Server{}
	// A fresh server is NOT armed. The zero value is the safe direction: a process that has not
	// said it launched a window must never be treated as waiting for one.
	if s.IdleExitArmed() {
		t.Error("a server that has never been told anything reports the idle-exit ARMED. The zero " +
			"value must be the safe direction — at P01.S04 this is what decides whether the " +
			"process exits when the window count reaches zero")
	}
	s.ArmIdleExit(true)
	if !s.IdleExitArmed() {
		t.Error("a process that launched a browser is not armed, so the last window closing would " +
			"leave exactly the orphan this plan exists to remove")
	}
	s.ArmIdleExit(false)
	if s.IdleExitArmed() {
		t.Error("a process that launched NO browser is armed. It has no window to wait for, so it " +
			"would exit on an empty count that was never going to fill — which is every harness, " +
			"and the NIB_ADDR tunnel mode")
	}
}

// TestTheIdleExitReadsTheLAUNCHAndNotJustTheEnvironment — D2's two facts.
//
// **This exists because a probe found it unreachable.** Reading only `NIB_NO_BROWSER` builds,
// passes every test in the tree and passes tier 3, because with a working browser both readings
// agree and every harness sets the variable. The difference appears only when `browser.Open`
// FAILS — a locked profile, snap or flatpak confinement, an Edge policy — which is precisely the
// user whose report begins "I double-clicked Nib and nothing happened". Arming them would exit
// Nib on the one person it already failed.
func TestTheIdleExitReadsTheLAUNCHAndNotJustTheEnvironment(t *testing.T) {
	failed := errors.New("no browser could be launched")
	cases := []struct {
		noBrowser bool
		openErr   error
		want      bool
		why       string
	}{
		{false, nil, true, "a normal launch: a window was asked for and one opened"},
		{true, nil, false, "NIB_NO_BROWSER: no window was asked for, so none is waited for"},
		{false, failed, false,
			"a window was asked for and NONE opened. Reading only the environment arms here, and " +
				"the process would exit for want of a window it never had"},
		{true, failed, false, "neither asked for nor opened"},
	}
	for _, c := range cases {
		if got := IdleExitDecision(c.noBrowser, c.openErr); got != c.want {
			t.Errorf("IdleExitDecision(noBrowser=%v, openErr=%v) = %v, want %v — %s",
				c.noBrowser, c.openErr, got, c.want, c.why)
		}
	}
	// STIMULUS: the table asserts both answers. A rule stuck at one constant satisfies half of it.
	var yes, no int
	for _, c := range cases {
		if c.want {
			yes++
		} else {
			no++
		}
	}
	if yes == 0 || no == 0 {
		t.Fatalf("the table asserts %d armed and %d not — a constant would pass it", yes, no)
	}
}

// TestNoHarnessCanArmTheIdleExit — P01.S03's first acceptance clause, "every harness reports it
// false, asserted rather than observed by eye".
//
// # Why this is a source scan and not a per-harness grep
//
// The property is not "each harness happens to print false today"; it is that **no harness can
// arm it at all**. That is decided before any of them runs, by whether they start `nib` with
// `NIB_NO_BROWSER` set — so the scan reads the launch lines rather than the output. A per-harness
// grep would also pass against a harness that had stopped starting nib entirely.
//
// At P01.S04 an armed harness does not fail visibly: it EXITS mid-run, and the failure surfaces as
// a connection refused somewhere unrelated. That is the shape this is worth a guard for.
func TestNoHarnessCanArmTheIdleExit(t *testing.T) {
	dir := filepath.Join("..", "..", "build")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	// **A launch is identified by NIB_NO_UPDATE_CHECK, not by the binary's name.** The binary is
	// spelled four ways across the harnesses (`"$WORK/nib"`, `"$EXE"`, `wine "$EXE"`, a bare
	// `env` prefix) and a name-matching scan found ZERO of them — caught by this test's own
	// stimulus floor on its first run. `NIB_NO_UPDATE_CHECK` is set on every launch for an
	// unrelated reason (hermeticity: without it the tier calls out to the release feed), which is
	// what makes it a non-circular marker for "this line starts nib".
	launch := regexp.MustCompile(`NIB_NO_UPDATE_CHECK\s*=`)

	files, launchers := 0, 0
	var bare []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sh") {
			continue
		}
		b, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil {
			t.Fatal(rerr)
		}
		files++
		for i, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "#") || !launch.MatchString(line) {
				continue
			}
			launchers++
			if !strings.Contains(line, "NIB_NO_BROWSER") {
				bare = append(bare, e.Name()+":"+itoa(i+1))
			}
		}
	}

	// STIMULUS, both halves. A scan that read no files, or matched no launch line, agrees with the
	// assertion below having checked nothing.
	if files < 5 {
		t.Fatalf("the scan read %d shell files in build/, want at least 5 — it is not reading the "+
			"harnesses and a clean result says nothing", files)
	}
	if launchers < 6 {
		t.Fatalf("the scan matched %d launch line(s), want at least 6 — the launch shape has "+
			"changed and this guard is matching nothing", launchers)
	}

	if len(bare) > 0 {
		t.Errorf("%d harness(es) start nib without NIB_NO_BROWSER, so they would ARM the idle-exit "+
			"(D2): %s\n\nAt P01.S04 an armed harness does not fail visibly — it exits mid-run, and "+
			"the failure surfaces as a connection refused somewhere unrelated.",
			len(bare), strings.Join(bare, ", "))
	}
}
