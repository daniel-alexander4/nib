package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAPendingTimestampProofCanBeUpgradedAndSaved — /pending 388, closed as overturned.
//
// # The item, and why it was wrong
//
// It said: "`ots.Stamp` assembles pending attestations, Bitcoin confirmation takes hours, and the
// completed attestation has to be fetched back from the calendar that issued it. Named search —
// `grep -niE 'upgrade|calendar' internal/ots/*.go` — returns the submit path and tests only:
// **nothing in the tree ever re-fetches a pending proof.** So a `.ots` Nib writes reports `pending`
// on every verification, forever."
//
// That search matches `func upgrade(` in verify.go and its call site. The whole chain was already
// built, and the entry misread its own grep — the same shape as /pending 450 and 445 in this
// backlog.
//
// # The chain, which is what this test pins
//
// Four links, in three files, and the item exists because somebody believed one of them was
// missing. A guard is the closure artefact: it stops the item being re-filed from a fresh reading,
// and it fails loudly if a link is removed — which would restore exactly the "pending forever"
// state the entry described.
//
// It also records that the entry's four open "decisions" were all taken, and taken the right way.
// The upgrade is triggered by the user's Verify, not by a background fetch on open — which matters
// on an offline-first product. And the upgraded proof is OFFERED for saving rather than written
// back over the user's `.ots`: silently rewriting a file somebody may be holding as evidence in a
// dispute is the one thing this product must not do.
func TestAPendingTimestampProofCanBeUpgradedAndSaved(t *testing.T) {
	read := func(p string) string {
		t.Helper()
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	verify := read(filepath.Join("..", "ots", "verify.go"))
	server := read("timestamp.go")
	client := read(filepath.Join("..", "..", "web", "app.js"))

	// Stimulus floors, one per file: a file that failed to read, or a scan pointed at the wrong
	// one, contains none of the needles below and would report every link missing rather than
	// present — loud, but for the wrong reason. These say which.
	for _, f := range []struct{ name, src, anchor string }{
		{"internal/ots/verify.go", verify, "func VerifyProof("},
		{"internal/server/timestamp.go", server, "func (s *Server) handleTimestampVerify("},
		{"web/app.js", client, "els.tvSave"},
	} {
		if !strings.Contains(f.src, f.anchor) {
			t.Fatalf("%s does not contain %q, so this scan is not reading the file it thinks it is",
				f.name, f.anchor)
		}
	}

	// LINK 1 — the pending path re-fetches from the calendar. This is the one the entry said did
	// not exist.
	if !strings.Contains(verify, "upgrade(ctx, client, s, p.digest)") {
		t.Error("VerifyProof no longer upgrades a pending sequence against its calendar. A .ots " +
			"Nib wrote then reports `pending` on every verification forever, and the state a user " +
			"means by \"once it is complete\" becomes unreachable — which is /pending 388 verbatim")
	}
	// LINK 2 — the upgraded proof is returned as something that can be persisted.
	if !strings.Contains(verify, "res.Upgraded = serialize(") {
		t.Error("VerifyProof no longer returns the upgraded proof, so the upgrade happens and is " +
			"thrown away: the user sees `confirmed` once and has nothing to keep")
	}
	// LINK 3 — the server publishes it.
	if !strings.Contains(server, `json:"upgraded,omitempty"`) || !strings.Contains(server, "base64.StdEncoding.EncodeToString(res.Upgraded)") {
		t.Error("the verify route no longer publishes the upgraded proof. NOTE: this field sits on " +
			"an ANONYMOUS response struct, so observables_test.go cannot see it — " +
			"`discoverObservables` collects only *ast.TypeSpec — and this is the only thing that " +
			"checks it has a reader at all")
	}
	// LINK 4 — the client reads it and offers a SAVE rather than overwriting the user's file.
	if !strings.Contains(client, "if (r.upgraded)") {
		t.Error("web/app.js no longer reads `upgraded` from the verify response, so the completed " +
			"proof crosses the wire and is discarded by the only thing that could keep it")
	}
	if !strings.Contains(client, "'Save complete proof'") {
		t.Error("the completed proof is no longer offered through a save dialog. It must not be " +
			"written back over the user's .ots: that file may be what they are holding as evidence, " +
			"and rewriting it silently is strictly more evidence delivered as a surprise")
	}
}
