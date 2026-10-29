package nib

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNibNeverTurnsOnPdfcpuNetworkCode — v0.16.2 carries network code that earlier releases did not:
// `validate/link_http.go` (fetches a document's link targets) and `pkg/pdfcpu/sign/revocation_http.go` (OCSP and CRL).
// Both are off by default (`ValidateLinks` false, model/configuration.go:833) and nib must keep them so: a document
// that makes this machine fetch a URL it names is an egress channel (D4's security pin,
// P02.S09). No nib file may set ValidateLinks true or import pdfcpu's sign package.
func TestNibNeverTurnsOnPdfcpuNetworkCode(t *testing.T) {
	linksOn := regexp.MustCompile(`ValidateLinks\s*(=|:)\s*(true|[^f\s,}])`)
	signImport := regexp.MustCompile(`"github\.com/pdfcpu/pdfcpu/pkg/pdfcpu/sign"`)
	seen := 0
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "third_party", "node_modules", ".git", "dist", "web":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || p == "egress_test.go" {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		seen++
		for i, ln := range strings.Split(string(raw), "\n") {
			if t := strings.TrimSpace(ln); strings.HasPrefix(t, "//") {
				continue
			}
			if linksOn.MatchString(ln) {
				t.Errorf("%s:%d sets ValidateLinks to something other than false — pdfcpu would fetch the URLs a document names", p, i+1)
			}
			if signImport.MatchString(ln) {
				t.Errorf("%s:%d imports pdfcpu's sign package, which carries OCSP/CRL fetching (revocation_http.go)", p, i+1)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen < 100 {
		t.Fatalf("setup: the scan saw %d Go files — it is looking in the wrong place", seen)
	}
}
