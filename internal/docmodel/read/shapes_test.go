package read

import (
	"testing"

	"nib/internal/docmodel"
	"nib/internal/pdfops"
)

// A Figure inside a paragraph or a caption is a picture of the page: the walk emits such an element whole and used to
// stop there (/pending 941 #11).
func TestAFigureInsideAParagraphOrCaptionIsReadFromTheTree(t *testing.T) {
	tree := pdfops.StructureTree{Tagged: true, Elements: []pdfops.StructureElement{
		{Parent: -1, Kind: "P", Standard: "P", Page: 1, Text: "Look", Kids: []int{1}},
		{Parent: 0, Kind: "Figure", Standard: "Figure", Page: 1, Alt: "a cat", Place: [4]float64{0.1, 0.1, 0.4, 0.4}},
		{Parent: -1, Kind: "Caption", Standard: "Caption", Page: 1, Text: "Fig 1", Kids: []int{3}},
		{Parent: 2, Kind: "Figure", Standard: "Figure", Page: 1, Alt: "a dog", Place: [4]float64{0.5, 0.5, 0.9, 0.9}},
		{Parent: -1, Kind: "H1", Standard: "H1", Page: 1, Text: "Title", Kids: []int{5}},
		{Parent: 4, Kind: "Figure", Standard: "Figure", Page: 1, Alt: "a bird", Place: [4]float64{0.2, 0.2, 0.3, 0.3}},
	}}
	got, boxes := treeBlocks(tree)
	var alts []string
	for _, b := range got[1] {
		if b.Kind == docmodel.Figure {
			alts = append(alts, b.Image.Alt)
			if boxes[b.Image][2] == 0 {
				t.Errorf("figure %q has no box to join the page's picture by", b.Image.Alt)
			}
		}
	}
	if len(alts) != 3 || alts[0] != "a cat" || alts[1] != "a dog" || alts[2] != "a bird" {
		t.Fatalf("figures from the tree = %v, want a cat, a dog and a bird (blocks %+v)", alts, got[1])
	}
}

// A picture the page draws and the tree never names is kept, after the blocks the page drew before it.
func TestAPictureTheTreeLeavesOutIsKept(t *testing.T) {
	ep := pdfops.ExportPage{Page: 1, Figures: []pdfops.ExportFigure{
		{Rect: pdfops.MapRect{0.1, 0.1, 0.4, 0.4}, Before: 1, Width: 3, Height: 3, Data: []byte("x")},
	}}
	blocks := []docmodel.Block{
		{Kind: docmodel.Paragraph, Spans: []docmodel.Span{{Text: "one"}}},
		{Kind: docmodel.Paragraph, Spans: []docmodel.Span{{Text: "two"}}},
	}
	got := withFigures(blocks, ep, nil)
	if len(got) != 3 || got[1].Kind != docmodel.Figure || got[1].Image.Width != 3 {
		t.Fatalf("blocks = %+v, want the picture between the two paragraphs", got)
	}
	// And a tree picture that names it is joined, not doubled.
	im := &docmodel.Image{Alt: "named"}
	blocks = []docmodel.Block{{Kind: docmodel.Figure, Image: im}}
	got = withFigures(blocks, ep, map[*docmodel.Image][4]float64{im: {0.1, 0.1, 0.4, 0.4}})
	if len(got) != 1 || got[0].Image.Alt != "named" || got[0].Image.Width != 3 {
		t.Fatalf("a named picture is joined once: %+v", got)
	}
	// A tree picture with no box takes the page's picture; it is not left unread beside a second copy of it.
	im = &docmodel.Image{Alt: "boxless"}
	blocks = []docmodel.Block{{Kind: docmodel.Figure, Image: im}}
	got = withFigures(blocks, ep, map[*docmodel.Image][4]float64{im: {}})
	if len(got) != 1 || got[0].Image.Alt != "boxless" || got[0].Image.Width != 3 || got[0].Image.Placeholder != "" {
		t.Fatalf("a boxless tree picture is joined to the page's: %+v", got)
	}
}

// A page that draws text is not a scan because that text was covered or sat off the page (/pending 941 #12).
func TestAPageThatDrawsTextIsNotAScan(t *testing.T) {
	fig := pdfops.ExportFigure{Rect: pdfops.MapRect{0, 0, 1, 1}, Width: 5, Height: 5, Data: []byte("x")}
	for _, c := range []struct {
		noText bool
		scan   bool
	}{{true, true}, {false, false}} {
		var rep docmodel.Report
		ep := pdfops.ExportPage{Page: 2, NoText: c.noText, Figures: []pdfops.ExportFigure{fig}}
		got := scanned(nil, ep, &rep)
		if (len(rep.ScannedPages) == 1) != c.scan || (len(got) == 1) != c.scan {
			t.Errorf("NoText=%v: scanned pages %v, blocks %d, want scan=%v", c.noText, rep.ScannedPages, len(got), c.scan)
		}
	}
}

// A picture has no direction: it must not vote ltr against a right-to-left document (/pending 941 #13).
func TestPicturesDoNotVoteOnTheDirection(t *testing.T) {
	blocks := []docmodel.Block{
		{Kind: docmodel.Paragraph, Dir: "rtl", Spans: []docmodel.Span{{Text: "שלום"}}},
		{Kind: docmodel.Figure, Dir: "ltr", Image: &docmodel.Image{}},
		{Kind: docmodel.Figure, Dir: "ltr", Image: &docmodel.Image{}},
	}
	if d := majorityDir(blocks); d != "rtl" {
		t.Fatalf("majorityDir = %q, want rtl: one right-to-left paragraph and two pictures", d)
	}
}
