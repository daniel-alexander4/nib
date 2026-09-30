package pdfops

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
)

// flowLine is a line as a reader finds it: its text and where it starts.
type flowLine struct {
	text string
	x, y float64
}

func linesOf(l pageLayout, skip int) []flowLine {
	var out []flowLine
	for i, p := range l.paragraphs {
		if i == skip {
			continue
		}
		for _, ln := range p.lines {
			out = append(out, flowLine{ln.text, ln.x0, ln.y})
		}
	}
	return out
}

// grewAsItShould checks the page after a grown edit of paragraph pi: the edited paragraph reads as edit on its own
// baselines and extra more at its pitch, every line of the region reads exactly `extra × pitch` lower, and every other
// line where it was. It returns the first difference, or "".
func grewAsItShould(before, after pageLayout, pi int, edit string, page [4]float64) string {
	para := before.paragraphs[pi]
	pitch := paragraphPitch(before, pi)
	n := len(para.lines)
	var got []string
	for i := 0; ; i++ {
		y := para.lines[min(i, n-1)].y - float64(max(0, i-n+1))*pitch
		ln, ok := lineAtBaseline(after, paragraphBox(para), y, para.lines[0].size)
		if !ok || len(strings.Join(got, " ")) >= len(edit) {
			break
		}
		got = append(got, ln.text)
	}
	if normalizedText(strings.Join(got, " ")) != normalizedText(edit) {
		return fmt.Sprintf("the edited paragraph's baselines read %q", got)
	}
	extra := len(got) - n
	if extra <= 0 {
		return fmt.Sprintf("the paragraph did not grow: %d lines", len(got))
	}
	moved := map[int]bool{}
	for _, q := range regionOf(before, pi, page).paragraphs {
		moved[q] = true
	}
	dy := float64(extra) * pitch
	after3 := linesOf(after, -1)
	for i, q := range before.paragraphs {
		if i == pi {
			continue
		}
		for _, ln := range q.lines {
			want := ln.y
			if moved[i] {
				want -= dy
			}
			found := false
			for _, a := range after3 {
				if a.text == ln.text && math.Abs(a.x-ln.x0) < 1e-6 && math.Abs(a.y-want) < 1e-6 {
					found = true
					break
				}
			}
			if !found {
				return fmt.Sprintf("line %q (region: %v) is not at y %v", ln.text, moved[i], want)
			}
		}
	}
	return ""
}

func reflowRead(t *testing.T, pdf []byte, pi int, edit string) (before, after pageLayout, cause, below string, page [4]float64) {
	t.Helper()
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pg := pageAt(ctx, nil, 1)
	before, err = readPageGlyphLayout(ctx, pg)
	if err != nil {
		t.Fatal(err)
	}
	out, refusal, err := ReflowParagraph(pdf, 1, pi, before.paragraphs[pi].text(), edit)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil {
		return before, pageLayout{}, refusal.Cause, refusal.Below, visibleBoxOf(pg)
	}
	after, _ = layoutOf(t, out)
	return before, after, "", "", visibleBoxOf(pg)
}

// oneMoreLine is paragraph 0 of flowPage with a clause added — enough for exactly one more line.
const oneMoreLine = "Words run across the column here and wrap Words run across the column here and wrap Words run across the column here and wrap and the end"

// TestAGrownParagraphPushesTheOnesBelowIt — `PLAN-text-reflow.md` P07.S03 through the door the server calls: an edit that
// needs one more line takes it at the paragraph's own pitch, the paragraphs below it in its column read back exactly one
// pitch lower, and the footer past the wide gap stays where it was.
func TestAGrownParagraphPushesTheOnesBelowIt(t *testing.T) {
	before, after, cause, _, page := reflowRead(t, flowPage("BT /F1 12 Tf 300 100 Td (Page 1) Tj ET"), 0, oneMoreLine)
	if cause != "" {
		t.Fatalf("refused: %s", cause)
	}
	if d := grewAsItShould(before, after, 0, oneMoreLine, page); d != "" {
		t.Error(d)
	}
	if ln, ok := lineAtBaseline(after, [4]float64{0, 0, 612, 792}, 634, 12); !ok || !strings.HasPrefix(ln.text, "Words run") {
		t.Errorf("paragraph 1 is not one pitch lower: %q %v", ln.text, ok)
	}
}

// TestAGrowthItCannotDoNamesWhy — P07.S03's refusals, each from a fixture built to reach it, and a paragraph below that
// cannot move is NAMED.
func TestAGrowthItCannotDoNamesWhy(t *testing.T) {
	form := "BT /F1 12 Tf 14 TL 72 648 Td (Words run across the column here and wrap) Tj T* (Formed words here) Tj ET"
	flagged, err := SetFlags(flowPage(""), []byte(`[{"page":1,"frac":{"x":0.2,"y":0.2},"type":"sig"}]`))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name  string
		pdf   []byte
		edit  string
		cause string
		below string
	}{
		{"no room below", flowPage("BT /F1 12 Tf 72 558 Td (Page 1) Tj ET"), oneMoreLine, causePageFull, ""},
		{"a rule in the band", flowPage("q 1 w 72 660 m 400 660 l S Q"), oneMoreLine, causeAnchored, ""},
		{"a link over a word it re-wraps, even without growing",
			pageWith(flowPageContent, "", "", "/Annots [6 0 R]", helvetica, "<< /Type /Annot /Subtype /Link /Rect [100 695 160 712] /Border [0 0 0] >>"),
			"Words run across the column here and wrap Words run across the column here and wrap Words run across the column there and wrap", causeAnchored, ""},
		{"a note in the band", pageWith(flowPageContent, "", "", "/Annots [6 0 R]", helvetica, "<< /Type /Annot /Subtype /Text /Rect [100 640 120 655] >>"),
			oneMoreLine, causeAnchored, ""},
		{"a signing flag in the band", flagged, oneMoreLine, causeAnchored, ""},
		{"a bookmark to a paragraph below",
			pageWith(flowPageContent, "", "/Outlines 6 0 R", "", helvetica, "<< /Type /Outlines /First 7 0 R /Last 7 0 R /Count 1 >>",
				"<< /Title (Second) /Parent 6 0 R /Dest [3 0 R /XYZ 72 660 0] >>"),
			oneMoreLine, causeAnchored, ""},
		{"a one-line paragraph with nothing to take a pitch from", helveticaPage("BT /F1 12 Tf 72 700 Td (A short line of words) Tj ET"),
			"A short line of words " + strings.Repeat("that goes on and on ", 6), causeNoPitch, ""},
		{"a paragraph below drawn inside a form",
			pageWith("BT /F1 12 Tf 14 TL 72 700 Td (Words run across the column here and wrap) Tj T* (Words run across the column here and wrap) Tj T* (Words run across the column here and wrap) Tj ET q /Fm1 Do Q",
				"/XObject << /Fm1 6 0 R >>", "", "", helvetica,
				fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(form), form)),
			oneMoreLine, causeTextInForm, "Words run across the column here and wrap Formed words here"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, _, cause, below, _ := reflowRead(t, c.pdf, 0, c.edit)
			if cause != c.cause || below != c.below {
				t.Errorf("cause %q below %q, want %q %q", cause, below, c.cause, c.below)
			}
		})
	}
}

// TestHowParagraphsGrowOverTheCorpora — P07.S03 over the generated and real-producer corpora: every paragraph P06 can
// reflow is given its own last line again, and each growth the door accepts reads back as it should — the edit on its
// baselines, the region exactly lower, everything else unmoved. The census of what refused says what S04 must unlock.
//
// **The stimulus is asserted**: growths are accepted, and among them ones that moved a region.
func TestHowParagraphsGrowOverTheCorpora(t *testing.T) {
	corpora := []lawOneCorpus{
		{name: "generated", docs: append(runCorpus(t), runCorpusDoc{"a column of paragraphs", flowPage("")})},
		externalPDFs(t, "real producers", "NIB_UA_PRODUCERS", filepath.Join(homeDir(), "nib", "producers")),
	}
	grown, withRegion := 0, 0
	refused := map[string]int{}
	for _, corp := range corpora {
		if corp.absent != "" {
			t.Logf("NOTE (a narrower population, not a pass over it): %s: %s", corp.name, corp.absent)
			continue
		}
		for _, doc := range corp.docs {
			ctx, err := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
			if err != nil || ctx.PageCount < 1 {
				continue
			}
			pg := pageAt(ctx, nil, 1)
			l, err := readPageGlyphLayout(ctx, pg)
			if err != nil {
				continue
			}
			for pi, para := range l.paragraphs {
				if paragraphRefusal(ctx, pg, para) != "" {
					continue
				}
				edit := para.text() + " " + para.lines[len(para.lines)-1].text
				out, refusal, err := ReflowParagraph(doc.pdf, 1, pi, para.text(), edit)
				if err != nil {
					t.Fatalf("%s ¶%d: %v", doc.name, pi, err)
				}
				if out == nil {
					refused[refusal.Cause]++
					continue
				}
				after, _ := layoutOf(t, out)
				r := regionOf(l, pi, visibleBoxOf(pg))
				if len(after.paragraphs) == 0 {
					t.Errorf("%s ¶%d: the page lost its text", doc.name, pi)
					continue
				}
				if d := grewAsItShould(l, after, pi, edit, visibleBoxOf(pg)); d != "" {
					if strings.HasPrefix(d, "the paragraph did not grow") {
						continue // fitted without a new line: P06's path, asserted elsewhere
					}
					t.Errorf("%s ¶%d %q: %s", doc.name, pi, para.text(), d)
					continue
				}
				grown++
				if len(r.paragraphs) > 0 {
					withRegion++
				}
			}
		}
	}
	var rs []string
	for k, n := range refused {
		rs = append(rs, fmt.Sprintf("%s %d", k, n))
	}
	sort.Strings(rs)
	t.Logf("grew %d paragraphs, %d pushing a region; refused: %s", grown, withRegion, strings.Join(rs, ", "))
	if grown == 0 || withRegion == 0 {
		t.Errorf("grew %d, with a region %d — the growth was never asked to push anything", grown, withRegion)
	}
}

// TestTheParagraphListSaysAnchoredBeforeTheUserTypes — P07.S03: a paragraph with a link over its words is listed with the
// refusal, so the dialog says why before anything is typed, not after.
func TestTheParagraphListSaysAnchoredBeforeTheUserTypes(t *testing.T) {
	pdf := pageWith(flowPageContent, "", "", "/Annots [6 0 R]", helvetica, "<< /Type /Annot /Subtype /Link /Rect [100 695 160 712] /Border [0 0 0] >>")
	ps, err := Paragraphs(pdf, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) < 2 || ps[0].Refusal != causeAnchored || ps[1].Refusal != "" {
		t.Errorf("listed %+v, want the first anchored and the second free", ps)
	}
}

// TestAOneLineParagraphIsSetToItsColumn — P07.S03: a one-line paragraph carries no record of its measure, so it is set in
// its column's. Longer text that still fits the column stays ONE line where it was; wrapped at its own old width it would
// grow a short line the reader takes for a paragraph's end.
func TestAOneLineParagraphIsSetToItsColumn(t *testing.T) {
	pdf := pageWith(flowPageContent+"BT /F1 12 Tf 72 560 Td (Short words.) Tj ET", "", "", "", helvetica)
	before, after, cause, _, _ := reflowRead(t, pdf, 3, "Short words and a few more of them.")
	if cause != "" {
		t.Fatalf("refused: %s", cause)
	}
	if before.paragraphs[3].text() != "Short words." {
		t.Fatalf("setup: paragraph 3 reads %q", before.paragraphs[3].text())
	}
	if ln, ok := lineAtBaseline(after, [4]float64{0, 0, 612, 792}, 560, 12); !ok || ln.text != "Short words and a few more of them." {
		t.Errorf("the one-liner reads %q on its baseline (%v), want the whole edit on one line", ln.text, ok)
	}
}

// TestAOneLinerStopsShortOfWhatIsBesideIt — P07.S03: widened to its column, a one-line paragraph still never runs into
// something drawn to its right on its line — a form label's field, the next label. Found on the real-producer corpus
// (irs-f1040's "Apt. no." set straight into "Check here if your main home").
func TestAOneLinerStopsShortOfWhatIsBesideIt(t *testing.T) {
	pdf := pageWith(flowPageContent+"BT /F1 12 Tf 72 560 Td (Apt. no.) Tj ET BT /F1 12 Tf 200 560 Td (Check here) Tj ET", "", "", "", helvetica)
	before, _ := layoutOf(t, pdf)
	pi := -1
	for i, p := range before.paragraphs {
		if p.text() == "Apt. no." {
			pi = i
		}
	}
	if pi < 0 {
		t.Fatal("setup: the label is not a paragraph of its own")
	}
	_, after, cause, _, _ := reflowRead(t, pdf, pi, "Apartment number here")
	if cause != "" {
		t.Fatalf("refused: %s", cause)
	}
	for _, p := range after.paragraphs {
		for _, ln := range p.lines {
			if math.Abs(ln.y-560) < 3 && strings.Contains(ln.text, "Check") && ln.text != "Check here" {
				t.Errorf("the label ran into its neighbour: %q", ln.text)
			}
		}
	}
}
