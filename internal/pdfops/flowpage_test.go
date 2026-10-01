package pdfops

import (
	"bytes"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

// carryPages is a document of two pages: page 1 draws content1 with /F1 bound to Helvetica, page 2 draws content2 with /F1
// bound to COURIER — so a paragraph carried from page 1 must not be drawn under page 2's /F1. extraRes is added to page
// 1's resources, and objects from 10 on are extra.
func carryPages(content1, content2, extraRes string, extra ...string) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 7 0 R >> " + extraRes + " >> /Contents 5 0 R >>",
		4: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 8 0 R >> >> /Contents 6 0 R >>",
		5: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content1), content1),
		6: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content2), content2),
		7: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
		8: "<< /Type /Font /Subtype /Type1 /BaseFont /Courier /Encoding /WinAnsiEncoding >>",
	}
	for i, o := range extra {
		objs[10+i] = o
	}
	return assembleFixture(objs)
}

// carried sets paragraph pi of page 1 on page 2, dy lower, and returns the written document — or the cause.
func carried(t *testing.T, pdf []byte, pi int, dy float64) ([]byte, string) {
	t.Helper()
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	src := pageAt(ctx, nil, 1)
	l, err := readPageGlyphLayout(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	cause, err := setParagraphOn(ctx, l, src, pi, pageAt(ctx, nil, 2), 0, dy)
	if err != nil {
		t.Fatal(err)
	}
	if cause != "" {
		return nil, cause
	}
	var buf bytes.Buffer
	if err := api.WriteContext(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), ""
}

func runsOnPage(t *testing.T, pdf []byte, p int) []textRun {
	t.Helper()
	return runsOf(t, pdf, p)
}

func findRun(runs []textRun, text string) (textRun, bool) {
	for _, r := range runs {
		if r.text == text {
			return r, true
		}
	}
	return textRun{}, false
}

// TestAParagraphSetOnAnotherPageReadsBack — `PLAN-text-reflow.md` P07.S05: a paragraph carried from page 1 to page 2 reads
// back there with its text, its font (Helvetica — page 2's own /F1 is Courier), its size and its colour, dy lower; page 1
// no longer draws it, and what page 1 draws after it — placed relative to it, after a `'` — is exactly where it was; page
// 2's own text is untouched.
func TestAParagraphSetOnAnotherPageReadsBack(t *testing.T) {
	p1 := "q 0.2 0.4 0.6 rg BT /F1 12 Tf 14 TL 72 700 Td (Carried) Tj (also carried) ' 0 -30 Td (Stays below) Tj ET Q"
	pdf := carryPages(p1, "BT /F1 12 Tf 72 700 Td (Page two text) Tj ET", "")
	before1 := runsOnPage(t, pdf, 1)
	out, cause := carried(t, pdf, 0, 300)
	if cause != "" {
		t.Fatalf("refused: %s", cause)
	}
	after1, after2 := runsOnPage(t, out, 1), runsOnPage(t, out, 2)
	for _, gone := range []string{"Carried", "also carried"} {
		if _, ok := findRun(after1, gone); ok {
			t.Errorf("page 1 still draws %q", gone)
		}
	}
	stay0, _ := findRun(before1, "Stays below")
	stay1, ok := findRun(after1, "Stays below")
	if !ok || stay1.x != stay0.x || stay1.y != stay0.y {
		t.Errorf("page 1's later text moved: %v %v → %v %v (%v)", stay0.x, stay0.y, stay1.x, stay1.y, ok)
	}
	own, ok := findRun(after2, "Page two text")
	if !ok || own.baseFont != "Courier" || own.x != 72 || own.y != 700 {
		t.Errorf("page 2's own text changed: %+v", own)
	}
	for _, c := range []struct {
		text string
		y    float64
	}{{"Carried", 400}, {"also carried", 386}} {
		r, ok := findRun(after2, c.text)
		if !ok {
			t.Errorf("page 2 does not draw %q", c.text)
			continue
		}
		if r.baseFont != "Helvetica" || math.Abs(r.size-12) > 1e-9 || math.Abs(r.x-72) > 1e-9 || math.Abs(r.y-c.y) > 1e-9 || r.state.fill != "0.2 0.4 0.6 rg" {
			t.Errorf("%q on page 2: font %s size %v at (%v, %v) fill %q, want Helvetica 12 at (72, %v) fill 0.2 0.4 0.6 rg",
				c.text, r.baseFont, r.size, r.x, r.y, r.state.fill, c.y)
		}
	}
}

// TestACarryItCannotDoNamesWhy — P07.S05: what another page cannot be given is refused, each from a fixture built to reach
// it, and the stimulus is on the page (the paragraph reads as one).
func TestACarryItCannotDoNamesWhy(t *testing.T) {
	para := "BT /F1 12 Tf 14 TL 72 700 Td (Carried words) Tj T* (more carried words) Tj ET"
	for _, c := range []struct {
		name, content, res string
		extra              []string
		want               string
	}{
		{"a graphics state dictionary", "/GS1 gs " + para, "/ExtGState << /GS1 10 0 R >>", []string{"<< /Type /ExtGState /ca 0.5 >>"}, causeStateNotCarried},
		{"a colour outside the device spaces", "/CS1 cs 0.5 scn " + para, "/ColorSpace << /CS1 [/CalGray << /WhitePoint [1 1 1] >>] >>", nil, causeStateNotCarried},
		{"a clip that cuts it", "q 0 0 612 705 re W n " + para + " Q", "", nil, causeStateNotCarried},
		{"a clip that does not cut it", "q 0 0 612 792 re W n " + para + " Q", "", nil, ""},
		{"marked content in the structure", "/P <</MCID 0>> BDC " + para + " EMC", "", nil, causeTaggedAcross},
		{"replacement text", "/Span <</ActualText (x)>> BDC " + para + " EMC", "", nil, causeReplacementText},
		{"a clipping render mode", "5 Tr " + para, "", nil, causeClips},
		{"a show relying on where it ended", "BT /F1 12 Tf 14 TL 72 700 Td (Carried words) Tj T* (more carried words) Tj /Artifact BMC (after) Tj EMC ET", "", nil, causeInlineFollower},
	} {
		t.Run(c.name, func(t *testing.T) {
			pdf := carryPages(c.content, "BT /F1 12 Tf 72 700 Td (Page two) Tj ET", c.res, c.extra...)
			l, _ := layoutOf(t, pdf)
			if len(l.paragraphs) == 0 || !strings.HasPrefix(l.paragraphs[0].text(), "Carried words") {
				t.Fatalf("setup: the paragraph does not read (%d paragraphs)", len(l.paragraphs))
			}
			if _, cause := carried(t, pdf, 0, 300); cause != c.want {
				t.Errorf("cause %q, want %q", cause, c.want)
			}
		})
	}
}

// TestParagraphsCarriedAcrossRealPagesReadBack — P07.S05 over the generated and real-producer corpora: every paragraph of
// page 1 a carry accepts (at most eight a document) is set on page 2 at the same height and read back there with its
// text, font, size, position and fill; every other run on page 1 stays where it was, and every run page 2 drew still is.
//
// **The stimulus is asserted**: carries are accepted, some of a colour other than black and some in a font page 2 did not
// already name.
func TestParagraphsCarriedAcrossRealPagesReadBack(t *testing.T) {
	corpora := []lawOneCorpus{
		{name: "generated", docs: append(runCorpus(t),
			runCorpusDoc{"two pages", carryPages("q 0.2 0.4 0.6 rg BT /F1 12 Tf 14 TL 72 700 Td (Carried) Tj (also carried) ' ET Q", "BT /F1 12 Tf 72 700 Td (Page two) Tj ET", "")})},
		externalPDFs(t, "real producers", "NIB_UA_PRODUCERS", filepath.Join(homeDir(), "nib", "producers")),
	}
	key := func(r textRun) string { return fmt.Sprintf("%q %.4f %.4f", r.text, r.x, r.y) }
	accepted, coloured, newFont := 0, 0, 0
	tagged, taggedDocs := 0, map[string]bool{} // P07.S07: carries that took their structure with them
	refused := map[string]int{}
	for _, corp := range corpora {
		if corp.absent != "" {
			t.Logf("NOTE (a narrower population, not a pass over it): %s: %s", corp.name, corp.absent)
			continue
		}
		for _, doc := range corp.docs {
			ctx, err := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
			if err != nil || ctx.PageCount < 2 {
				continue
			}
			l, err := readPageGlyphLayout(ctx, pageAt(ctx, nil, 1))
			if err != nil {
				continue
			}
			before1, before2 := runsOnPage(t, doc.pdf, 1), runsOnPage(t, doc.pdf, 2)
			tried := 0
			for pi := range l.paragraphs {
				if tried == 8 {
					break
				}
				out, cause := carried(t, doc.pdf, pi, 0)
				if cause != "" {
					refused[cause]++
					continue
				}
				tried++
				accepted++
				moved := paragraphRunsWithBlanks(l, pi)
				if carriedStructure(t, doc.pdf, out, moved) {
					tagged++
					taggedDocs[doc.name] = true
				}
				gone := map[string]bool{}
				for _, r := range moved {
					gone[key(r)] = true
					if r.state.fill != "0 g" {
						coloured++
					}
				}
				after1, after2 := runsOnPage(t, out, 1), runsOnPage(t, out, 2)
				have1 := map[string]int{}
				for _, r := range after1 {
					have1[key(r)]++
				}
				for _, r := range before1 {
					if !gone[key(r)] && have1[key(r)] == 0 {
						t.Errorf("%s ¶%d: page 1's %q left its place", doc.name, pi, r.text)
					}
				}
				have2 := map[string]textRun{}
				for _, r := range after2 {
					have2[key(r)] = r
				}
				for _, r := range before2 {
					if _, ok := have2[key(r)]; !ok {
						t.Errorf("%s ¶%d: page 2's own %q moved or went", doc.name, pi, r.text)
					}
				}
				for _, r := range moved {
					if strings.TrimSpace(r.text) == "" {
						continue
					}
					got, ok := have2[key(r)]
					switch {
					case !ok:
						t.Errorf("%s ¶%d: %q is not drawn on page 2 where it was on page 1", doc.name, pi, r.text)
					case got.baseFont != r.baseFont || math.Abs(got.size-r.size) > 1e-6*r.size || got.state.fill != r.state.fill:
						t.Errorf("%s ¶%d: %q on page 2 is %s %v %q, was %s %v %q", doc.name, pi, r.text, got.baseFont, got.size, got.state.fill, r.baseFont, r.size, r.state.fill)
					case got.font != r.font:
						newFont++
					}
				}
			}
		}
	}
	var rs []string
	for k, n := range refused {
		rs = append(rs, fmt.Sprintf("%s %d", k, n))
	}
	sort.Strings(rs)
	t.Logf("carried %d paragraphs (%d runs not black, %d runs under a new font name; %d tagged, from %d documents); refused: %s",
		accepted, coloured, newFont, tagged, len(taggedDocs), strings.Join(rs, ", "))
	if accepted == 0 || coloured == 0 || newFont == 0 {
		t.Errorf("carried %d, coloured runs %d, renamed-font runs %d — the carry was not asked what it exists for", accepted, coloured, newFont)
	}
	if corpora[1].absent == "" && tagged == 0 {
		t.Error("no tagged paragraph of a real-producer document was carried — the structure carry was not asked what it exists for")
	}
}

// carriedStructure checks a carry of moved from page 1 of before to page 2 of after against the structure: each tagged run
// is drawn on page 2 under an MCID page 2's row gives the element that owned it on page 1, the structure reader reads its
// text on page 2, and the carry added no consistency defect. It reports whether any run was tagged.
func carriedStructure(t *testing.T, before, after []byte, moved []textRun) bool {
	t.Helper()
	var tagged []textRun
	for _, r := range moved {
		if r.mcid >= 0 && strings.TrimSpace(r.text) != "" {
			tagged = append(tagged, r)
		}
	}
	if len(tagged) == 0 {
		return false
	}
	_, was, bctx, btree := structureOf(t, before)
	got, now, actx, atree := structureOf(t, after)
	if len(now) > len(was) {
		t.Errorf("the carry added consistency defects: %d before, %d after (%+v)", len(was), len(now), now)
	}
	owner := func(ctx *model.Context, tree *structTree, p, mcid int) int {
		k, ok := pdfNumber(ctx.XRefTable, pageAt(ctx, nil, p).Dict["StructParents"])
		if !ok {
			return 0
		}
		row, _, _ := rowFor(ctx, tree, int(k))
		if mcid >= len(row) {
			return 0
		}
		if r, isRef := row[mcid].(types.IndirectRef); isRef {
			return r.ObjectNumber.Value()
		}
		return 0
	}
	// Every page-2 run of the carried run's text and x — page 2's own bullets share both — and the carried one is the one
	// its element owns: keying one run by text and x read whichever the stream drew LAST, which was the carried block until
	// the P07 phase-close review drew it first.
	on2 := map[string][]textRun{}
	for _, r := range runsOf(t, after, 2) {
		k := fmt.Sprintf("%q %.4f", r.text, r.x)
		on2[k] = append(on2[k], r)
	}
	for _, r := range tagged {
		elem := owner(bctx, btree, 1, r.mcid)
		cands := on2[fmt.Sprintf("%q %.4f", r.text, r.x)]
		a, ok := textRun{mcid: -1}, len(cands) > 0
		for _, c := range cands {
			if a.mcid < 0 || c.mcid >= 0 && owner(actx, atree, 2, c.mcid) == elem {
				a = c
			}
		}
		switch {
		case elem == 0:
			t.Errorf("setup: page 1's %q (MCID %d) has no element and was carried", r.text, r.mcid)
		case !ok || a.mcid < 0:
			t.Errorf("%q is not drawn under an MCID on page 2", r.text)
		case owner(actx, atree, 2, a.mcid) != elem:
			t.Errorf("%q on page 2 (MCID %d) belongs to element %d, was element %d's", r.text, a.mcid, owner(actx, atree, 2, a.mcid), elem)
		case !strings.Contains(got[elem].Text, strings.TrimSpace(r.text)):
			t.Errorf("element %d reads %q, which lacks the carried %q", elem, got[elem].Text, r.text)
		}
	}
	return true
}

// TestACarriedParagraphDrawsUnderTheDefaultState — P07.S05: a target page whose content leaves two `q`s unrestored under a
// scaling `cm`, and a text object open, still receives the paragraph at the position asked, at its size — one `Q` of the
// wrapper's own would pop only the inner `q`.
func TestACarriedParagraphDrawsUnderTheDefaultState(t *testing.T) {
	pdf := carryPages("BT /F1 12 Tf 14 TL 72 700 Td (Carried) Tj (also carried) ' ET",
		"q 2 0 0 2 0 0 cm q BT /F1 12 Tf 10 10 Td (Left open) Tj", "")
	out, cause := carried(t, pdf, 0, 300)
	if cause != "" {
		t.Fatalf("refused: %s", cause)
	}
	r, ok := findRun(runsOnPage(t, out, 2), "Carried")
	if !ok || math.Abs(r.x-72) > 1e-9 || math.Abs(r.y-400) > 1e-9 || math.Abs(r.size-12) > 1e-9 {
		t.Errorf("carried onto an unbalanced page: %+v at (%v, %v) size %v, want (72, 400) 12", ok, r.x, r.y, r.size)
	}
}

// TestDeletingAQuoteShowKeepsItsSpacing — P07.S05: a carried paragraph whose last line is shown by `"` leaves behind the
// word spacing `"` set, so the page's later text — whose spaces that spacing widens — is exactly as wide as it was.
func TestDeletingAQuoteShowKeepsItsSpacing(t *testing.T) {
	pdf := carryPages("BT /F1 12 Tf 14 TL 72 700 Td (Carried) Tj 5 0 (also carried) \" 0 -30 Td (Stays with spaces) Tj ET",
		"BT /F1 12 Tf 72 700 Td (Page two) Tj ET", "")
	before, _ := findRun(runsOnPage(t, pdf, 1), "Stays with spaces")
	if l, _ := layoutOf(t, pdf); len(l.paragraphs) < 2 || l.paragraphs[1].text() != "also carried" {
		t.Fatal("setup: paragraph 1 is not the line the quote shows")
	}
	out, cause := carried(t, pdf, 1, 300)
	if cause != "" {
		t.Fatalf("refused: %s", cause)
	}
	after, ok := findRun(runsOnPage(t, out, 1), "Stays with spaces")
	if !ok || math.Abs(after.width-before.width) > 1e-9 || after.x != before.x || after.y != before.y {
		t.Errorf("the later text is %v wide at (%v, %v), was %v at (%v, %v)", after.width, after.x, after.y, before.width, before.x, before.y)
	}
	if before.state.tw != 5 {
		t.Fatalf("setup: the later text was not set under the quote's word spacing (Tw %v)", before.state.tw)
	}
}

// TestACarriedParagraphIsDrawnOverTheTargetsBackdrop — P07 phase-close re-review: a next page that paints a backdrop
// (here a white page fill, which `regionOf` lets text land on) is drawn BEFORE what arrives, so the carried paragraph is
// visible on it. Drawing the carried block first, for stream order, hid it under the fill.
func TestACarriedParagraphIsDrawnOverTheTargetsBackdrop(t *testing.T) {
	pdf := cascadeDoc([]string{cascadePage(1, 16, ""), "1 1 1 rg 0 0 612 792 re f\n" + cascadePage(2, 3, "")}, nil, "")
	orig, edit := threeLinesMore()
	out, refusal, err := ReflowParagraph(pdf, 1, 0, orig, edit)
	if err != nil || out == nil {
		t.Fatalf("refused %+v (%v)", refusal, err)
	}
	ctx, err := pdfread.Validated(out, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pg := pageAt(ctx, nil, 2)
	c, err := pdfread.PageContent(ctx, pg.Dict, pg.Nr)
	if err != nil {
		t.Fatal(err)
	}
	arrived, fill := bytes.Index(c, []byte(cascadeLine(1, 15))), bytes.Index(c, []byte("0 0 612 792 re f"))
	if arrived < 0 || fill < 0 || arrived < fill {
		t.Errorf("page 2 paints its backdrop at %d and draws what arrived at %d: the arrival must come after, or it is hidden", fill, arrived)
	}
}
