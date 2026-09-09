package ceremony

import "testing"

// TestTheAttestedAndDerivedVocabulariesAreDisjoint — the collision class, closed structurally.
//
// **`Receipt.State` carries BOTH vocabularies and its conflict rule compares strings.**
// `WriteReceipt` refuses a second, different state with `ErrReceiptConflict` and returns nil when
// `prev.State == r.State` — so two states that happen to share a word are treated as the same
// observation and merge with no trace. Its own doc says why that matters: without the rule *"a
// re-sweep would overwrite 'they declined on the 2nd' with 'nothing ever said', destroying the
// better answer with the worse one and leaving no trace that it had been there."* A shared word
// defeats the rule silently, because the guard never fires.
//
// **Until `/pending 428` the sets were disjoint by ACCIDENT.** `{declined, completed}` and
// `{expired, abandoned, left}` simply happened not to overlap; nothing enforced it, and the first
// proposal for this item was to attest under the word `abandoned` — which would have collided with
// the derived state meaning the exact opposite, *"a proceeding that ended without reaching this
// machine at all"*.
//
// **This is why the receipt needs no `source` field.** A per-receipt "attested or derived" marker
// was the other candidate remedy; with the vocabularies provably disjoint the state word already
// answers it, and a field that restates what the word says is a second copy of one fact. The guard
// is what makes the word sufficient — so it replaces the field rather than accompanying it.
func TestTheAttestedAndDerivedVocabulariesAreDisjoint(t *testing.T) {
	attested := []string{StateDeclined, StateCompleted, StateStopped}
	derived := []string{StateExpired, StateAbandoned, StateLeft}
	for _, a := range attested {
		for _, d := range derived {
			if a == d {
				t.Errorf("%q is both attested and derived. Receipt.State carries both "+
					"vocabularies and WriteReceipt's conflict rule compares STRINGS, so the two "+
					"meanings merge and the guard that exists to stop the worse answer "+
					"overwriting the better one never fires", a)
			}
		}
	}
	// The stimulus floor: both sets are non-empty and populated with the real constants, so this
	// is not a loop over nothing. A test that iterated two empty slices would pass forever.
	if len(attested) < 3 || len(derived) < 3 {
		t.Fatalf("setup: %d attested and %d derived states — the comparison below is vacuous",
			len(attested), len(derived))
	}
	// And every attested state really is attestable, so the first list cannot drift into being a
	// list of words nobody signs.
	for _, a := range attested {
		if !attestable(a) {
			t.Errorf("%q is in this test's attested list and SignTermination will not mint it", a)
		}
	}
}

// TestASignedEndStateSurvivesTheCloseOut — ADR-012's whole point, applied to the attestation.
//
// **The close-out MOVES a ceremony's folder and nothing deletes it**, because on every machine but
// the convener's the mirror holds the only copy of that party's own signature. `ReadTermination`
// looked only in `ceremonies/<id>`, so the convener's signed end state became unreadable the moment
// the sweep ran — and `nib verify` fell through to the unattested receipt, reporting the outcome as
// *"this machine's own note … not signed"*.
//
// **It matters most for a stop.** The value of attesting one is that a party can later show the
// convener ended it; the folder they would show it from is the moved one.
func TestASignedEndStateSurvivesTheCloseOut(t *testing.T) {
	rec, cert, key := terminationFixture(t)
	root := t.TempDir()
	if _, err := WriteMirror(root, rec, nil); err != nil {
		t.Fatal(err)
	}
	term, err := SignTermination(rec, StateStopped, cert, key)
	if err != nil {
		t.Fatal(err)
	}
	if werr := WriteTermination(root, term); werr != nil {
		t.Fatal(werr)
	}
	// The CONTROL: it reads before the close-out. Without this the assertion after the move is
	// satisfied by a reader that never worked at all.
	if got, rerr := ReadTermination(root, rec); rerr != nil || got.State != StateStopped {
		t.Fatalf("setup: the attestation does not read before the close-out (%v, %q)", rerr, got.State)
	}
	if cerr := CloseOutMirror(root, rec.ID); cerr != nil {
		t.Fatal(cerr)
	}
	got, rerr := ReadTermination(root, rec)
	if rerr != nil {
		t.Fatalf("after the close-out the convener's SIGNED end state is unreadable (%v) — so "+
			"`nib verify` reports the outcome as this machine's own unsigned note, in the one tool "+
			"whose reader is deciding whether to rely on the document", rerr)
	}
	if got.State != StateStopped {
		t.Errorf("the recovered end state is %q, want %q", got.State, StateStopped)
	}
}
