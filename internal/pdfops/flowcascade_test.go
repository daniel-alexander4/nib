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
