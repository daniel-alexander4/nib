package nib

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The build & verify contract lives in CONTRIBUTING.md, and this is what stops it
// becoming a description of a repo that no longer exists.
//
// It sits beside TestNoticesUpToDate for the same reason that one exists: a
// document nothing checks drifts silently, and the drift is only ever discovered
// by someone following it and finding it wrong. CLAUDE.md cannot host this
// contract — it is git-ignored here by convention, so a fresh clone would never
// see it — which is why the committed copy is the one under test.
func TestVerifyContractIsTrue(t *testing.T) {
	doc, err := os.ReadFile("CONTRIBUTING.md")
	if err != nil {
		t.Fatalf("the build/verify contract is missing: %v", err)
	}
	contract := string(doc)

	// Every tier's command must exist, be runnable, and be named in the contract.
	// All three conditions matter: a script that exists but is not executable
	// fails for the reader in a way the doc will not explain, and a script named
	// nowhere is one nobody runs.
	// winrepro.sh is not one of the three tiers — it is the out-of-loop Windows
	// harness — but it is held to the same three conditions, because as of P07.S03
	// it carries a claim nothing else in the tree can make: that a second launch
	// hands its document to the running instance on the platform where double-click
	// is the ordinary way in. A harness that quietly stopped existing would take
	// that claim with it and no tier would notice.
	for _, cmd := range []string{"build/jsdomtest.sh", "build/uirepro.sh", "build/pairrepro.sh", "build/mcastrepro.sh", "build/winrepro.sh", "build/dhtlive.sh", "build/redproof.sh", "build/ceremonyrepro.sh"} {
		info, err := os.Stat(cmd)
		if err != nil {
			t.Errorf("%s is named in the contract but does not exist: %v", cmd, err)
			continue
		}
		if info.Mode()&0o111 == 0 {
			t.Errorf("%s is not executable (mode %v); the contract tells people to run it directly", cmd, info.Mode())
		}
		if !strings.Contains(contract, cmd) {
			t.Errorf("%s exists but CONTRIBUTING.md does not name it", cmd)
		}
	}

	// dhtlive.sh must run EVERY live test, not one named test.
	//
	// It ran `-run TestLiveSelfAddressProbe` — a single literal name — and nothing
	// checked that against the package. A live test added by a later slice would be
	// gated behind NIB_LIVE_DHT, therefore skipped by `go test ./...`, therefore never
	// executed by anything at all, and every tier would stay green. That is the vacuous
	// green one level out: the harness reports a pass for the tests it happens to name.
	{
		// **Every FUNCTION with an NIB_LIVE_DHT gate, discovered, not a path written here.**
		//
		// This read one file and matched `TestLive*`. internal/cli's
		// TestTheBannerPrecedesTheSocket has a live half behind the same variable and is
		// named nothing like that, so it was skipped by `go test ./...` (variable unset) AND
		// never reached by dhtlive.sh (package not named): its live half was executed by
		// nothing, with every tier green. The guard could not see it because the guard, too,
		// named one file and one prefix.
		//
		// Per FUNCTION, not per file: rendezvous_test.go holds one gated test among a dozen
		// hermetic ones, and treating the whole file as gated demands the harness run tests
		// that `go test ./...` already runs — a guard that fails on correct code.
		var names [][]string
		var gated []string
		funcStart := regexp.MustCompile(`(?m)^func (Test\w+)\(`)
		err := filepath.Walk(".", func(path string, info os.FileInfo, werr error) error {
			// **Skip `.claude`, which holds a second session's git worktrees.** CLAUDE.md tells a
			// concurrent session to build in a worktree, and this repo's live at
			// `.claude/worktrees/<name>` — a complete second copy of the tree, gitignored and
			// therefore not source. Without this the walk finds every gated test twice, once at a
			// path no harness runs, and reports the duplicate as an unexecuted test. Found the
			// first time two sessions worked this repo at once.
			if werr == nil && info.IsDir() && (info.Name() == ".claude" || info.Name() == ".git") {
				return filepath.SkipDir
			}
			if werr != nil || info.IsDir() || !strings.HasSuffix(path, "_test.go") || path == "verify_test.go" {
				return nil
			}
			b, rerr := os.ReadFile(path)
			if rerr != nil || !bytes.Contains(b, []byte("NIB_LIVE_DHT")) {
				return nil
			}
			src := string(b)
			locs := funcStart.FindAllStringSubmatchIndex(src, -1)
			hit := false
			for n, loc := range locs {
				end := len(src)
				if n+1 < len(locs) {
					end = locs[n+1][0]
				}
				if !strings.Contains(src[loc[0]:end], "NIB_LIVE_DHT") {
					continue
				}
				names = append(names, []string{"", src[loc[2]:loc[3]]})
				hit = true
			}
			if hit {
				gated = append(gated, path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("cannot walk for live tests: %v", err)
		}
		if len(gated) < 2 || len(names) == 0 {
			t.Fatalf("found %d gated function(s) across %d file(s) (%v) — the walk has gone "+
				"blind, and a blind walk reports full coverage", len(names), len(gated), gated)
		}

		harness, err := os.ReadFile("build/dhtlive.sh")
		if err != nil {
			t.Fatalf("cannot read build/dhtlive.sh: %v", err)
		}
		// Match the INVOCATION LINE, not the file.
		//
		// A whole-file `strings.Contains` is satisfied by the pattern appearing in a
		// comment — this repo's own recurring hole, previously found in `published.test.mjs`
		// and in a `.deb` guard satisfied by a word inside a comment. It also false-reds on
		// the functionally identical unquoted form. So: find the actual `go test` line, read
		// its -run pattern, and check that pattern really selects every discovered name.
		// The packages the harness names must cover the packages the gated files live in.
		for _, g := range gated {
			pkg := "./" + filepath.ToSlash(filepath.Dir(g)) + "/"
			if !strings.Contains(string(harness), pkg) {
				t.Errorf("%s is gated on NIB_LIVE_DHT but build/dhtlive.sh does not run %s — "+
					"`go test ./...` skips it (variable unset) and the harness never reaches "+
					"it, so it is executed by NOTHING and every tier stays green", g, pkg)
			}
		}

		var pattern string
		for _, line := range strings.Split(string(harness), "\n") {
			t := strings.TrimSpace(line)
			if strings.HasPrefix(t, "#") || !strings.Contains(t, "go test") {
				continue
			}
			m := regexp.MustCompile(`-run\s+'?"?([^\s'"]+)'?"?`).FindStringSubmatch(t)
			if m != nil {
				pattern = m[1]
			}
		}
		if pattern == "" {
			t.Fatal("build/dhtlive.sh has no `go test … -run` invocation line — this guard " +
				"would pass on a harness that runs nothing")
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			t.Fatalf("build/dhtlive.sh's -run pattern %q does not compile: %v", pattern, err)
		}
		for _, n := range names {
			// Go's -run is an unanchored match against the test name.
			if !re.MatchString(n[1]) {
				t.Errorf("build/dhtlive.sh runs -run %q, which does not select %s. A live "+
					"test is gated behind NIB_LIVE_DHT, so `go test ./...` skips it too — "+
					"it would be executed by NOTHING and every tier would stay green.",
					pattern, n[1])
			}
		}
	}

	// The two Go commands have no file to check, so only their presence in the
	// contract can be asserted.
	for _, cmd := range []string{"go build ./...", "go test ./..."} {
		if !strings.Contains(contract, cmd) {
			t.Errorf("CONTRIBUTING.md does not name %q", cmd)
		}
	}

	// Each tier states what it CANNOT see, and that half is the one worth
	// guarding: a contract listing three commands and no blind spots invites
	// exactly the over-trust the tiering exists to prevent.
	//
	// Checked in the HARNESS FILES too, not only here. The contract sends readers to
	// "the ceiling written in test/jsdom/boot.mjs" and "the ceiling written in
	// build/uirepro.sh", and those are the copies that rot — this file is the one
	// people edit when they edit the contract, and the harness comment is the one they
	// forget. A ceiling that exists only in the document the guard reads is a ceiling
	// the guard cannot tell from a deleted one.
	for _, c := range []struct{ file, ceiling string }{
		{"CONTRIBUTING.md", "Cannot see: the client at all"},
		{"CONTRIBUTING.md", "Cannot see: anything that needs rendering"},
		{"CONTRIBUTING.md", "Cannot see: other engines"},
		{"test/jsdom/boot.mjs", "Where it stops"},
		{"build/uirepro.sh", "Where it still stops"},
		{"CONTRIBUTING.md", "Cannot see: two networks"},
		{"build/pairrepro.sh", "Where it still stops"},
		{"CONTRIBUTING.md", "Cannot see: a network"},
		{"CONTRIBUTING.md", "black-hole default route"},
		{"build/winrepro.sh", "What this harness CANNOT discharge"},
		{"build/mcastrepro.sh", "Where it still stops"},
		{"build/dhtlive.sh", "What this harness CANNOT discharge"},
		// **Tier 6, added at P08.S09 — it was in none of the three places.** The harness has
		// existed and been green since P07.S02b and appeared in neither the contract's table, nor
		// the row list below, nor here; so it could have been deleted outright with every tier
		// green and the contract silent. Its ceiling heading is "Where it stops" rather than the
		// "Where it still stops" its siblings use, and it is matched as written rather than
		// normalised: the guard's job is to find the words a reader is sent to, not to prefer a
		// house style over what the file says.
		{"build/ceremonyrepro.sh", "Where it stops"},
		{"CONTRIBUTING.md", "Cannot see: a hop completing"},
	} {
		body := contract
		if c.file != "CONTRIBUTING.md" {
			b, err := os.ReadFile(c.file)
			if err != nil {
				t.Errorf("%s is named in the contract as holding a ceiling, and is missing: %v", c.file, err)
				continue
			}
			body = string(b)
		}
		if !strings.Contains(body, c.ceiling) {
			t.Errorf("%s no longer states its ceiling (%q) — the contract sends readers there for it", c.file, c.ceiling)
		}
	}

	// The four-row TABLE, not merely the four commands somewhere in the prose.
	//
	// strings.Contains over the whole file was the entire check, so deleting the table
	// and leaving the commands mentioned in a sentence kept this green — and the table
	// is the part that pairs each command with a tier number and with what it verifies.
	// Asserted as rows so the structure has to survive, not just the words.
	// **The tier column is a LITERAL, not a loop index, and that is the fix.** This
	// built its prefix as `"| " + strconv.Itoa(i) + " | "` over i in 0..5, so it
	// matched rows 0-5 and nothing else — while CONTRIBUTING.md carries EIGHT rows.
	// `| 4b |` (`--lan`) and `| 4c |` (`--v6`) matched no prefix any iteration
	// produced, so both could be deleted from the contract with every tier green.
	// 4c is P05.S05's hermetic half of criterion 1, i.e. a phase-close criterion
	// whose harness row nothing held in place. Found by the verification SME pack
	// during P07's plan-review (/pending 279).
	for _, row := range []struct{ tier, cmd string }{
		{"0", "`go build ./...`"},
		{"1", "`go test ./...`"},
		{"2", "`./build/jsdomtest.sh`"},
		{"3", "`./build/uirepro.sh`"},
		{"4", "`./build/pairrepro.sh`"},
		{"4b", "`./build/pairrepro.sh --lan`"},
		{"4c", "`./build/pairrepro.sh --v6`"},
		{"4d", "`./build/pairrepro.sh -n 4`"},
		{"5", "`./build/mcastrepro.sh`"},
		{"6", "`./build/ceremonyrepro.sh`"},
	} {
		want := "| " + row.tier + " | " + row.cmd
		if !strings.Contains(contract, want) {
			t.Errorf("the tier table has lost its row %s (%q) — the commands may still be named in prose, which is what made this check pass over a deleted table", row.tier, want)
		}
	}

	// The REPLAY SET is counted here, outside the harness that reads it.
	//
	// docs/red-proofs.md says in prose that two rows are recorded. Nothing counted the
	// directory, and `redproof.sh` prints "(none)" and exits 0 on an empty one — so deleting
	// both pairs left the doc asserting two and the harness reporting no error. That is V2:
	// a rule inventory whose count lives only inside the thing it describes.
	//
	// A floor rather than an equality, because adding a row is the direction this should
	// move in and should not need a test edit; losing one is the direction that must fail.
	{
		rows, err := filepath.Glob("test/redproofs/*.sh")
		if err != nil {
			t.Fatal(err)
		}
		// The floor moves with the set, and that is the point of a floor. It was 2
		// when two rows existed; leaving it there while the set grew would have
		// tolerated losing four of six silently, which is the same shape as the
		// prose count it replaced — a number that stops describing the thing it
		// counts. Raising it is the tax a new row pays.
		//
		// **What this count cannot see, stated so nobody reads it as more than it is:**
		// a row that EXISTS but no longer re-proves. `zone-bypasses-reserved` was
		// recorded with its patch reversed — it applied the fix rather than the defect —
		// so it had never been a valid row, and this count was satisfied by it for as
		// long as it sat there. Only `./build/redproof.sh <name>` can tell a row from a
		// file, and running the whole set is a minutes-long job that belongs in a sweep
		// rather than in `go test`. **The gap is still here and it is no longer without a
		// door:** `./build/redproof.sh --all` is that sweep's one command (v1.117.156),
		// added when the set was first replayed whole and EIGHT of its eighty-one rows
		// turned out to be invalid — seven stale patches and one whose EXPECT token no
		// longer matched. This count had been satisfied by all eight for as long as their
		// files existed, which is the paragraph above, measured.
		// **And the tax went unpaid, which is why there is a second arm below.** This
		// constant last moved at v1.117.50, to 27. P05.S05-S12 and the phase close then
		// added NINE rows without touching it, so on the tree that shipped P05 this check
		// would have tolerated losing nine of thirty-six — the exact erosion the paragraph
		// above says a floor prevents. The original reasoning is refuted by measurement:
		// an edit that does not HAVE to happen is an edit that does not happen. So the
		// count is bounded on both sides now. It still fails when a row disappears, and it
		// fails when the set outgrows it, naming the number to write.
		const recorded = 271
		if len(rows) < recorded {
			t.Errorf("test/redproofs holds %d replayable row(s), want at least %d; "+
				"build/redproof.sh reports no error on an empty directory, so a row that "+
				"disappears is invisible to everything except this count",
				len(rows), recorded)
		}
		if len(rows) > recorded {
			t.Errorf("test/redproofs holds %d replayable row(s) and this constant says %d: "+
				"raise it to %d in the same commit as the new row. A floor that stops moving "+
				"stops describing the set — at 27 against 36, nine rows could have been "+
				"deleted with nothing failing.",
				len(rows), recorded, len(rows))
		}
		for _, r := range rows {
			body, rerr := os.ReadFile(r)
			if rerr != nil {
				t.Fatal(rerr)
			}
			// Every row must name the token its check prints. A row without one is
			// satisfied by the check having been deleted — the defect this file's own
			// ledger exists to make impossible.
			if !strings.Contains(string(body), "EXPECT=") {
				t.Errorf("%s records no EXPECT token, so its replay is satisfied by any "+
					"non-zero exit — including a check that no longer exists", r)
			}
			patch := strings.TrimSuffix(r, ".sh") + ".patch"
			if _, perr := os.Stat(patch); perr != nil {
				t.Errorf("%s has no matching .patch: %v", r, perr)
			}
		}

		// And the OTHER direction: a `.patch` with no `.sh`.
		//
		// The loop above walks the drivers, so a patch left behind when a row is renamed is
		// invisible to it — it satisfies nothing and nothing complains. **Produced for real
		// while writing P07.S05a**: a row was recorded as `completeness-gated-on-a-claim`,
		// renamed to `-on-a-signature` when the first patch would not build, and the original
		// patch sat in the directory afterwards looking exactly like a row.
		//
		// It matters because a stray patch is not inert: `redproof.sh --all` reports on the
		// rows it finds, so an orphan reads as coverage to anyone counting files rather than
		// replays.
		patches, perr := filepath.Glob("test/redproofs/*.patch")
		if perr != nil {
			t.Fatal(perr)
		}
		for _, pf := range patches {
			if _, serr := os.Stat(strings.TrimSuffix(pf, ".patch") + ".sh"); serr != nil {
				t.Errorf("%s has no matching .sh, so it is a defect nothing replays — delete "+
					"it, or write the driver it was recorded for", pf)
			}
			// **A patch must contain ONLY the defect it names (/pending 278).**
			//
			// The harness structurally cannot see this: a red proof asserts that a defect makes a
			// check fail, never that the patch contains nothing else. Four rows were once
			// generated with a bare `git diff` while `test/redproofs/*.patch` are themselves
			// tracked, so each regeneration swept the previous ones in — one came out at 214 lines
			// and six hunks for a one-comment mutation, and all four still replayed GREEN. Every
			// check passed and every row was wrong about what it recorded.
			//
			// Two arms, and the second is the specific one. A patch touching more than one file is
			// almost always the bare-`git diff` mistake; a patch touching `test/redproofs/` is
			// that mistake exactly, and it stays detectable even if a legitimate multi-file
			// mutation is ever needed.
			body, berr := os.ReadFile(pf)
			if berr != nil {
				t.Fatal(berr)
			}
			// Counted on `+++`, not on `diff --git`. Four rows in the set are hand-written
			// unified diffs with no git header at all — they apply, they replay red, and a
			// header-based count reported every one of them as touching ZERO files, which is a
			// guard that fires on the wrong thing while missing what it was written for.
			files, self := 0, false
			for _, ln := range strings.Split(string(body), "\n") {
				if !strings.HasPrefix(ln, "+++ ") {
					continue
				}
				files++
				if strings.Contains(ln, "test/redproofs/") {
					self = true
				}
			}
			if files != 1 {
				t.Errorf("%s patches %d files; a red proof plants ONE defect, and a patch with "+
					"more is the bare-`git diff` mistake that once swept four rows into each "+
					"other while every one of them still replayed green", pf, files)
			}
			if self {
				t.Errorf("%s patches a file under test/redproofs/ — it was generated with a bare "+
					"`git diff` and captured other rows' patches as part of its own defect. Use "+
					"`git diff -- <the one file>`", pf)
			}
		}
	}

	// The "proven red at least once" claim needs a record behind it.
	//
	// This is the V1 shape in the file that teaches V1: the sentence was guarded (it is
	// in `contract`, so a deletion would fail the ceiling checks above) while the FACT it
	// asserts was backed by nothing a reader could check. docs/red-proofs.md is that
	// record; a claim of having been proven red, with no ledger, is a claim nobody can
	// audit and therefore one nobody should believe.
	if strings.Contains(contract, "proven red") {
		ledger, err := os.ReadFile("docs/red-proofs.md")
		if err != nil {
			t.Fatalf("CONTRIBUTING.md claims every tier has been proven red, and docs/red-proofs.md — the record of it — is missing: %v", err)
		}
		for _, tier := range []string{"Tier 1", "Tier 2", "Tier 3", "Tier 4", "Tier 5"} {
			if !strings.Contains(string(ledger), tier) {
				t.Errorf("docs/red-proofs.md has no entries for %s, so the contract's claim is unbacked for that tier", tier)
			}
		}
	}
}

// The .deb declares that nib needs a browser, and names the same ones nib looks for.
//
// nfpm.yaml had no dependency stanza at all while its own description said nib "runs a
// loopback-only web app in a chromeless browser window" — so installing on a machine with
// no browser SUCCEEDED, nib started, bound its loopback port, and had no way to display
// itself. The user sees a process that did nothing, which is the worst shape a missing
// dependency can take: not an error, an absence.
//
// The candidate names are checked against internal/browser's OWN list rather than against
// a literal here. That is the point of the test: the packaging and the code that does the
// looking are two statements of the same fact, and a browser added to one and not the
// other is exactly the drift that produces a package recommending something nib will
// never launch.
// A red proof asserts that a defect makes a check fail. It does NOT assert that the
// patch contains only the defect — and it structurally cannot, because a patch carrying
// extra hunks still applies and the PROVE command still fails for its own reason and
// prints its own EXPECT.
//
// That gap was not theoretical. Four P07.S08 rows were generated with a bare `git diff`
// while `test/redproofs/*.patch` are themselves TRACKED, so each regeneration swept the
// previously-rewritten patches into the next; one reached 214 lines and six hunks for a
// one-comment mutation, and all four replayed green in that state.
//
// One file per patch was already true of all 71 — a rule the set obeyed and nothing
// enforced, which is the definition of the thing that drifts.
func TestEveryRedProofPatchTouchesOneFile(t *testing.T) {
	patches, err := filepath.Glob("test/redproofs/*.patch")
	if err != nil {
		t.Fatal(err)
	}
	if len(patches) < 2 {
		t.Fatalf("found %d red-proof patches — this scan is not reading the directory, so "+
			"every assertion below would pass over nothing", len(patches))
	}
	for _, p := range patches {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var files []string
		for _, ln := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(ln, "+++ ") {
				files = append(files, strings.TrimPrefix(ln, "+++ "))
			}
		}
		if len(files) != 1 {
			t.Errorf("%s patches %d files, want 1 — a red proof whose patch carries more than "+
				"its own defect still replays green, because the check fails for its own reason "+
				"either way. Regenerate it with `git diff -- <one file>`. Files: %v",
				filepath.Base(p), len(files), files)
		}
	}
}

func TestPackageDeclaresABrowser(t *testing.T) {
	spec, err := os.ReadFile("build/nfpm.yaml")
	if err != nil {
		t.Fatalf("the package spec is missing: %v", err)
	}
	// COMMENT LINES STRIPPED FIRST. Without that, this whole check is satisfied by the
	// paragraph above the stanza explaining why the stanza exists — a mention is not a
	// declaration, and the prose here is unusually long precisely because the reasoning
	// matters. Caught by red-proving it: deleting the recommends block left the comment
	// behind, and the xdg-utils check went on passing.
	var live []string
	for _, line := range strings.Split(string(spec), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			live = append(live, line)
		}
	}
	pkg := strings.Join(live, "\n")
	if !strings.Contains(pkg, "recommends:") && !strings.Contains(pkg, "depends:") {
		t.Fatal("build/nfpm.yaml declares no dependencies, yet its own description says nib runs its UI in a browser window — installing with no browser leaves a process that starts and cannot show itself")
	}
	// xdg-utils is the OTHER route: with no Chromium-family binary, Open() falls back to
	// xdg-open, which is in that package.
	if !strings.Contains(pkg, "xdg-utils") {
		t.Error("build/nfpm.yaml does not mention xdg-utils — it is the fallback route internal/browser.tabOpener uses when no Chromium-family browser is found")
	}
	src, err := os.ReadFile("internal/browser/browser.go")
	if err != nil {
		t.Fatal(err)
	}
	// The Linux candidates, read out of the switch arm that serves this package's
	// platform. A name nib hunts for and the package never mentions is a machine where
	// nib would have worked and apt did not say so.
	linux := string(src)
	// The Linux arm is `default:`, not `case "linux"` — chromiumCandidates serves "linux
	// and friends" from the default branch. The first draft looked for the case label,
	// found nothing, and said so: its own stimulus assertion is what caught the wrong
	// assumption rather than letting the comparison below run over an empty list.
	at := strings.Index(linux, "default: // linux and friends")
	if at == -1 {
		t.Fatal("chromiumCandidates' linux arm is not where this check looks (it read the `default:` branch) — the comparison below would run over an empty list")
	}
	arm := linux[at:]
	if end := strings.Index(arm, "}"); end != -1 {
		arm = arm[:end]
	}
	names := regexp.MustCompile(`"([a-z0-9-]+)"`).FindAllStringSubmatch(arm, -1)
	if len(names) < 3 {
		t.Fatalf("only %d browser names parsed out of chromiumCandidates' linux arm — the scan is not reading it", len(names))
	}
	var unmentioned []string
	for _, m := range names {
		if !strings.Contains(pkg, m[1]) {
			unmentioned = append(unmentioned, m[1])
		}
	}
	// Not every candidate needs to be a Debian package name — google-chrome-stable is,
	// microsoft-edge and brave-browser ship their own repos — so this reports rather than
	// demands the whole set, and fails only if NONE of them is named.
	if len(unmentioned) == len(names) {
		t.Errorf("build/nfpm.yaml names none of the browsers nib actually looks for (%v) — whatever it recommends, nib will not launch it in app mode", unmentioned)
	}
}

// TestSourceIsFormatted keeps gofmt drift from accumulating silently.
//
// Three files were unformatted when this was added, one of them for long enough
// that a full-repo review reported the repo had exactly one — the count was wrong
// because nothing measured it. Formatting is not a quality bar worth arguing about;
// it is worth a guard precisely because it is mechanical, and a guard costs less
// than the recurring question of whether it matters.
//
// Reports every offender rather than the first, so one run fixes the set.
// gofmtCmd lists the source gofmt should check, EXCLUDING dot-directories.
//
// `gofmt -l .` walks into `.claude/worktrees/<name>` — another session's git worktree, a full
// second copy of this tree (CLAUDE.md's rule for two sessions on one repo). cmd/gofmt skips
// dot-prefixed FILES, not dot-prefixed DIRECTORIES, so today it is only clean by luck: a
// concurrent session with an unformatted buffer on disk reddens this test in THIS tree, naming a
// path nobody here owns. The three repo walks this file's siblings do were fixed the same way.
func gofmtCmd() *exec.Cmd {
	args := []string{"-l"}
	ents, err := os.ReadDir(".")
	if err != nil {
		return exec.Command("gofmt", "-l", ".")
	}
	for _, e := range ents {
		n := e.Name()
		if strings.HasPrefix(n, ".") {
			continue
		}
		if e.IsDir() || strings.HasSuffix(n, ".go") {
			args = append(args, n)
		}
	}
	return exec.Command("gofmt", args...)
}

func TestSourceIsFormatted(t *testing.T) {
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skip("gofmt not on PATH (a toolchain-less environment); formatting is unchecked here")
	}
	out, err := gofmtCmd().Output()
	if err != nil {
		t.Fatalf("gofmt -l: %v", err)
	}
	var bad []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		bad = append(bad, line)
	}
	if len(bad) > 0 {
		t.Errorf("unformatted files (run: gofmt -w .):\n  %s", strings.Join(bad, "\n  "))
	}
}

// TestDocsREADMEMatchesTheCaps keeps the README's cap figures from drifting off the
// constants they describe.
//
// **Numbers, not prose.** Seven of the P05 phase-close review's findings were comments
// that outlived their code, and a README paragraph is the same failure with a wider
// audience — but a guard over prose is a guard that fails on rewording, gets loosened,
// and then guards nothing. What can be checked mechanically is the part a reader will
// act on: "eight documents, or 512 MB of them". If someone raises maxOpenBytes and the
// README still says 512, a user plans around a number that is not true.
//
// The same shape verify_test.go already uses for the tier table: assert that what the
// document states is what the code does, for the states that are checkable.
// The README's -w/--in-place list names every command that actually offers it.
//
// It named two — optimize and sanitize — while ten wire the flag. Someone reading the
// README would not know that `nib rotate -w *.pdf` works, and someone changing the set has
// nothing telling them the README exists. The list drifted because keeping it accurate was
// nobody's job in particular; this makes it the compiler's.
//
// Derived from the flag registration, which is the mechanical marker: a command supports
// -w exactly when it calls inPlaceFlag. The stdin/`-` list in the same section is NOT
// guarded here and that is a real gap, named rather than left silent — reading from stdin
// has no single marker (it goes through readInput, singleInput and runTransform in
// different shapes), so deriving it would mean a scan that is wrong in a way nobody
// notices, which is worse than an unguarded sentence.
func TestREADMEListsEveryInPlaceCommand(t *testing.T) {
	src, err := os.ReadFile("internal/cli/commands.go")
	if err != nil {
		t.Fatal(err)
	}
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}

	// Walk the file remembering the enclosing `func cmdX`, and record those that register
	// the in-place flag.
	var cmds []string
	cur := ""
	fnRe := regexp.MustCompile(`^func cmd([A-Za-z]+)\(`)
	for _, line := range strings.Split(string(src), "\n") {
		if m := fnRe.FindStringSubmatch(line); m != nil {
			cur = strings.ToLower(m[1])
		}
		if strings.Contains(line, "inPlaceFlag(fs") && cur != "" {
			cmds = append(cmds, cur)
			cur = "" // one registration per command
		}
	}
	// The stimulus. A scan that matched nothing would report the README complete forever.
	if len(cmds) < 5 {
		t.Fatalf("only %d in-place commands found in internal/cli/commands.go — the scan is not reading it", len(cmds))
	}

	// The sentence that lists them, so a command named anywhere else in the README (in the
	// command table, say) does not count as documented here.
	at := strings.Index(string(readme), "`--in-place` to rewrite each file")
	if at == -1 {
		t.Fatal("the README no longer has the in-place paragraph this checks")
	}
	start := strings.LastIndex(string(readme)[:at], "\n\n")
	para := string(readme)[start:at]

	var missing []string
	for _, c := range cmds {
		if !strings.Contains(para, "`"+c+"`") {
			missing = append(missing, c)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("these commands take -w/--in-place and the README's list does not name them: %s", strings.Join(missing, ", "))
	}
}

func TestDocsREADMEMatchesTheCaps(t *testing.T) {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile("internal/server/server.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(readme)

	// The stimulus: the README must actually describe the caps, or every assertion
	// below is a scan over a document that says nothing — health reported over silence.
	if !strings.Contains(text, "whichever comes first") {
		t.Fatal("the README no longer describes the open-document caps, so this guard covers nothing")
	}

	for _, c := range []struct{ decl, phrase, what string }{
		{"maxOpenDocs = 8", "eight documents", "the document-count cap"},
		{"maxOpenBytes = 512 << 20", "512 MB", "the aggregate byte ceiling"},
	} {
		if !strings.Contains(string(src), c.decl) {
			t.Errorf("%s is no longer `%s` — the README's figure is now describing something else", c.what, c.decl)
		}
		if !strings.Contains(text, c.phrase) {
			t.Errorf("the README does not state %s as %q — a reader plans around a number that is not true", c.what, c.phrase)
		}
	}
}

// TestEveryPlatformCompiles is the guard that was missing when `nib.exe` stopped existing.
//
// internal/discovery called syscall.SetsockoptInt with an `int` fd, which is a
// syscall.Handle on Windows, so `GOOS=windows go build ./cmd/nib` failed and the binary
// could not be produced AT ALL — on the platform whose `nib register` command exists only
// for it. Every tier stayed green, because every tier builds for the host.
//
// The cost is three cross-compiles of a small tree; the thing it catches is the whole
// product missing on a platform the README documents.
func TestEveryPlatformCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cross-compile guard under -short")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	for _, goos := range []string{"windows", "darwin", "linux"} {
		t.Run(goos, func(t *testing.T) {
			cmd := exec.Command("go", "build", "-o", filepath.Join(t.TempDir(), "out"), "./cmd/nib")
			cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH=amd64", "CGO_ENABLED=0")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("GOOS=%s does not build:\n%s", goos, out)
			}
		})
	}
}

// TestNoBuildTaggedSiblingIsAStub. Two build-tagged shims exist in this tree, and mcast.go's
// own note names the hazard they carry: "a no-op sibling is the shape that already shipped
// one silent defect here (ReplaceOthers returning 0 off Linux)". That note argued against
// having them at all, and the result was worse — no Windows binary. So: have them, and
// assert that a deliberate gap is DECLARED rather than merely present.
func TestNoBuildTaggedSiblingIsAStub(t *testing.T) {
	// setReuseAddr must do the real thing on both platforms; oNoFollow is genuinely
	// unavailable on Windows and its file must say so in as many words.
	for path, must := range map[string]string{
		"internal/discovery/reuseaddr_unix.go":    "SetsockoptInt",
		"internal/discovery/reuseaddr_windows.go": "SetsockoptInt",
		"internal/cli/nofollow_unix.go":           "O_NONBLOCK",
		"internal/cli/nofollow_windows.go":        "real gap",
	} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s is gone — if the shim was collapsed, check that every GOOS still "+
				"builds (TestEveryPlatformCompiles) and delete this row deliberately: %v",
				path, err)
			continue
		}
		if !bytes.Contains(b, []byte(must)) {
			t.Errorf("%s no longer contains %q — a build-tagged sibling that quietly stopped "+
				"doing its job is invisible to every tier that builds for the host", path, must)
		}
		if !bytes.Contains(b, []byte("//go:build")) {
			t.Errorf("%s has no build tag, so both siblings compile into every build", path)
		}
	}
}
