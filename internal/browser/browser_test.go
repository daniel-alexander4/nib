package browser

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Nib opens its UI by launching an installed Chromium-family browser, and the
// tier-3 test harness (build/uirepro.sh) drives one too. Those are two lists of
// the same thing in two languages, and Go cannot export a bash array — so the
// duplication was accepted with a comment naming this file as the source.
//
// This is what makes that acceptable rather than a debt. If the lists drift, the
// harness verifies Nib against a browser no user is given, which is worse than
// not testing at all: it reports confidence about the wrong engine.
func TestHarnessHuntsTheSameBrowsersWeDo(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("uirepro.sh is a bash harness with the Linux candidate list")
	}

	script, err := os.ReadFile("../../build/uirepro.sh")
	if err != nil {
		t.Fatalf("cannot read the tier-3 harness: %v", err)
	}

	// The harness's list is the `for c in …; do` line that searches PATH.
	const marker = "for c in "
	i := strings.Index(string(script), marker)
	if i < 0 {
		t.Fatal("uirepro.sh no longer has a browser-candidate loop; this guard needs updating with it")
	}
	rest := string(script)[i+len(marker):]
	end := strings.Index(rest, ";")
	if end < 0 {
		t.Fatal("could not parse the candidate loop in uirepro.sh")
	}
	inScript := strings.Fields(rest[:end])

	// Absolute paths in the Go list are macOS .app bundles, which a Linux harness
	// has no business hunting for; compare only the bare command names.
	var want []string
	for _, c := range chromiumCandidates() {
		if !strings.HasPrefix(c, "/") {
			want = append(want, c)
		}
	}

	// The stimulus, before the comparison: a parse that silently produced nothing
	// would report perfect agreement between two empty lists forever.
	if len(want) == 0 || len(inScript) == 0 {
		t.Fatalf("nothing to compare (go=%d, script=%d) — the guard is not reading what it thinks", len(want), len(inScript))
	}

	if strings.Join(inScript, " ") != strings.Join(want, " ") {
		t.Errorf("the tier-3 harness hunts different browsers than Nib does — it would test an engine users are not given\n  uirepro.sh: %v\n  browser.go: %v", inScript, want)
	}
}

// TestEveryPlatformOffersTheBrowsersWeAdvertise checks all three candidate lists,
// on every platform, against what the README promises.
//
// The sibling test above skips unless GOOS is linux, so the Windows and macOS lists
// had no guard of any kind — and the Windows list contained no Brave entry at all
// (install path or bare name) and no Chromium, while README:640 promises "Chrome /
// Edge / Brave / Chromium (app mode)". A Brave-only Windows user therefore fell
// through to the rundll32 tab fallback and got an ordinary tabbed window, silently.
//
// The lists are read through the same source-scan the sibling uses rather than by
// calling chromiumCandidates(), because that function returns only the list for the
// platform the test happens to run on — which is exactly how three of the four lists
// went unchecked.
func TestEveryPlatformOffersTheBrowsersWeAdvertise(t *testing.T) {
	src, err := os.ReadFile("browser.go")
	if err != nil {
		t.Fatalf("cannot read the candidate lists: %v", err)
	}
	// Comments stripped FIRST. Without this the guard matched the prose explaining
	// which browsers were missing — so removing Brave from the Windows list left it
	// green, because the comment above the list still said "Brave". A scan that reads
	// its own documentation confirms whatever the documentation claims.
	var kept []string
	for _, line := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		kept = append(kept, line)
	}
	body := strings.Join(kept, "\n")

	// One block per platform, sliced at the case labels so a browser named in the
	// darwin list cannot satisfy the windows one.
	blocks := map[string][2]string{
		"darwin":  {`case "darwin":`, `case "windows":`},
		"windows": {`case "windows":`, `default:`},
		"linux":   {`default:`, "func tabOpener"},
	}
	// The four families README:640 advertises. Matched case-insensitively on a
	// substring, since the same browser is "brave-browser", "Brave Browser" and
	// "brave.exe" depending on the platform.
	families := []string{"chrome", "edge", "brave", "chromium"}

	for platform, se := range blocks {
		start := strings.Index(body, se[0])
		end := strings.Index(body, se[1])
		if start < 0 || end <= start {
			t.Fatalf("%s: cannot locate the candidate block (anchors moved — this guard is not reading what it thinks)", platform)
		}
		block := strings.ToLower(body[start:end])
		for _, fam := range families {
			if !strings.Contains(block, fam) {
				t.Errorf("%s offers no %s, but README advertises it — that user silently gets the plain-tab fallback", platform, fam)
			}
		}
	}
}

// TestABrowserThatStartsAndDiesIsNotReportedAsSuccess.
//
// `cmd.Start()` reports only exec-level failure, and `reap` explicitly discarded `Wait`'s
// status ("the error is deliberately dropped"). So a browser that launches and exits
// immediately — a locked user-data-dir, snap or flatpak confinement, an Edge policy, a
// broken profile — was a successful launch as far as Nib was concerned, with no fallback
// and no diagnostic a double-clicked process could show anyone. The user's whole report is
// "I double-clicked Nib and nothing happened", which is the local-first SRE seat's named
// failure shape.
func TestABrowserThatStartsAndDiesIsNotReportedAsSuccess(t *testing.T) {
	// STIMULUS first: a process that STAYS UP must be reported alive, or the assertion
	// below is satisfied by an `alive` that always says false.
	long := exec.Command("sleep", "5")
	if err := long.Start(); err != nil {
		t.Skipf("no sleep binary here: %v", err)
	}
	defer func() { _ = long.Process.Kill() }()
	if !alive(long, 100*time.Millisecond) {
		t.Fatal("setup: a running process was reported as dead, so this test cannot " +
			"distinguish the case it is for")
	}

	// The case: a command that exits at once, which is what a refusing browser does.
	quick := exec.Command("false")
	if err := quick.Start(); err != nil {
		t.Skipf("no false binary here: %v", err)
	}
	if alive(quick, appModeSettle) {
		t.Error("a process that exited immediately was reported as a live browser — Nib " +
			"then serves a window nobody can see and never reaches the tab fallback")
	}
}
