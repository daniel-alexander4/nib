package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/sign"
	"nib/internal/testpdf"
)

// P01.S03's CLI reader (ADR-060): a refused record is not a signer, so it can be present while
// `AddedAfter` is false and `State` valid, and a document can carry no signer at all and still be
// `Invalid` because something signature-shaped is there. Each is said, and a refusal exits 2.

// TestVerifySaysARefusalThatSetNoWarning — the /pending 736 shape: Alice signs, a copy of her
// dictionary is appended, and Bob signs over the lot. Bob's signature reaches EOF, so nothing is
// added after the last signature and the document is valid — the refusal is the ONLY fact, and
// before S03 the status line said nothing and the command exited 0.
func TestVerifySaysARefusalThatSetNoWarning(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	aCert, aKey, _ := sign.GenerateIdentity("Alice")
	bCert, bKey, _ := sign.GenerateIdentity("Bob")
	signed, err := sign.SignApproval(base, aCert, aKey, sign.Options{Name: "Alice", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	copied, obj, err := testpdf.CopiedSignatureDictionary(signed, testpdf.CopyExact)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := sign.SignApproval(copied, bCert, bKey, sign.Options{Name: "Bob", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	// STIMULUS: valid, two real signers, nothing added after the last, and the copy refused — so the
	// refusal is the only thing that can reach the line or the exit code.
	st := sign.Verify(doc)
	if st.State != sign.Valid || len(st.Signers) != 2 || st.AddedAfter || len(st.Refused) != 1 || st.Refused[0].Obj != uint32(obj) {
		t.Fatalf("STIMULUS: state %s signers %d addedAfter %v refused %+v; want valid/2, no append, object %d refused",
			st.State, len(st.Signers), st.AddedAfter, st.Refused, obj)
	}
	path := filepath.Join(t.TempDir(), "doc.pdf")
	if err := os.WriteFile(path, doc, 0o600); err != nil {
		t.Fatal(err)
	}
	out, code := captureStdout(t, func() int { return cmdVerify([]string{path}) })
	if code != 2 {
		t.Errorf("exit %d, want 2 — a document carrying a refused signature is not one a script should wave through", code)
	}
	if !strings.Contains(out, "valid (2 signer(s)); a signature Nib refused is present") {
		t.Errorf("the status line does not name the refusal:\n%s", out)
	}
	if want := fmt.Sprintf("refused: object %d (filter %q): %s", obj, "Adobe.PPKLite", sign.CauseContentsElsewhere); !strings.Contains(out, want) {
		t.Errorf("output does not carry %q:\n%s", want, out)
	}
}

// TestVerifyNamesATimestampAndNoCheckableSignature — a document whose only signature-shaped record is
// a document timestamp (a real RFC 3161 token): no record counts as a signer, the document is
// `Invalid`, and "modified since signing" would claim a signature nib never found to check. The line
// says there is none, and the timestamp is named on its own line.
func TestVerifyNamesATimestampAndNoCheckableSignature(t *testing.T) {
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := testpdf.DocTimeStamped(base)
	if err != nil {
		t.Fatal(err)
	}
	st := sign.Verify(doc)
	if st.State != sign.Invalid || len(st.Signers) != 0 || len(st.Timestamps) != 1 {
		t.Fatalf("STIMULUS: state %s signers %d timestamps %v; want invalid, none, one", st.State, len(st.Signers), st.Timestamps)
	}
	path := filepath.Join(t.TempDir(), "doc.pdf")
	if err := os.WriteFile(path, doc, 0o600); err != nil {
		t.Fatal(err)
	}
	out, code := captureStdout(t, func() int { return cmdVerify([]string{path}) })
	if code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
	if !strings.Contains(out, "INVALID — no signature Nib could check (see below)") || strings.Contains(out, "modified since signing") {
		t.Errorf("the status line does not say no signature could be checked:\n%s", out)
	}
	if want := fmt.Sprintf("timestamp: object %d — a document timestamp; Nib does not check timestamps", st.Timestamps[0]); !strings.Contains(out, want) {
		t.Errorf("output does not carry %q:\n%s", want, out)
	}
}

// TestDescribeStatusComposesTheFacts — the line's rules one fact at a time, where a document for
// each would need a TSA or a join error. A refusal beside a check that could not run says BOTH.
func TestDescribeStatusComposesTheFacts(t *testing.T) {
	one := []sign.SignerInfo{{Valid: true}}
	ref := []sign.RefusedSignature{{Obj: 9, Cause: sign.CauseMalformedByteRange}}
	for _, tc := range []struct {
		name     string
		st       sign.Status
		want     string
		refusals int
	}{
		{"could-not-check and a refusal", sign.Status{State: sign.Valid, Signers: one, AddedAfter: true, AddedAfterCause: sign.AddedAfterCouldNotCheck, Refused: ref},
			"valid (1 signer(s)); content added after the last signature could not be ruled out; a signature Nib refused is present", 1},
		{"the refused cause says it once", sign.Status{State: sign.Valid, Signers: one, AddedAfter: true, AddedAfterCause: sign.AddedAfterRefusedSignature, Refused: ref},
			"valid (1 signer(s)); content added after the last signature — and a signature Nib refused is present", 1},
		{"nothing refused, nothing said", sign.Status{State: sign.Valid, Signers: one},
			"valid (1 signer(s))", 0},
		{"invalid with a signer is modified", sign.Status{State: sign.Invalid, Signers: one, Refused: ref},
			"INVALID — modified since signing", 0},
		{"invalid, no signer, a refusal", sign.Status{State: sign.Invalid, Refused: ref},
			"INVALID — no signature Nib could check (see below)", 0},
		{"invalid, no signer, nothing named", sign.Status{State: sign.Invalid},
			"INVALID — modified since signing", 0},
	} {
		got := describeStatus(tc.st)
		if got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
		if n := strings.Count(got, "a signature Nib refused is present"); n != tc.refusals {
			t.Errorf("%s: the refusal is said %d time(s), want %d: %q", tc.name, n, tc.refusals, got)
		}
	}
}
