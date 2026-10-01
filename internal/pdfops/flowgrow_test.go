package pdfops

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

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

// flowNext is the page after the edited one, before and after the edit: where what leaves must arrive.
type flowNext struct{ before, after pageLayout }

// grewAsItShould checks the page after a grown edit of paragraph pi: the edited paragraph reads as edit on its own
// baselines and extra more at its pitch, every line of the region reads exactly `extra × pitch` lower, every other line
// where it was — and every line that leaves ARRIVES on the next page (next), the block's first baseline on that page's
// first paragraph's, each line the same distance below it (P07 phase-close review: leaving lines were asserted only
// gone). It returns the first difference, or "".
func grewAsItShould(before, after pageLayout, pi int, edit string, page [4]float64, next *flowNext) string {
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
	region := regionOf(before, pi, page)
	moved := map[int]bool{}
	for _, q := range region.paragraphs {
		moved[q] = true
	}
	dy := float64(extra) * pitch
	// A region paragraph pushed below the floor LEAVES for the next page (P07.S06's `pushDown`), so it is asserted gone
	// from this page rather than lower on it — reached by the real corpus only once tagged text could leave (P07.S07).
	leaves := func(i int) bool { return moved[i] && before.paragraphs[i].bottom()-dy < region.floor-measureSlack }
	land := math.NaN() // how far up a leaving line is drawn on the next page
	for _, q := range region.paragraphs {
		if leaves(q) {
			if next == nil || len(next.before.paragraphs) == 0 {
				return fmt.Sprintf("paragraph %d left for the next page, which was not read", q)
			}
			land = before.paragraphs[q].lines[0].y - next.before.paragraphs[0].lines[0].y
			break
		}
	}
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
			found, anywhere := false, false
			for _, a := range after3 {
				if a.text == ln.text && math.Abs(a.x-ln.x0) < 1e-6 {
					anywhere = anywhere || math.Abs(a.y-ln.y) < 1e-6 || math.Abs(a.y-want) < 1e-6
					if math.Abs(a.y-want) < 1e-6 {
						found = true
						break
					}
				}
			}
			if leaves(i) {
				if anywhere {
					return fmt.Sprintf("line %q should have left for the next page and is still on this one", ln.text)
				}
				arrived := false
				for _, a := range linesOf(next.after, -1) {
					arrived = arrived || a.text == ln.text && math.Abs(a.x-ln.x0) < 1e-6 && math.Abs(a.y-(ln.y-land)) < 1e-6
				}
				if !arrived {
					return fmt.Sprintf("line %q left and did not arrive on the next page at y %v", ln.text, ln.y-land)
				}
				continue
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
	if d := grewAsItShould(before, after, 0, oneMoreLine, page, nil); d != "" {
		t.Error(d)
	}
	// Paragraph 1 was drawn at 648 and 634, so a line at 634 is there on the unedited page too: one pitch lower is its
	// lines at 634 AND 620, and nothing left at 648 (P07 phase-close review — the check passed on an unedited page).
	all := [4]float64{0, 0, 612, 792}
	for _, y := range []float64{634, 620} {
		if ln, ok := lineAtBaseline(after, all, y, 12); !ok || !strings.HasPrefix(ln.text, "Words run") {
			t.Errorf("paragraph 1 is not one pitch lower: nothing at %v (%q)", y, ln.text)
		}
	}
	if ln, ok := lineAtBaseline(after, all, 648, 12); ok {
		t.Errorf("a line is still at paragraph 1's old first baseline, 648: %q", ln.text)
	}
}

// TestAGrowthItCannotDoNamesWhy — P07.S03's refusals, each from a fixture built to reach it, and a paragraph below that
// cannot move is NAMED.
func TestAGrowthItCannotDoNamesWhy(t *testing.T) {
	form := "BT /F1 12 Tf 14 TL 72 648 Td (Words run across the column here and wrap) Tj T* (Formed words here) Tj ET"
	for _, c := range []struct {
		name  string
		pdf   []byte
		edit  string
		cause string
		below string
	}{
		{"no room below", flowPage("BT /F1 12 Tf 72 558 Td (Page 1) Tj ET"), oneMoreLine, causePageFull, ""},
		{"a rule in the band", flowPage("q 1 w 72 660 m 400 660 l S Q"), oneMoreLine, causeAnchored, ""},
		{"a change bar in the margin beside the region", flowPage("q 1 w 560 600 m 560 655 l S Q"), oneMoreLine, causeAnchored, ""},
		// Over the edited paragraph's last line: `annotatedOver` refuses it, before any band is asked about.
		{"a note over the edited paragraph's last line", pageWith(flowPageContent, "", "", "/Annots [6 0 R]", helvetica,
			"<< /Type /Annot /Subtype /Text /Rect [100 660 120 675] >>"), oneMoreLine, causeAnchored, ""},
		// Beside the edited paragraph, clear of its box, across the top of what moves: only the band check reaches it (P07
		// phase-close review — the case above was named for this check and refused by the other).
		{"a note beside the edited paragraph straddling the top of what moves", pageWith(flowPageContent, "", "", "/Annots [6 0 R]", helvetica,
			"<< /Type /Annot /Subtype /Text /Rect [450 662 470 676] >>"), oneMoreLine, causeAnchored, ""},
		{"a note straddling the moved region's bottom edge", pageWith(flowPageContent, "", "", "/Annots [6 0 R]", helvetica,
			"<< /Type /Annot /Subtype /Text /Rect [100 585 120 600] >>"), oneMoreLine, causeAnchored, ""},
		{"a note in the free room the text moves into", pageWith(flowPageContent, "", "", "/Annots [6 0 R]", helvetica,
			"<< /Type /Annot /Subtype /Text /Rect [100 575 120 590] >>"), oneMoreLine, causeAnchored, ""},
		{"a note in another column at the region's height", pageWith(flowPageContent+"BT /F1 12 Tf 470 640 Td (Aside) Tj ET", "", "", "/Annots [6 0 R]", helvetica,
			"<< /Type /Annot /Subtype /Text /Rect [480 600 500 615] >>"), oneMoreLine, causeAnchored, ""},
		{"a link over a word it re-wraps, even without growing",
			pageWith(flowPageContent, "", "", "/Annots [6 0 R]", helvetica, "<< /Type /Annot /Subtype /Link /Rect [100 695 160 712] /Border [0 0 0] >>"),
			"Words run across the column here and wrap Words run across the column here and wrap Words run across the column there and wrap", causeAnchored, ""},
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
				var next *flowNext
				if ctx.PageCount >= 2 {
					nb, nerr := readPageGlyphLayout(ctx, pageAt(ctx, nil, 2))
					if nerr != nil {
						t.Fatal(nerr)
					}
					next = &flowNext{before: nb, after: layoutOfPage(t, out, 2)}
				}
				if d := grewAsItShould(l, after, pi, edit, visibleBoxOf(pg), next); d != "" {
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

// TestWhatIsAnchoredMovesWithTheText — `PLAN-text-reflow.md` P07.S04: when a paragraph grows, everything anchored inside
// the part of the page that moves goes with it, by exactly the growth — a note, a link and its quadrilaterals, an ink
// drawing, a form field's widget, a note in the margin beside a moved paragraph (a one-column page), a signing flag, and a
// destination shared by a
// bookmark, a link and a name, which moves ONCE. What lies above the edited paragraph does not move.
func TestWhatIsAnchoredMovesWithTheText(t *testing.T) {
	const dy = 14 // one line at the paragraph's pitch
	pdf := pageWith(flowPageContent, "",
		"/Outlines 6 0 R /Names << /Dests << /Names [(sec2) 8 0 R] >> >> /AcroForm << /Fields [14 0 R] >>",
		"/Annots [9 0 R 10 0 R 11 0 R 12 0 R 13 0 R 14 0 R]", helvetica,
		"<< /Type /Outlines /First 7 0 R /Last 7 0 R /Count 1 >>",
		"<< /Title (Second) /Parent 6 0 R /Dest (sec2) >>",
		"[3 0 R /XYZ 72 660 0]",
		"<< /Type /Annot /Subtype /Text /Rect [100 640 120 655] >>",
		"<< /Type /Annot /Subtype /Link /Rect [80 606 150 621] /QuadPoints [80 621 150 621 80 606 150 606] /Dest (sec2) >>",
		"<< /Type /Annot /Subtype /Ink /Rect [99 614 111 631] /InkList [[100 615 110 630]] >>",
		"<< /Type /Annot /Subtype /Text /Rect [560 640 580 655] >>",
		"<< /Type /Annot /Subtype /Text /Rect [560 740 580 755] >>",
		"<< /Type /Annot /Subtype /Widget /FT /Tx /T (name) /DA (/Helv 0 Tf 0 g) /P 3 0 R /Rect [200 640 260 655] >>")
	pdf, err := SetFlags(pdf, []byte(`[{"page":1,"frac":{"x":0.5,"y":0.2},"type":"sig","label":"kept"},{"page":1,"frac":{"x":0.5,"y":0.05},"type":"sig"}]`))
	if err != nil {
		t.Fatal(err)
	}
	_, _, cause, _, _ := reflowRead(t, pdf, 0, oneMoreLine)
	if cause != "" {
		t.Fatalf("refused: %s", cause)
	}
	out, _, err := ReflowParagraph(pdf, 1, 0, flowParagraph0, oneMoreLine)
	if err != nil || out == nil {
		t.Fatalf("the growth wrote nothing: %v", err)
	}
	ctx, err := pdfread.Validated(out, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pg := pageAt(ctx, nil, 1)
	var rects []string
	for _, o := range derefArray(ctx.XRefTable, pg.Dict["Annots"]) {
		d := derefDict(ctx.XRefTable, o)
		llx, lly, urx, ury, _ := rectOf(ctx, d["Rect"])
		line := fmt.Sprintf("%s %v %v %v %v", nameVal(d, "Subtype"), llx, lly, urx, ury)
		for _, k := range []string{"QuadPoints", "InkList"} {
			if v, ok := d[k]; ok {
				line += " " + k + numbersOf(ctx.XRefTable, v)
			}
		}
		rects = append(rects, line)
	}
	want := []string{
		"Text 100 626 120 641",
		"Link 80 592 150 607 QuadPoints[80 607 150 607 80 592 150 592]",
		"Ink 99 600 111 617 InkList[[100 601 110 616]]",
		"Text 560 626 580 641",
		"Text 560 740 580 755", // above the edited paragraph: it does not move
		"Widget 200 626 260 641",
	}
	if strings.Join(rects, "\n") != strings.Join(want, "\n") {
		t.Errorf("annotations\n got  %q\n want %q", rects, want)
	}
	if b, ok := destinationBox(ctx.XRefTable, types.StringLiteral("sec2"), pg.Ref.ObjectNumber.Value()); !ok || b[1] != 660-dy {
		t.Errorf("the shared destination's top is %v (%v), want %v — moved once", b[1], ok, 660-dy)
	}
	raw, err := FlagsJSON(out)
	if err != nil {
		t.Fatal(err)
	}
	var flags []map[string]any
	if err := json.Unmarshal(raw, &flags); err != nil || len(flags) != 2 {
		t.Fatalf("flags %s (%v)", raw, err)
	}
	f0 := flags[0]["frac"].(map[string]any)
	if y := f0["y"].(float64); math.Abs(y-(0.2+dy/792.0)) > 1e-12 || flags[0]["label"] != "kept" {
		t.Errorf("the flag in the zone reads %v, want y %v and its other fields kept", flags[0], 0.2+dy/792.0)
	}
	if y := flags[1]["frac"].(map[string]any)["y"].(float64); y != 0.05 {
		t.Errorf("the flag above the paragraph moved: y %v", y)
	}
}

const flowParagraph0 = "Words run across the column here and wrap Words run across the column here and wrap Words run across the column here and wrap"

// numbersOf prints a (nested) array of numbers by value, as %v prints float64s — not as pdfcpu's two-decimal String.
func numbersOf(xt *model.XRefTable, o types.Object) string {
	if arr := derefArray(xt, o); arr != nil {
		parts := make([]string, len(arr))
		for i, e := range arr {
			parts[i] = numbersOf(xt, e)
		}
		return "[" + strings.Join(parts, " ") + "]"
	}
	v, _ := pdfNumber(xt, o)
	return fmt.Sprint(v)
}

// layoutOfPage is page p's glyph layout in pdf.
func layoutOfPage(t *testing.T, pdf []byte, p int) pageLayout {
	t.Helper()
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	l, err := readPageGlyphLayout(ctx, pageAt(ctx, nil, p))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// TestWhatLeavesArrivesOnTheNextPage — P07 phase-close review: `grewAsItShould` asserts what leaves ARRIVES, line by line,
// where the block lands on the next page — asked of a growth that sends page 1's last paragraph over.
func TestWhatLeavesArrivesOnTheNextPage(t *testing.T) {
	pdf := cascadeDoc([]string{cascadePage(1, 16, ""), cascadePage(2, 3, "")}, nil, "")
	orig, edit := threeLinesMore()
	out, refusal, err := ReflowParagraph(pdf, 1, 0, orig, edit)
	if err != nil || out == nil {
		t.Fatalf("refused %+v (%v)", refusal, err)
	}
	before := layoutOfPage(t, pdf, 1)
	next := &flowNext{before: layoutOfPage(t, pdf, 2), after: layoutOfPage(t, out, 2)}
	if d := grewAsItShould(before, layoutOfPage(t, out, 1), 0, edit, [4]float64{0, 0, 612, 792}, next); d != "" {
		t.Error(d)
	}
	if d := grewAsItShould(before, layoutOfPage(t, out, 1), 0, edit, [4]float64{0, 0, 612, 792}, nil); !strings.Contains(d, "not read") {
		t.Errorf("a paragraph left and the check did not ask where it went: %q", d)
	}
}

// TestAOneLineParagraphIsNotSetUnderAFieldBesideIt — P07 phase-close review: a one-line paragraph is re-set at its column's
// measure, which a form label's field beside it had not bounded — the edited label ran under the field. The measure now
// stops an em short of any annotation beside the line, as it does of text and drawings: an edit that fits before the
// field stays on its line, and one that does not wraps below, where the field it would run under refuses it.
func TestAOneLineParagraphIsNotSetUnderAFieldBesideIt(t *testing.T) {
	long := "A long paragraph line that runs well across the whole column width here ok"
	c := "BT /F1 12 Tf 72 700 Td (Your name) Tj ET\n" +
		fmt.Sprintf("BT /F1 12 Tf 14 TL 72 660 Td (%s) Tj T* (%s) Tj ET\n", long, long) +
		fmt.Sprintf("BT /F1 12 Tf 14 TL 72 610 Td (%s) Tj T* (%s) Tj ET\n", long, long)
	field := [4]float64{160, 696, 400, 712}
	pdf := cascadeDoc([]string{c}, map[int]string{0: "/Annots [30 0 R]"}, "/AcroForm << /Fields [30 0 R] >>",
		fmt.Sprintf("<< /Type /Annot /Subtype /Widget /FT /Tx /T (name) /DA (/Helv 0 Tf 0 g) /P 10 0 R /Rect [%v %v %v %v] >>",
			field[0], field[1], field[2], field[3]))
	for _, e := range []struct{ edit, cause string }{
		{"Your full legal name as applicant", causeAnchored},
		{"Your names", ""},
	} {
		out, refusal, err := ReflowParagraph(pdf, 1, 0, "Your name", e.edit)
		if err != nil {
			t.Fatal(err)
		}
		if refusal.Cause != e.cause {
			t.Errorf("%q: refusal %+v, want %q", e.edit, refusal, e.cause)
			continue
		}
		if out == nil {
			continue
		}
		for _, q := range layoutOfPage(t, out, 1).paragraphs {
			if meets(paragraphBox(q), field) {
				t.Errorf("%q: %q is drawn under the field", e.edit, q.text())
			}
		}
	}
}
