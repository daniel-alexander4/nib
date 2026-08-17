package cli

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"image"
	"image/png"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

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

// encryptPDF builds an AES-encrypted copy of pdf via pdfcpu (the fixture for the
// decrypt tests), mirroring pdfops/decrypt_test.go.
func encryptPDF(t *testing.T, pdf []byte, userPW, ownerPW string) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := api.Encrypt(bytes.NewReader(pdf), &out, model.NewAESConfiguration(userPW, ownerPW, 256)); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDecrypt(t *testing.T) {
	dir := t.TempDir()
	plain := readPDF(t, writePDF(t, dir, "plain.pdf", "a", "b", "c"))

	// Open-password protected → unlock with the right password from a file.
	enc := filepath.Join(dir, "enc.pdf")
	mustWrite(t, enc, encryptPDF(t, plain, "open123", "owner456"))
	pwf := filepath.Join(dir, "pw.txt")
	mustWrite(t, pwf, []byte("open123\n"))
	out := filepath.Join(dir, "out.pdf")
	if code := cmdDecrypt([]string{enc, "-o", out, "--password-file", pwf}); code != 0 {
		t.Fatalf("decrypt with correct password exit = %d, want 0", code)
	}
	if err := pdfops.Validate(readPDF(t, out)); err != nil {
		t.Fatalf("decrypted output invalid: %v", err)
	}

	// Wrong password → exit 1.
	bad := filepath.Join(dir, "bad.txt")
	mustWrite(t, bad, []byte("nope"))
	if code := cmdDecrypt([]string{enc, "-o", out, "--password-file", bad}); code != 1 {
		t.Errorf("decrypt with wrong password exit = %d, want 1", code)
	}

	// Already-plain PDF → pass through unchanged, exit 0 (batch-idempotent).
	pin := writePDF(t, dir, "p2.pdf", "x")
	pout := filepath.Join(dir, "p2out.pdf")
	if code := cmdDecrypt([]string{pin, "-o", pout}); code != 0 {
		t.Fatalf("decrypt of a plain PDF exit = %d, want 0 (idempotent pass-through)", code)
	}
	if err := pdfops.Validate(readPDF(t, pout)); err != nil {
		t.Fatalf("passed-through output invalid: %v", err)
	}

	// Owner-only restrictions drop with an empty password (no --password-file).
	owner := filepath.Join(dir, "owner.pdf")
	mustWrite(t, owner, encryptPDF(t, plain, "", "owner456"))
	oout := filepath.Join(dir, "oout.pdf")
	if code := cmdDecrypt([]string{owner, "-o", oout}); code != 0 {
		t.Fatalf("decrypt owner-only (empty password) exit = %d, want 0", code)
	}
}

func TestEncrypt(t *testing.T) {
	dir := t.TempDir()
	plain := writePDF(t, dir, "plain.pdf", "a", "b", "c")
	pwf := filepath.Join(dir, "pw.txt")
	mustWrite(t, pwf, []byte("secret123\n"))

	// Encrypt → the output needs the password; RemovePassword with it round-trips
	// to a valid plain PDF.
	out := filepath.Join(dir, "enc.pdf")
	if code := cmdEncrypt([]string{plain, "-o", out, "--password-file", pwf}); code != 0 {
		t.Fatalf("encrypt exit = %d, want 0", code)
	}
	enc := readPDF(t, out)
	if err := pdfops.Validate(enc); err == nil {
		t.Fatal("encrypted output validated without a password — encryption did not take")
	}
	back, err := pdfops.RemovePassword(enc, "secret123")
	if err != nil {
		t.Fatalf("round-trip RemovePassword: %v", err)
	}
	if err := pdfops.Validate(back); err != nil {
		t.Fatalf("round-tripped output invalid: %v", err)
	}

	// No password supplied → exit 1 (encrypt requires one, unlike decrypt).
	if code := cmdEncrypt([]string{plain, "-o", filepath.Join(dir, "x.pdf")}); code != 1 {
		t.Errorf("encrypt with no password exit = %d, want 1", code)
	}
}

func TestNup(t *testing.T) {
	dir := t.TempDir()
	in := writePDF(t, dir, "in.pdf", "a", "b", "c", "d")
	out := filepath.Join(dir, "out.pdf")
	if code := cmdNup([]string{in, "-o", out, "--n", "2"}); code != 0 {
		t.Fatalf("nup exit = %d, want 0", code)
	}
	if n, _ := pdfops.PageCount(readPDF(t, out)); n != 2 { // 4 pages 2-up → 2 sheets
		t.Errorf("4-page 2-up → %d sheets, want 2", n)
	}
	if code := cmdNup([]string{in, "-o", out}); code != 1 { // missing --n
		t.Errorf("nup without --n exit = %d, want 1", code)
	}
}

func TestNormalize(t *testing.T) {
	dir := t.TempDir()
	in := writePDF(t, dir, "in.pdf", "a", "b", "c")
	out := filepath.Join(dir, "out.pdf")
	if code := cmdNormalize([]string{in, "-o", out}); code != 0 {
		t.Fatalf("normalize exit = %d, want 0", code)
	}
	if err := pdfops.Validate(readPDF(t, out)); err != nil {
		t.Fatalf("normalized output invalid: %v", err)
	}
	if n, _ := pdfops.PageCount(readPDF(t, out)); n != 3 {
		t.Errorf("normalize changed page count → %d, want 3", n)
	}
}

func TestPagenum(t *testing.T) {
	dir := t.TempDir()
	in := writePDF(t, dir, "in.pdf", "a", "b")
	out := filepath.Join(dir, "out.pdf")
	if code := cmdPagenum([]string{in, "-o", out, "--prefix", "ABC", "--pad", "4"}); code != 0 {
		t.Fatalf("pagenum exit = %d, want 0", code)
	}
	if err := pdfops.Validate(readPDF(t, out)); err != nil {
		t.Fatalf("page-numbered output invalid: %v", err)
	}
}

func TestPagelabels(t *testing.T) {
	dir := t.TempDir()
	in := writePDF(t, dir, "in.pdf", "a", "b", "c", "d") // 4 pages
	out := filepath.Join(dir, "out.pdf")
	if code := cmdPagelabels([]string{in, "-o", out, "--range", "1:roman-lower", "--range", "3:decimal:1:A-"}); code != 0 {
		t.Fatalf("pagelabels exit = %d, want 0", code)
	}
	if err := pdfops.Validate(readPDF(t, out)); err != nil {
		t.Fatalf("page-labelled output invalid: %v", err)
	}
}

func TestPagelabelsBadArgs(t *testing.T) {
	dir := t.TempDir()
	in := writePDF(t, dir, "in.pdf", "a", "b")
	out := filepath.Join(dir, "out.pdf")
	if code := cmdPagelabels([]string{in, "-o", out}); code == 0 {
		t.Fatal("expected nonzero exit with no --range")
	}
	if code := cmdPagelabels([]string{in, "-o", out, "--range", "1"}); code == 0 {
		t.Fatal("expected nonzero exit for a malformed --range (missing style)")
	}
}

// fillTestForm writes a fillable form (one text field "fullName") into dir.
func fillTestForm(t *testing.T, dir string) string {
	t.Helper()
	base, err := testpdf.Text("x")
	if err != nil {
		t.Fatal(err)
	}
	form, err := pdfops.AuthorForm(base, []pdfops.FormField{
		{Page: 1, Rect: [4]float64{50, 50, 250, 70}, Kind: "text", Name: "fullName"},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "form.pdf")
	if err := os.WriteFile(p, form, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFill(t *testing.T) {
	dir := t.TempDir()
	formPath := fillTestForm(t, dir)

	// single JSON fill → one PDF
	jsonPath := filepath.Join(dir, "d.json")
	if err := os.WriteFile(jsonPath, []byte(`{"forms":[{"textfield":[{"name":"fullName","value":"Jane"}]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.pdf")
	if code := cmdFill([]string{formPath, "--data", jsonPath, "-o", out}); code != 0 {
		t.Fatalf("json fill exit = %d, want 0", code)
	}
	if err := pdfops.Validate(readPDF(t, out)); err != nil {
		t.Fatalf("filled output invalid: %v", err)
	}

	// CSV mail-merge → one PDF per row
	csvPath := filepath.Join(dir, "d.csv")
	if err := os.WriteFile(csvPath, []byte("fullName\nJane\nJohn\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	od := filepath.Join(dir, "merged")
	if code := cmdFill([]string{formPath, "--data", csvPath, "--out-dir", od}); code != 0 {
		t.Fatalf("csv mail-merge exit = %d, want 0", code)
	}
	if n := countPDFs(t, od); n != 2 {
		t.Fatalf("mail-merge wrote %d files, want 2", n)
	}
}

func TestFillBadModes(t *testing.T) {
	dir := t.TempDir()
	formPath := fillTestForm(t, dir)
	csvPath := filepath.Join(dir, "d.csv")
	_ = os.WriteFile(csvPath, []byte("fullName\nJane\n"), 0o644)
	jsonPath := filepath.Join(dir, "d.json")
	_ = os.WriteFile(jsonPath, []byte(`{"forms":[{}]}`), 0o644)

	if code := cmdFill([]string{formPath, "--data", csvPath, "-o", filepath.Join(dir, "o.pdf")}); code == 0 {
		t.Error("CSV with -o should fail (mail-merge writes many files)")
	}
	if code := cmdFill([]string{formPath, "--data", jsonPath, "--out-dir", dir}); code == 0 {
		t.Error("JSON with --out-dir should fail")
	}
	if code := cmdFill([]string{formPath}); code == 0 {
		t.Error("missing --data should fail")
	}
}

// TestPDFA: `nib pdfa` refuses a non-embedded-font document (exit 1) and converts
// a fontless image PDF to a valid PDF/A candidate carrying the identifier.
func TestPDFA(t *testing.T) {
	dir := t.TempDir()

	// testpdf.Text uses the standard-14 Courier (not embedded) → refused.
	textPDF, err := testpdf.Text("hi")
	if err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "text.pdf")
	mustWrite(t, bad, textPDF)
	if code := cmdPDFA([]string{bad, "-o", filepath.Join(dir, "bad-out.pdf")}); code != 1 {
		t.Fatalf("pdfa on a non-embedded-font PDF exit = %d, want 1", code)
	}

	// An image-only PDF has no fonts → converts.
	imgPDF, err := pdfops.ImagesToPDF([]pdfops.RasterPage{{Image: cliTinyPNG(t), W: 200, H: 200}})
	if err != nil {
		t.Fatal(err)
	}
	img := filepath.Join(dir, "img.pdf")
	mustWrite(t, img, imgPDF)
	out := filepath.Join(dir, "img-pdfa.pdf")
	if code := cmdPDFA([]string{img, "-o", out}); code != 0 {
		t.Fatalf("pdfa on an image PDF exit = %d, want 0", code)
	}
	got := readPDF(t, out)
	if err := pdfops.Validate(got); err != nil {
		t.Fatalf("PDF/A output invalid: %v", err)
	}
	if !bytes.Contains(got, []byte("pdfaid:part")) {
		t.Error("PDF/A output missing the pdfaid identifier")
	}
}

// cliTinyPNG encodes a small blank image for the fontless PDF/A test.
func cliTinyPNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 16, 16))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// TestExportXFDFAndFill round-trips through the CLI: fill a form from an XFDF
// record, then export the filled form's data back out as XFDF.
func TestExportXFDFAndFill(t *testing.T) {
	dir := t.TempDir()
	formPath := fillTestForm(t, dir)

	xfdfPath := filepath.Join(dir, "d.xfdf")
	if err := os.WriteFile(xfdfPath, []byte(`<xfdf xmlns="http://ns.adobe.com/xfdf/"><fields><field name="fullName"><value>Grace</value></field></fields></xfdf>`), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "filled.pdf")
	if code := cmdFill([]string{formPath, "--data", xfdfPath, "-o", out}); code != 0 {
		t.Fatalf("xfdf fill exit = %d, want 0", code)
	}
	if err := pdfops.Validate(readPDF(t, out)); err != nil {
		t.Fatalf("filled output invalid: %v", err)
	}

	xout := filepath.Join(dir, "exported.xfdf")
	if code := cmdExportXFDF([]string{out, "-o", xout}); code != 0 {
		t.Fatalf("export-xfdf exit = %d, want 0", code)
	}
	data, err := os.ReadFile(xout)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Grace") {
		t.Fatalf("exported XFDF missing value:\n%s", data)
	}
}

// TestContinuousPagenum threads ONE Bates counter across a file set: file 1's
// pages number from --start, file 2 continues where file 1 ended, and --total's
// "of N" is the set's grand total — not each file's. Covers both output modes,
// the start clamp, and the mode guards.
func TestContinuousPagenum(t *testing.T) {
	dir := t.TempDir()
	a := writePDF(t, dir, "a.pdf", "alpha", "beta", "gamma") // 3 pages
	b := writePDF(t, dir, "b.pdf", "delta", "epsilon")       // 2 pages

	// --out-dir: copies numbered as one 1..5 run, "of 5" on every page.
	od := filepath.Join(dir, "stamped")
	if code := cmdPagenum([]string{a, b, "--continuous", "--out-dir", od, "--prefix", "BATES", "--pad", "4", "--total"}); code != 0 {
		t.Fatalf("continuous --out-dir exit = %d, want 0", code)
	}
	if got := countPDFs(t, od); got != 2 {
		t.Fatalf("continuous --out-dir → %d files, want 2", got)
	}
	for _, name := range []string{"a.pdf", "b.pdf"} {
		if err := pdfops.Validate(readPDF(t, filepath.Join(od, name))); err != nil {
			t.Fatalf("%s invalid: %v", name, err)
		}
	}
	// Strong check (pdftotext-gated): the counter threads across the boundary.
	if _, err := exec.LookPath("pdftotext"); err == nil {
		atxt := pdfText(t, filepath.Join(od, "a.pdf"))
		btxt := pdfText(t, filepath.Join(od, "b.pdf"))
		for _, want := range []string{"BATES0001", "BATES0002", "BATES0003", "of 5"} {
			if !strings.Contains(atxt, want) {
				t.Errorf("file a missing %q\n--- pdftotext gave ---\n%s", want, atxt)
			}
		}
		for _, want := range []string{"BATES0004", "BATES0005", "of 5"} {
			if !strings.Contains(btxt, want) {
				t.Errorf("file b missing %q\n--- pdftotext gave ---\n%s", want, btxt)
			}
		}
	}

	// -w rewrites each file in place, same continuous sequence.
	a2 := writePDF(t, dir, "a2.pdf", "alpha", "beta", "gamma")
	b2 := writePDF(t, dir, "b2.pdf", "delta", "epsilon")
	if code := cmdPagenum([]string{a2, b2, "--continuous", "-w", "--prefix", "X"}); code != 0 {
		t.Fatalf("continuous -w exit = %d, want 0", code)
	}
	if _, err := exec.LookPath("pdftotext"); err == nil {
		if txt := pdfText(t, b2); !strings.Contains(txt, "X4") || !strings.Contains(txt, "X5") {
			t.Errorf("in-place file b2 should continue at X4/X5, got:\n%s", txt)
		}
	}

	// --start ≤ 0 clamps to 1 (clamp must live in the continuous path, so the
	// threaded offset agrees): file b still starts at 4, no duplicate.
	od0 := filepath.Join(dir, "start0")
	if code := cmdPagenum([]string{a, b, "--continuous", "--out-dir", od0, "--prefix", "Z", "--start", "0"}); code != 0 {
		t.Fatalf("continuous --start 0 exit = %d, want 0", code)
	}
	if _, err := exec.LookPath("pdftotext"); err == nil {
		if txt := pdfText(t, filepath.Join(od0, "b.pdf")); !strings.Contains(txt, "Z4") || !strings.Contains(txt, "Z5") {
			t.Errorf("--start 0 should clamp to 1 and thread to Z4/Z5, got:\n%s", txt)
		}
	}

	// Mode guards: -o rejected; neither/both of -w and --out-dir rejected; stdin rejected.
	if code := cmdPagenum([]string{a, "--continuous", "-o", filepath.Join(dir, "x.pdf")}); code != 1 {
		t.Errorf("continuous with -o exit = %d, want 1", code)
	}
	if code := cmdPagenum([]string{a, b, "--continuous"}); code != 1 {
		t.Errorf("continuous with no output mode exit = %d, want 1", code)
	}
	if code := cmdPagenum([]string{a, b, "--continuous", "-w", "--out-dir", od}); code != 1 {
		t.Errorf("continuous with both -w and --out-dir exit = %d, want 1", code)
	}
	if code := cmdPagenum([]string{"-", "--continuous", "--out-dir", od}); code != 1 {
		t.Errorf("continuous with stdin exit = %d, want 1", code)
	}
}

// pdfText extracts a PDF's text via poppler's pdftotext, for asserting on stamped
// content. Callers gate on exec.LookPath("pdftotext") and skip when it's absent.
func pdfText(t *testing.T, path string) string {
	t.Helper()
	out, err := exec.Command("pdftotext", "-enc", "UTF-8", path, "-").Output()
	if err != nil {
		t.Fatalf("pdftotext %s: %v", path, err)
	}
	return string(out)
}

// captureStdout runs fn with os.Stdout redirected to a temp file and returns what
// it printed plus the exit code — for asserting the report/list commands' output.
func captureStdout(t *testing.T, fn func() int) (string, int) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "out-*")
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = f
	code := fn()
	os.Stdout = old
	f.Close()
	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(b), code
}

func TestAttachments(t *testing.T) {
	dir := t.TempDir()
	base := readPDF(t, writePDF(t, dir, "base.pdf", "a", "b"))
	withAtt, err := pdfops.AddAttachment(base, "notes.txt", []byte("hello attached"))
	if err != nil {
		t.Fatal(err)
	}
	in := filepath.Join(dir, "withatt.pdf")
	mustWrite(t, in, withAtt)

	if out, code := captureStdout(t, func() int { return cmdAttachments([]string{in}) }); code != 0 || !strings.Contains(out, "notes.txt") {
		t.Fatalf("attachments list: code=%d out=%q, want 0 + notes.txt", code, out)
	}
	if out, code := captureStdout(t, func() int { return cmdAttachments([]string{in, "--json"}) }); code != 0 || !strings.Contains(out, `"name":"notes.txt"`) {
		t.Fatalf("attachments --json: code=%d out=%q", code, out)
	}

	// extract round-trips the exact bytes.
	ex := filepath.Join(dir, "out.txt")
	if code := cmdAttachments([]string{in, "--extract", "notes.txt", "-o", ex}); code != 0 {
		t.Fatalf("attachments --extract exit = %d, want 0", code)
	}
	if got := readPDF(t, ex); string(got) != "hello attached" {
		t.Errorf("extracted bytes = %q, want %q", got, "hello attached")
	}
	if code := cmdAttachments([]string{in, "--extract", "nope.txt", "-o", ex}); code != 1 {
		t.Errorf("extract of a missing name exit = %d, want 1", code)
	}

	// add a second file, then the listing shows both.
	af := filepath.Join(dir, "extra.txt")
	mustWrite(t, af, []byte("more"))
	added := filepath.Join(dir, "added.pdf")
	if code := cmdAttachments([]string{in, "--add", af, "-o", added}); code != 0 {
		t.Fatalf("attachments --add exit = %d, want 0", code)
	}
	if aa, _ := pdfops.Attachments(readPDF(t, added)); len(aa) != 2 {
		t.Errorf("after add: %d attachments, want 2", len(aa))
	}
}

func TestOutline(t *testing.T) {
	dir := t.TempDir()
	base := readPDF(t, writePDF(t, dir, "base.pdf", "a", "b", "c"))
	withBm, err := pdfops.SetOutline(base, []pdfops.OutlineItem{
		{Title: "Intro", Page: 1, Level: 0},
		{Title: "Body", Page: 2, Level: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	in := filepath.Join(dir, "bm.pdf")
	mustWrite(t, in, withBm)

	if out, code := captureStdout(t, func() int { return cmdOutline([]string{in}) }); code != 0 || !strings.Contains(out, "Intro") || !strings.Contains(out, "Body") {
		t.Fatalf("outline list: code=%d out=%q", code, out)
	}

	// A bookmark-less PDF lists nothing and exits 0 (rescrut: api.Bookmarks → nil,nil).
	plain := writePDF(t, dir, "plain.pdf", "x")
	if out, code := captureStdout(t, func() int { return cmdOutline([]string{plain}) }); code != 0 {
		t.Fatalf("outline on a bookmark-less PDF exit = %d, want 0 (out %q)", code, out)
	}
	if out, code := captureStdout(t, func() int { return cmdOutline([]string{plain, "--json"}) }); code != 0 || strings.TrimSpace(out) != "[]" {
		t.Errorf("outline --json on a bookmark-less PDF = %q (code %d), want []", out, code)
	}
}

func TestRunDispatch(t *testing.T) {
	if handled, _ := Run(nil, "1.2.3"); handled {
		t.Error("empty args should not be handled (falls through to desktop boot)")
	}
	if handled, _ := Run([]string{"open-this.pdf"}, "1.2.3"); handled {
		t.Error("a non-verb first arg should fall through to the desktop boot")
	}
	// A mistyped verb alongside a transform-only flag is an unknown command, not a
	// file to open — booting the GUI would silently discard the -o.
	if handled, code := Run([]string{"optmize", "in.pdf", "-o", "out.pdf"}, "1.2.3"); !handled || code != 1 {
		t.Errorf("typo'd verb with -o: handled=%v code=%d, want true/1", handled, code)
	}
	if handled, code := Run([]string{"optmize", "in.pdf", "--out=out.pdf"}, "1.2.3"); !handled || code != 1 {
		t.Errorf("typo'd verb with --out=: handled=%v code=%d, want true/1", handled, code)
	}
	// A bare non-verb (no transform flag) still falls through to open a file.
	if handled, _ := Run([]string{"my report.pdf"}, "1.2.3"); handled {
		t.Error("a bare non-verb should still fall through to the desktop boot")
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
	seen, processed, failed := map[string]fileState{}, map[string]bool{}, map[string]fileState{}
	calls := 0
	act := func(path string) (string, error) { calls++; return "done", nil }

	// First scan only records the file (not yet settled).
	scanOnce(dir, seen, processed, failed, act)
	if calls != 0 {
		t.Fatalf("acted on first sight: calls = %d, want 0", calls)
	}
	// Second scan: unchanged since the first → settled → act exactly once.
	scanOnce(dir, seen, processed, failed, act)
	if calls != 1 {
		t.Fatalf("after settle: calls = %d, want 1", calls)
	}
	// A later scan must not reprocess (even though a real in-place op would have
	// changed the mtime) — process-once per path.
	if err := os.Chtimes(p, time.Now(), time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	scanOnce(dir, seen, processed, failed, act)
	if calls != 1 {
		t.Fatalf("reprocessed an already-handled file: calls = %d, want 1", calls)
	}
}

// TestWatchScanRetriesFailureOnlyAfterChange proves the failure path: a settled
// file whose action errors is NOT retried while its size+mtime stay the same (no
// per-scan error spam), but a change to the file makes it eligible again — and a
// then-successful action marks it processed for good.
func TestWatchScanRetriesFailureOnlyAfterChange(t *testing.T) {
	dir := t.TempDir()
	p := writePDF(t, dir, "bad.pdf")
	seen, processed, failed := map[string]fileState{}, map[string]bool{}, map[string]fileState{}
	calls := 0
	fail := true
	act := func(path string) (string, error) {
		calls++
		if fail {
			return "", errors.New("boom")
		}
		return "done", nil
	}

	scanOnce(dir, seen, processed, failed, act) // record
	scanOnce(dir, seen, processed, failed, act) // settle → act → fail
	if calls != 1 {
		t.Fatalf("after settle: calls = %d, want 1", calls)
	}
	// Unchanged file: the failure must not be retried on every scan.
	scanOnce(dir, seen, processed, failed, act)
	scanOnce(dir, seen, processed, failed, act)
	if calls != 1 {
		t.Fatalf("retried an unchanged failing file: calls = %d, want 1", calls)
	}
	// Change the file: eligible again after it re-settles.
	fail = false
	if err := os.Chtimes(p, time.Now(), time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	scanOnce(dir, seen, processed, failed, act) // sees the change — let it settle
	scanOnce(dir, seen, processed, failed, act) // settled → retry → success
	if calls != 2 {
		t.Fatalf("after change: calls = %d, want 2", calls)
	}
	if !processed[p] {
		t.Error("successful retry did not mark the file processed")
	}
}

func TestWatchScanSkipsNonPDFAndUnsettled(t *testing.T) {
	dir := t.TempDir()
	writePDF(t, dir, "doc.pdf")
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	seen, processed, failed := map[string]fileState{}, map[string]bool{}, map[string]fileState{}
	var acted []string
	act := func(path string) (string, error) { acted = append(acted, filepath.Base(path)); return "ok", nil }
	scanOnce(dir, seen, processed, failed, act) // record
	scanOnce(dir, seen, processed, failed, act) // settle → act
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
		// Pin stdin to a non-terminal so the interactive prompt path is skipped —
		// otherwise an interactive `go test` (tty stdin) would block on ReadPassword.
		devnull, err := os.Open(os.DevNull)
		if err != nil {
			t.Fatal(err)
		}
		defer devnull.Close()
		old := os.Stdin
		os.Stdin = devnull
		defer func() { os.Stdin = old }()
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

// TestTimestampDoesNotClobberAnExistingProof pins the guard on a destructive write.
//
// A .ots proof starts pending and is anchored to a Bitcoin block hours to days
// later. Re-stamping replaces a confirmed proof with a fresh pending one and the
// anchoring cannot be recovered — so the unguarded write turned the README's own
// `for f in *.pdf; do nib timestamp "$f"; done` into silent destruction of every
// proof in the directory on a second run. watchTimestamp already had this guard;
// the two paths disagreed about one invariant.
//
// All three arms are offline by construction: the skip returns before any calendar
// is contacted, and the two non-skip arms use a missing input file so they fail at
// the read rather than on the network. That is also what makes them a positive
// control — without them, a guard that skipped unconditionally would pass.
func TestTimestampDoesNotClobberAnExistingProof(t *testing.T) {
	t.Run("an existing proof is left untouched", func(t *testing.T) {
		dir := t.TempDir()
		pdf := filepath.Join(dir, "doc.pdf")
		if err := os.WriteFile(pdf, []byte("%PDF-1.7\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		proof := pdf + ".ots"
		original := []byte("a confirmed proof that took days to anchor")
		if err := os.WriteFile(proof, original, 0o644); err != nil {
			t.Fatal(err)
		}

		if code := timestampCreate([]string{pdf}, false); code != 0 {
			t.Errorf("skipping an already-stamped file should exit 0, got %d", code)
		}
		got, err := os.ReadFile(proof)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, original) {
			t.Errorf("the existing proof was overwritten: had %q, now %q", original, got)
		}
	})

	t.Run("a file with no proof is not skipped", func(t *testing.T) {
		// No .ots beside it, so the guard must let it through to the read — which
		// fails, because the input does not exist. A guard that skipped everything
		// would return 0 here.
		missing := filepath.Join(t.TempDir(), "absent.pdf")
		if code := timestampCreate([]string{missing}, false); code == 0 {
			t.Error("a file with no proof was skipped — the guard is refusing everything, not just re-stamps")
		}
	})

	t.Run("--force gets past the guard", func(t *testing.T) {
		dir := t.TempDir()
		proof := filepath.Join(dir, "absent.pdf.ots")
		if err := os.WriteFile(proof, []byte("existing"), 0o644); err != nil {
			t.Fatal(err)
		}
		// The proof exists, so without force this would skip and exit 0. With force
		// it must proceed to the read and fail on the missing input.
		if code := timestampCreate([]string{filepath.Join(dir, "absent.pdf")}, true); code == 0 {
			t.Error("--force still skipped a file that already had a proof")
		}
	})
}

// TestWriteAtomicFollowsASymlink pins the in-place rewrite against a link.
//
// os.Stat follows a symlink to read the mode, but CreateTemp+Rename replaces the
// LINK — so `nib optimize -w link.pdf` used to destroy the link, leave the real file
// untouched, and write the output into the link's directory. A watch directory of
// symlinks into a document tree is the case that makes this ordinary rather than
// exotic.
func TestWriteAtomicFollowsASymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.pdf")
	link := filepath.Join(dir, "link.pdf")
	if err := os.WriteFile(real, []byte("ORIGINAL"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := writeAtomic(link, []byte("REWRITTEN")); err != nil {
		t.Fatal(err)
	}

	// The link must still be a link...
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("the symlink was replaced by a regular file — the link is gone and the real file was not updated")
	}
	// ...and the REAL file must carry the new bytes.
	got, err := os.ReadFile(real)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "REWRITTEN" {
		t.Errorf("the target was not rewritten: %q", got)
	}
}

// TestWatchIgnoresFilesAlreadyPresent pins what `nib watch` has always claimed to
// do — "each PDF ADDED to it" (watch.go), "each new PDF" (the command table),
// "dropped into DIR" (README) — and did not.
//
// scanOnce walks every settled .pdf in the directory, so pointing
// `nib watch ~/Documents --do sanitize` at an existing folder rewrote every PDF
// already in it, in place. This drives scanOnce directly with the baseline
// watchLoop now takes, rather than starting the daemon.
func TestWatchIgnoresFilesAlreadyPresent(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "already-here.pdf")
	if err := os.WriteFile(existing, []byte("%PDF-1.7\nOLD"), 0o644); err != nil {
		t.Fatal(err)
	}

	var acted []string
	act := func(path string) (string, error) {
		acted = append(acted, filepath.Base(path))
		return "acted", nil
	}

	seen := map[string]fileState{}
	processed := map[string]bool{}
	failed := map[string]fileState{}

	// The baseline watchLoop takes before its first scan.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		processed[filepath.Join(dir, e.Name())] = true
	}

	// Two scans: a file must settle (size+mtime unchanged) before it is acted on.
	scanOnce(dir, seen, processed, failed, act)
	scanOnce(dir, seen, processed, failed, act)
	if len(acted) != 0 {
		t.Errorf("a pre-existing file was processed: %v — the watch rewrote a file the user never dropped in", acted)
	}

	// The positive control: a file that ARRIVES must still be acted on, or a watch
	// that ignored everything would pass the assertion above.
	arrived := filepath.Join(dir, "dropped-in.pdf")
	if err := os.WriteFile(arrived, []byte("%PDF-1.7\nNEW"), 0o644); err != nil {
		t.Fatal(err)
	}
	scanOnce(dir, seen, processed, failed, act)
	scanOnce(dir, seen, processed, failed, act)
	if len(acted) != 1 || acted[0] != "dropped-in.pdf" {
		t.Errorf("a newly-arrived file was not processed: acted = %v", acted)
	}
}
