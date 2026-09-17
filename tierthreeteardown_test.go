package nib

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// /pending 474 — tier 3 was RED at HEAD for months, and one of its three failures was a
// document an EARLIER file in the run left open being read as the current file's.
//
// # Why closing the browser was never enough
//
// `build/uirepro.sh` serves every file in this tier from ONE nib process. The browser belongs to
// the file; the document registry belongs to the shared server. So a file that ends with
// `browser.close()` hands the next file a server still holding its documents, and the app
// activates one as soon as the new file closes its own. Measured at `PLAN-accessibility.md`
// P08.S06c: `tagsreview.test.mjs` found 0 page divs at its start, opened and closed its own
// 2-page document, and was then showing a 3-page document it never opened.
//
// **That is why the symptom moves.** A "leaves the shared server as it found it" check that
// counts page divs blames whichever file closes NEXT, so one leak reads as an unrelated failure
// somewhere else — and a flake and a regression become indistinguishable, which is the cost
// `/pending 474` and `/pending 475` were both filed against.
//
// # Why this is a guard and not a line in each `after`
//
// Thirty-seven files each writing the teardown by hand agree with each other and say nothing
// about the thirty-eighth. That is ADR-009's rule — a rule holding at more than one call site is
// written once and every site calls it, and the guard asserts the ROUTING rather than the text
// each site prints. `harness.mjs`'s `shutdown` is the door; this is the check on it.
func TestEveryTierThreeFileEndsThroughTheOneDoor(t *testing.T) {
	dir := filepath.Join("test", "ui")
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("no tier-3 directory (%v)", err)
	}

	var (
		launches  = regexp.MustCompile(`\blaunch\(`)
		routes    = regexp.MustCompile(`\bshutdown\(`)
		bareClose = regexp.MustCompile(`\b\w*\.?browser\.close\(`)
		stripped  = regexp.MustCompile(`(?m)^\s*//.*$`)
	)

	var scanned, missing, direct []string
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".test.mjs") {
			continue
		}
		b, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil {
			t.Fatalf("read %s: %v", e.Name(), rerr)
		}
		// Comments out first, or a file that merely DESCRIBES the old teardown reads as one —
		// `signingpanelgap.test.mjs` explains the hang a wrong teardown once caused, in prose.
		src := stripped.ReplaceAllString(string(b), "")
		if !launches.MatchString(src) {
			continue
		}
		scanned = append(scanned, e.Name())
		if !routes.MatchString(src) {
			missing = append(missing, e.Name())
		}
		if bareClose.MatchString(src) {
			direct = append(direct, e.Name())
		}
	}

	// **The floor, because an empty scan looks exactly like a clean one.** This tier has well
	// over thirty files; a regexp that stops matching would otherwise report nothing and pass.
	if len(scanned) < 30 {
		t.Fatalf("the scan found %d tier-3 file(s) calling launch() and this tier has well over "+
			"thirty — it is not reading them, and an empty report is what that looks like", len(scanned))
	}

	sort.Strings(missing)
	sort.Strings(direct)
	if len(missing) > 0 {
		t.Errorf("%d tier-3 file(s) open a browser and never route their teardown through "+
			"harness.mjs's `shutdown` (/pending 474). Closing the browser leaves the SHARED nib "+
			"process still holding this file's documents, and the next file inherits them:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
	if len(direct) > 0 {
		t.Errorf("%d tier-3 file(s) call browser.close() directly instead of going through "+
			"`shutdown` (/pending 474). The door exists so a file cannot close the browser "+
			"without closing what the server holds:\n  %s",
			len(direct), strings.Join(direct, "\n  "))
	}
}
