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
		name     string
		trailing bool
		err      error
		want     bool
	}{
		{"clean and readable", false, nil, false},
		{"content found", true, nil, true},
		{"unreadable is a warning, not clean", false, errParseFail, true},
		{"unreadable stays a warning even if it also found content", true, errParseFail, true},
	} {
		if got := addedAfterVerdict(tc.trailing, tc.err); got != tc.want {
			t.Errorf("%s: addedAfterVerdict(%v, %v) = %v, want %v — a trailing check that "+
				"errored must not report the document clean", tc.name, tc.trailing, tc.err, got, tc.want)
		}
	}
}

var errParseFail = errors.New("malformed PDF")
