package nib

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
//
// # It applies patches the way `redproof.sh` does — with patch(1)'s default fuzz — on purpose
//
// An agent in the /pending 505 sweep reported "48 patches fail to apply against a clean HEAD, yet
// this test passes". Measured for /pending 505, from the repository root: **0 of 414** fail under
// `patch -p1 --dry-run --forward` (this test's and the harness's invocation), and **53** fail
// under `--fuzz=0` — the strictness `git apply --check` also has, since git apply takes no fuzz.
// `a-left-ceremony-reads-as-damage` applies at an offset of 2,308 lines with fuzz 1;
// `empty-address-refused` at 124 lines with fuzz 2. Those rows are not stale as the harness
// defines stale: `redproof.sh` applies them, and whether the fuzzed hunk still expresses its
// defect is the SECOND failure mode, which only a replay can judge. Tightening this test to zero
// fuzz would disagree with the harness it stands in for and fail 53 rows the harness replays.
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

// TestNoRedProofTokenIsItsTestsName — /pending 505.
//
// `redproof.sh` accepts a red only when the check's output carries the row's EXPECT token, because
// a deleted or uncompilable check also exits non-zero. That is defeated when the token is part of
// the test's own NAME: `node --test` prints every test title, passing or failing, and `go test`
// prints `--- FAIL: TestName` for any failure in it — so a row whose token is its title re-proves
// on ANY red in that file, including one that has nothing to do with the recorded defect.
// `empty-state-message` was exactly that (EXPECT "empty-state message", title "no empty-state
// message is generated content").
//
// It reads the two row shapes whose test names are recoverable from PROVE: `node --test <file>`
// (titles parsed from the file) and `go test … -run <names>`.
//
// # The harness rows, which this used to skip (/pending 518)
//
// The paragraph here used to say a harness row "has no title to compare against; its token is
// checked for being unique to the failing branch by review", and left it at that — 46 of the
// rows, every tier-3, -4, -5 and -6 one. A harness has something better than a title: it has its
// OWN output, and most of that output is printed on a run that never reached the recorded check.
// A row whose EXPECT is any of it re-proves on ANY red in the tier.
//
// `ambientHarnessOutput` collects the three sources of it, all three measured against the set:
//
//   - the harness's own progress banners — `echo "building the discovery tests…"`, and the
//     PASS line it prints when everything worked;
//   - every test title in a suite the harness drives with `node --test <dir>`, because node
//     prints the title of every test it runs whichever way that test went. This is the same
//     hazard the `node --test <file>` branch below already refuses, and `./build/uirepro.sh`
//     and `./build/jsdomtest.sh` hid it behind one level of shell;
//   - `=== RUN` / `--- PASS:` / `--- FAIL:` / `--- SKIP:` for every Go test the harness names,
//     because a harness that runs `go test -v` and cats the log prints those for every test on
//     its own run line, red or green.
//
// **What it cannot see, so it is not read as more than it is.** The banners are matched as
// LITERALS, so a token that spans a `$variable` in the harness's own string is missed; and a
// harness that prints something this does not model still prints it. It is a floor under the
// review the old paragraph relied on, not a replacement for reading the row.
func TestNoRedProofTokenIsItsTestsName(t *testing.T) {
	rows, err := filepath.Glob("test/redproofs/*.sh")
	if err != nil {
		t.Fatal(err)
	}
	field := func(src, name string) (string, bool) {
		m := regexp.MustCompile(`(?m)^` + name + `="((?:[^"\\]|\\.)*)"\s*$`).FindStringSubmatch(src)
		if m == nil {
			return "", false
		}
		// Undo the shell's double-quote escapes, which are the only ones that matter here.
		return regexp.MustCompile(`\\([\\"$`+"`"+`])`).ReplaceAllString(m[1], "$1"), true
	}
	title := regexp.MustCompile(`\btest\(\s*(?:'((?:[^'\\]|\\.)*)'|"((?:[^"\\]|\\.)*)"|` + "`((?:[^`\\\\]|\\\\.)*)`" + `)`)
	interp := regexp.MustCompile(`\$\{[^}]*\}`)
	harnessRe := regexp.MustCompile(`(?:^|\s)\.?/?(build/[A-Za-z0-9_.-]+\.sh)`)
	ambientCache := map[string][]string{}
	var nodeRows, goRows, harnessRows int
	for _, r := range rows {
		b, rerr := os.ReadFile(r)
		if rerr != nil {
			t.Fatal(rerr)
		}
		src := string(b)
		prove, ok1 := field(src, "PROVE")
		expect, ok2 := field(src, "EXPECT")
		if !ok1 || !ok2 || expect == "" {
			continue // a missing EXPECT is redproof.sh's own hard error
		}
		name := strings.TrimSuffix(filepath.Base(r), ".sh")
		if m := regexp.MustCompile(`^node --test (\S+)`).FindStringSubmatch(prove); m != nil {
			nodeRows++
			tb, ferr := os.ReadFile(m[1])
			if ferr != nil {
				t.Errorf("red proof %s runs %s, which does not exist: %v", name, m[1], ferr)
				continue
			}
			for _, tm := range title.FindAllStringSubmatch(string(tb), -1) {
				text := tm[1] + tm[2] + tm[3]
				// A template title is checked segment by segment: the interpolations are not text.
				for _, seg := range append([]string{text}, interp.Split(text, -1)...) {
					if seg != "" && strings.Contains(seg, expect) {
						t.Errorf("red proof %s: EXPECT %q is inside the test title %q, which node prints whether "+
							"that test passes or fails — any red in %s re-proves this row. Use a phrase only the "+
							"assertion's failure message prints.", name, expect, text, m[1])
						break
					}
				}
			}
			continue
		}
		if strings.HasPrefix(prove, "go test") {
			goRows++
			for _, tn := range regexp.MustCompile(`\bTest\w+`).FindAllString(prove, -1) {
				if strings.Contains(tn, expect) {
					t.Errorf("red proof %s: EXPECT %q is part of the test name %s, which `--- FAIL:` prints on any "+
						"failure in that test. Use a phrase only the assertion's failure message prints.", name, expect, tn)
				}
			}
			continue
		}
		if m := harnessRe.FindStringSubmatch(prove); m != nil {
			harnessRows++
			harness := m[1]
			ambient, ok := ambientCache[harness]
			if !ok {
				var aerr error
				if ambient, aerr = ambientHarnessOutput(harness); aerr != nil {
					t.Errorf("red proof %s runs %s, which cannot be read: %v", name, harness, aerr)
					continue
				}
				// A per-harness floor, because this is a guard over guards and the way one of
				// those goes quiet is by reading nothing. Every harness in the set yields
				// dozens; single digits means the banner or the suite parse stopped matching
				// and every row against that harness would pass by being compared to nothing.
				if len(ambient) < 8 {
					t.Fatalf("%s yielded %d ambient output string(s); every harness in this set prints "+
						"far more than that, so the parse is reading nothing and the rows driven by it "+
						"are checked against an empty list", harness, len(ambient))
				}
				ambientCache[harness] = ambient
			}
			for _, a := range ambient {
				if !strings.Contains(a, expect) {
					continue
				}
				t.Errorf("red proof %s: EXPECT %q is inside %q, which %s prints on a run that never "+
					"reached the recorded check — so the row re-proves on ANY red in that tier, including "+
					"a build break. Use a phrase only the failing branch prints.", name, expect, a, harness)
				break
			}
		}
	}
	// Floors: the set holds 45 node rows, 323 go rows and 48 harness rows — 46 when the harness
	// branch was written, plus the two tier-5 ones added beside it. Far fewer means the PROVE
	// parse stopped matching and every row passed by being read by nothing.
	if nodeRows < 40 || goRows < 300 || harnessRows < 40 {
		t.Fatalf("parsed %d `node --test`, %d `go test` and %d harness red-proof rows; the set holds "+
			"~45, ~323 and ~48 — the row parse is not reading PROVE", nodeRows, goRows, harnessRows)
	}
}

// ambientHarnessOutput is the population a harness prints WITHOUT any recorded check having
// fired — see TestNoRedProofTokenIsItsTestsName for why a red-proof EXPECT may not be in it.
func ambientHarnessOutput(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	src := string(b)
	var out []string

	// 1. The harness's own banners. Statement-initial `echo`/`printf` only: an `|| fail "…"` or a
	//    `no "…" "…"` is a failure branch and is exactly what an EXPECT is supposed to be. Lines
	//    redirected to stderr, and literals that open a `FAIL:` report, are failure output too.
	echoLit := regexp.MustCompile(`^\s*(?:echo|printf)\s+(?:'([^']*)'|"((?:[^"\\]|\\.)*)")`)
	for _, line := range strings.Split(src, "\n") {
		if strings.Contains(line, ">&2") {
			continue
		}
		m := echoLit.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		lit := m[1] + m[2]
		if strings.Contains(lit, "FAIL:") {
			continue
		}
		out = append(out, lit)
	}

	// 2. Every title in a suite the harness drives with `node --test <dir>`. node prints the title
	//    of every test it runs, passing or failing, so a title is ambient by construction.
	title := regexp.MustCompile(`\btest\(\s*(?:'((?:[^'\\]|\\.)*)'|"((?:[^"\\]|\\.)*)"|` + "`((?:[^`\\\\]|\\\\.)*)`" + `)`)
	for _, m := range regexp.MustCompile(`node --test[^\n]*?\s(test/[A-Za-z0-9_-]+)/`).FindAllStringSubmatch(src, -1) {
		files, gerr := filepath.Glob(m[1] + "/*.test.mjs")
		if gerr != nil {
			return nil, gerr
		}
		for _, f := range files {
			tb, rerr := os.ReadFile(f)
			if rerr != nil {
				return nil, rerr
			}
			for _, tm := range title.FindAllStringSubmatch(string(tb), -1) {
				out = append(out, tm[1]+tm[2]+tm[3])
			}
		}
	}

	// 3. `go test -v`'s own lines for every Go test the harness names. A harness that runs a test
	//    binary and cats its log prints `=== RUN` and a verdict line for each of them whichever way
	//    the run went — which is what makes a bare test name the worst possible EXPECT here.
	seen := map[string]bool{}
	for _, tn := range regexp.MustCompile(`\bTest[A-Z]\w+`).FindAllString(src, -1) {
		if seen[tn] {
			continue
		}
		seen[tn] = true
		out = append(out, "=== RUN   "+tn, "--- PASS: "+tn+" (", "--- FAIL: "+tn+" (", "--- SKIP: "+tn+" (")
	}
	return out, nil
}
