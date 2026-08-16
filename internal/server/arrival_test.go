package server

import (
	"bytes"
	"testing"

	"nib/internal/testpdf"
)

// P03.S05's acceptance: an arrival ADDS (D10).
//
// The distinction this file exists to hold is that "the arrival is open" and "the
// arrival replaced what was open" look identical from the arrival's side — both end
// with the co-signed document active and rendering. The whole change is in what
// happens to the OTHER document, which nothing observed before this slice, and which
// is why the loss was silent: completing a co-signature discarded the recipient's
// open document and its undo history, work they never asked to close.
func TestArrivalAddsRatherThanReplacing(t *testing.T) {
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	s := openTestServer(t, pdf)

	existing := s.activeDoc()
	existingID := existing.id
	// Give the open document a history, so "it survived" is a claim with something
	// behind it. A document with no history survives an arrival that wiped every
	// history, and the assertion would not know the difference.
	s.commitMutation(existing, pdf, pdf)
	if len(existing.undo) == 0 {
		t.Fatal("setup: the open document has no history, so its survival would prove nothing")
	}

	arrived := []byte("co-signed bytes")
	installed := s.addDoc(&document{data: arrived})

	// The stimulus first: the arrival must actually have been installed, or every
	// assertion below is graded against an operation that did nothing.
	if installed == nil || !installed.id.valid() {
		t.Fatal("addDoc installed nothing")
	}

	// 1. The arrival is open and active.
	if s.activeDoc() != installed {
		t.Error("the arrival is not the active document — with no tab strip it would be unreachable")
	}
	if !bytes.Equal(s.activeDoc().data, arrived) {
		t.Error("the active document is not the arrived bytes")
	}

	// 2. And the previously open document SURVIVED, with its history.
	if documentByID(s, existingID) == nil {
		t.Fatal("the arrival discarded the open document — the user's work closed without being asked")
	}
	if len(existing.undo) == 0 {
		t.Error("the previously open document lost its undo history to an arrival")
	}
	if existing.historyEvicted {
		t.Error("the surviving document was marked history-evicted by an arrival")
	}

	// 3. Two documents, distinct ids, ADR-001's law intact across the add.
	if len(s.docs) != 2 {
		t.Errorf("registry holds %d documents, want 2", len(s.docs))
	}
	if installed.id == existingID {
		t.Fatal("the arrival reused the open document's id")
	}
	if installed.id.Seq <= existingID.Seq {
		t.Errorf("ids must strictly increase: existing %d, arrival %d", existingID.Seq, installed.id.Seq)
	}
}

// setDoc keeps its replace contract. Asserted alongside addDoc rather than assumed,
// because the two now differ only in what they do NOT do, and a split that quietly
// made both add would pass every test that only looks at the new document.
func TestSetDocStillReplaces(t *testing.T) {
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	s := openTestServer(t, pdf)
	first := s.activeDoc()
	s.commitMutation(first, pdf, pdf)

	second := s.setDoc(&document{data: []byte("replacement")})

	if len(s.docs) != 1 {
		t.Errorf("setDoc left %d documents open, want 1 — it must still replace", len(s.docs))
	}
	if s.activeDoc() != second {
		t.Error("setDoc did not make its document active")
	}
	// And the outgoing document's history is released, not left for the GC — the
	// property P01's "close retains no document bytes" criterion rests on.
	if len(first.undo) != 0 {
		t.Errorf("the replaced document kept %d undo states", len(first.undo))
	}
}

// Ids stay monotonic across a MIX of adds and replaces. Worth its own test because
// the two paths now share one minting site, and a split that gave addDoc its own
// counter would produce colliding ids the moment both were used — ADR-001 defeated
// by a second issuer rather than by reuse.
func TestIDsStayUniqueAcrossAddsAndReplaces(t *testing.T) {
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	s := openTestServer(t, pdf)

	seen := map[docID]bool{s.activeDoc().id: true}
	for i := 0; i < 4; i++ {
		d := s.addDoc(&document{data: pdf})
		if seen[d.id] {
			t.Fatalf("addDoc reissued id %s", d.id)
		}
		seen[d.id] = true

		r := s.setDoc(&document{data: pdf})
		if seen[r.id] {
			t.Fatalf("setDoc reissued id %s", r.id)
		}
		seen[r.id] = true
	}
	if len(seen) != 9 {
		t.Errorf("got %d distinct ids across 9 opens", len(seen))
	}
}
