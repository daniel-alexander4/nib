package cli

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nib/internal/pdfops"
	"nib/internal/sign"
	"nib/internal/testpdf"

	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

// writePDF writes a one-page test PDF into dir and returns its path.
func writePDF(t *testing.T, dir, name string, pages ...string) string {
	t.Helper()
	if len(pages) == 0 {
		pages = []string{"x"}
	}
	data, err := testpdf.Text(pages...)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func readPDF(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRunDispatch(t *testing.T) {
	if handled, _ := Run(nil, "1.2.3"); handled {
		t.Error("empty args should not be handled (falls through to desktop boot)")
	}
	if handled, _ := Run([]string{"open-this.pdf"}, "1.2.3"); handled {
		t.Error("a non-verb first arg should fall through to the desktop boot")
	}
	if handled, code := Run([]string{"version"}, "1.2.3"); !handled || code != 0 {
		t.Errorf("version: handled=%v code=%d, want true/0", handled, code)
	}
	if handled, code := Run([]string{"help"}, "1.2.3"); !handled || code != 0 {
		t.Errorf("help: handled=%v code=%d, want true/0", handled, code)
	}
}

func TestOptimize(t *testing.T) {
	dir := t.TempDir()
	in := writePDF(t, dir, "in.pdf")
	out := filepath.Join(dir, "out.pdf")
	if code := cmdOptimize([]string{in, "-o", out}); code != 0 {
		t.Fatalf("optimize exit = %d, want 0", code)
	}
	if err := pdfops.Validate(readPDF(t, out)); err != nil {
		t.Fatalf("optimized output invalid: %v", err)
	}
}

func TestMerge(t *testing.T) {
	dir := t.TempDir()
	a := writePDF(t, dir, "a.pdf", "a")
	b := writePDF(t, dir, "b.pdf", "b1", "b2")
	out := filepath.Join(dir, "out.pdf")
	if code := cmdMerge([]string{a, b, "-o", out}); code != 0 {
		t.Fatalf("merge exit = %d, want 0", code)
	}
	n, err := pdfops.PageCount(readPDF(t, out))
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("merged page count = %d, want 3 (1+2)", n)
	}
}

func TestMergeNeedsTwoInputs(t *testing.T) {
	dir := t.TempDir()
	a := writePDF(t, dir, "a.pdf")
	if code := cmdMerge([]string{a, "-o", filepath.Join(dir, "out.pdf")}); code != 1 {
		t.Errorf("merge with one input exit = %d, want 1", code)
	}
}

func TestSanitize(t *testing.T) {
	dir := t.TempDir()
	in := writePDF(t, dir, "in.pdf")
	out := filepath.Join(dir, "out.pdf")
	if code := cmdSanitize([]string{in, "-o", out}); code != 0 {
		t.Fatalf("sanitize exit = %d, want 0", code)
	}
	if err := pdfops.Validate(readPDF(t, out)); err != nil {
		t.Fatalf("sanitized output invalid: %v", err)
	}
}

func TestMissingOut(t *testing.T) {
	dir := t.TempDir()
	in := writePDF(t, dir, "in.pdf")
	if code := cmdOptimize([]string{in}); code != 1 {
		t.Errorf("optimize without -o exit = %d, want 1", code)
	}
}

func TestVerifyUnsigned(t *testing.T) {
	dir := t.TempDir()
	in := writePDF(t, dir, "in.pdf")
	if code := cmdVerify([]string{in}); code != 2 {
		t.Errorf("verify unsigned exit = %d, want 2", code)
	}
	if code := cmdVerify([]string{in, "--json"}); code != 2 {
		t.Errorf("verify --json unsigned exit = %d, want 2", code)
	}
}

// TestSignThenVerify ties the slice together: sign with a generated .p12, then
// verify the output reports a valid signature (exit 0).
func TestSignThenVerify(t *testing.T) {
	dir := t.TempDir()
	in := writePDF(t, dir, "in.pdf")
	out := filepath.Join(dir, "signed.pdf")
	p12 := filepath.Join(dir, "id.p12")
	if err := os.WriteFile(p12, makeP12(t, "CLI Tester", "secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("env passphrase", func(t *testing.T) {
		t.Setenv("NIB_P12_PASSWORD", "secret")
		if code := cmdSign([]string{in, "-o", out, "--cert", p12}); code != 0 {
			t.Fatalf("sign exit = %d, want 0", code)
		}
		if st := sign.Verify(readPDF(t, out)); st.State != sign.Valid {
			t.Fatalf("signed doc verify state = %q, want valid", st.State)
		}
		if code := cmdVerify([]string{out}); code != 0 {
			t.Errorf("verify signed exit = %d, want 0", code)
		}
	})

	t.Run("password-file", func(t *testing.T) {
		pf := filepath.Join(dir, "pass.txt")
		if err := os.WriteFile(pf, []byte("secret\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		out2 := filepath.Join(dir, "signed2.pdf")
		if code := cmdSign([]string{in, "-o", out2, "--cert", p12, "--password-file", pf}); code != 0 {
			t.Fatalf("sign --password-file exit = %d, want 0", code)
		}
	})

	t.Run("wrong passphrase", func(t *testing.T) {
		t.Setenv("NIB_P12_PASSWORD", "nope")
		if code := cmdSign([]string{in, "-o", filepath.Join(dir, "x.pdf"), "--cert", p12}); code != 1 {
			t.Errorf("sign wrong passphrase exit = %d, want 1", code)
		}
	})

	t.Run("no passphrase", func(t *testing.T) {
		os.Unsetenv("NIB_P12_PASSWORD")
		if code := cmdSign([]string{in, "-o", filepath.Join(dir, "x.pdf"), "--cert", p12}); code != 1 {
			t.Errorf("sign with no passphrase source exit = %d, want 1", code)
		}
	})
}

// makeP12 builds a passphrase-protected .p12 (leaf CN=cn issued by a self-signed
// CA), mirroring the fixture in internal/sign.
func makeP12(t *testing.T, cn, passphrase string) []byte {
	t.Helper()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	caDER, _ := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	caCert, _ := x509.ParseCertificate(caDER)

	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leafTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(2, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	leafDER, _ := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	leaf, _ := x509.ParseCertificate(leafDER)

	pfx, err := pkcs12.Modern.Encode(leafKey, leaf, []*x509.Certificate{caCert}, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	return pfx
}
