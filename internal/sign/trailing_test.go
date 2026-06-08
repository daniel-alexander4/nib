package sign

import (
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
