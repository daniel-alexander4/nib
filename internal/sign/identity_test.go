package sign

import (
	"errors"
	"strings"
	"testing"
	"time"

	"nib/internal/testpdf"
)

func TestSignAndVerifyRoundTrip(t *testing.T) {
	certPEM, keyPEM, err := GenerateIdentity("Nib Test")
	if err != nil {
		t.Fatal(err)
	}
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}

	signed, err := Sign(pdf, certPEM, keyPEM, Options{
		Name:   "Nib Test",
		Reason: "Finalized in Nib",
		When:   time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// A freshly signed document must verify as untampered.
	if st := Verify(signed); st.State != Valid {
		t.Fatalf("signed doc verify = %q, want valid", st.State)
	}

	// Any modification within the signed byte range must drop the valid status.
	tampered := append([]byte(nil), signed...)
	tampered[len(tampered)/3] ^= 0xFF
	if st := Verify(tampered); st.State == Valid {
		t.Error("tampered doc still verifies as valid — tamper-evidence broken")
	}
}

func TestSignRejectsBadIdentity(t *testing.T) {
	pdf, _ := testpdf.Form()
	if _, err := Sign(pdf, []byte("not pem"), []byte("not pem"), Options{}); err == nil {
		t.Error("Sign with invalid identity should fail")
	}
}

// TestAnUnreachableTimestampAuthoritySaysSoAndSignsNothing — the salvageable half of the
// standing doubt "TSA adds an optional network dependency; fall back to a local date +
// warning when offline".
//
// **The fallback half is deliberately NOT built**, and this test is where that is asserted
// rather than left in a comment. `SignerInfo.TimeBacking` distinguishes an independent
// authority's token from the signer's own clock, and a verifier reports which. Signing
// without the timestamp the user asked for would produce a document whose verifier makes a
// true statement the user believes is a different one — worse than a refusal.
//
// So the assertions are: nothing is signed, the failure is identifiable by sentinel, and the
// message names the authority and says the document was not signed. `127.0.0.1:1` is
// "offline" without touching the network.
func TestAnUnreachableTimestampAuthoritySaysSoAndSignsNothing(t *testing.T) {
	pdf, err := testpdf.Text("hello", "world")
	if err != nil {
		t.Fatal(err)
	}
	cert, key, err := GenerateIdentity("Ada")
	if err != nil {
		t.Fatal(err)
	}
	const tsa = "http://127.0.0.1:1/tsa"

	// SETUP: the very same call signs fine with no TSA. Without this, "signing failed" is
	// equally consistent with a broken fixture, and the assertion below would be about
	// nothing.
	if _, err := Sign(pdf, cert, key, Options{Name: "Ada", Reason: "r", When: time.Now().UTC()}); err != nil {
		t.Fatalf("setup: signing without a TSA already fails (%v), so a TSA failure proves nothing", err)
	}

	out, err := Sign(pdf, cert, key, Options{Name: "Ada", Reason: "r", When: time.Now().UTC(), TSAURL: tsa})
	if err == nil {
		t.Fatalf("signing SUCCEEDED (%d bytes) with an unreachable timestamp authority — if a "+
			"fallback was added, the document now claims a weaker time backing than the user "+
			"asked for and nothing told them", len(out))
	}
	if out != nil {
		t.Errorf("bytes were returned alongside the error (%d) — a partially signed document "+
			"must not reach a caller that only checks err on the happy path", len(out))
	}
	if !errors.Is(err, ErrTimestampAuthority) {
		t.Errorf("the failure is not identifiable as a timestamp problem: %v", err)
	}
	if !strings.Contains(err.Error(), tsa) {
		t.Errorf("the message does not name the authority that failed, so a user with a typo "+
			"in the address cannot see it: %v", err)
	}
	if !strings.Contains(err.Error(), "NOT signed") {
		t.Errorf("the message does not say the document was not signed, which is the fact the "+
			"user acts on: %v", err)
	}
}

// A signing failure that has nothing to do with a timestamp must NOT be dressed up as one —
// the string match is the fragile part of describeSignFailure and this is what bounds it.
func TestANonTimestampFailureIsNotReportedAsATimestampFailure(t *testing.T) {
	cert, key, err := GenerateIdentity("Ada")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Sign([]byte("this is not a pdf"), cert, key, Options{
		Name: "Ada", When: time.Now().UTC(), TSAURL: "http://127.0.0.1:1/tsa",
	})
	if err == nil {
		t.Fatal("setup: signing garbage succeeded, so this test cannot distinguish anything")
	}
	if errors.Is(err, ErrTimestampAuthority) {
		t.Errorf("a malformed-PDF failure was reported as a timestamp-authority failure, so the "+
			"user is sent to check a network address over a broken file: %v", err)
	}
}
