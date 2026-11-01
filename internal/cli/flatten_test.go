package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

func filledForm(t *testing.T) []byte {
	t.Helper()
	form, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	out, err := pdfops.FillFormJSON(form, []byte(`{"forms":[{"textfield":[{"name":"fullName","value":"Ada Lovelace"}],"checkbox":[{"name":"agree","value":true}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestFlattenTakesAFormOnStdinAndGivesTheVectorPageOnStdout(t *testing.T) {
	filled := filledForm(t)
	stdout, stderr, code := fillChild(t, string(filled), "", "flatten", "-", "-o", "-", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var counts map[string]int
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &counts); err != nil || counts["flattened"] < 2 {
		t.Errorf("counts %q (%v), want a JSON object with flattened >= 2 on stderr", stderr, err)
	}
	pages, err := pdfops.DocumentText(stdout)
	if err != nil {
		t.Fatalf("the output is not a readable PDF: %v", err)
	}
	var all string
	for _, p := range pages {
		for _, r := range p.Runs {
			all += r.Text + " "
		}
	}
	if !strings.Contains(all, "Ada Lovelace") {
		t.Errorf("the value is not page text after the flatten: %q", all)
	}
	if strings.Contains(stderr, "Ada") {
		t.Errorf("the result repeats a value: %q", stderr)
	}
}

func TestFlattenRefusesASignedDocumentUnlessAskedToDropItsSignature(t *testing.T) {
	signed := signedTestPDF(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "signed.pdf")
	os.WriteFile(in, signed, 0o600)
	_, stderr, code := fillChild(t, "", "", "flatten", in, "-o", filepath.Join(dir, "out.pdf"))
	if code != 1 || !strings.Contains(stderr, "is signed") {
		t.Errorf("a signed document: exit %d, stderr %q, want 1 and the signed refusal", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "out.pdf")); err == nil {
		t.Error("an output was written for a refused document")
	}
	// With the signature given up the refusal is gone, and the signature with it.
	_, stderr, code = fillChild(t, "", "", "flatten", in, "-o", filepath.Join(dir, "out.pdf"), "--drop-signature")
	if code != 0 {
		t.Fatalf("--drop-signature: exit %d, stderr %q", code, stderr)
	}
	out, err := os.ReadFile(filepath.Join(dir, "out.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if sign.HasSignatureBlob(out) {
		t.Error("the flattened output still carries a signature")
	}
}

func TestFlattenNamesAValueNothingDrawsAndWritesNothing(t *testing.T) {
	// A form field with a value and no appearance: pdfcpu's Form() has none until filled; strip by flattening a
	// hand-made one.
	pdf := testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] /DA (/Helv 10 Tf 0 g) /DR << /Font << /Helv 5 0 R >> >> >> >>",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Annots [4 0 R] >>",
		4: "<< /Type /Annot /Subtype /Widget /FT /Tx /T (ssn) /DA (/Helv 10 Tf 0 g) /V (987-65-4321) /Rect [10 10 90 30] >>",
	})
	dir := t.TempDir()
	in := filepath.Join(dir, "f.pdf")
	os.WriteFile(in, pdf, 0o600)
	_, stderr, code := fillChild(t, "", "", "flatten", in, "-o", filepath.Join(dir, "out.pdf"))
	if code != 1 || !strings.Contains(stderr, "ssn") {
		t.Errorf("exit %d stderr %q, want 1 naming the field", code, stderr)
	}
	if strings.Contains(stderr, "987-65-4321") {
		t.Errorf("the refusal repeats the value: %q", stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "out.pdf")); err == nil {
		t.Error("an output was written for a refused document")
	}
}
