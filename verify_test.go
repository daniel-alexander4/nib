package nib

import (
	"os"
	"os/exec"
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
	for _, cmd := range []string{"build/jsdomtest.sh", "build/uirepro.sh"} {
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
	for _, ceiling := range []string{"Cannot see: the client at all", "Cannot see: anything that needs rendering", "Cannot see: other engines"} {
		if !strings.Contains(contract, ceiling) {
			t.Errorf("CONTRIBUTING.md is missing a tier ceiling: %q", ceiling)
		}
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
