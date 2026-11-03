package pdfops

import (
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// PLAN-import-export P02.S03a — each fixture of the export set is read by the page map as the property it exists to
// hold, so the set cannot drift from what the oracle will assume of it.
func TestTheExportSetHoldsTheProperties(t *testing.T) {
	for _, f := range testpdf.ExportSet() {
		m, err := MapPage(f.PDF, 1)
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		var all []string
		for _, r := range m.Text {
			all = append(all, r.Text)
		}
		joined := strings.Join(all, "|")
		switch f.Name {
		case "two-column":
			if len(m.Text) != 12 || m.Text[0].Rect[0] >= 0.3 || m.Text[1].Rect[0] <= 0.5 {
				t.Errorf("two-column: %d runs, first two at %v and %v; want 12 runs alternating left and right", len(m.Text), m.Text[0].Rect, m.Text[1].Rect)
			}
		case "right-to-left":
			if !strings.ContainsRune(joined, 'א') || !strings.ContainsRune(joined, 'ג') {
				t.Errorf("right-to-left: text = %q, want Hebrew letters", joined)
			}
		case "scanned":
			if !m.NoText {
				t.Error("scanned: the page sets text")
			}
		case "covered":
			if !strings.Contains(joined, "secret under the box") {
				t.Errorf("covered: the map lost the covered text: %q", joined)
			}
			var box bool
			for _, s := range m.Shapes {
				box = box || (s.Kind == "box" && s.Filled)
			}
			if !box {
				t.Errorf("covered: no filled box over the text: %+v", m.Shapes)
			}
		case "styled-linked":
			var bold, plain bool
			for _, r := range m.Text {
				if r.Text == "A bold title" {
					bold = r.Bold
				}
				if strings.HasPrefix(r.Text, "plain") {
					plain = !r.Bold
				}
			}
			l, _ := PageLinks(f.PDF, 1)
			if !bold || !plain || len(l.Links) != 1 {
				t.Errorf("styled-linked: bold title %v, plain %v, links %d", bold, plain, len(l.Links))
			}
		default:
			t.Errorf("fixture %q has no check", f.Name)
		}
	}
}
