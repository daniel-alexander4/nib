package nib

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestEveryRedProofStillApplies is the cheap half of `./build/redproof.sh --all`, and it exists
// because the expensive half is not run often enough to be a gate (/pending 348).
//
// # The gap it closes, measured rather than argued
//
// `verify_test.go`'s count guard names this blind spot in its own comment: it can see a row that
// DISAPPEARS and not one that no longer re-proves, and "running the whole set is a minutes-long job
// that belongs in a sweep rather than in `go test`". That was true of the WHOLE job and false of
// the half that actually catches things.
//
// A full `--all` on 2026-08-31 — the first since v1.117.156 — found **seven rows that no longer
// re-proved, and six of the seven were stale patches**: the code had moved under them, silently,
// the moment an unrelated slice touched their file. Five of those six were not known to anybody.
// `redproof.sh`'s own header separates the two failures, and this is the first of them:
//
//	the patch did not apply       → the row is STALE; the code moved under it
//	the patch applied and the     → the check no longer catches its own defect
//	check still PASSED
//
// **Measured cost of each: 24 minutes for `--all`, 0.24 seconds for this.** Four orders of
// magnitude, for the failure that accounted for six of the seven. A gate nobody runs catches
// nothing, and that is the whole argument for putting the cheap half where it runs every time.
//
// # What this CANNOT see, stated so it is not read as more than it is
//
// The second failure mode entirely. A patch that applies cleanly may still no longer make its check
// go red — because the check was weakened, because the defect became unreachable, or because the
// patch was recorded in the wrong direction in the first place (`zone-bypasses-reserved` was, and
// had never been a valid row). Only `./build/redproof.sh <name>` can tell a valid row from a file
// that merely parses, and `--all` remains the sweep's job. This test makes the cheap failure loud;
// it does not retire the expensive one.
//
// It also reads the WORKING TREE rather than an export of HEAD, which is the one place it is
// STRICTER than the harness: it fails on the edit that stales a row, before that edit is committed,
// which is exactly when the fix is a one-line context refresh rather than an archaeology problem.
func TestEveryRedProofStillApplies(t *testing.T) {
	// Skips cleanly when its one dependency is absent, matching how tiers 2-4 treat theirs — a
	// fresh clone with no `patch(1)` still runs everything else rather than reporting a failure
	// about its own environment.
	if _, err := exec.LookPath("patch"); err != nil {
		t.Skip("patch(1) not installed; the red-proof staleness scan needs it (so does build/redproof.sh)")
	}

	patches, err := filepath.Glob("test/redproofs/*.patch")
	if err != nil {
		t.Fatal(err)
	}
	// A floor, or a glob that matches nothing reports every row healthy. Deliberately loose
	// against `verify_test.go`'s exact count, which is that guard's job — what this one must
	// refuse is reading an empty or wrong directory.
	if len(patches) < 100 {
		t.Fatalf("found %d red-proof patch(es) under test/redproofs; the set is in the hundreds, "+
			"so this scan is reading the wrong directory and a clean result would mean nothing",
			len(patches))
	}

	var stale []string
	for _, p := range patches {
		f, oerr := os.Open(p)
		if oerr != nil {
			t.Fatal(oerr)
		}
		// --dry-run makes no change; --forward refuses an already-applied (or reversed) patch
		// rather than silently offering to un-apply it, which is the state that produced
		// "Reversed (or previously applied) patch detected" on a row recorded backwards.
		cmd := exec.Command("patch", "-p1", "--dry-run", "--forward", "--silent")
		cmd.Stdin = f
		out, rerr := cmd.CombinedOutput()
		f.Close()
		if rerr != nil {
			name := strings.TrimSuffix(filepath.Base(p), ".patch")
			detail := strings.TrimSpace(string(out))
			if len(detail) > 200 {
				detail = detail[:200] + "…"
			}
			stale = append(stale, name+" — "+detail)
		}
	}
	sort.Strings(stale)
	for _, s := range stale {
		t.Errorf("red proof %s no longer applies to this tree. The row therefore claims a "+
			"coverage it cannot demonstrate: `./build/redproof.sh` would report it STALE rather "+
			"than re-proving it. Re-record the patch against the current code — expressing THE "+
			"DEFECT THE ROW NAMES, not merely something that applies — or retire the row.", s)
	}
	if len(stale) > 0 {
		t.Logf("%d of %d rows are stale; %d still apply", len(stale), len(patches), len(patches)-len(stale))
	}
}
