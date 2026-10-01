package pdfops

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

// cascadeLine is paragraph k of page p's line: every line of every paragraph the same width, so only the vertical step
// breaks paragraphs.
func cascadeLine(p, k int) string { return fmt.Sprintf("Page %d paragraph %02d words run on", p, k) }

// cascadePage is n paragraphs of two lines, 14 points apart, each a 24-point step below the last, from y 700; extra is
// drawn after them.
func cascadePage(p, n int, extra string) string {
	var b strings.Builder
	for k := 0; k < n; k++ {
		fmt.Fprintf(&b, "BT /F1 12 Tf 14 TL 72 %d Td (%s) Tj T* (%s) Tj ET\n", 700-38*k, cascadeLine(p, k), cascadeLine(p, k))
	}
	return b.String() + extra
}

// cascadeDoc is a document of the given page contents, every page with /F1 = Helvetica; page i's dictionary gains
// pageExtra[i] when given, the catalog catalogExtra, and objects from 30 on are extra.
func cascadeDoc(contents []string, pageExtra map[int]string, catalogExtra string, extra ...string) []byte {
	kids := make([]string, len(contents))
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R " + catalogExtra + ">>",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
	}
	for i, c := range contents {
		pageObj, contentObj := 10+2*i, 11+2*i
		kids[i] = fmt.Sprintf("%d 0 R", pageObj)
		objs[pageObj] = fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents %d 0 R %s>>", contentObj, pageExtra[i])
		objs[contentObj] = fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(c), c)
	}
	objs[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(contents))
	for i, o := range extra {
		objs[30+i] = o
	}
	return assembleFixture(objs)
}

// threeLinesMore is paragraph 0 of page 1 with three lines' worth added.
func threeLinesMore() (orig, edit string) {
	l := cascadeLine(1, 0)
	orig = l + " " + l
	return orig, orig + " " + l + " " + l + " " + l
}

// lineYs maps each line's text on page p to its baselines, top to bottom.
func lineYs(t *testing.T, pdf []byte, p int) map[string][]float64 {
	t.Helper()
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	l, err := readPageGlyphLayout(ctx, pageAt(ctx, nil, p))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]float64{}
	for _, q := range l.paragraphs {
		for _, ln := range q.lines {
			out[ln.text] = append(out[ln.text], ln.y)
		}
	}
	return out
}

func near6(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// TestAGrowthFlowsOntoTheNextPages — `PLAN-text-reflow.md` P07.S06: a paragraph that grows by three lines on a full page
// pushes its page's paragraphs down; the last no longer fits and moves to the top of page 2, whose paragraphs are pushed in
// turn; page 2's last moves to page 3, which has room. Every line is read back where the rule puts it.
func TestAGrowthFlowsOntoTheNextPages(t *testing.T) {
	pdf := cascadeDoc([]string{cascadePage(1, 16, ""), cascadePage(2, 16, ""), cascadePage(3, 5, "")}, nil, "")
	orig, edit := threeLinesMore()
	out, refusal, err := ReflowParagraph(pdf, 1, 0, orig, edit)
	if err != nil || out == nil {
		t.Fatalf("refused %+v (%v)", refusal, err)
	}
	p1, p2, p3 := lineYs(t, out, 1), lineYs(t, out, 2), lineYs(t, out, 3)
	check := func(page map[string][]float64, text string, want ...float64) {
		t.Helper()
		got := page[text]
		if len(got) != len(want) {
			t.Errorf("%q at %v, want %v", text, got, want)
			return
		}
		for i := range want {
			if !near6(got[i], want[i]) {
				t.Errorf("%q at %v, want %v", text, got, want)
				return
			}
		}
	}
	check(p1, cascadeLine(1, 0), 700, 686, 672, 658, 644) // grown by three lines
	check(p1, cascadeLine(1, 1), 662-42, 648-42)          // pushed down by the growth
	check(p1, cascadeLine(1, 14), 700-38*14-42, 686-38*14-42)
	check(p1, cascadeLine(1, 15))                // gone from page 1
	check(p2, cascadeLine(1, 15), 700, 686)      // landed on page 2's first baseline
	check(p2, cascadeLine(2, 0), 700-38, 686-38) // pushed by the block's span and a paragraph step
	check(p2, cascadeLine(2, 15))                // gone from page 2
	check(p3, cascadeLine(2, 15), 700, 686)      // and on page 3
	check(p3, cascadeLine(3, 0), 700-38, 686-38)
	check(p3, cascadeLine(3, 4), 700-38*4-38, 686-38*4-38)
}

// TestAFlowThatReachesTheLastPageFullRefuses — P07.S06: when the last page has no room either, the growth refuses
// `page-full` and nothing is written.
func TestAFlowThatReachesTheLastPageFullRefuses(t *testing.T) {
	pdf := cascadeDoc([]string{cascadePage(1, 16, ""), cascadePage(2, 16, "")}, nil, "")
	orig, edit := threeLinesMore()
	out, refusal, err := ReflowParagraph(pdf, 1, 0, orig, edit)
	if err != nil || out != nil || refusal.Cause != causePageFull {
		t.Errorf("wrote %d bytes, refusal %+v (%v), want page-full", len(out), refusal, err)
	}
}

// TestWhatIsAnchoredCrossesWithItsParagraph — P07.S06: a note, its popup, a form field's widget, a signing flag and a
// bookmark on the paragraph
// that leaves page 1 arrive on page 2 with it, exactly where it lands; page 1 keeps nothing of them.
func TestWhatIsAnchoredCrossesWithItsParagraph(t *testing.T) {
	// Paragraph 15 of page 1 is drawn at baselines 130 and 116 and lands at 700 and 686: 570 points up.
	pdf := cascadeDoc([]string{cascadePage(1, 16, ""), cascadePage(2, 3, "")},
		map[int]string{0: "/Annots [30 0 R 31 0 R 34 0 R]"}, "/Outlines 32 0 R /AcroForm << /Fields [34 0 R] >>",
		"<< /Type /Annot /Subtype /Text /Rect [80 118 96 134] /Popup 31 0 R >>",
		"<< /Type /Annot /Subtype /Popup /Rect [400 85 500 100] /Parent 30 0 R >>", // its window, in the free room: not an anchor
		"<< /Type /Outlines /First 33 0 R /Last 33 0 R /Count 1 >>",
		"<< /Title (Leaving) /Parent 32 0 R /Dest [10 0 R /XYZ 72 142 0] >>",
		"<< /Type /Annot /Subtype /Widget /FT /Tx /T (w) /DA (/Helv 0 Tf 0 g) /P 10 0 R /Rect [200 118 260 134] >>")
	flagY := (792.0 - 125) / 792
	pdf, err := SetFlags(pdf, []byte(fmt.Sprintf(`[{"page":1,"frac":{"x":0.5,"y":%v},"type":"sig"}]`, flagY)))
	if err != nil {
		t.Fatal(err)
	}
	orig, edit := threeLinesMore()
	out, refusal, err := ReflowParagraph(pdf, 1, 0, orig, edit)
	if err != nil || out == nil {
		t.Fatalf("refused %+v (%v)", refusal, err)
	}
	ctx, err := pdfread.Validated(out, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pg1, pg2 := pageAt(ctx, nil, 1), pageAt(ctx, nil, 2)
	if n := len(derefArray(ctx.XRefTable, pg1.Dict["Annots"])); n != 0 {
		t.Errorf("page 1 keeps %d annotation(s)", n)
	}
	var got []string
	for _, o := range derefArray(ctx.XRefTable, pg2.Dict["Annots"]) {
		d := derefDict(ctx.XRefTable, o)
		llx, lly, urx, ury, _ := rectOf(ctx, d["Rect"])
		p, _ := d["P"].(types.IndirectRef)
		got = append(got, fmt.Sprintf("%s %v %v %v %v P=%d", nameVal(d, "Subtype"), llx, lly, urx, ury, p.ObjectNumber.Value()))
	}
	want := []string{"Text 80 688 96 704 P=12", "Popup 400 655 500 670 P=12", "Widget 200 688 260 704 P=12"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("page 2's annotations %q, want %q", got, want)
	}
	found := false
	for _, arr := range destinationArrays(ctx, pg2.Ref.ObjectNumber.Value()) {
		if b, ok := destinationBox(ctx.XRefTable, arr, pg2.Ref.ObjectNumber.Value()); ok && near6(b[1], 712) {
			found = true
		}
	}
	if !found {
		t.Error("the bookmark does not point at page 2 where its paragraph landed")
	}
	raw, _ := FlagsJSON(out)
	var flags []map[string]any
	if err := json.Unmarshal(raw, &flags); err != nil || len(flags) != 1 {
		t.Fatalf("flags %s (%v)", raw, err)
	}
	fy := flags[0]["frac"].(map[string]any)["y"].(float64)
	if flags[0]["page"].(float64) != 2 || !near6(fy, (792.0-695)/792) {
		t.Errorf("the flag is %v, want page 2 at y fraction %v", flags[0], (792.0-695)/792)
	}
}

// TestAFlowRefusesWhatItCannotCarry — P07.S06: a page of several columns does not flow; a drawing over the next page's first
// paragraph refuses, since that paragraph is pushed out from under it.
func TestAFlowRefusesWhatItCannotCarry(t *testing.T) {
	orig, edit := threeLinesMore()
	for _, c := range []struct {
		name string
		pdf  []byte
		want string
	}{
		{"a page of two columns", cascadeDoc([]string{cascadePage(1, 16, "BT /F1 12 Tf 470 400 Td (Aside) Tj ET"), cascadePage(2, 3, "")}, nil, ""), causePageFull},
		{"a drawing over the next page's first paragraph", cascadeDoc([]string{cascadePage(1, 16, ""), cascadePage(2, 3, "q 1 w 72 698 m 300 698 l S Q")}, nil, ""), causeAnchored},
		{"a next page of two columns", cascadeDoc([]string{cascadePage(1, 16, ""), cascadePage(2, 3, "BT /F1 12 Tf 470 400 Td (Aside) Tj ET")}, nil, ""), causePageFull},
		{"a next page whose body is too short for what lands", cascadeDoc([]string{cascadePage(1, 16, ""),
			"BT /F1 12 Tf 72 700 Td (" + cascadeLine(2, 0) + ") Tj ET q 1 w 72 680 m 300 680 l S Q", cascadePage(3, 3, "")}, nil, ""), causePageFull},
		{"an untagged next page with nothing on it", cascadeDoc([]string{cascadePage(1, 16, ""), "q Q"}, nil, ""), causePageFull},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, refusal, err := ReflowParagraph(c.pdf, 1, 0, orig, edit)
			if err != nil || out != nil || refusal.Cause != c.want {
				t.Errorf("wrote %d bytes, refusal %+v (%v), want %s", len(out), refusal, err, c.want)
			}
		})
	}
}

// flagsOf is the document's NibFlags, decoded.
func flagsOf(t *testing.T, pdf []byte) []map[string]any {
	t.Helper()
	raw, err := FlagsJSON(pdf)
	if err != nil {
		t.Fatal(err)
	}
	var flags []map[string]any
	if err := json.Unmarshal(raw, &flags); err != nil {
		t.Fatalf("flags %s: %v", raw, err)
	}
	return flags
}

// TestAKeptAnchorStaysOnItsPage — P07 phase-close review: a link, a bookmark and a flag on the LAST paragraph that STAYS
// on page 1 (baselines 168 and 154) are moved down with it, 42 points, and stay on page 1; the link and flag on the
// paragraph that leaves (130 and 116) cross to page 2. They had been re-selected by zone after the in-page shift, which had
// moved the kept ones into the zone of what left — and they were carried off with it.
func TestAKeptAnchorStaysOnItsPage(t *testing.T) {
	pdf := cascadeDoc([]string{cascadePage(1, 16, ""), cascadePage(2, 3, "")},
		map[int]string{0: "/Annots [30 0 R 33 0 R]"}, "/Outlines 31 0 R",
		"<< /Type /Annot /Subtype /Link /Rect [80 160 96 172] /Border [0 0 0] /Dest [12 0 R /Fit] >>",
		"<< /Type /Outlines /First 32 0 R /Last 32 0 R /Count 1 >>",
		"<< /Title (Kept) /Parent 31 0 R /Dest [10 0 R /XYZ 72 170 0] >>",
		"<< /Type /Annot /Subtype /Link /Rect [80 118 96 134] /Border [0 0 0] /Dest [12 0 R /Fit] >>")
	pdf, err := SetFlags(pdf, []byte(fmt.Sprintf(`[{"page":1,"frac":{"x":0.5,"y":%v}},{"page":1,"frac":{"x":0.5,"y":%v}}]`,
		(792.0-165)/792, (792.0-125)/792)))
	if err != nil {
		t.Fatal(err)
	}
	orig, edit := threeLinesMore()
	out, refusal, err := ReflowParagraph(pdf, 1, 0, orig, edit)
	if err != nil || out == nil {
		t.Fatalf("refused %+v (%v)", refusal, err)
	}
	ctx, err := pdfread.Validated(out, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	rects := func(p int) string {
		var got []string
		for _, o := range derefArray(ctx.XRefTable, pageAt(ctx, nil, p).Dict["Annots"]) {
			llx, lly, urx, ury, _ := rectOf(ctx, derefDict(ctx.XRefTable, o)["Rect"])
			got = append(got, fmt.Sprintf("%v %v %v %v", llx, lly, urx, ury))
		}
		return strings.Join(got, "|")
	}
	if got, want := rects(1), "80 118 96 130"; got != want {
		t.Errorf("page 1's links %q, want %q — the kept link, 42 lower", got, want)
	}
	if got, want := rects(2), "80 688 96 704"; got != want {
		t.Errorf("page 2's links %q, want %q — the leaving link, where its paragraph landed", got, want)
	}
	pg1 := pageAt(ctx, nil, 1)
	kept := false
	for _, arr := range destinationArrays(ctx, pg1.Ref.ObjectNumber.Value()) {
		if b, ok := destinationBox(ctx.XRefTable, arr, pg1.Ref.ObjectNumber.Value()); ok && near6(b[1], 128) {
			kept = true
		}
	}
	if !kept {
		t.Error("the bookmark on the kept paragraph does not point at page 1, 42 lower")
	}
	flags := flagsOf(t, out)
	fy := func(i int) float64 { return flags[i]["frac"].(map[string]any)["y"].(float64) }
	if len(flags) != 2 || flags[0]["page"].(float64) != 1 || !near6(fy(0), (792.0-123)/792) {
		t.Errorf("the kept flag is %v, want page 1 at y fraction %v", flags, (792.0-123)/792)
	}
	if len(flags) == 2 && (flags[1]["page"].(float64) != 2 || !near6(fy(1), (792.0-695)/792)) {
		t.Errorf("the leaving flag is %v, want page 2 at y fraction %v", flags[1], (792.0-695)/792)
	}
}

// TestAFlowOnlyLeavesAPageItsMarginBounds — P07 phase-close review: a region whose floor is content below it that STAYS —
// the next section, past a wide gap — does not send its last paragraphs to the next page, where they would be read after
// that section; and a block does not land in a region that content bounds — a running header, the next page's first
// paragraph — whether or not the header would then have to leave too. Each refuses `page-full`.
func TestAFlowOnlyLeavesAPageItsMarginBounds(t *testing.T) {
	orig, edit := threeLinesMore()
	hdr := func(p, y int) string {
		return fmt.Sprintf("BT /F1 12 Tf 72 %d Td (Running header of page %d) Tj ET\n", y, p)
	}
	// A page-2 body set lower, so the header's own region has room below it to be pushed into and nothing on page 2 leaves.
	lowBody := strings.ReplaceAll(cascadePage(2, 3, ""), "72 700 Td", "72 600 Td")
	lowBody = strings.ReplaceAll(lowBody, "72 662 Td", "72 562 Td")
	lowBody = strings.ReplaceAll(lowBody, "72 624 Td", "72 524 Td")
	fiveMore := orig + strings.Repeat(" "+cascadeLine(1, 0), 5) // more than the 61 points above Section B's floor
	for _, c := range []struct {
		name string
		pdf  []byte
		edit string
	}{
		{"content past a wide gap below the region stays", cascadeDoc([]string{
			cascadePage(1, 12, "BT /F1 12 Tf 72 180 Td (Section B heading stays here) Tj ET\n"), cascadePage(2, 3, "")}, nil, ""), fiveMore},
		{"a running header on the next page, which would leave in turn", cascadeDoc([]string{
			cascadePage(1, 16, ""), hdr(2, 760) + cascadePage(2, 3, ""), hdr(3, 760) + cascadePage(3, 3, "")}, nil, ""), edit},
		{"a running header on the next page, with room below it", cascadeDoc([]string{
			cascadePage(1, 16, ""), hdr(2, 760) + lowBody}, nil, ""), edit},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, refusal, err := ReflowParagraph(c.pdf, 1, 0, orig, c.edit)
			if err != nil || out != nil || refusal.Cause != causePageFull {
				t.Errorf("wrote %d bytes, refusal %+v (%v), want page-full", len(out), refusal, err)
			}
		})
	}
}

// TestEveryDestinationToWhatLeavesCrosses — P07 phase-close review: a destination reached by a GoTo on a widget's `/A`, in
// an annotation's, a page's or the catalog's `/AA`, or chained after another action by `/Next`, names page 1 where the
// leaving paragraph was (top 142) and must name page 2 where it landed (top 712) — only a link's first action and a
// bookmark's were read, and the rest stayed pointing at what had left.
func TestEveryDestinationToWhatLeavesCrosses(t *testing.T) {
	goTo := "<< /S /GoTo /D [10 0 R /XYZ 72 142 0] >>"
	orig, edit := threeLinesMore()
	for _, c := range []struct {
		name               string
		p2, catalog, extra string
	}{
		{"a widget's action", "/Annots [30 0 R]", "/AcroForm << /Fields [30 0 R] >>",
			"<< /Type /Annot /Subtype /Widget /FT /Btn /Ff 65536 /T (go) /P 12 0 R /Rect [300 740 360 760] /A " + goTo + " >>"},
		{"an annotation's additional actions", "/Annots [30 0 R]", "",
			"<< /Type /Annot /Subtype /Square /P 12 0 R /Rect [300 740 360 760] /AA << /U " + goTo + " >> >>"},
		{"a page's additional actions", "/AA << /O " + goTo + " >>", "", ""},
		{"the catalog's additional actions", "", "/AA << /WC " + goTo + " >>", ""},
		{"a GoTo chained after a URI", "/Annots [30 0 R]", "",
			"<< /Type /Annot /Subtype /Link /P 12 0 R /Rect [300 740 360 760] /A << /S /URI /URI (https://example.org/) /Next [" + goTo + "] >> >>"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var extra []string
			if c.extra != "" {
				extra = []string{c.extra}
			}
			pdf := cascadeDoc([]string{cascadePage(1, 16, ""), cascadePage(2, 3, "")}, map[int]string{1: c.p2}, c.catalog, extra...)
			out, refusal, err := ReflowParagraph(pdf, 1, 0, orig, edit)
			if err != nil || out == nil {
				t.Fatalf("refused %+v (%v)", refusal, err)
			}
			ctx, err := pdfread.Validated(out, model.NewDefaultConfiguration())
			if err != nil {
				t.Fatal(err)
			}
			for p, top := range map[int]float64{1: 142, 2: 712} {
				nr := pageAt(ctx, nil, p).Ref.ObjectNumber.Value()
				found := false
				for _, arr := range destinationArrays(ctx, nr) {
					if b, ok := destinationBox(ctx.XRefTable, arr, nr); ok && near6(b[1], top) {
						found = true
					}
				}
				if want := p == 2; found != want {
					t.Errorf("a destination at top %v on page %d: %v, want %v", top, p, found, want)
				}
			}
		})
	}
}

// TestAFlowRefusesWhatNothingMoves — P07 phase-close review: an article bead over what moves — nothing moves a bead; text
// drawn invisibly (a search layer over a scan) moved or carried; text in a sequence whose property list names no MCID
// (`/OC`, a `/Lang` span), which a carry re-opens only the MCID's sequence for; and a flag carried onto a page turned
// otherwise than its own, where the same fraction is another place. Each is refused by name, and each fixture without its
// one difference flows (the control), so the refusal is about that difference.
func TestAFlowRefusesWhatNothingMoves(t *testing.T) {
	orig, edit := threeLinesMore()
	last := func(wrap func(string) string) string { // page 1 with paragraph 15 drawn through wrap
		p := cascadePage(1, 16, "")
		i := strings.LastIndex(strings.TrimSuffix(p, "\n"), "\n") + 1
		return p[:i] + wrap(p[i:])
	}
	flagAt125 := func(pdf []byte) []byte {
		out, err := SetFlags(pdf, []byte(fmt.Sprintf(`[{"page":1,"frac":{"x":0.5,"y":%v}}]`, (792.0-125)/792)))
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	ocDoc := func(wrapped bool) []byte {
		p1 := cascadePage(1, 16, "")
		if wrapped {
			p1 = last(func(s string) string { return "/OC /oc1 BDC " + s + "EMC\n" })
		}
		p2 := cascadePage(2, 3, "")
		return assembleFixture(map[int]string{
			1:  "<< /Type /Catalog /Pages 2 0 R /OCProperties << /OCGs [40 0 R] /D << /OFF [40 0 R] >> >> >>",
			2:  "<< /Type /Pages /Kids [10 0 R 12 0 R] /Count 2 >>",
			5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
			10: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> /Properties << /oc1 40 0 R >> >> /Contents 11 0 R >>",
			11: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(p1), p1),
			12: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 13 0 R >>",
			13: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(p2), p2),
			40: "<< /Type /OCG /Name (Hidden layer) >>",
		})
	}
	plain := cascadeDoc([]string{cascadePage(1, 16, ""), cascadePage(2, 3, "")}, nil, "")
	for _, c := range []struct {
		name         string
		pdf, control []byte
		cause, below string
	}{
		// Wholly inside the zone that leaves, where a note would cross with it: a bead does not.
		{"an article bead over the paragraph that leaves",
			cascadeDoc([]string{cascadePage(1, 16, ""), cascadePage(2, 3, "")}, map[int]string{0: "/B [30 0 R]"}, "",
				"<< /Type /Bead /R [72 115 300 140] /P 10 0 R >>"), plain, causeAnchored, ""},
		// Paragraph 1 only: every text object states its mode, which outlives `ET` (it is graphics state).
		{"a search layer moved within the page", cascadeDoc([]string{
			strings.Replace(strings.ReplaceAll(cascadePage(1, 16, ""), "BT /F1", "BT 0 Tr /F1"), "BT 0 Tr /F1 12 Tf 14 TL 72 662 Td", "BT 3 Tr /F1 12 Tf 14 TL 72 662 Td", 1),
			cascadePage(2, 3, "")}, nil, ""), plain, causeInvisible, cascadeLine(1, 1) + " " + cascadeLine(1, 1)},
		{"a search layer carried to the next page", cascadeDoc([]string{
			last(func(s string) string { return strings.Replace(s, "BT ", "BT 7 Tr ", 1) }), cascadePage(2, 3, "")}, nil, ""), plain,
			causeInvisible, cascadeLine(1, 15) + " " + cascadeLine(1, 15)},
		{"text in an optional-content layer carried", ocDoc(true), ocDoc(false), causeStateNotCarried, cascadeLine(1, 15) + " " + cascadeLine(1, 15)},
		{"text in a language span carried", cascadeDoc([]string{
			last(func(s string) string { return "/Span <</Lang (fr-FR)>> BDC " + s + "EMC\n" }), cascadePage(2, 3, "")}, nil, ""), plain,
			causeStateNotCarried, cascadeLine(1, 15) + " " + cascadeLine(1, 15)},
		{"a flag carried onto a turned page",
			flagAt125(cascadeDoc([]string{cascadePage(1, 16, ""), cascadePage(2, 3, "")}, map[int]string{1: "/Rotate 90"}, "")),
			cascadeDoc([]string{cascadePage(1, 16, ""), cascadePage(2, 3, "")}, map[int]string{1: "/Rotate 90"}, ""), causeAnchored, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, refusal, err := ReflowParagraph(c.control, 1, 0, orig, edit); err != nil || out == nil {
				t.Fatalf("setup: the control refused %+v (%v)", refusal, err)
			}
			out, refusal, err := ReflowParagraph(c.pdf, 1, 0, orig, edit)
			if err != nil || out != nil || refusal.Cause != c.cause || refusal.Below != c.below {
				t.Errorf("wrote %d bytes, refusal %+v (%v), want %s below %q", len(out), refusal, err, c.cause, c.below)
			}
		})
	}
}

// TestOneMalformedFlagDoesNotHideTheOthers — P07 phase-close review: the refusal check decoded the whole flag list into one
// typed shape, so one flag whose fraction was not an object read as NO flags, and a growth into the room where a valid flag
// sat went through — while the mover, decoding its own way, still saw the valid one. One decoder now reads each flag alone.
func TestOneMalformedFlagDoesNotHideTheOthers(t *testing.T) {
	pdf := cascadeDoc([]string{cascadePage(1, 12, "")}, nil, "")
	orig, edit := threeLinesMore()
	if out, refusal, err := ReflowParagraph(pdf, 1, 0, orig, edit); err != nil || out == nil {
		t.Fatalf("setup: with no flag the growth refused %+v (%v)", refusal, err)
	}
	// y 200 lies in the free room below page 1's twelve paragraphs, which the growth moves them into.
	flagged, err := SetFlags(pdf, []byte(fmt.Sprintf(`[{"page":3,"frac":"not a fraction"},{"page":1,"frac":{"x":0.5,"y":%v}}]`, (792.0-200)/792)))
	if err != nil {
		t.Fatal(err)
	}
	out, refusal, err := ReflowParagraph(flagged, 1, 0, orig, edit)
	if err != nil || out != nil || refusal.Cause != causeAnchored {
		t.Errorf("wrote %d bytes, refusal %+v (%v), want anchored — the valid flag sits where the text moves", len(out), refusal, err)
	}
}
