package p2p

import (
	"os"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

func TestRenderReadmeOnePage(t *testing.T) {
	pdf, err := RenderReadme()
	if err != nil {
		t.Fatal(err)
	}
	n, err := pdfops.PageCount(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("readme page count = %d, want 1", n)
	}
}

func TestAppendReadmeAddsTrailingPage(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	before, err := pdfops.PageCount(base)
	if err != nil {
		t.Fatal(err)
	}
	out, err := AppendReadme(base)
	if err != nil {
		t.Fatal(err)
	}
	after, err := pdfops.PageCount(out)
	if err != nil {
		t.Fatal(err)
	}
	if after != before+1 {
		t.Errorf("page count %d -> %d, want +1", before, after)
	}
}

// The rendered readme body must state every trust claim.
func TestReadmeContainsTrustClaims(t *testing.T) {
	body := readmeBody()
	for _, c := range trustClaims {
		if !strings.Contains(body, c) {
			t.Errorf("readme missing trust claim %q", c)
		}
	}
}

// Drift-guard: the in-app About dialog must make the same honest-trust claims as
// the appended readme. trustClaims is the single source; this pins the About
// copy to it so the two explanations cannot silently diverge.
func TestAboutCopyContainsTrustClaims(t *testing.T) {
	html, err := os.ReadFile("../../web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	s := string(html)
	for _, c := range trustClaims {
		if !strings.Contains(s, c) {
			t.Errorf("About dialog (web/index.html) missing trust claim %q — readme and About have drifted", c)
		}
	}
}
