package sign

import (
	"errors"
	"testing"
	"time"

	"nib/internal/testpdf"
)

// A normally signed doc — single or co-signed — has its last signature covering
// to EOF, so nothing reads as "added after signing".
func TestNoTrailingContentOnSignedDocs(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	certA, keyA, _ := GenerateIdentity("Alice")
	certB, keyB, _ := GenerateIdentity("Bob")

	one, err := SignApproval(base, certA, keyA, Options{Name: "Alice", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if st := Verify(one); st.AddedAfter {
		t.Error("singly-signed doc wrongly flagged as having content added after signing")
	}
	two, err := SignApproval(one, certB, keyB, Options{Name: "Bob", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if st := Verify(two); st.AddedAfter {
		t.Error("co-signed doc wrongly flagged — content between signatures is normal, only after-last counts")
	}
}

// Content appended after the last signature must be flagged (covered by no
// signature), without invalidating the signature itself.
func TestTrailingContentAfterLastSignatureFlagged(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	cert, key, _ := GenerateIdentity("Signer")
	signed, err := SignApproval(base, cert, key, Options{Name: "Signer", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	appended := append(append([]byte{}, signed...), []byte("\n% content added after signing\n")...)

	st := Verify(appended)
	if !st.AddedAfter {
		t.Error("content appended after the last signature was not flagged")
	}
	if len(st.Signers) == 0 || !st.Signers[0].Valid {
		t.Error("the signature itself should still verify over its own byte range")
	}
}

func TestUnsignedHasNoTrailingFlag(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	if st := Verify(base); st.AddedAfter {
		t.Error("unsigned doc wrongly flagged")
	}
}

// TestAddedAfterFailsClosed — an unreadable trailing-content check reports "warn", never
// "clean".
//
// Until P01.S02 two enumerations stood behind a verify — the library's xref walk that finds
// signatures and a separate AcroForm/Fields walk that read their byte ranges — and the "two
// enumerations" review's worry was that the added-after-signing warning could go quiet
// independently of the Valid verdict. It used to: `st.AddedAfter, _ = trailingContent…`
// discarded the error, so a document the check could not read reported AddedAfter=false and
// looked wholly signed.
//
// P01.S02 deleted the `/Fields` walk (ADR-059): coverage is now measured over the sweep's
// records (`coverage`), and the error reaching the verdict is the sweep's or the join's
// (`errJoin`, ADR-058) — reachable through Verify, and each named route is tested in
// `addedafter_test.go` and `revisions_test.go`. The discard was a trap for the same reason it
// always was: a verdict that ignores its error reports "clean" with nothing failing. So the rule
// is tested where it is decidable — the combine itself, which is why it is a named function with
// one caller, `addedAfter`.
func TestAddedAfterFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name       string
		trailing   bool
		sawSig     bool
		err        error
		libSigners bool
		want       bool
	}{
		{"clean and readable", false, true, nil, true, false},
		{"content found", true, true, nil, true, true},
		// **The row that isolates the ERROR arm, and it was missing (2026-08-25, v1.117.156).**
		//
		// Both enumerations agree a signature is there, so the disagreement rule above does not
		// fire; nothing trailing was found, so `trailing` is false. The ONLY thing that can make
		// this warn is `err != nil` — which is what this test is named for. Found by re-recording
		// this row's red proof against HEAD: with `return trailing || err != nil` cut down to
		// `return trailing`, every other row here still passed, because /pending 270's
		// disagreement rule (added later) satisfies one and `trailing` satisfies the other. The
		// arm had been uncovered since that rule landed and the table still read as if it were
		// tested — the guard's own claim, gone quiet exactly the way the verdict it guards could.
		{"readable-signature disagreement aside, an errored check alone warns", false, true, errParseFail, true, true},
		{"unreadable is a warning, not clean", false, false, errParseFail, true, true},
		{"unreadable stays a warning even if it also found content", true, true, errParseFail, true, true},
		// An unsigned document: neither enumeration sees a signature, and there is nothing to
		// warn about. This is the row that stops the disagreement rule below from firing on
		// every unsigned file.
		{"unsigned document is not a disagreement", false, false, nil, false, false},
	} {
		if got := addedAfterVerdict(tc.trailing, tc.sawSig, tc.err, tc.libSigners); got != tc.want {
			t.Errorf("%s: addedAfterVerdict(%v, %v, %v, %v) = %v, want %v — a trailing check that "+
				"errored must not report the document clean", tc.name, tc.trailing, tc.sawSig,
				tc.err, tc.libSigners, got, tc.want)
		}
	}
}

// TestTheTwoEnumerationsDisagreeingIsAWarning — /pending 270, and it is the case the "two
// enumerations" review was actually about.
//
// It was written when `AddedAfter` read coverage from its own `AcroForm/Fields` walk while the
// library walked `rdr.Xref()` — two walks that a document with `/SigFlags` and an unlisted
// signature satisfied one of and not the other. P01.S02 deleted that walk (ADR-059), and the rule
// survives with its meaning moved: `sawSignature` is now "some record BOUNDS coverage" (verified,
// well-formed, not a document timestamp), so the library reporting a signer that no such record
// stands for — every signer failed, was refused, or is a stamp — still cannot be reported as a
// wholly signed document. The name is kept for the replayed red proof.
//
// The old shape could not express that: `trailingContentAfterLastSignature` returned
// `(false, nil)` for "no signature fields here" AND for "the signatures cover everything", so
// the caller could not tell an agreement from an absence — and a Valid document whose bytes
// after the signature are covered by nothing reported clean.
//
// The end-to-end cases are now built (`TestASoleFailedSignatureCannotBeChecked`,
// `TestNoBoundingSignatureOutranksARefusal`); this row stays because the composition rule is
// where the defect lives, which is why it is a named function.
func TestTheTwoEnumerationsDisagreeingIsAWarning(t *testing.T) {
	// The library found a signature; this walk found no signature field to measure against.
	if !addedAfterVerdict(false, false, nil, true) {
		t.Error("the signature walk found a signer and the byte-range walk found no signature " +
			"field at all, and the document was reported as wholly signed — the two enumerations " +
			"disagree, which is exactly the case that cannot be confirmed either way")
	}
	// The control, and it is what keeps the rule from being "always warn": when both walks
	// agree that a signature exists and nothing follows it, that is a clean document.
	if addedAfterVerdict(false, true, nil, true) {
		t.Error("both enumerations agree the document ends at its signature, and it was still " +
			"reported as added-after — the rule has become unconditional")
	}
}

var errParseFail = errors.New("malformed PDF")
