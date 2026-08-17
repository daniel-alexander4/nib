package pdfops

import "testing"

// TestOutlineRoundTrip proves a nested outline written by SetOutline reads back
// identically (title, page, and nesting level) through Outline.
func TestOutlineRoundTrip(t *testing.T) {
	base := sizedPDF(t, 5, 120, 160)
	items := []OutlineItem{
		{Title: "Chapter 1", Page: 1, Level: 0},
		{Title: "Section 1.1", Page: 2, Level: 1},
		{Title: "Section 1.2", Page: 3, Level: 1},
		{Title: "Chapter 2", Page: 4, Level: 0},
	}
	out, err := SetOutline(base, items)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Outline(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(items) {
		t.Fatalf("read back %d items, want %d: %+v", len(got), len(items), got)
	}
	for i, want := range items {
		if got[i] != want {
			t.Errorf("item %d = %+v, want %+v", i, got[i], want)
		}
	}
}

// TestSetOutlineClears proves an empty list removes the outline.
func TestSetOutlineClears(t *testing.T) {
	base := sizedPDF(t, 3, 120, 160)
	withBm, err := SetOutline(base, []OutlineItem{{Title: "A", Page: 1, Level: 0}})
	if err != nil {
		t.Fatal(err)
	}
	cleared, err := SetOutline(withBm, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Outline(cleared)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("after clear, outline has %d items, want 0", len(got))
	}
}

// TestSetOutlineRejectsBadInput proves the validation: duplicate/empty titles,
// out-of-range pages, and over-deep nesting are all rejected.
func TestSetOutlineRejectsBadInput(t *testing.T) {
	base := sizedPDF(t, 3, 120, 160)
	cases := map[string][]OutlineItem{
		"duplicate title":   {{Title: "A", Page: 1, Level: 0}, {Title: "A", Page: 2, Level: 0}},
		"empty title":       {{Title: "  ", Page: 1, Level: 0}},
		"page out of range": {{Title: "A", Page: 9, Level: 0}},
		"over-deep nesting": {{Title: "A", Page: 1, Level: 0}, {Title: "B", Page: 2, Level: 2}},
	}
	for name, items := range cases {
		if _, err := SetOutline(base, items); err == nil {
			t.Errorf("%s: expected an error, got nil", name)
		}
	}
}
