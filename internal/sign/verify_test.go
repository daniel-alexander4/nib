package sign

import (
	"bytes"
	"testing"
	"time"

	"github.com/digitorus/pdfsign/verify"
	"github.com/digitorus/timestamp"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfops"
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

// TestVerifyAfterDecryptStaysSigned pins the fact the on-open decrypt warning
// relies on: encrypting a signed PDF then unlocking it rewrites the file (so the
// signature breaks), but the signature must remain DETECTABLE as Invalid — not
// vanish to Unsigned — so the UI can warn that unlocking dropped it. If a pdfcpu
// upgrade ever made decrypt strip the signature dict, this catches it.
func TestVerifyAfterDecryptStaysSigned(t *testing.T) {
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
	if st := Verify(signed); st.State != Valid {
		t.Fatalf("precondition: freshly signed doc state = %q, want %q", st.State, Valid)
	}

	conf := model.NewDefaultConfiguration()
	conf.UserPW = "secret"
	conf.OwnerPW = "secret"
	var enc bytes.Buffer
	if err := api.Encrypt(bytes.NewReader(signed), &enc, conf); err != nil {
		t.Fatal(err)
	}

	dec, err := pdfops.RemovePassword(enc.Bytes(), "secret")
	if err != nil {
		t.Fatal(err)
	}
	if st := Verify(dec); st.State != Invalid {
		t.Errorf("after unlock, signature state = %q, want %q (the broken signature must stay detectable so the UI can warn)", st.State, Invalid)
	}
}

// TestVerifyUnparseableSignatureIsInvalid pins the discriminator: a document whose
// signature /Contents blob is present but corrupt must report Invalid, not Unsigned.
// The library drops an unparseable signer with no top-level error, which would
// otherwise land in the same "zero signers" bucket as a genuinely unsigned PDF and
// silently hide a tampered signature.
func TestVerifyUnparseableSignatureIsInvalid(t *testing.T) {
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
	if st := Verify(signed); st.State != Valid {
		t.Fatalf("precondition: freshly signed doc state = %q, want %q", st.State, Valid)
	}

	// Corrupt the DER at the start of the /Contents hex blob in place: same length
	// (ByteRange offsets and PDF structure stay intact) and still valid hex (the PDF
	// string parser is happy), but the PKCS#7 SEQUENCE tag is destroyed so the blob
	// won't parse. This is the "signature present but unparseable" case.
	mangled := append([]byte(nil), signed...)
	ci := bytes.Index(mangled, []byte("/Contents"))
	if ci < 0 {
		t.Fatal("signed doc has no /Contents")
	}
	lt := bytes.IndexByte(mangled[ci:], '<')
	if lt < 0 {
		t.Fatal("no < opening the /Contents hex string")
	}
	start := ci + lt + 1
	flipped := 0
	for j := start; j < len(mangled) && flipped < 8; j++ {
		c := mangled[j]
		if isHexDigit(c) {
			if c == '0' {
				mangled[j] = '1'
			} else {
				mangled[j] = '0'
			}
			flipped++
		}
	}
	if flipped < 8 {
		t.Fatal("could not find hex digits to corrupt in /Contents")
	}

	if !signatureBlobPresent(mangled) {
		t.Fatal("corruption removed the signature blob; test would not exercise the path")
	}
	if st := Verify(mangled); st.State != Invalid {
		t.Errorf("mangled signature state = %q, want %q (must not downgrade to Unsigned)", st.State, Invalid)
	}
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
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
