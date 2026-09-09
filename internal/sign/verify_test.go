package sign

import (
	"bytes"
	"os"
	"strings"
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

// TestHasSignatureBlobSurvivesCorruption — /pending 453(b), and the doc comment it falsified.
//
// **`signatureBlobPresent`'s own doc said "best-effort, never panics" and it panicked on 115 of 300
// single-byte flips** of a 3,715-byte signed document. `digitorus/pdf` panics out of `applyFilter`,
// `readXref` and the object parser on ordinary corruption. The only production caller is
// `ceremonyid.go`'s arrival gate, reached from `sessionConfirmer.Confirm` on the **p2p arm**, where
// `net/http`'s per-request recover does not apply — so the panic takes the process.
//
// **Swept, not pinned to one offset.** The finding was first recorded at a single magic offset;
// a sweep is what showed it was 38% of a window rather than one unlucky byte, and a test pinned to
// one offset would go green the moment the fixture's layout shifted by a byte.
func TestHasSignatureBlobSurvivesCorruption(t *testing.T) {
	signed := signedFixture(t)
	// The CONTROL, and without it every assertion below is satisfied by a function that always
	// returns false and never parses anything.
	if !HasSignatureBlob(signed) {
		t.Fatal("setup: the intact signed document reports no signature blob, so the sweep proves nothing")
	}
	for off := 600; off < 900 && off < len(signed); off++ {
		bad := append([]byte(nil), signed...)
		bad[off] ^= 0x20
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("HasSignatureBlob panicked on a single-byte flip at offset %d: %v — "+
						"its only production caller is on the p2p arm, where a panic takes the "+
						"process rather than the request", off, r)
				}
			}()
			_ = HasSignatureBlob(bad)
		}()
	}
}

// TestATamperedSignedDocumentIsNotReportedAsNeverSigned — /pending 453(c).
//
// **`Verify`'s cross-check was gated on `err == nil`, which skipped it exactly when it mattered.**
// The comment above that guard states the rule — a signature blob that fails to parse must not be
// "silently downgraded to Unsigned" — and the conjunct disabled it on the commonest way a tampered
// document arrives. Measured over 75 flips inside the pre-signature revision: 40 reported
// `unsigned`, 39 of those with a blob still present. `web/app.js` renders `unsigned` as "Unsigned",
// which a reader takes as *never signed* rather than *broken*.
func TestATamperedSignedDocumentIsNotReportedAsNeverSigned(t *testing.T) {
	signed := signedFixture(t)
	if st := Verify(signed).State; st != Valid {
		t.Fatalf("setup: the intact signed document verifies as %q, not valid", st)
	}
	// The other control: a genuinely unsigned document must still report Unsigned, or this test is
	// satisfied by a Verify that calls everything Invalid.
	plain, err := testpdf.Text("no signature here")
	if err != nil {
		t.Fatal(err)
	}
	if st := Verify(plain).State; st != Unsigned {
		t.Fatalf("an unsigned document reports %q, want unsigned — the fix must not call every "+
			"document invalid", st)
	}
	neverSigned := 0
	for off := 600; off < 900 && off < len(signed); off++ {
		bad := append([]byte(nil), signed...)
		bad[off] ^= 0x20
		if Verify(bad).State == Unsigned {
			neverSigned++
		}
	}
	if neverSigned > 0 {
		t.Errorf("%d of 300 tampered SIGNED documents report state=unsigned, which the panel "+
			"renders as \"Unsigned\" — a document carrying a signature blob told the user it was "+
			"never signed", neverSigned)
	}
}

// signedFixture builds a real signed document for the corruption sweeps above.
func signedFixture(t *testing.T) []byte {
	t.Helper()
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	cert, key, err := GenerateIdentity("A")
	if err != nil {
		t.Fatal(err)
	}
	signed, err := SignApproval(base, cert, key, Options{Name: "A", Reason: "r"})
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

// TestAnUnsignedDocumentNeverEntersTheThirdPartyParser — /pending 453(a)'s mitigation.
//
// **The crash it reduces cannot be fixed here.** `digitorus/pdf`'s `readByte` returns `'\n'` at EOF
// forever and `readLiteralString` loops appending with no EOF check, so an unterminated literal
// string allocates until the process dies — a `fatal error: out of memory`, which no `recover`
// catches and which Go gives no per-goroutine allocation cap to bound. Upstream defect.
//
// What is testable is the reduction: a document carrying no `/ByteRange` has no signature to verify,
// so `Verify` answers by inspection and the parser is never entered. This drives malformed bytes
// that are NOT valid PDFs at all — the shape an upload most often takes when it goes wrong.
func TestAnUnsignedDocumentNeverEntersTheThirdPartyParser(t *testing.T) {
	for _, c := range []struct {
		name string
		data []byte
	}{
		{"not a pdf at all", []byte("this is not a PDF")},
		{"a truncated pdf header", []byte("%PDF-1.7\n1 0 obj\n<</Type/Catalog")},
		{"an unterminated literal string — the exact shape that OOMs the library", []byte("%PDF-1.7\n1 0 obj\n(unterminated")},
		{"empty", []byte{}},
	} {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Verify panicked on unsigned malformed bytes: %v", r)
				}
			}()
			if st := Verify(c.data).State; st != Unsigned {
				t.Errorf("state is %q, want unsigned — a document with no signature blob should be "+
					"answered by inspection rather than handed to a parser that cannot be contained", st)
			}
		})
	}
	// The CONTROL, and it is what stops this asserting a Verify that calls everything unsigned: a
	// real signed document still goes through the parser and still verifies.
	if st := Verify(signedFixture(t)).State; st != Valid {
		t.Fatalf("a genuinely signed document reports %q — the gate is refusing documents it should "+
			"be letting through to the verifier", st)
	}

	// **The ROUTING, because the behaviour above is identical with and without the gate.**
	//
	// That is not a flaw in the gate, it is what a pure exposure reduction looks like: the answer
	// does not change, only who computes it. So the behavioural cases cannot see the gate, and a
	// first version of this test passed with the gate deleted — inert, and caught by probing it.
	//
	// **The case that CAN see it cannot be shipped as a test.** Sweeping single-byte flips of an
	// unsigned fixture through `digitorus/pdfsign` — every offset, skipping any that introduce
	// `/ByteRange` — took the test binary with `signal: killed`. The OOM killer, on an UNSIGNED
	// document, which is precisely what the gate keeps away from the parser. A guard that
	// reproduces that would kill the suite, so what is asserted instead is the ordering: the scan
	// answers before `verify.Verify` is ever reached.
	src, err := os.ReadFile("verify.go")
	if err != nil {
		t.Fatal(err)
	}
	code := string(src)
	body := code[strings.Index(code, "func Verify(data []byte) Status {"):]
	gate := strings.Index(body, "if !scanForSignatureBlob(data)")
	parse := strings.Index(body, "verify.Verify(")
	if gate < 0 || parse < 0 {
		t.Fatalf("Verify no longer has both the scan gate (%d) and the parser call (%d)", gate, parse)
	}
	if gate > parse {
		t.Error("Verify hands the document to the third-party parser BEFORE asking whether it " +
			"carries a signature at all. An unsigned malformed document then reaches an unbounded " +
			"read that no recover can contain — measured: a sweep of an unsigned fixture through " +
			"the library was killed by the OOM killer")
	}
}
