package nib

import (
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
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
	for _, cmd := range []string{"build/jsdomtest.sh", "build/uirepro.sh", "build/pairrepro.sh", "build/mcastrepro.sh", "build/winrepro.sh", "build/dhtlive.sh"} {
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
		live, err := os.ReadFile("internal/rendezvous/live_test.go")
		if err != nil {
			t.Fatalf("cannot read the live tests: %v", err)
		}
		harness, err := os.ReadFile("build/dhtlive.sh")
		if err != nil {
			t.Fatalf("cannot read build/dhtlive.sh: %v", err)
		}
		names := regexp.MustCompile(`(?m)^func (TestLive\w*)\(`).FindAllStringSubmatch(string(live), -1)
		if len(names) == 0 {
			t.Fatal("no TestLive* functions found — this guard would pass on nothing")
		}
		if !strings.Contains(string(harness), "-run 'TestLive'") {
			t.Errorf("build/dhtlive.sh does not run every TestLive* (it has %d to run); "+
				"a named -run means a new live test is executed by nothing and nothing says so",
				len(names))
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
	for i, cmd := range []string{"`go build ./...`", "`go test ./...`", "`./build/jsdomtest.sh`", "`./build/uirepro.sh`", "`./build/pairrepro.sh`", "`./build/mcastrepro.sh`"} {
		row := "| " + strconv.Itoa(i) + " | " + cmd
		if !strings.Contains(contract, row) {
			t.Errorf("the tier table has lost its row %d (%q) — the commands may still be named in prose, which is what made this check pass over a deleted table", i, row)
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
func TestSourceIsFormatted(t *testing.T) {
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skip("gofmt not on PATH (a toolchain-less environment); formatting is unchecked here")
	}
	out, err := exec.Command("gofmt", "-l", ".").Output()
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
