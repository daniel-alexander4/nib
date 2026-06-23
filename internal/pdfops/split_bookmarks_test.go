package pdfops

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
)

// bookmarkedPDF builds a `pages`-page PDF with the given top-level bookmarks.
func bookmarkedPDF(t *testing.T, pages int, bms []pdfcpu.Bookmark) []byte {
	t.Helper()
	base := sizedPDF(t, pages, 120, 160)
	var out bytes.Buffer
	if err := api.AddBookmarks(bytes.NewReader(base), &out, bms, true, nil); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// TestSplitByBookmarks pins the core: one part per top-level bookmark, correct
// multi-page ranges (each runs to the page before the next), duplicate titles
// deduped, and a path-hostile title reduced to a safe base name.
func TestSplitByBookmarks(t *testing.T) {
	pdf := bookmarkedPDF(t, 7, []pdfcpu.Bookmark{
		{Title: "Violin I", PageFrom: 1},
		{Title: "Violin I", PageFrom: 3}, // duplicate title → deduped
		{Title: "../evil", PageFrom: 5},  // path-escape attempt → sanitized
		{Title: "Cello", PageFrom: 6},    // last → runs to the end (6-7)
	})
	parts, err := SplitByBookmarks(pdf, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 4 {
		t.Fatalf("got %d parts, want 4", len(parts))
	}
	wantName := []string{"Violin I", "Violin I (2)", "evil", "Cello"}
	wantPages := []int{2, 2, 1, 2}
	for i, p := range parts {
		if p.Name != wantName[i] {
			t.Errorf("part %d name = %q, want %q", i, p.Name, wantName[i])
		}
		if strings.ContainsAny(p.Name, `/\`) {
			t.Errorf("part %d name %q contains a path separator", i, p.Name)
		}
		if n, _ := PageCount(p.Data); n != wantPages[i] {
			t.Errorf("part %d (%s) = %d pages, want %d", i, p.Name, n, wantPages[i])
		}
	}
}

// TestSplitByBookmarksPrefix confirms the prefix is prepended before sanitizing.
func TestSplitByBookmarksPrefix(t *testing.T) {
	pdf := bookmarkedPDF(t, 2, []pdfcpu.Bookmark{{Title: "Flute", PageFrom: 1}, {Title: "Oboe", PageFrom: 2}})
	parts, err := SplitByBookmarks(pdf, "Sonata - ")
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 2 || parts[0].Name != "Sonata - Flute" || parts[1].Name != "Sonata - Oboe" {
		t.Fatalf("prefix not applied: %q / %q", parts[0].Name, parts[1].Name)
	}
}

// TestSplitByBookmarksNoOutline: a PDF with no bookmarks yields no parts (no error).
func TestSplitByBookmarksNoOutline(t *testing.T) {
	parts, err := SplitByBookmarks(sizedPDF(t, 3, 120, 160), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 0 {
		t.Errorf("plain PDF: got %d parts, want 0", len(parts))
	}
}

func TestSanitizeFilename(t *testing.T) {
	clean := map[string]string{
		"Violin I":    "Violin I",
		"Violin I/II": "Violin I II",
		".hidden":     "hidden",
		"trailing...": "trailing",
		"CON":         "_CON",
		"":            "",
		"   ":         "",
	}
	for in, want := range clean {
		if got := SanitizeFilename(in); got != want {
			t.Errorf("SanitizeFilename(%q) = %q, want %q", in, got, want)
		}
	}
	// Adversarial inputs must always reduce to a safe base name (no separators,
	// no NUL, never "." or ".." — those would escape the chosen folder).
	for _, in := range []string{"../../etc/passwd", `..\..\win`, "/abs/path", "..", ".", "a/b\\c", "x\x00y"} {
		got := SanitizeFilename(in)
		if strings.ContainsAny(got, "/\\\x00") || got == "." || got == ".." {
			t.Errorf("SanitizeFilename(%q) = %q is not a safe base name", in, got)
		}
	}
}
