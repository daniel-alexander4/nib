package browser

import (
	"os"
	"runtime"
	"strings"
	"testing"
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
