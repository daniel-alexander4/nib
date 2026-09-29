package sign

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAMemberTheCensusCannotDecodeIsUnseen is /pending 752(2)'s own case. `unseenSignatures` decodes an
// object-stream member only when its raw bytes could hold a signature key, and a member that passed
// that filter and did not decode was SKIPPED — so the cross-check vouched for an object it never read.
// Here Bob's second signature is a hybrid-stream member (so the signature reader never sees it) whose
// dictionary pdfcpu cannot parse: on the pre-change tree `Verify` read `valid` over Alice alone.
func TestAMemberTheCensusCannotDecodeIsUnseen(t *testing.T) {
	a := newIdentity(t, "Alice")
	doc := secondRevision(t, a, "<</Type/Sig/Filter/Adobe.PPKLite/Contents<0A1B", map[int]bool{7: true})
	// STIMULUS: pdfcpu reads the file, it is hybrid, and its object 7 is present and does not decode.
	ctx, err := pdfcpuRead(doc)
	if err != nil || !hybridReference(doc) {
		t.Fatalf("STIMULUS: pdfcpu err=%v hybrid=%v", err, hybridReference(doc))
	}
	if e := ctx.XRefTable.Table[7]; e == nil || e.Free {
		t.Fatalf("STIMULUS: pdfcpu holds no object 7")
	}
	st := Verify(doc)
	if len(st.Signers) != 1 || st.Signers[0].Fingerprint != a.fp {
		t.Fatalf("STIMULUS: signers %+v, want Alice alone", st.Signers)
	}
	if st.State != Invalid || st.Unchecked != UncheckedHybridReference || st.AddedAfterCause != AddedAfterCouldNotCheck {
		t.Errorf("state=%q unchecked=%q cause=%q, want invalid/%q/%q — a member nib could not read was counted as no signature",
			st.State, st.Unchecked, st.AddedAfterCause, UncheckedHybridReference, AddedAfterCouldNotCheck)
	}
}

// TestNoProducerMemberFailsTheCensusDecode holds the fail-closed rule to the corpus: no real producer's
// file may read `unseen`, or the rule would call an honest document `Invalid`.
func TestNoProducerMemberFailsTheCensusDecode(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("SKIP (not a pass): no home directory: %v", err)
	}
	files, _ := filepath.Glob(filepath.Join(home, "nib", "producers", "*", "*.pdf"))
	if len(files) == 0 {
		t.Skip("SKIP (not a pass): the real-producer corpus is absent")
	}
	for _, f := range files {
		doc, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		ctx, err := pdfcpuRead(doc)
		if err != nil {
			continue // unreadable is ADR-041's answer, not this census's
		}
		revs, err := sweepRevisions(doc)
		if err != nil {
			t.Errorf("%s: the sweep errored: %v", f, err)
			continue
		}
		if unseenSignatures(ctx, revs) {
			t.Errorf("%s: reads as carrying a signature the sweep did not see", f)
		}
	}
	t.Logf("%d producer files", len(files))
}
