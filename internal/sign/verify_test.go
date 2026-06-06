package sign

import (
	"testing"
	"time"

	"github.com/digitorus/pdfsign/verify"
	"github.com/digitorus/timestamp"

	"nib/internal/testpdf"
)

func TestVerifyUnsigned(t *testing.T) {
	data, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	got := Verify(data)
	if got.State != Unsigned {
		t.Errorf("Verify(unsigned form) state = %q, want %q", got.State, Unsigned)
	}
}

// A non-PDF must not panic or error out — it's simply "unsigned".
func TestVerifyGarbage(t *testing.T) {
	got := Verify([]byte("this is not a pdf"))
	if got.State != Unsigned {
		t.Errorf("Verify(garbage) state = %q, want %q", got.State, Unsigned)
	}
}

// A document signed by Nib's own identity round-trips to a single valid signer
// whose time is self-asserted — Nib sets no TSA by default, so the only time is
// the signer-supplied /M value.
func TestVerifySelfAssertedSigner(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM, err := GenerateIdentity("Jane Doe")
	if err != nil {
		t.Fatal(err)
	}
	signed, err := Sign(base, certPEM, keyPEM, Options{Name: "Jane Doe", Reason: "Test", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	st := Verify(signed)
	if st.State != Valid {
		t.Fatalf("state = %q, want %q", st.State, Valid)
	}
	if len(st.Signers) != 1 {
		t.Fatalf("signers = %d, want 1", len(st.Signers))
	}
	s := st.Signers[0]
	if !s.Valid {
		t.Error("signer should be valid")
	}
	if s.TimeBacking != SelfAsserted {
		t.Errorf("timeBacking = %q, want %q", s.TimeBacking, SelfAsserted)
	}
	if s.Name != "Jane Doe" {
		t.Errorf("name = %q, want %q", s.Name, "Jane Doe")
	}
}

// signerInfo must read time backing from token presence, not the library's
// TimeSource field: an RFC3161 token wins (TSA), else a /M time is self-asserted,
// else there is no time.
func TestSignerInfoTimeBacking(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
	tests := []struct {
		name  string
		in    verify.Signer
		want  TimeBacking
		valid bool
	}{
		{"tsa", verify.Signer{ValidSignature: true, TimeStamp: &timestamp.Timestamp{Time: ts}}, TSA, true},
		{"self-asserted", verify.Signer{ValidSignature: true, SignatureTime: &ts}, SelfAsserted, true},
		{"none", verify.Signer{ValidSignature: false}, NoTime, false},
		{"tsa beats self-asserted", verify.Signer{ValidSignature: true, TimeStamp: &timestamp.Timestamp{Time: ts}, SignatureTime: &ts}, TSA, true},
	}
	for _, tc := range tests {
		got := signerInfo(&tc.in)
		if got.TimeBacking != tc.want {
			t.Errorf("%s: timeBacking = %q, want %q", tc.name, got.TimeBacking, tc.want)
		}
		if got.Valid != tc.valid {
			t.Errorf("%s: valid = %v, want %v", tc.name, got.Valid, tc.valid)
		}
	}
}
