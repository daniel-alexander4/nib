package read

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"nib/internal/docmodel"
	"nib/internal/testpdf"
)

func fixture(t *testing.T, name string) docmodel.Document {
	t.Helper()
	for _, f := range testpdf.ExportModelSet() {
		if f.Name == name {
			d, err := Read(context.Background(), f.PDF, Options{Title: "t"})
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			return d
		}
	}
	t.Fatalf("no fixture %q", name)
	return docmodel.Document{}
}

func text(d docmodel.Document) string {
	var s []string
	for _, b := range d.Blocks {
		s = append(s, b.Text())
	}
	return strings.Join(s, "|")
}

func TestVisibilityOutcomes(t *testing.T) {
	cases := []struct {
		name      string
		has, not  string
		check     func(docmodel.Report) bool
		checkName string
	}{
		{"vis-alpha", "text under a box", "", func(r docmodel.Report) bool { return r.MaybeCovered == 1 && r.CoveredText == 0 }, "maybeCovered=1"},
		{"vis-clip", "text under a box", "", func(r docmodel.Report) bool { return r.MaybeCovered == 1 && r.CoveredText == 0 }, "maybeCovered=1"},
		{"vis-shaded-header", "text under a box", "", func(r docmodel.Report) bool { return r.CoveredText == 0 && r.MaybeCovered == 0 }, "nothing covered"},
		{"vis-image-over", "kept heading words", "text under a box", func(r docmodel.Report) bool { return r.CoveredText == 1 }, "covered=1"},
		{"vis-softmask-over", "text under a box", "", func(r docmodel.Report) bool { return r.CoveredText == 0 }, "covered=0"},
		{"vis-layer-off", "kept heading words", "text under a box", func(r docmodel.Report) bool { return r.HiddenLayer == 1 }, "hiddenLayer=1"},
		{"vis-offpage", "kept heading words", "far off", func(r docmodel.Report) bool { return r.OffPage == 1 }, "offPage=1"},
		{"vis-scan-header", "scanned body words", "", func(r docmodel.Report) bool { return r.HiddenText == 0 }, "hidden OCR kept"},
		{"vis-hidden-over-visible", "kept heading words", "", func(r docmodel.Report) bool { return r.HiddenText == 1 && strings.Count(r.Source, "x") == 0 }, "hiddenText=1"},
	}
	for _, c := range cases {
		d := fixture(t, c.name)
		got := text(d)
		if !strings.Contains(got, c.has) {
			t.Errorf("%s: %q missing from %q", c.name, c.has, got)
		}
		if c.not != "" && strings.Contains(got, c.not) {
			t.Errorf("%s: %q leaked into %q", c.name, c.not, got)
		}
		if !c.check(d.Report) {
			t.Errorf("%s: report %+v does not satisfy %s", c.name, d.Report, c.checkName)
		}
	}
	// A word hidden twice over must be exported once.
	if got := text(fixture(t, "vis-hidden-over-visible")); strings.Count(got, "kept heading words") != 1 {
		t.Errorf("hidden duplicate exported: %q", got)
	}
}

func TestTreeAgainstProposal(t *testing.T) {
	d := fixture(t, "tagged-complete")
	if d.Report.Source != docmodel.SourceTree || len(d.Report.FlaggedPages) != 0 {
		t.Fatalf("complete tree not used: %+v", d.Report)
	}
	if len(d.Blocks) != 2 || d.Blocks[0].Kind != docmodel.Heading || d.Blocks[0].Level != 1 || d.Blocks[1].Kind != docmodel.Paragraph {
		t.Fatalf("blocks: %+v", d.Blocks)
	}
	if !d.Blocks[0].Spans[0].Bold {
		t.Errorf("the tree heading did not borrow the page's bold style: %+v", d.Blocks[0].Spans)
	}
	if d.Meta.Lang != "en-US" || d.Meta.Title != "t" {
		t.Errorf("meta %+v", d.Meta)
	}

	d = fixture(t, "tagged-untagged-paragraph")
	if d.Report.Source != docmodel.SourceProposal || !flagged(d, 1, docmodel.FlagTreeIncomplete) {
		t.Errorf("incomplete tree not named: %+v", d.Report)
	}
	if !strings.Contains(text(d), "Untagged extra words") {
		t.Errorf("a visible paragraph was lost: %q", text(d))
	}

	d = fixture(t, "tagged-covered")
	if !flagged(d, 1, docmodel.FlagTreeIncomplete) || strings.Contains(text(d), "Body paragraph") {
		t.Errorf("text under a box travelled through the tree: %q %+v", text(d), d.Report)
	}
	if d.Report.CoveredText == 0 {
		t.Errorf("covered text not counted: %+v", d.Report)
	}
}

func TestScannedPageIsItsOwnPicture(t *testing.T) {
	for _, f := range testpdf.ExportSet() {
		if f.Name != "scanned" {
			continue
		}
		d, err := Read(context.Background(), f.PDF, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Blocks) != 1 || d.Blocks[0].Kind != docmodel.Figure || d.Blocks[0].Image.Alt != "Scanned page 1" || len(d.Report.ScannedPages) != 1 {
			t.Fatalf("scan: %+v", d)
		}
		return
	}
	t.Fatal("no scanned fixture")
}

func TestUntaggedUsesProposalAndRTL(t *testing.T) {
	for _, f := range testpdf.ExportSet() {
		if f.Name != "right-to-left" {
			continue
		}
		d, err := Read(context.Background(), f.PDF, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if d.Report.Source != docmodel.SourceProposal || d.Meta.Lang != "und" || d.Meta.Dir != "rtl" {
			t.Fatalf("rtl: %+v %+v", d.Meta, d.Report)
		}
	}
}

func flagged(d docmodel.Document, page int, cause string) bool {
	for _, f := range d.Report.FlaggedPages {
		if f.Page == page && f.Cause == cause {
			return true
		}
	}
	return false
}

func blk(text string) docmodel.Block {
	return docmodel.Block{Kind: docmodel.Paragraph, Spans: []docmodel.Span{{Text: text}}}
}

// The word test is equality both ways: a word only the page shows, a word only the tree holds, and a repeated word.
func TestSameWordsIsEqualityBothWays(t *testing.T) {
	for name, c := range map[string]struct {
		a, b string
		want bool
	}{
		"equal":            {"a b", "b a", true},
		"page has extra":   {"a b", "a b c", false},
		"tree has extra":   {"a b c", "a b", false},
		"repeat count":     {"a a b", "a b b", false},
		"same words twice": {"a a", "a a", true},
	} {
		if got := sameWords([]docmodel.Block{blk(c.a)}, []docmodel.Block{blk(c.b)}); got != c.want {
			t.Errorf("%s: sameWords(%q, %q) = %v", name, c.a, c.b, got)
		}
	}
}

// A tagged document some of whose pages the tree does not account for is "mixed".
func TestATaggedDocumentWithAnUntaggedPageIsMixed(t *testing.T) {
	font := "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"
	one := "/P <</MCID 0>> BDC\nBT /F1 12 Tf 72 700 Td (Tagged page words) Tj ET\nEMC\n"
	two := "BT /F1 12 Tf 72 700 Td (Second page has no tags) Tj ET\n"
	page := func(parent, content string, sp bool) string {
		x := ""
		if sp {
			x = "/StructParents 0 "
		}
		return fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] %s/Resources << /Font << /F1 5 0 R >> >> /Contents %s 0 R >>", x, parent)
	}
	stream := func(b string) string { return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(b), b) }
	pdf := testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R 6 0 R] /Count 2 >>",
		3: page("4", one, true), 4: stream(one), 5: font,
		6: page("8", two, false), 8: stream(two),
		7:  "<< /Type /StructTreeRoot /K [10 0 R] /ParentTree 9 0 R /ParentTreeNextKey 1 >>",
		9:  "<< /Nums [0 [10 0 R]] >>",
		10: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 0 >>",
	})
	d, err := Read(context.Background(), pdf, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if d.Report.Source != docmodel.SourceMixed || !flagged(d, 2, docmodel.FlagTreeIncomplete) {
		t.Fatalf("report %+v", d.Report)
	}
}

// ctxTrip is a context whose Err() says Canceled from its nth call on, and counts the calls.
type ctxTrip struct {
	context.Context
	n     int64
	calls atomic.Int64
}

func (c *ctxTrip) Err() error {
	if c.calls.Add(1) >= c.n && c.n > 0 {
		return context.Canceled
	}
	return nil
}

// TestACancelledReadStopsAtEveryPlaceItLooks — /pending 941 #1. Read observes the context at the start, between pages of
// the page read, after each picture is decoded and between pages of the model; a cancel at ANY of them is the
// context's error and no document, and a reader that looks once at entry has too few places to look.
func TestACancelledReadStopsAtEveryPlaceItLooks(t *testing.T) {
	pdf := testpdf.ExportPictureTwoPages()
	probe := &ctxTrip{Context: context.Background()}
	if _, err := Read(probe, pdf, Options{}); err != nil {
		t.Fatalf("a read with a live context: %v", err)
	}
	total := probe.calls.Load()
	const pages, pictures = 2, 2 // ExportPictureTwoPages draws one picture on each page
	// The places, counted: ExportRead's entry (1), its page loop (2), each picture decoded (4 apiece: the pass's own look
	// before it, and the three pdfcpu's renderer takes now that it is handed this context and not Background — /pending
	// 946 I10), the pass's end (1), Read's entry after it (1) and its page loop (2). A place removed is a place a cancel is
	// not seen at.
	if want := int64(1 + pages + 4*pictures + 1 + 1 + pages); total != want {
		t.Fatalf("Read looked at the context %d times for a %d-page document with %d pictures, want %d", total, pages, pictures, want)
	}
	for n := int64(1); n <= total; n++ {
		d, err := Read(&ctxTrip{Context: context.Background(), n: n}, pdf, Options{})
		if err != context.Canceled || len(d.Blocks) != 0 {
			t.Fatalf("a cancel at observation %d of %d: %d blocks, err %v — want context.Canceled and no document", n, total, len(d.Blocks), err)
		}
	}
}

// TestTheReaderCountsTheTextDirectionControlsAWriterWillDrop (/pending 947): the page holds three, the report says
// three, and the count is the model's own (BidiControlsIn), so it is what Clean removes at the writers.
func TestTheReaderCountsTheTextDirectionControlsAWriterWillDrop(t *testing.T) {
	d, err := Read(context.Background(), testpdf.ExportBidi().PDF, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if d.Report.BidiControls != 3 || docmodel.BidiControlsIn(d) != 3 {
		t.Fatalf("Report.BidiControls = %d, model holds %d, want 3; text %q", d.Report.BidiControls, docmodel.BidiControlsIn(d), text(d))
	}
	if got := docmodel.Clean(text(d)); strings.ContainsAny(got, "‮⁦") || docmodel.BidiControls(got) != 0 {
		t.Errorf("Clean left a control in %q", got)
	}
}
