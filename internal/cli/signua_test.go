package cli

import (
	"os"
	"path/filepath"
	"testing"

	"nib/internal/pdfops"
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
	if code := cmdSign([]string{in, "-o", out, "--cert", p12}); code != 0 {
		t.Fatalf("sign exit = %d, want 0", code)
	}
	signed := readPDF(t, out)
	if st := sign.Verify(signed); st.State != sign.Valid {
		t.Fatalf("the signed output does not verify (%q), so a write followed the signature", st.State)
	}
	if ok, err := testpdf.ClaimsUA(signed); err != nil || ok {
		t.Errorf("the signed output still claims PDF/UA (err %v)", err)
	}
}
