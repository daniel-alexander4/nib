package sign

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
)

// signers is every production signing entry point that reaches runSign with a vault identity.
// SignExternal takes a PKCS#12 and reaches the same runSign; it is covered by the door, not re-run.
func signers(id identity) map[string]func([]byte) ([]byte, error) {
	o := Options{Name: "Alice", When: time.Now()}
	return map[string]func([]byte) ([]byte, error){
		"Sign":         func(b []byte) ([]byte, error) { return Sign(b, id.certPEM, id.keyPEM, o) },
		"SignApproval": func(b []byte) ([]byte, error) { return SignApproval(b, id.certPEM, id.keyPEM, o) },
	}
}

// unsignedHybrid is a one-page UNSIGNED hybrid-reference file, the objects in inStm reachable only
// through the trailer's `/XRefStm`.
func unsignedHybrid(inStm map[int]bool) []byte {
	return hybridDoc([]sobj{
		{num: 1, body: "<</Type/Catalog/Pages 2 0 R>>"},
		{num: 2, body: "<</Type/Pages/Kids[3 0 R]/Count 1>>"},
		{num: 3, body: "<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 200]/Resources<</Font<</F1 4 0 R>>>>>>"},
		{num: 4, body: "<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>"},
		{num: 5, body: "<</Producer(nib test)>>"},
	}, inStm)
}

// omitting rewrites hybridDoc's classic table to OMIT the objects only the hybrid stream lists, rather
// than mark them free in the same section. pdfcpu treats a free entry in the same section as final (it
// skips a hybrid-stream entry for an object the table already has, `read.go` extractXRefTableEntries…),
// so on hybridDoc's own layout NEITHER reader sees the hidden objects; with them omitted, pdfcpu's
// validated read sees them and digitorus still does not — the divergence /pending 740 is about.
func omitting(doc []byte) []byte {
	m := regexp.MustCompile(`(?s)xref\n0 \d+\n(.*?)trailer`).FindSubmatchIndex(doc)
	lines := bytes.Split(bytes.TrimRight(doc[m[2]:m[3]], "\n"), []byte("\n"))
	var t bytes.Buffer
	t.WriteString("xref\n0 1\n0000000000 65535 f \n")
	for i, l := range lines {
		if i == 0 || bytes.HasSuffix(bytes.TrimSpace(l), []byte("f")) {
			continue
		}
		fmt.Fprintf(&t, "%d 1\n%s\n", i, l)
	}
	t.WriteString("trailer")
	return append(append(append([]byte(nil), doc[:m[0]]...), t.Bytes()...), doc[m[1]:]...)
}

// requireOneValidSigner is the success shape: pdfcpu reads the output, nib verifies it Valid with
// exactly one signer, and the hybrid form is gone (the rewrite is what was signed).
func requireOneValidSigner(t *testing.T, out []byte) {
	t.Helper()
	if err := pdfcpuCanRead(out); err != nil {
		t.Fatalf("pdfcpu cannot read the signed output: %v", err)
	}
	st := Verify(out)
	if st.State != Valid || len(st.Signers) != 1 {
		t.Fatalf("signed output verifies %q with %d signer(s), want valid with 1", st.State, len(st.Signers))
	}
	if hybridReference(out) {
		t.Errorf("signed output still carries /XRefStm — the hybrid input was signed as it was, not rewritten")
	}
}

// TestASignedHybridReferenceFileIsRefusedByName is /pending 740's first arm: co-signing a SIGNED hybrid
// file wrote an Invalid, 0-signer file, in two of three shapes one pdfcpu could not read. A rewrite would
// destroy the signature it carries, so it is refused by name and nothing is returned.
func TestASignedHybridReferenceFileIsRefusedByName(t *testing.T) {
	a, b := newIdentity(t, "Alice"), newIdentity(t, "Bob")
	for _, c := range []struct {
		name string
		stm  map[int]bool
	}{
		{"catalog only in the hybrid stream", map[int]bool{1: true}},
		{"signature field only in the hybrid stream", map[int]bool{4: true}},
		{"catalog and signature dictionary only in the hybrid stream", map[int]bool{1: true, 5: true}},
	} {
		for layout, doc := range map[string][]byte{
			// /pending 733's layout: the hidden objects marked free in the classic table.
			"free-marked": signedHybrid(t, a, c.stm),
			// The layout pdfcpu's validated read resolves: the hidden objects absent from the table.
			"omitted": fillSig(t, omitting(hybridDoc(baseObjs(sigDict("1", "")), c.stm)), "1", nil, detached(t, a)),
		} {
			if !HasSignatureBlob(doc) || !hybridReference(doc) {
				t.Fatalf("STIMULUS %s/%s: not a signed hybrid-reference file", c.name, layout)
			}
			if layout == "omitted" {
				if st := Verify(doc); st.State == Unsigned {
					t.Fatalf("STIMULUS %s/%s: verifies unsigned", c.name, layout)
				}
			}
			for m, fn := range signers(b) {
				t.Run(c.name+"/"+layout+"/"+m, func(t *testing.T) {
					out, err := fn(doc)
					if !errors.Is(err, ErrSignedHybridReference) {
						t.Errorf("err = %v, want ErrSignedHybridReference (/pending 740)", err)
					}
					if out != nil {
						t.Errorf("a refused signature returned %d bytes; nothing may be written", len(out))
					}
				})
			}
		}
	}
}

// TestAnUnsignedHybridReferenceFileSignsReadably is /pending 740's second arm: an unsigned hybrid with the
// catalog in the stream signed to an unreadable output, and with the pages there failed `page number 1 not
// found`. It carries no signature to lose, so it is rewritten and the rewrite is signed.
func TestAnUnsignedHybridReferenceFileSignsReadably(t *testing.T) {
	a := newIdentity(t, "Alice")
	for _, c := range []struct {
		name string
		stm  map[int]bool
	}{
		{"catalog only in the hybrid stream", map[int]bool{1: true}},
		{"pages only in the hybrid stream", map[int]bool{2: true, 3: true}},
	} {
		// The layout pdfcpu reads: the hidden objects are absent from the classic table.
		doc := omitting(unsignedHybrid(c.stm))
		if _, err := pdfread.ReadOptimized(doc, relaxed()); err != nil {
			t.Fatalf("STIMULUS %s: pdfcpu's validated read refuses the fixture: %v", c.name, err)
		}
		if HasSignatureBlob(doc) || !hybridReference(doc) {
			t.Fatalf("STIMULUS %s: not an unsigned hybrid-reference file", c.name)
		}
		for m, fn := range signers(a) {
			t.Run(c.name+"/"+m, func(t *testing.T) {
				out, err := fn(doc)
				if err != nil {
					t.Fatalf("signing an unsigned hybrid-reference file failed: %v (/pending 740)", err)
				}
				requireOneValidSigner(t, out)
			})
		}
		// The layout NEITHER reader resolves (the hidden objects marked free in the same section): pdfcpu's
		// validated read refuses it too, so there is no readable rewrite — it is refused, nothing returned.
		unreadable := unsignedHybrid(c.stm)
		for m, fn := range signers(a) {
			t.Run(c.name+"/free-marked/"+m, func(t *testing.T) {
				out, err := fn(unreadable)
				if err == nil || out != nil {
					t.Errorf("a hybrid file pdfcpu cannot validate signed (err=%v, %d bytes); want a refusal", err, len(out))
				}
			})
		}
	}
}

func relaxed() *model.Configuration {
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	return conf
}

// TestRealProducerHybridFilesSign: 2 of the 36 real-producer files carry /XRefStm (InDesign, Acrobat).
// They must keep signing — refusing every hybrid file would refuse real documents. Skips on a fresh clone.
func TestRealProducerHybridFilesSign(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	a := newIdentity(t, "Alice")
	for _, f := range []string{"indesign/cdc-mm7201-H.pdf", "acrobat/fda-71021.pdf"} {
		in, err := os.ReadFile(filepath.Join(home, "nib", "producers", f))
		if err != nil {
			t.Skipf("real-producer corpus absent (%v)", err)
		}
		if !hybridReference(in) || HasSignatureBlob(in) {
			t.Fatalf("STIMULUS %s: expected an unsigned hybrid-reference file", f)
		}
		for m, fn := range signers(a) {
			t.Run(f+"/"+m, func(t *testing.T) {
				out, err := fn(in)
				if err != nil {
					t.Fatalf("signing %s failed: %v", f, err)
				}
				requireOneValidSigner(t, out)
			})
		}
	}
}
