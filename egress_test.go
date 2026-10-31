package nib

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// assignsLinks finds `ValidateLinks = x`, `ValidateLinks: x` and `ValidateLinks := x`; a comparison is not a setting.
var assignsLinks = regexp.MustCompile(`ValidateLinks\s*(:=|==|!=|=|:)\s*(\S*)`)

// linkValidationOn reports whether a source line sets ValidateLinks to anything but the bare literal `false`
// (nothing computed, nothing from a variable: those are the shapes a later edit can turn on without touching this scan).
func linkValidationOn(line string) bool {
	m := assignsLinks.FindStringSubmatchIndex(line)
	if m == nil {
		return false
	}
	op := line[m[2]:m[3]]
	if op == "==" || op == "!=" {
		return false
	}
	rhs := strings.TrimRight(line[m[4]:m[5]], ",}")
	if rhs != "false" {
		return true
	}
	tail := strings.TrimSpace(line[m[5]:])
	return !(tail == "" || strings.HasPrefix(tail, "//"))
}

// TestTheEgressScanSeesTheShapesItGuards plants each way ValidateLinks can be switched on, and each way it is only
// read or switched off, so the scan below is shown to fire and to stay quiet rather than assumed to.
func TestTheEgressScanSeesTheShapesItGuards(t *testing.T) {
	for _, on := range []string{
		"conf.ValidateLinks = true", "c.ValidateLinks=flag", "ValidateLinks: f(),", "x := Config{ValidateLinks: !off}",
		"conf.ValidateLinks = false || other", "ValidateLinks := true",
	} {
		if !linkValidationOn(on) {
			t.Errorf("the scan does not see %q as switching link validation on", on)
		}
	}
	for _, off := range []string{
		"conf.ValidateLinks = false", "ValidateLinks: false,", "if conf.ValidateLinks == true {", "conf.ValidateLinks = false // pdfcpu would fetch",
		"nothing to see here",
	} {
		if linkValidationOn(off) {
			t.Errorf("the scan flags %q, which does not switch link validation on", off)
		}
	}
}

// TestNibNeverTurnsOnPdfcpuNetworkCode — v0.16.2 carries network code that earlier releases did not:
// `validate/link_http.go` (fetches a document's link targets) and `pkg/pdfcpu/sign/revocation_http.go` (OCSP and CRL).
// Both are off by default (`ValidateLinks` false, model/configuration.go:833) and nib must keep them so: a document
// that makes this machine fetch a URL it names is an egress channel (D4's security pin,
// P02.S09). No nib file may set ValidateLinks true or import pdfcpu's sign package.
func TestNibNeverTurnsOnPdfcpuNetworkCode(t *testing.T) {
	signImport := regexp.MustCompile(`"github\.com/pdfcpu/pdfcpu/pkg/pdfcpu/sign"`)
	seen := 0
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "third_party", "node_modules", ".git", ".claude", "dist", "web":
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
			if linkValidationOn(ln) {
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
