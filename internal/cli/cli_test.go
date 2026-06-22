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

func TestRotate(t *testing.T) {
	dir := t.TempDir()
	in := writePDF(t, dir, "in.pdf", "a", "b")
	out := filepath.Join(dir, "out.pdf")
	if code := cmdRotate([]string{in, "-o", out, "--deg", "90"}); code != 0 {
		t.Fatalf("rotate exit = %d, want 0", code)
	}
	if err := pdfops.Validate(readPDF(t, out)); err != nil {
		t.Fatalf("rotated output invalid: %v", err)
	}
	if code := cmdRotate([]string{in, "-o", out, "--deg", "45"}); code != 1 {
		t.Errorf("rotate --deg 45 exit = %d, want 1 (not a multiple of 90)", code)
	}
}

func TestPagesKeepRemove(t *testing.T) {
	dir := t.TempDir()
	in := writePDF(t, dir, "in.pdf", "a", "b", "c", "d")
	keep := filepath.Join(dir, "keep.pdf")
	if code := cmdPages([]string{in, "-o", keep, "--keep", "1-2"}); code != 0 {
		t.Fatalf("pages --keep exit = %d, want 0", code)
	}
	if n, _ := pdfops.PageCount(readPDF(t, keep)); n != 2 {
		t.Errorf("--keep 1-2 → %d pages, want 2", n)
	}
	rem := filepath.Join(dir, "rem.pdf")
	if code := cmdPages([]string{in, "-o", rem, "--remove", "1,2"}); code != 0 {
		t.Fatalf("pages --remove exit = %d, want 0", code)
	}
	if n, _ := pdfops.PageCount(readPDF(t, rem)); n != 2 {
		t.Errorf("--remove 1,2 of 4 → %d pages, want 2", n)
	}
	if code := cmdPages([]string{in, "-o", rem}); code != 1 {
		t.Errorf("pages with neither flag exit = %d, want 1", code)
	}
	if code := cmdPages([]string{in, "-o", rem, "--keep", "1", "--remove", "2"}); code != 1 {
		t.Errorf("pages with both flags exit = %d, want 1", code)
	}
}

func TestSplit(t *testing.T) {
	dir := t.TempDir()
	in := writePDF(t, dir, "in.pdf", "a", "b", "c", "d", "e")

	od := filepath.Join(dir, "every")
	if code := cmdSplit([]string{in, "--out-dir", od, "--every", "2"}); code != 0 {
		t.Fatalf("split --every exit = %d, want 0", code)
	}
	if got := countPDFs(t, od); got != 3 { // 1-2, 3-4, 5
		t.Errorf("--every 2 of 5 → %d files, want 3", got)
	}

	od2 := filepath.Join(dir, "ranges")
	if code := cmdSplit([]string{in, "--out-dir", od2, "--ranges", "1-2,3-5"}); code != 0 {
		t.Fatalf("split --ranges exit = %d, want 0", code)
	}
	if got := countPDFs(t, od2); got != 2 {
		t.Errorf("--ranges → %d files, want 2", got)
	}

	if code := cmdSplit([]string{in, "--out-dir", od2}); code != 1 {
		t.Errorf("split with no mode exit = %d, want 1", code)
	}
	if code := cmdSplit([]string{in, "--out-dir", od2, "--every", "2", "--bookmarks"}); code != 1 {
		t.Errorf("split with two modes exit = %d, want 1", code)
	}
	if code := cmdSplit([]string{in, "--every", "2"}); code != 1 {
		t.Errorf("split without --out-dir exit = %d, want 1", code)
	}
}

func countPDFs(t *testing.T, dir string) int {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range ents {
		if filepath.Ext(e.Name()) == ".pdf" {
			n++
		}
	}
	return n
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

func TestInPlaceRewritesAndKeepsMode(t *testing.T) {
	dir := t.TempDir()
	a := writePDF(t, dir, "a.pdf")
	b := writePDF(t, dir, "b.pdf", "b1", "b2")
	if err := os.Chmod(a, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(b, 0o640); err != nil {
		t.Fatal(err)
	}
	if code := cmdOptimize([]string{"-w", a, b}); code != 0 {
		t.Fatalf("optimize -w exit = %d, want 0", code)
	}
	for _, f := range []string{a, b} {
		if err := pdfops.Validate(readPDF(t, f)); err != nil {
			t.Errorf("%s invalid after in-place: %v", f, err)
		}
	}
	// The rewrite must preserve each file's original permission mode, not the
	// 0600 of the temp file it was written through.
	if got := mode(t, a); got != 0o644 {
		t.Errorf("a.pdf mode = %o after in-place, want 644", got)
	}
	if got := mode(t, b); got != 0o640 {
		t.Errorf("b.pdf mode = %o after in-place, want 640", got)
	}
}

func TestInPlaceRejectsOut(t *testing.T) {
	dir := t.TempDir()
	a := writePDF(t, dir, "a.pdf")
	if code := cmdOptimize([]string{"-w", "-o", filepath.Join(dir, "x.pdf"), a}); code != 1 {
		t.Errorf("optimize -w with -o exit = %d, want 1", code)
	}
}

func TestInPlaceLeavesOriginalOnError(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.pdf")
	junk := []byte("this is not a PDF")
	if err := os.WriteFile(bad, junk, 0o644); err != nil {
		t.Fatal(err)
	}
	if code := cmdOptimize([]string{"-w", bad}); code != 1 {
		t.Errorf("optimize -w on a non-PDF exit = %d, want 1", code)
	}
	// The atomic rewrite must leave the unreadable original untouched.
	if got := readPDF(t, bad); string(got) != string(junk) {
		t.Errorf("original modified after a failed in-place rewrite: %q", got)
	}
}

func TestPipelineStdinStdout(t *testing.T) {
	dir := t.TempDir()
	in := writePDF(t, dir, "in.pdf")

	// "-" input: feed the PDF on stdin, write to a file.
	stdin, err := os.Open(in)
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = stdin
	out := filepath.Join(dir, "out.pdf")
	code := cmdOptimize([]string{"-", "-o", out})
	os.Stdin = old
	stdin.Close()
	if code != 0 {
		t.Fatalf("optimize - -o out exit = %d, want 0", code)
	}
	if err := pdfops.Validate(readPDF(t, out)); err != nil {
		t.Fatalf("stdin->file output invalid: %v", err)
	}

	// "-" output: read a file, write the PDF to stdout (redirected to a file, so
	// the terminal guard sees a regular file and allows it).
	captured := filepath.Join(dir, "captured.pdf")
	cf, err := os.Create(captured)
	if err != nil {
		t.Fatal(err)
	}
	oldOut := os.Stdout
	os.Stdout = cf
	code = cmdSanitize([]string{in, "-o", "-"})
	os.Stdout = oldOut
	cf.Close()
	if code != 0 {
		t.Fatalf("sanitize -o - exit = %d, want 0", code)
	}
	if err := pdfops.Validate(readPDF(t, captured)); err != nil {
		t.Fatalf("file->stdout output invalid: %v", err)
	}
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestWatchScanSettlesThenActsOnce(t *testing.T) {
	dir := t.TempDir()
	p := writePDF(t, dir, "drop.pdf")
	seen, processed := map[string]fileState{}, map[string]bool{}
	calls := 0
	act := func(path string) (string, error) { calls++; return "done", nil }

	// First scan only records the file (not yet settled).
	scanOnce(dir, seen, processed, act)
	if calls != 0 {
		t.Fatalf("acted on first sight: calls = %d, want 0", calls)
	}
	// Second scan: unchanged since the first → settled → act exactly once.
	scanOnce(dir, seen, processed, act)
	if calls != 1 {
		t.Fatalf("after settle: calls = %d, want 1", calls)
	}
	// A later scan must not reprocess (even though a real in-place op would have
	// changed the mtime) — process-once per path.
	if err := os.Chtimes(p, time.Now(), time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	scanOnce(dir, seen, processed, act)
	if calls != 1 {
		t.Fatalf("reprocessed an already-handled file: calls = %d, want 1", calls)
	}
}

func TestWatchScanSkipsNonPDFAndUnsettled(t *testing.T) {
	dir := t.TempDir()
	writePDF(t, dir, "doc.pdf")
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	seen, processed := map[string]fileState{}, map[string]bool{}
	var acted []string
	act := func(path string) (string, error) { acted = append(acted, filepath.Base(path)); return "ok", nil }
	scanOnce(dir, seen, processed, act) // record
	scanOnce(dir, seen, processed, act) // settle → act
	if len(acted) != 1 || acted[0] != "doc.pdf" {
		t.Fatalf("acted on %v, want only [doc.pdf]", acted)
	}
}

func TestWatchTransformInPlace(t *testing.T) {
	dir := t.TempDir()
	p := writePDF(t, dir, "in.pdf")
	status, err := watchTransform(p, sanitize, "sanitized")
	if err != nil {
		t.Fatalf("watchTransform: %v", err)
	}
	if status != "sanitized" {
		t.Errorf("status = %q, want sanitized", status)
	}
	if err := pdfops.Validate(readPDF(t, p)); err != nil {
		t.Fatalf("rewritten file invalid: %v", err)
	}
}

func TestWatchTimestampSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	p := writePDF(t, dir, "in.pdf")
	if err := os.WriteFile(p+".ots", []byte("proof"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := watchTimestamp(p) // must short-circuit before any network call
	if err != nil {
		t.Fatalf("watchTimestamp: %v", err)
	}
	if status != "skipped (.ots exists)" {
		t.Errorf("status = %q, want skip", status)
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
