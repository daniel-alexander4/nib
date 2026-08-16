package nib

import (
	"os"
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
