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
// The two enumerations behind a verify — the xref walk that finds signatures and the
// AcroForm/Fields walk that reads their byte ranges — can in principle disagree, and the
// "two enumerations" review's worry was that the added-after-signing warning could go quiet
// independently of the Valid verdict. It used to: `st.AddedAfter, _ = trailingContent…`
// discarded the error, so a document the check could not read reported AddedAfter=false and
// looked wholly signed.
//
// It is unreachable through Verify TODAY — both calls run dpdf over the same bytes, so one
// cannot fail to parse while the other succeeds — which is exactly why the discard was a trap
// and not a caught bug: the day the trailing check grows an error path the signature walk
// does not share, "clean" becomes a lie with nothing failing. So the rule is tested where it
// is decidable: the combine itself, which is why it is a named function.
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
// The library gates on `Root/AcroForm/SigFlags` and then walks `rdr.Xref()` for objects whose
// `/Filter` is `Adobe.PPKLite`. This check walks `AcroForm/Fields` for `FT /Sig` byte ranges.
// Those are genuinely different walks over the same PARSED document — no malformed file
// required — so a document carrying `/SigFlags` whose `/Fields` does not list the signature
// satisfies one and not the other.
//
// The old shape could not even express that: `trailingContentAfterLastSignature` returned
// `(false, nil)` for "no signature fields here" AND for "the signatures cover everything", so
// the caller could not tell an agreement from an absence — and a Valid document whose bytes
// after the signature are covered by nothing reported clean.
//
// **What this does not cover, stated rather than implied:** the end-to-end case still wants a
// crafted document, and building one means hand-rolling an xref-STREAM incremental update,
// because that is what the signing library writes. The composition rule is where the defect
// lives and where it is decidable, which is why it is a named function.
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
