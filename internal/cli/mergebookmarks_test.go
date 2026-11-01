package cli

import (
	"os"
	"path/filepath"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// TestMergeBookmarkPerFile — RelaxTax B6. One top-level bookmark per input at its first page, an input's own
// outline nested under it, a repeated name made unique; without the flag the outline is exactly what it was.
func TestMergeBookmarkPerFile(t *testing.T) {
	dir := t.TempDir()
	three, _ := testpdf.Text("a1", "a2", "a3")
	three, err := pdfops.SetOutline(three, []pdfops.OutlineItem{{Title: "Intro", Page: 1}, {Title: "Body", Page: 3}})
	if err != nil {
		t.Fatal(err)
	}
	two, _ := testpdf.Text("b1", "b2")
	two, err = pdfops.SetOutline(two, []pdfops.OutlineItem{{Title: "Part", Page: 2}})
	if err != nil {
		t.Fatal(err)
	}
	a, b, b2 := filepath.Join(dir, "alpha.pdf"), filepath.Join(dir, "beta.pdf"), filepath.Join(dir, "sub")
	os.MkdirAll(b2, 0o755)
	b2 = filepath.Join(b2, "beta.pdf")
	os.WriteFile(a, three, 0o600)
	os.WriteFile(b, two, 0o600)
	os.WriteFile(b2, two, 0o600)
	out := filepath.Join(dir, "all.pdf")

	if _, code := captureStdout(t, func() int { _, c := Run([]string{"merge", a, b, b2, "-o", out, "--bookmark-per-file"}, "t"); return c }); code != 0 {
		t.Fatalf("merge exited %d", code)
	}
	merged, _ := os.ReadFile(out)
	got, err := pdfops.Outline(merged)
	if err != nil {
		t.Fatal(err)
	}
	want := []pdfops.OutlineItem{
		{Title: "alpha", Page: 1, Level: 0}, {Title: "Intro", Page: 1, Level: 1}, {Title: "Body", Page: 3, Level: 1},
		{Title: "beta", Page: 4, Level: 0}, {Title: "Part", Page: 5, Level: 1},
		{Title: "beta (2)", Page: 6, Level: 0}, {Title: "Part (2)", Page: 7, Level: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("outline %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("item %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	plain := filepath.Join(dir, "plain.pdf")
	if _, code := captureStdout(t, func() int { _, c := Run([]string{"merge", b, b2, "-o", plain}, "t"); return c }); code != 0 {
		t.Fatalf("plain merge exited %d", code)
	}
	pm, _ := os.ReadFile(plain)
	// Whatever outline a plain merge carries from its inputs, it has no per-file bookmarks.
	for _, it := range func() []pdfops.OutlineItem { x, _ := pdfops.Outline(pm); return x }() {
		if it.Title == "beta" || it.Title == "beta (2)" {
			t.Errorf("a merge without the flag has the per-file bookmark %+v", it)
		}
	}
}
