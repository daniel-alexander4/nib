package sign

import (
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
