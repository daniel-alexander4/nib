package server

import (
	"os"
	"regexp"
	"testing"

	"nib/internal/pdfops"
)

// TestTheHelperPageLinksAreTheVendors — Settings → Helper Programs links each program to its site, written in the page
// (ADR-039: no response chooses where the page navigates). The page and `MissingTool.Vendor` are two spellings of one
// fact, so this holds them equal, and holds that the key the status sends (`toolKey`) names a section that exists
// (ADR-218).
func TestTheHelperPageLinksAreTheVendors(t *testing.T) {
	b, err := os.ReadFile("../../web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(b)
	for _, tool := range []pdfops.MissingTool{pdfops.ToolLibreOffice, pdfops.ToolGhostscript, pdfops.ToolHEIF, pdfops.ToolBrowser, pdfops.ToolPdftocairo} {
		re := regexp.MustCompile(`(?s)<div class="setrow helper" id="helper-` + regexp.QuoteMeta(tool.Key()) + `">.*?<a href="([^"]+)"`)
		m := re.FindStringSubmatch(html)
		if m == nil {
			t.Errorf("the Helper Programs page has no section for %s (helper-%s)", tool.Name(), tool.Key())
			continue
		}
		if m[1] != tool.Vendor() {
			t.Errorf("the page links %s to %q, and its refusals to %q", tool.Name(), m[1], tool.Vendor())
		}
	}
	if !regexp.MustCompile(`id="helper-scanner"`).MatchString(html) {
		t.Error("the page has no section for scanimage, which Scan… needs")
	}
}
