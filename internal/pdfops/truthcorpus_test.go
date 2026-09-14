package pdfops

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// The truth corpus and the metric — `PLAN-accessibility.md` P08.S04, exit criterion 1's reader.
//
// # What "truth" is here
//
// LibreOffice tags its default conversion (P07.S05 measured it), so a document LibreOffice converts
// carries a structure tree written by the one party that knows the document's structure: the
// producer. Strip that tree and the result is an arbitrary PDF whose correct answer is known. The
// autotagger (S05) is scored against the tree it never saw.
//
// # The two numbers
//
// Both are computed over the page text with whitespace removed, so the two sides are compared on
// what they both contain rather than on how each chose to space it:
//
//   - **paragraph boundaries** — the character offsets where a paragraph starts, the document's own
//     start excluded. Precision, recall and F1 against the truth's offsets.
//   - **headings** — the offsets where a HEADING starts. "Heading/body agreement" is scored as how well
//     the headings are found, because body is everything else and a count of body agreement rewards a
//     proposer that calls everything body.
//
// "No tree" infers nothing, so it scores zero recall on both — asserted, because a metric that did not
// score the absence of structure at zero could not show anything is better than it.
//
// # Why the strip is here and not `dropTaggingClaim`
//
// `dropTaggingClaim`'s own comment says it is called only through `honest`, so the decision to strip a
// document is taken in one place. Building a test corpus is not that decision; a second caller would be
// the drift that comment exists to prevent.

// truthBlock is one paragraph of a document: its text (whitespace removed) and whether it is a heading.
type truthBlock struct {
	heading bool
	text    string
}

// blockScore is precision, recall and F1 of one set of offsets against another.
type blockScore struct {
	precision, recall, f1 float64
}

func (s blockScore) String() string {
	return fmt.Sprintf("P=%.2f R=%.2f F1=%.2f", s.precision, s.recall, s.f1)
}

// truthCorpus is S04's documents: LibreOffice conversions whose source states the structure.
func truthCorpus(t *testing.T) []runCorpusDoc {
	t.Helper()
	if !LibreOfficeAvailable() {
		return nil
	}
	p := func(s string) string { return "<text:p>" + s + "</text:p>" }
	h := func(level int, s string) string {
		return fmt.Sprintf(`<text:h text:outline-level="%d">%s</text:h>`, level, s)
	}
	convert := func(name, styles, body string) runCorpusDoc {
		pdf, err := ConvertOfficeToPDF(odtDocument(t, styles, body), "odt")
		if err != nil {
			t.Fatalf("truth corpus: LibreOffice is present and could not convert %q: %v", name, err)
		}
		return runCorpusDoc{name, pdf}
	}

	lists := convert("headings, paragraphs and a list", "",
		h(1, "Top heading")+
			p("An opening paragraph of body text that is long enough to wrap onto a second line on the page.")+
			h(2, "A second-level heading")+p("Another paragraph.")+
			`<text:list><text:list-item>`+p("first item")+`</text:list-item><text:list-item>`+p("second item")+`</text:list-item></text:list>`+
			p("A closing paragraph."))

	col := strings.TrimSpace(strings.Repeat("Words that fill a narrow column and wrap several times before the paragraph ends. ", 3))
	columns := convert("two columns",
		`<style:style style:name="Sect1" style:family="section"><style:section-properties>`+
			`<style:columns fo:column-count="2" fo:column-gap="0.5in"/></style:section-properties></style:style>`,
		h(1, "Two columns")+`<text:section text:style-name="Sect1" text:name="S1">`+
			p("First "+col)+p("Second "+col)+p("Third "+col)+p("Fourth "+col)+`</text:section>`)

	var report strings.Builder
	report.WriteString(h(1, "Annual report"))
	for s := 1; s <= 6; s++ {
		report.WriteString(h(2, fmt.Sprintf("Section %d", s)))
		for q := 1; q <= 4; q++ {
			report.WriteString(p(fmt.Sprintf("Paragraph %d.%d. ", s, q) + strings.Repeat(
				"The figures in this part of the report are set out in enough prose to fill several lines of the page. ", 4)))
		}
	}
	long := convert("a multi-page report", "", report.String())

	numbered := convert("a numbered list",
		`<text:list-style style:name="L1"><text:list-level-style-number text:level="1" style:num-format="1" style:num-suffix="."/></text:list-style>`,
		h(1, "Steps")+
			p("Follow these steps in order, and read each one through before you begin the next one in the list.")+
			`<text:list text:style-name="L1"><text:list-item>`+p("Open the document.")+`</text:list-item>`+
			`<text:list-item>`+p("Check every page.")+`</text:list-item>`+
			`<text:list-item>`+p("Save a copy.")+`</text:list-item></text:list>`+
			p("That is all."))

	return []runCorpusDoc{lists, columns, long, numbered}
}

// stripStructTree removes the tree and the claim, leaving the page content exactly as it was.
func stripStructTree(t *testing.T, pdf []byte) []byte {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	cat, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	delete(cat, "StructTreeRoot")
	delete(cat, "MarkInfo")
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// standardRole resolves a structure type through the document's role map.
func standardRole(tree *structTree, kind string) string {
	for i := 0; i < 10; i++ {
		next, ok := tree.roleMap[kind]
		if !ok || next == kind {
			return kind
		}
		kind = next
	}
	return kind
}

func isHeadingRole(role string) bool {
	switch role {
	case "H", "H1", "H2", "H3", "H4", "H5", "H6":
		return true
	}
	return false
}

// readTruth reads a tagged document's paragraphs in tree order: a list item is one paragraph (its
// label and body share a line, as S03 groups them), and so is any other element that owns marked
// content directly. Each block's text is the runs drawn under its MCIDs — the run reader's own answer,
// so truth and inference read the same text by construction.
func readTruth(t *testing.T, pdf []byte) []truthBlock {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	live, pageNr := map[int]bool{}, map[int]int{}
	textAt := map[[2]int]string{}
	for pg := 1; pg <= ctx.PageCount; pg++ {
		ir, ierr := ctx.PageDictIndRef(pg)
		if ierr != nil || ir == nil {
			t.Fatalf("page %d has no reference: %v", pg, ierr)
		}
		live[ir.ObjectNumber.Value()] = true
		pageNr[ir.ObjectNumber.Value()] = pg
		pr, perr := readPageRuns(ctx, pg)
		if perr != nil {
			t.Fatal(perr)
		}
		for _, r := range pr.runs {
			if r.mcid >= 0 {
				textAt[[2]int{pg, r.mcid}] += r.text
			}
		}
	}
	tree, err := readStructTree(ctx, live)
	if err != nil {
		t.Fatalf("the truth document has no readable tree: %v", err)
	}
	var collect func(e *structElem) string
	collect = func(e *structElem) string {
		var b strings.Builder
		for _, k := range e.kids {
			switch k.kind {
			case kidMCID, kidMCR:
				b.WriteString(textAt[[2]int{pageNr[k.pgObj], k.mcid}])
			case kidElement:
				if k.elem != nil {
					b.WriteString(collect(k.elem))
				}
			}
		}
		return b.String()
	}
	ownsContent := func(e *structElem) bool {
		for _, k := range e.kids {
			if k.kind == kidMCID || k.kind == kidMCR {
				return true
			}
		}
		return false
	}
	var blocks []truthBlock
	var walk func(e *structElem)
	walk = func(e *structElem) {
		role := standardRole(tree, e.kind)
		if role == "LI" || isHeadingRole(role) || ownsContent(e) {
			if text := squeeze(collect(e)); text != "" {
				blocks = append(blocks, truthBlock{heading: isHeadingRole(role), text: text})
			}
			return
		}
		for _, k := range e.kids {
			if k.kind == kidElement && k.elem != nil {
				walk(k.elem)
			}
		}
	}
	for _, e := range tree.elems {
		if e.parent == nil {
			walk(e)
		}
	}
	return blocks
}

// layoutBlocks is S03's grouping of a document, every paragraph unlabelled — what the proposer starts
// from, and the floor the proposer must beat on headings.
func layoutBlocks(t *testing.T, pdf []byte) []truthBlock {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	var out []truthBlock
	for pg := 1; pg <= ctx.PageCount; pg++ {
		l, lerr := readPageLayout(ctx, pg)
		if lerr != nil {
			t.Fatal(lerr)
		}
		for _, par := range l.paragraphs {
			if text := squeeze(par.text()); text != "" {
				out = append(out, truthBlock{text: text})
			}
		}
	}
	return out
}

// starts returns the offsets where blocks begin — every block, or only headings.
func starts(blocks []truthBlock, headingsOnly bool) map[int]bool {
	out := map[int]bool{}
	off := 0
	for _, b := range blocks {
		if !headingsOnly || b.heading {
			out[off] = true
		}
		off += len(b.text)
	}
	if !headingsOnly {
		delete(out, 0) // the document's own start is not a boundary anybody found
	}
	return out
}

func scoreOffsets(truth, got map[int]bool) blockScore {
	tp := 0
	for o := range got {
		if truth[o] {
			tp++
		}
	}
	var s blockScore
	if len(got) > 0 {
		s.precision = float64(tp) / float64(len(got))
	}
	if len(truth) > 0 {
		s.recall = float64(tp) / float64(len(truth))
	}
	if s.precision+s.recall > 0 {
		s.f1 = 2 * s.precision * s.recall / (s.precision + s.recall)
	}
	return s
}

var errTextsDiffer = errors.New("the two sides do not contain the same text, so their offsets cannot be compared")

// scoreBlocks scores inferred blocks against truth on both numbers. It refuses when the texts differ:
// an offset means nothing once the two sides disagree about the characters it counts.
func scoreBlocks(truth, inferred []truthBlock) (boundaries, headings blockScore, err error) {
	join := func(bs []truthBlock) string {
		var b strings.Builder
		for _, x := range bs {
			b.WriteString(x.text)
		}
		return b.String()
	}
	if join(truth) != join(inferred) {
		return blockScore{}, blockScore{}, errTextsDiffer
	}
	return scoreOffsets(starts(truth, false), starts(inferred, false)),
		scoreOffsets(starts(truth, true), starts(inferred, true)), nil
}

// TestTheMetricScoresAWorkedExample — the arithmetic, by hand, before it is trusted on documents.
func TestTheMetricScoresAWorkedExample(t *testing.T) {
	truth := []truthBlock{{heading: true, text: "A"}, {text: "BC"}, {heading: true, text: "D"}, {text: "E"}}
	got := []truthBlock{{heading: true, text: "AB"}, {text: "C"}, {text: "D"}, {heading: true, text: "E"}}
	// Boundaries: truth {1, 3, 4}, inferred {2, 3, 4} → two hits of three each way.
	// Headings: truth {0, 3}, inferred {0, 4} → one hit of two each way.
	b, h, err := scoreBlocks(truth, got)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(b.precision, 2.0/3) || !approxEqual(b.recall, 2.0/3) {
		t.Errorf("boundaries scored %v, want P=R=2/3", b)
	}
	if h.precision != 0.5 || h.recall != 0.5 {
		t.Errorf("headings scored %v, want P=R=0.5", h)
	}
	// Precision and recall must not be interchangeable: one boundary found of three is P=1, R=1/3.
	few := []truthBlock{{heading: true, text: "ABC"}, {text: "DE"}}
	b, _, err = scoreBlocks(truth, few)
	if err != nil {
		t.Fatal(err)
	}
	if b.precision != 1 || !approxEqual(b.recall, 1.0/3) || !approxEqual(b.f1, 0.5) {
		t.Errorf("one boundary of three scored %v, want P=1 R=1/3 F1=0.5", b)
	}
	if _, _, err := scoreBlocks(truth, []truthBlock{{text: "ABCE"}}); err != errTextsDiffer {
		t.Errorf("different texts scored anyway (err=%v)", err)
	}
}

// TestTheTruthCorpusScoresTheMetricAtItsBounds — S04's acceptance: the truth is a tree and the stripped
// copy has none; the metric scores the truth perfectly, "no tree" at zero recall, and S03's grouping in
// between — on both numbers, per document.
func TestTheTruthCorpusScoresTheMetricAtItsBounds(t *testing.T) {
	corpus := truthCorpus(t)
	if corpus == nil {
		t.Skip("LibreOffice is absent, so there is no document whose structure a producer stated — the metric cannot be measured on this machine")
	}
	// Counted from each document's own source: the truth reader must find exactly what was written, or
	// the metric is scoring against a truth nobody stated.
	shapes := map[string][2]int{
		"headings, paragraphs and a list": {7, 2},  // H1, P, H2, P, two list items, P
		"two columns":                     {5, 1},  // H1 and four paragraphs
		"a multi-page report":             {31, 7}, // H1, six H2, 24 paragraphs
		"a numbered list":                 {6, 1},  // H1, P, three items, P
	}
	for _, doc := range corpus {
		truth := readTruth(t, doc.pdf)
		headings := 0
		for _, b := range truth {
			if b.heading {
				headings++
			}
		}
		if want, ok := shapes[doc.name]; !ok || len(truth) != want[0] || headings != want[1] {
			t.Fatalf("%s: the truth reads %d block(s) with %d heading(s); the source states %v", doc.name, len(truth), headings, shapes[doc.name])
		}
		stripped := stripStructTree(t, doc.pdf)
		sctx, err := api.ReadValidateAndOptimize(bytes.NewReader(stripped), model.NewDefaultConfiguration())
		if err != nil {
			t.Fatal(err)
		}
		if _, terr := readStructTree(sctx, nil); !errors.Is(terr, errNoStructTree) {
			t.Fatalf("%s: the stripped copy still has a tree (err=%v) — the proposer could read the answer", doc.name, terr)
		}

		if b, h, serr := scoreBlocks(truth, truth); serr != nil || b.f1 != 1 || h.f1 != 1 {
			t.Errorf("%s: the truth scored against itself %v / %v (err=%v), want 1.0 on both", doc.name, b, h, serr)
		}

		var all strings.Builder
		for _, b := range truth {
			all.WriteString(b.text)
		}
		if b, h, serr := scoreBlocks(truth, []truthBlock{{text: all.String()}}); serr != nil || b.recall != 0 || h.recall != 0 {
			t.Errorf("%s: no tree scored %v / %v (err=%v), want zero recall on both", doc.name, b, h, serr)
		}

		grouped := layoutBlocks(t, stripped)
		b, h, serr := scoreBlocks(truth, grouped)
		if serr != nil {
			t.Errorf("%s: S03's grouping of the stripped copy cannot be scored: %v\n  truth:   %d blocks\n  grouped: %d blocks", doc.name, serr, len(truth), len(grouped))
			continue
		}
		if b.f1 <= 0 {
			t.Errorf("%s: S03's grouping scored %v on boundaries — it finds no paragraph the producer stated", doc.name, b)
		}
		if h.recall != 0 {
			t.Errorf("%s: unlabelled paragraphs found headings (%v) — the heading number is not measuring labels", doc.name, h)
		}
		t.Logf("%s: %d truth blocks (%d headings), %d grouped; boundaries %v; headings %v", doc.name, len(truth), headings, len(grouped), b, h)
		if math.IsNaN(b.f1) {
			t.Errorf("%s: boundary F1 is NaN", doc.name)
		}
	}
}
