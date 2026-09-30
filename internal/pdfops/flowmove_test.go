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

	"nib/internal/pdfread"
)

// moved applies moveParagraph to paragraph pi of page p and returns the written document, or the cause.
func moved(t *testing.T, pdf []byte, p, pi int, dy float64) ([]byte, string) {
	t.Helper()
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pg := pageAt(ctx, nil, p)
	l, err := readPageGlyphLayout(ctx, pg)
	if err != nil {
		t.Fatal(err)
	}
	out, err := moveParagraph(ctx, l, pg, pi, dy)
	if err != nil {
		t.Fatal(err)
	}
	if out.content == nil {
		return nil, out.cause
	}
	if err := setPageContent(ctx, pg.Dict, out.content); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := api.WriteContext(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), ""
}

// runsOf reads page p's runs, glyphs kept, in draw order.
func runsOf(t *testing.T, pdf []byte, p int) []textRun {
	t.Helper()
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pr, err := readPageGlyphRuns(ctx, pageAt(ctx, nil, p))
	if err != nil {
		t.Fatal(err)
	}
	return pr.runs
}

// runKey is what a run draws, apart from where: its text, font, size, advance, codes and marked content.
func runKey(r textRun) string {
	var codes []string
	for _, g := range r.glyphs {
		codes = append(codes, fmt.Sprintf("%X", g.code))
	}
	return fmt.Sprintf("%q %s %v mcid=%d art=%v", r.text, r.font, codes, r.mcid, r.artifact)
}

// closeTo compares user-space figures a move recomputes through the CTM's inverse: equal to a part in a million, or
// within a millionth of a point.
func closeTo(a, b float64) bool {
	return math.Abs(a-b) <= 1e-6*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}

// sameMove checks after against before: every run in movedIdx drawn exactly dy lower, every other run where it was, and
// nothing else changed about any of them. It returns the first difference, or "".
func sameMove(before, after []textRun, movedIdx map[int]bool, dy float64) string {
	if len(before) != len(after) {
		return fmt.Sprintf("%d runs became %d", len(before), len(after))
	}
	for i := range before {
		b, a := before[i], after[i]
		if runKey(b) != runKey(a) || !closeTo(a.size, b.size) || !closeTo(a.width, b.width) {
			return fmt.Sprintf("run %d: %s %v %v became %s %v %v", i, runKey(b), b.size, b.width, runKey(a), a.size, a.width)
		}
		want := b.y
		if movedIdx[i] {
			want -= dy
		}
		if !closeTo(a.x, b.x) || !closeTo(a.y, want) {
			return fmt.Sprintf("run %d %q: at (%v, %v), want (%v, %v)", i, b.text, a.x, a.y, b.x, want)
		}
	}
	return ""
}

// indexOfRuns maps each of a paragraph's runs, and its blanks, to its index in the page's draw order.
func indexOfRuns(all []textRun, moved []textRun) map[int]bool {
	key := func(r textRun) string { return fmt.Sprintf("%d:%d:%d", r.span.start, r.span.end, r.stm) }
	want := map[string]bool{}
	for _, r := range moved {
		want[key(r)] = true
	}
	out := map[int]bool{}
	for i, r := range all {
		if want[key(r)] {
			out[i] = true
		}
	}
	return out
}

// TestAMovedParagraphIsDrawnExactlyLower — `PLAN-text-reflow.md` P07.S02: over the generated and real-producer corpora,
// every paragraph a move accepts reads back with every run — its text, codes, font, size, advance and marked content —
// exactly dy lower, and every other run on the page exactly where it was.
//
// **The stimulus is asserted**: moves are accepted, among them paragraphs of more than one line and paragraphs set with
// `TJ` kerning, so "exactly" was asked of the cases a re-set would have changed.
func TestAMovedParagraphIsDrawnExactlyLower(t *testing.T) {
	const dy = 7.25
	corpora := []lawOneCorpus{
		// Hand-built pages carry a kerned TJ and a space drawn as its own show, so a fresh clone without the external
		// corpus still asks both.
		{name: "generated", docs: append(runCorpus(t),
			runCorpusDoc{"a kerned line", helveticaPage("BT /F1 12 Tf 72 600 Td [(Ju) -120 (stified) 250 (text)] TJ ET")},
			runCorpusDoc{"a space drawn as its own show", helveticaPage("BT /F1 12 Tf 72 600 Td (Blank) Tj ( ) Tj (spaced words) Tj ET")})},
		externalPDFs(t, "real producers", "NIB_UA_PRODUCERS", filepath.Join(homeDir(), "nib", "producers")),
	}
	accepted, multi, kerned, blanks := 0, 0, 0, 0
	refused := map[string]int{}
	for _, corp := range corpora {
		if corp.absent != "" {
			t.Logf("NOTE (a narrower population, not a pass over it): %s: %s", corp.name, corp.absent)
			continue
		}
		for _, doc := range corp.docs {
			ctx, err := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
			if err != nil {
				continue
			}
			for p := 1; p <= ctx.PageCount && p <= 2; p++ {
				pg := pageAt(ctx, nil, p)
				l, err := readPageGlyphLayout(ctx, pg)
				if err != nil {
					continue
				}
				bpr, err := readPageGlyphRuns(ctx, pg)
				if err != nil {
					continue
				}
				before := bpr.runs
				orig := pg.Dict["Contents"]
				for pi := range l.paragraphs {
					// Read back in the same context — the moved content set on the page, walked, and the page's own
					// contents put back — so the census costs a walk per paragraph, not a validation and a write.
					// The written round trip is asserted once, by TestAMoveWritesAndReadsBack.
					o, err := moveParagraph(ctx, l, pg, pi, dy)
					if err != nil {
						t.Fatal(err)
					}
					if o.content == nil {
						refused[o.cause]++
						continue
					}
					if err := setPageContent(ctx, pg.Dict, o.content); err != nil {
						t.Fatal(err)
					}
					apr, err := readPageGlyphRuns(ctx, pageAt(ctx, nil, p))
					pg.Dict["Contents"] = orig
					if err != nil {
						t.Fatal(err)
					}
					accepted++
					if len(l.paragraphs[pi].lines) > 1 {
						multi++
					}
					mv := paragraphRunsWithBlanks(l, pi)
					if kernedRuns(mv) {
						kerned++
					}
					if len(mv) > lineRunCount(l.paragraphs[pi]) {
						blanks++
					}
					if d := sameMove(before, apr.runs, indexOfRuns(before, mv), dy); d != "" {
						t.Errorf("%s p%d ¶%d %q: %s", doc.name, p, pi, l.paragraphs[pi].text(), d)
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
	t.Logf("moved %d paragraphs (%d of several lines, %d kerned, %d carrying blank runs); refused: %s", accepted, multi,
		kerned, blanks, strings.Join(rs, ", "))
	if accepted == 0 || multi == 0 || kerned == 0 || blanks == 0 {
		t.Errorf("accepted %d, multi-line %d, kerned %d, with blanks %d — the move was not asked what a re-set would change",
			accepted, multi, kerned, blanks)
	}
}

func kernedRuns(rs []textRun) bool {
	for _, r := range rs {
		if r.kernAfter != 0 {
			return true
		}
		for _, g := range r.glyphs {
			if g.kern != 0 {
				return true
			}
		}
	}
	return false
}

// TestAMoveWritesAndReadsBack — P07.S02 through the page write door and pdfcpu's writer, on the show operators a move
// rewrites: `'` and `"` (which move to a new line first, and `"` sets spacing), a kerned `TJ`, text under a flipped CTM,
// and a tagged paragraph whose MCID must stay with its glyphs. Each is moved and read back exactly lower, everything else
// where it was.
func TestAMoveWritesAndReadsBack(t *testing.T) {
	const dy = 9.5
	content := strings.Join([]string{
		`q 1 0 0 -1 0 792 cm BT /F1 12 Tf 14 TL 1 0 0 -1 72 92 Tm (Line one of it) Tj (Line two of it) ' 2 0.5 (Line three) " ET Q`,
		`BT /F1 12 Tf 72 600 Td [(Ju) -120 (stified) 250 (text)] TJ ET`,
		`/P <</MCID 0>> BDC BT /F1 12 Tf 72 500 Td (Tagged words) Tj ET EMC`,
	}, "\n")
	pdf := pageWith(content, "", "", "", helvetica)
	l, _ := layoutOf(t, pdf)
	if len(l.paragraphs) != 3 || len(l.paragraphs[0].lines) != 3 {
		t.Fatalf("fixture reads as %d paragraphs, the first of %d lines", len(l.paragraphs), len(l.paragraphs[0].lines))
	}
	before := runsOf(t, pdf, 1)
	for pi, p := range l.paragraphs {
		out, cause := moved(t, pdf, 1, pi, dy)
		if cause != "" {
			t.Errorf("¶%d %q refused: %s", pi, p.text(), cause)
			continue
		}
		if d := sameMove(before, runsOf(t, out, 1), indexOfRuns(before, paragraphRunsWithBlanks(l, pi)), dy); d != "" {
			t.Errorf("¶%d %q: %s", pi, p.text(), d)
		}
	}
}

// TestAMoveRefusesWhatItCannotDo — P07.S02: each refusal names its cause (law 3), and the stimulus for each is present.
func TestAMoveRefusesWhatItCannotDo(t *testing.T) {
	for _, c := range []struct{ name, content, resources, extra, want string }{
		{"a show drawn straight after a moved run", "BT /F1 12 Tf 72 400 Td (Left words) Tj /Artifact BMC (Mark) Tj EMC ET", "", "", causeInlineFollower},
		{"text drawn as a clip", "q BT /F1 12 Tf 5 Tr 72 400 Td (Clipping words) Tj ET 0 0 612 792 re f Q", "", "", causeClips},
		{"text drawn inside a form", "q 1 0 0 1 72 400 cm /Fm1 Do Q", "/XObject << /Fm1 6 0 R >>",
			"<< /Type /XObject /Subtype /Form /BBox [0 0 300 50] /Resources << /Font << /F1 5 0 R >> >> /Length 39 >>\nstream\nBT /F1 12 Tf 0 10 Td (Formed) Tj ET\nendstream", causeTextInForm},
	} {
		t.Run(c.name, func(t *testing.T) {
			var extra []string
			if c.extra != "" {
				extra = append(extra, c.extra)
			}
			pdf := pageWith(c.content, c.resources, "", "", helvetica, extra...)
			l, _ := layoutOf(t, pdf)
			if len(l.paragraphs) == 0 {
				t.Fatal("the fixture draws no paragraph")
			}
			if _, cause := moved(t, pdf, 1, 0, 5); cause != c.want {
				t.Errorf("cause %q, want %q", cause, c.want)
			}
		})
	}
	t.Run("a distance that is not a number", func(t *testing.T) {
		_, runs := layoutOf(t, helveticaPage(reflowPara+" ET"))
		if _, cause := moveRuns(nil, runs, math.NaN()); cause != causeDegenerate {
			t.Errorf("cause %q, want %q", cause, causeDegenerate)
		}
	})
}

func lineRunCount(p textParagraph) int {
	n := 0
	for _, ln := range p.lines {
		n += len(ln.runs)
	}
	return n
}
