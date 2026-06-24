package pdfops

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// veraPDF is the reference PDF/A validator (Java). Like the poppler oracles it is
// a TEST-ONLY external tool — it never touches Nib's cgo-free runtime — used here
// to prove that what the PDF/A paths emit actually conforms, not just that it
// asserts conformance. It's located via $NIB_VERAPDF, then PATH, then the default
// ~/verapdf install dir; tests skip cleanly when it's absent.
func verapdfPath() string {
	if p := os.Getenv("NIB_VERAPDF"); p != "" {
		return p
	}
	if p, err := exec.LookPath("verapdf"); err == nil {
		return p
	}
	if home, _ := os.UserHomeDir(); home != "" {
		cand := filepath.Join(home, "verapdf", "verapdf")
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	return ""
}

// requireVeraPDFCompliant fails the test unless veraPDF validates pdf at the given
// flavour (e.g. "2b"). veraPDF exits non-zero for a non-compliant file, so its
// error is ignored and the XML report is parsed instead.
func requireVeraPDFCompliant(t *testing.T, pdf []byte, flavour string) {
	t.Helper()
	vp := verapdfPath()
	if vp == "" {
		t.Skip("veraPDF not installed (set NIB_VERAPDF, put verapdf on PATH, or install to ~/verapdf)")
	}
	f := filepath.Join(t.TempDir(), "out.pdf")
	if err := os.WriteFile(f, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := exec.Command(vp, "--flavour", flavour, f).Output()
	if !bytes.Contains(out, []byte(`isCompliant="true"`)) {
		t.Errorf("veraPDF: output is not PDF/A-%s compliant:\n%s", flavour, verapdfFailures(out))
	}
}

// verapdfFailures pulls the failed-rule and error-message lines out of veraPDF's
// XML report for an actionable failure message.
func verapdfFailures(out []byte) string {
	var b strings.Builder
	for _, line := range strings.Split(string(out), "\n") {
		l := strings.TrimSpace(line)
		if strings.Contains(l, `status="failed"`) || strings.Contains(l, "<errorMessage") || strings.Contains(l, "<description") {
			b.WriteString(l + "\n")
		}
	}
	if b.Len() == 0 {
		return string(out)
	}
	return b.String()
}

// TestPDFAConformanceVeraPDF proves both converters emit genuinely conformant
// PDF/A-2b — the pure-Go normalizer (on a fontless image PDF it accepts) and the
// Ghostscript path (on a non-embedded-font doc pure-Go refuses). Skips when
// veraPDF (and, for the gs leg, Ghostscript) isn't installed.
func TestPDFAConformanceVeraPDF(t *testing.T) {
	img, err := ImagesToPDF([]RasterPage{rasterPage(t, 200, 200)})
	if err != nil {
		t.Fatal(err)
	}
	pure, blockers, err := PreparePDFA(img)
	if err != nil || len(blockers) > 0 {
		t.Fatalf("PreparePDFA: err=%v blockers=%v", err, blockers)
	}
	requireVeraPDFCompliant(t, pure, "2b")

	if GhostscriptAvailable() {
		src, err := testpdf.Text("veraPDF Ghostscript conformance check")
		if err != nil {
			t.Fatal(err)
		}
		gsOut, err := ConvertPDFAGhostscript(src)
		if err != nil {
			t.Fatalf("ConvertPDFAGhostscript: %v", err)
		}
		requireVeraPDFCompliant(t, gsOut, "2b")
	}
}
