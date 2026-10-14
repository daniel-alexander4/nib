package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/pdfread"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// TestSignDropsTheClaimBeforeTheSignature — `/pending 492`'s CLI signing door. The signature verifying is
// what proves the drop came before it rather than after.
func TestSignDropsTheClaimBeforeTheSignature(t *testing.T) {
	dir := t.TempDir()
	base, err := testpdf.Text("to be signed")
	if err != nil {
		t.Fatal(err)
	}
	titled, err := pdfops.SetTitle(base, "To be signed")
	if err != nil {
		t.Fatal(err)
	}
	labelled, err := testpdf.WithUAIdentification(titled)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := testpdf.ClaimsUA(labelled); err != nil || !ok {
		t.Fatalf("setup: the input does not claim PDF/UA (err %v)", err)
	}
	in := filepath.Join(dir, "in.pdf")
	mustWrite(t, in, labelled)
	p12 := filepath.Join(dir, "id.p12")
	if err := os.WriteFile(p12, makeP12(t, "CLI Tester", "secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NIB_P12_PASSWORD", "secret")
	out := filepath.Join(dir, "signed.pdf")
	var code int
	stderr := captureStderr(t, func() { code = cmdSign([]string{in, "-o", out, "--cert", p12}) })
	if code != 0 {
		t.Fatalf("sign exit = %d, want 0", code)
	}
	// ADR-147, `/pending 520`: said before it signs, and never a refusal.
	if !strings.Contains(stderr, pdfops.LabelRemovedBySigning) {
		t.Errorf("a labelled document was signed and its label removed with nothing said (stderr %q)", stderr)
	}
	signed := readPDF(t, out)
	if st := sign.Verify(signed); st.State != sign.Valid {
		t.Fatalf("the signed output does not verify (%q), so a write followed the signature", st.State)
	}
	if ok, err := testpdf.ClaimsUA(signed); err != nil || ok {
		t.Errorf("the signed output still claims PDF/UA (err %v)", err)
	}
}

// TestSignRefusesADynamicXFADocument — `/pending 876`, ADR-145: `nib sign` reaches the refusal of a document
// whose pages are a placeholder for a dynamic XFA form from its own call site, which had no test. A signature
// over the placeholder would seal a page that says "open this in another reader".
func TestSignRefusesADynamicXFADocument(t *testing.T) {
	dir := t.TempDir()
	xfa := func(extra string) []byte {
		packet := `<xdp:xdp xmlns:xdp="http://ns.adobe.com/xdp/"><template/></xdp:xdp>`
		return testpdf.Assemble(map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [6 0 R] /XFA 5 0 R >>" + extra + " >>",
			2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Annots [6 0 R] >>",
			5: "<< /Length " + strconv.Itoa(len(packet)) + " >>\nstream\n" + packet + "\nendstream",
			6: "<< /Type /Annot /Subtype /Widget /FT /Tx /T (name) /DA (/Helv 10 Tf 0 g) /Rect [10 10 100 30] /P 3 0 R >>",
		})
	}
	p12 := filepath.Join(dir, "id.p12")
	if err := os.WriteFile(p12, makeP12(t, "CLI Tester", "secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NIB_P12_PASSWORD", "secret")

	in := filepath.Join(dir, "dynamic.pdf")
	mustWrite(t, in, xfa(" /NeedsRendering true"))
	out := filepath.Join(dir, "signed.pdf")
	var code int
	stderr := captureStderr(t, func() { code = cmdSign([]string{in, "-o", out, "--cert", p12}) })
	if code == 0 || !strings.Contains(stderr, pdfread.DynamicXFARefusal) {
		t.Errorf("sign of a dynamic XFA document: exit %d, stderr %q — want a refusal carrying the sentence", code, stderr)
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("the refused signing wrote an output file")
	}
	// Control: the same document without /NeedsRendering is signed, so the refusal is about the placeholder.
	static := filepath.Join(dir, "static.pdf")
	mustWrite(t, static, xfa(""))
	if code = cmdSign([]string{static, "-o", filepath.Join(dir, "static-signed.pdf"), "--cert", p12}); code != 0 {
		t.Fatalf("control: sign of a static XFA document exit = %d", code)
	}
}

// TestExportOfADocumentWithNoFormFieldsSaysSo — `/pending 888`: `nib export-xfdf` on a document whose only field
// is a signature answers with the sentence and writes nothing, not with pdfcpu's "no form fields affected".
func TestExportOfADocumentWithNoFormFieldsSaysSo(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "sig.pdf")
	mustWrite(t, in, testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [6 0 R] /SigFlags 1 >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Annots [6 0 R] >>",
		6: "<< /Type /Annot /Subtype /Widget /FT /Sig /T (Signature1) /Rect [10 10 150 60] /P 3 0 R >>",
	}))
	out := filepath.Join(dir, "data.xfdf")
	var code int
	stderr := captureStderr(t, func() { code = cmdExportXFDF([]string{in, "-o", out}) })
	if code == 0 || !strings.Contains(stderr, pdfops.NoFormFieldsRefusal) {
		t.Errorf("export-xfdf of a signature-only document: exit %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("the refused export wrote a file")
	}
}
