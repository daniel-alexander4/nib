package ceremony

import (
	"errors"
	"nib/internal/sign"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/p2p"
	"nib/internal/testpdf"
)

// mirrorNow is the clock these tests verify against. A fixed instant, because ReadMirror now
// verifies the stored record and Record.Verify takes a `now` — a wall-clock read inside a
// verdict is nondeterminism reaching a decision, and a test that passed time.Now() would be
// asserting against a different record every run.
var mirrorNow = time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

// convened builds a real convened document and its record, so the mirror tests operate on
// bytes that genuinely hash to what the record says.
func convened(t *testing.T) (Record, []byte) {
	t.Helper()
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Convene(base, ConveneRequest{
		Roster:        []Party{{Fingerprint: cfp, Label: "Convener", Signs: true}, {Fingerprint: afp, Label: "A", Signs: true}},
		Intent:        "We agree to co-sign the lease",
		Expires:       time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
		HopBudget:     29*time.Minute + 20*time.Second,
		ConvenerSigns: true,
	}, cert, key, time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return got.Record, got.Document
}

func TestTheMirrorRoundTripsAndVerifiesWhatItStored(t *testing.T) {
	root := t.TempDir()
	rec, doc := convened(t)

	dir, err := WriteMirror(root, rec, doc)
	if err != nil {
		t.Fatal(err)
	}
	// Stimulus: both files really landed, or the round trip below is reading nothing.
	for _, name := range []string{"document.pdf", "record.json"} {
		if _, serr := os.Stat(filepath.Join(dir, name)); serr != nil {
			t.Fatalf("setup: %s was not written: %v", name, serr)
		}
	}
	back, pdf, err := ReadMirror(root, rec.ID, mirrorNow)
	if err != nil {
		t.Fatal(err)
	}
	if back.ID != rec.ID || back.DocHash != rec.DocHash {
		t.Error("the mirror returned a different record")
	}
	if len(pdf) != len(doc) {
		t.Errorf("the mirror returned %d bytes, stored %d", len(pdf), len(doc))
	}
}

// TestAHalfWrittenMirrorIsReportedRatherThanBlamedOnACounterparty.
//
// Before P07.S02a a torn document came back as success, and the resuming party then failed at
// CheckDocument with "these are not the same document" — an accusation about a counterparty,
// for damage that happened on this machine's own disk.
func TestAHalfWrittenMirrorIsReportedRatherThanBlamedOnACounterparty(t *testing.T) {
	root := t.TempDir()
	rec, doc := convened(t)
	dir, err := WriteMirror(root, rec, doc)
	if err != nil {
		t.Fatal(err)
	}
	// Stimulus: it reads clean BEFORE the damage, so the refusal below is about the damage
	// and not about the fixture.
	if _, _, rerr := ReadMirror(root, rec.ID, mirrorNow); rerr != nil {
		t.Fatalf("setup: an undamaged mirror does not read back (%v)", rerr)
	}

	// Truncate the document the way an interrupted write does.
	if err := os.WriteFile(filepath.Join(dir, "document.pdf"), doc[:len(doc)/2], 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err = ReadMirror(root, rec.ID, mirrorNow)
	if !errors.Is(err, ErrMirrorDamaged) {
		t.Fatalf("a truncated stored document reported %v, want ErrMirrorDamaged — returning it "+
			"as success hands the caller bytes that will be refused later, by a sentence blaming "+
			"the far party", err)
	}
	if strings.Contains(err.Error(), "not the same document") {
		t.Errorf("the refusal uses CheckDocument's accusatory wording: %v", err)
	}
}

// TestTheRecordIsTheCommitPoint — the WRITE ORDER, observed by making the second write fail.
//
// The first draft wrote successfully, removed record.json by hand, and asserted ReadMirror
// failed. That is a property of the READER: measured at the slice's diff review, swapping the
// two writes in mirror.go — the exact defect the test is named after — left it green.
//
// The order is what makes a torn write legible. With record.json first, the surviving state
// is (record, no document), which is byte-identical to a deliberately document-less mirror
// that WriteMirror legitimately creates — so nothing can tell "no document yet" from "the
// document was lost". Document first inverts that: the record's presence means both landed.
//
// The stimulus is a record.json path that cannot be written because a DIRECTORY is already
// there. Whichever file WriteMirror writes first is the one that exists afterwards.
func TestTheRecordIsTheCommitPoint(t *testing.T) {
	root := t.TempDir()
	rec, doc := convened(t)
	dir, err := MirrorDir(root, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "record.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	// Stimulus: writing record.json must genuinely be impossible, or "document.pdf exists"
	// below says nothing about ordering.
	if err := os.WriteFile(filepath.Join(dir, "record.json"), []byte("x"), 0o600); err == nil {
		t.Fatal("setup: record.json is writable, so this test cannot make the second write fail")
	}

	_, werr := WriteMirror(root, rec, doc)
	if werr == nil {
		t.Fatal("WriteMirror reported success with record.json unwritable")
	}
	if _, serr := os.Stat(filepath.Join(dir, "document.pdf")); serr != nil {
		t.Errorf("document.pdf is absent after a failed record write (%v) — the record was "+
			"written FIRST, so a torn write leaves (record, no document), which is "+
			"indistinguishable from the document-less mirror WriteMirror legitimately creates",
			serr)
	}

	// And the inverse state — a record with no document — is a LEGITIMATE mirror, which is
	// exactly why it must not also be what a torn write produces.
	root2 := t.TempDir()
	if _, werr := WriteMirror(root2, rec, nil); werr != nil {
		t.Fatal(werr)
	}
	got, pdf, err := ReadMirror(root2, rec.ID, mirrorNow)
	if err != nil {
		t.Fatalf("a deliberately document-less mirror must read back cleanly: %v", err)
	}
	if len(pdf) != 0 {
		t.Error("a document-less mirror returned bytes")
	}
	if got.ID != rec.ID {
		t.Error("the record did not survive")
	}
}

var _ = p2p.SessionBudget

// TestASignedMirrorTruncatedAtAPriorRevisionIsCaught — P08.S02's C02, and the window the existing
// check could never see.
//
// `ReadMirror`'s `DocHash` comparison is gated on the document being UNSIGNED, and that gate is
// correct: `DocHash` is a convene-time identity, a visible signature adds a widget annot, and
// `ContentDigest` covers `/Annots`, so from the first signature the document legitimately stops
// hashing to it. The consequence, which that gate's own comment states, is that **from hop 2 onward
// the mirror is stored with no self-check at all** — and hop 2 onward is every mirror a resumption
// actually reads.
//
// The truncation that matters there is not a random cut. A signed PDF cut at a PRIOR `%%EOF` is a
// well-formed document one revision short: it parses, `sign.Verify` reports Valid with fewer
// signers, and every existing check passes it. That is the shape D24 describes as "a partial
// contribution that reads as *a* contribution".
func TestASignedMirrorTruncatedAtAPriorRevisionIsCaught(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	r := draft(t, cfp, afp)
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	base, err := testpdf.Text("the document in flight")
	if err != nil {
		t.Fatal(err)
	}
	// **SIGNED, and that is the whole fixture.** An unsigned document is covered by the `DocHash`
	// comparison, which would catch the truncation below and prove nothing about the window this
	// test is for. Signing it switches that comparison off — which is precisely the state every
	// mirror is in from hop 2 onward, and precisely where nothing checked anything until now.
	doc, err := sign.SignApproval(base, cert, key, sign.Options{Name: "Convener", Reason: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if sign.Verify(doc).State == sign.Unsigned {
		t.Fatal("setup: the fixture is unsigned, so the DocHash check is still live and this test " +
			"would be exercising it rather than the sidecar")
	}
	root := t.TempDir()
	dir, err := WriteMirror(root, r, doc)
	if err != nil {
		t.Fatal(err)
	}
	// Stimulus: it reads back cleanly first, so the refusal below is caused by the damage and not
	// by the fixture.
	if _, _, rerr := ReadMirror(root, r.ID, time.Now()); rerr != nil {
		t.Fatalf("setup: the intact mirror does not read back: %v", rerr)
	}

	// Truncate. The sidecar must catch this whether or not the document is signed — which is the
	// whole point, since the `DocHash` check is switched off for the signed case.
	if err := os.WriteFile(filepath.Join(dir, "document.pdf"), doc[:len(doc)-64], 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, rerr := ReadMirror(root, r.ID, time.Now())
	if rerr == nil {
		t.Fatal("a truncated stored document read back as clean — a resuming party would then " +
			"hand a partial contribution to a peer, or sign on top of one")
	}
	if !errors.Is(rerr, ErrMirrorDamaged) {
		t.Errorf("the truncation is reported as %v, want ErrMirrorDamaged — this happened on THIS "+
			"machine's disk, and the alternative sentence accuses a counterparty of substituting "+
			"a file", rerr)
	}

	// And a mirror with no sidecar at all still opens: every mirror written before this slice has
	// none, and refusing them would strand ceremonies in flight the day it ships.
	if err := os.WriteFile(filepath.Join(dir, "document.pdf"), doc, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "document.sha256")); err != nil {
		t.Fatal(err)
	}
	if _, _, rerr := ReadMirror(root, r.ID, time.Now()); rerr != nil {
		t.Errorf("a mirror written before the sidecar existed is refused: %v", rerr)
	}
}

// TestAFailedRewriteLeavesNoChecksumForADocumentItDidNotWrite — /pending 321.
//
// # The defect
//
// `WriteMirror`'s document-then-record ordering is a FIRST-WRITE argument, and P08.S01's scope said
// so before S02 added the third file: at hop 2 or later `record.json` and `document.sha256` both
// already exist. A crash between the document and the sidecar leaves a complete, valid document
// beside the PREVIOUS hop's checksum, and `ReadMirror`'s unconditional sidecar check then reports
// `ErrMirrorDamaged` forever — a false accusation against the user's own disk, with no repair path.
//
// # Why the probe is shaped this way
//
// The torn state itself needs a crash, and this repo refuses a fault knob in the product
// (`build/redproof.sh` argues it: "a switch whose whole purpose is to break the program is the same
// gun with a better excuse"). So the observable is the OTHER side of the same ordering: make the
// DOCUMENT write fail while a good sidecar exists, and ask what the sidecar is afterwards.
//
// That question **discriminates the two orderings**, which is the whole point:
//   - document-first (the defect): the document write fails before the sidecar is touched, so the
//     old checksum survives — a checksum for bytes this mirror may no longer hold.
//   - unlink-first (the fix): the sidecar is gone before the document is attempted, so a failed
//     write leaves no checksum at all, which ReadMirror tolerates by design.
//
// A probe that merely broke both orderings would prove the test runs, not that the fix works.
func TestAFailedRewriteLeavesNoChecksumForADocumentItDidNotWrite(t *testing.T) {
	root := t.TempDir()
	rec, doc := convened(t)

	dir, err := WriteMirror(root, rec, doc)
	if err != nil {
		t.Fatal(err)
	}
	// SETUP: hop 1 really did leave a readable sidecar, or "it is gone afterwards" is vacuously
	// true and this test says nothing about ordering.
	side := filepath.Join(dir, "document.sha256")
	before, err := os.ReadFile(side)
	if err != nil || len(before) == 0 {
		t.Fatalf("setup: no sidecar after the first write (%v) — nothing here can be stranded", err)
	}

	// Make the DOCUMENT write fail: a non-empty directory cannot be removed and cannot be renamed
	// over. Nothing touches the sidecar.
	docPath := filepath.Join(dir, "document.pdf")
	if err := os.Remove(docPath); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(docPath, "occupied"), 0o700); err != nil {
		t.Fatal(err)
	}
	// SETUP: the write must genuinely be impossible, or the assertion below is about nothing.
	if err := os.WriteFile(docPath, []byte("x"), 0o600); err == nil {
		t.Fatal("setup: document.pdf is writable, so this test cannot make the second write fail")
	}

	if _, werr := WriteMirror(root, rec, doc); werr == nil {
		t.Fatal("WriteMirror reported success with document.pdf unwritable")
	}

	if _, serr := os.Stat(side); serr == nil {
		t.Errorf("a checksum survived a failed document write. The sidecar is removed BEFORE the " +
			"document precisely so a torn rewrite leaves 'no sidecar' — which ReadMirror tolerates " +
			"— rather than 'a document beside the previous hop's checksum', which it reports as " +
			"ErrMirrorDamaged permanently, against the user's own disk, with no repair path.")
	}
}

// TestAMirrorWithNoSidecarReadsBackClean is the other half: the torn state the ordering above
// produces must actually be benign, or the fix trades a permanent false accusation for a different
// permanent failure. `ReadMirror` says a missing sidecar is tolerated; this drives it.
func TestAMirrorWithNoSidecarReadsBackClean(t *testing.T) {
	root := t.TempDir()
	rec, doc := convened(t)
	dir, err := WriteMirror(root, rec, doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "document.sha256")); err != nil {
		t.Fatal(err)
	}
	got, pdf, err := ReadMirror(root, rec.ID, mirrorNow)
	if err != nil {
		t.Fatalf("a mirror with no sidecar must read back cleanly — the ordering fix relies on it: %v", err)
	}
	if got.ID != rec.ID || len(pdf) != len(doc) {
		t.Error("the record or the document did not survive")
	}
}
