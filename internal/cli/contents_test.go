package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// `nib contents` — /pending 929. It lists the bookmarks, says how many entries and pages, grows the document by the
// pages inserted, and refuses a document with nothing to list.
func TestContentsInsertsThePageAndSaysSo(t *testing.T) {
	dir := t.TempDir()
	plain, err := testpdf.Text("a", "b", "c")
	if err != nil {
		t.Fatal(err)
	}
	marked, err := pdfops.SetOutline(plain, []pdfops.OutlineItem{{Title: "First", Page: 1}, {Title: "Last", Page: 3}})
	if err != nil {
		t.Fatal(err)
	}
	in, bare := filepath.Join(dir, "in.pdf"), filepath.Join(dir, "bare.pdf")
	for p, b := range map[string][]byte{in: marked, bare: plain} {
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(dir, "out.pdf")
	var code int
	said := captureStderr(t, func() { code = cmdContents([]string{in, "-o", out, "--depth", "1", "--title", "Inside"}) })
	if code != 0 {
		t.Fatalf("contents exit = %d: %s", code, said)
	}
	if !strings.Contains(said, "2 entries from the bookmarks on 1 page(s)") {
		t.Errorf("stderr %q does not say what was made", said)
	}
	if n, _ := pdfops.PageCount(readPDF(t, out)); n != 4 {
		t.Errorf("the output has %d pages, want 4", n)
	}
	if c := cmdContents([]string{bare, "-o", filepath.Join(dir, "x.pdf")}); c == 0 {
		t.Error("a document with nothing to list was given a contents page")
	}
	if _, err := os.Stat(filepath.Join(dir, "x.pdf")); err == nil {
		t.Error("a refused contents page left an output file")
	}
	if c := cmdContents([]string{in, "-o", filepath.Join(dir, "y.pdf"), "--after", "99"}); c == 0 {
		t.Error("--after 99 on a three-page document was accepted")
	}
}
