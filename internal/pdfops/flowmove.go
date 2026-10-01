package pdfops

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/contentstream"
	"nib/internal/pdfread"
)

// Moving a paragraph — `PLAN-text-reflow.md` P07.S02.
//
// A move changes WHERE a paragraph is drawn and nothing else, so it is not a re-set: each of its runs keeps its own show
// operator — its codes, its `TJ` kerning, the spacing a justified line was set with — and gains a `Tm` in front that puts
// it `dy` lower in user space. After it, the run's own line matrix is restored, so every relative `Td` that follows lands
// where it did. Nothing between the runs is touched: a colour change, marked content, a `Tc` — they stay where they are, in
// the order they were, and an MCID keeps its glyphs.
//
// What a move cannot do, it refuses (law 3): a run drawn invisibly (`invisible-text` — a search layer belongs over its
// scan), a run drawn inside a form (the page's stream does not hold it), a show that
// relies on where a moved run's glyphs ended (`inline-follower` — a `Tm` resets the text matrix to the line's start), a run
// drawn in a clipping mode (`text-clips` — the clip would move with it), and a matrix it cannot invert or that is not
// finite (`degenerate-state`).

// runMove is one moved run: where it was drawn, the bytes that now draw it there and `dy` lower.
type runMove struct {
	span opSpan
	with []byte
}

// moveRuns plans the move of runs by dy in user space over src, the page stream they were read from — or the cause it
// cannot. The runs must be the page's own (not a form's); it does not check that they form a paragraph.
func moveRuns(src []byte, runs []textRun, dy float64) ([]runMove, string) {
	if !finite(dy) {
		return nil, causeDegenerate
	}
	sorted := append([]textRun(nil), runs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].span.start < sorted[j].span.start })
	spans := make([]opSpan, len(sorted))
	for i, r := range sorted {
		if r.inForm {
			return nil, causeTextInForm
		}
		// Modes 3 and 7 draw nothing: an OCR layer's words, placed over the scan they transcribe. Moving them detaches them
		// from the image they describe, so a move refuses them as P06's edit does (P07 phase-close review).
		if invisibleMode(r.state.tr) {
			return nil, causeInvisible
		}
		// Modes 4-7 add the glyphs' outlines to the clip, which everything drawn after the text object is cut by: moving
		// the glyphs would move that window and change what shows through it.
		if clipMode(r.state.tr) {
			return nil, causeClips
		}
		spans[i] = r.span
	}
	if c := followersReposition(src, spans); c != "" {
		return nil, c
	}
	var out []runMove
	for _, r := range sorted {
		st := r.state
		tm, ok := shiftedTm(st, dy)
		if !ok {
			return nil, causeDegenerate
		}
		show, ok := asTj(src, r.span)
		if !ok {
			return nil, causeMixedContent
		}
		var b strings.Builder
		// Separated on both sides: the stream may run straight into the span (`/F1 10 Tf<01…>Tj` is LibreOffice's), and a
		// `Tm` written against it would read as the operator `TfTm`'s neighbour `Tf1`.
		fmt.Fprintf(&b, " %s Tm %s %s Tm ", matrixOperands(tm), show, matrixOperands(st.tlm))
		out = append(out, runMove{span: r.span, with: []byte(b.String())})
	}
	return out, ""
}

// invisibleMode says whether text rendering mode tr draws nothing: 3 (neither fill nor stroke) and 7 (clip only). The one
// reading P06's edit (`paragraphWords`), a move (`moveRuns`) and a carry (`carryRefusal`) share.
func invisibleMode(tr int) bool { return tr == 3 || tr == 7 }

// clipMode says whether text rendering mode tr adds the glyphs' outlines to the clip (4-7): moved or re-set, the window
// everything after the text object is cut by moves with them. The one reading an edit, a move and a carry share.
func clipMode(tr int) bool { return tr >= 4 && tr <= 7 }

func matrixOperands(m runMatrix) string {
	return fmt.Sprintf("%s %s %s %s %s %s", num(m[0]), num(m[1]), num(m[2]), num(m[3]), num(m[4]), num(m[5]))
}

// asTj rewrites the show operator in span as one that shows the same string without moving to a new line first: `Tj`
// and `TJ` as they are, `'` as `Tj`, and `"` as its word and character spacing set, then `Tj` — the state `"` leaves
// behind is the state it set.
func asTj(src []byte, span opSpan) (string, bool) {
	body := src[span.start:span.end]
	toks := contentstream.Tokenize(body)
	var parts []contentstream.Token
	for _, t := range toks {
		if t.Kind != contentstream.Whitespace {
			parts = append(parts, t)
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	op := string(parts[len(parts)-1].Bytes(body))
	text := func(t contentstream.Token) string { return string(t.Bytes(body)) }
	switch op {
	case "Tj", "TJ":
		return string(body), true
	case "'":
		if len(parts) != 2 {
			return "", false
		}
		return text(parts[0]) + " Tj", true
	case "\"":
		if len(parts) != 4 {
			return "", false
		}
		return text(parts[0]) + " Tw " + text(parts[1]) + " Tc " + text(parts[2]) + " Tj", true
	}
	return "", false
}

// shiftedTm is the text matrix that draws what st's text matrix draws, dy lower in user space — the one shift a move and a
// grown paragraph's new lines share. tm·CTM places the glyphs, so tm′ = tm·CTM·T(0, −dy)·CTM⁻¹: under a flipped or scaled
// CTM it still moves down the PAGE. False when the CTM cannot be inverted or a figure is not finite.
func shiftedTm(st runTextState, dy float64) (runMatrix, bool) { return shiftedTmBy(st, 0, dy) }

// shiftedTmBy is shiftedTm also moved dx along the page — a centred line re-set about its axis (P08.S04).
func shiftedTmBy(st runTextState, dx, dy float64) (runMatrix, bool) {
	inv, ok := st.ctm.inverse()
	if !ok || !finite(dx, dy) || !finite(st.tm[:]...) || !finite(st.tlm[:]...) || !finite(st.ctm[:]...) {
		return runMatrix{}, false
	}
	return st.tm.mul(st.ctm).mul(runTranslate(dx, -dy)).mul(inv), true
}

// inverse is m's inverse, or false when m is singular.
func (m runMatrix) inverse() (runMatrix, bool) {
	det := m[0]*m[3] - m[1]*m[2]
	if det == 0 || !finite(det) {
		return runMatrix{}, false
	}
	a, b, c, d := m[3]/det, -m[1]/det, -m[2]/det, m[0]/det
	return runMatrix{a, b, c, d, -(m[4]*a + m[5]*c), -(m[4]*b + m[5]*d)}, true
}

// followersReposition checks that no show operator outside spans draws straight after one inside them: a moved run ends
// on its line's start (`Tm` sets both matrices), so a show relying on where the run's glyphs ended would land there.
// It returns causeInlineFollower, or "".
func followersReposition(src []byte, spans []opSpan) string {
	inSpan := func(at int) bool {
		i := sort.Search(len(spans), func(i int) bool { return spans[i].start > at }) - 1
		return i >= 0 && at < spans[i].end
	}
	after := false // the last text-positioning event was the end of a moved run
	for _, tk := range contentstream.Tokenize(src) {
		if tk.Kind != contentstream.Operator {
			continue
		}
		if inSpan(tk.Start) {
			after = true
			continue
		}
		switch string(tk.Bytes(src)) {
		case "Tj", "TJ":
			if after {
				return causeInlineFollower
			}
		case "Td", "TD", "Tm", "T*", "'", "\"", "BT", "ET":
			after = false
		}
	}
	return ""
}

// moveParagraph re-draws paragraph pi of the page `dy` lower in user space and returns the page's new content — or the
// cause it cannot.
func moveParagraph(ctx *model.Context, layout pageLayout, pg pdfread.Page, pi int, dy float64) (reflowOutcome, error) {
	if pi < 0 || pi >= len(layout.paragraphs) {
		return reflowOutcome{cause: causeNoParagraph}, nil
	}
	d, err := pg.Dict, pg.Err
	if err != nil || d == nil {
		return reflowOutcome{}, fmt.Errorf("pdfops: page %d does not resolve: %w", pg.Nr, err)
	}
	src, err := pdfread.PageContent(ctx, d, pg.Nr)
	if err != nil {
		return reflowOutcome{}, err
	}
	moves, cause := moveRuns(src, paragraphRunsWithBlanks(layout, pi), dy)
	if cause != "" {
		return reflowOutcome{cause: cause}, nil
	}
	e := contentstream.NewEdit(src)
	for _, m := range moves {
		e.Replace(m.span.start, m.span.end, m.with)
	}
	out, err := e.Apply()
	if err != nil {
		return reflowOutcome{}, err
	}
	return reflowOutcome{content: out}, nil
}

// paragraphRunsWithBlanks is paragraph pi's runs and the blank runs among them — a run showing only white space, which no
// line holds (`joinsALine`) but which sits on one of the paragraph's baselines inside its extent. A producer that sets each
// space as its own show draws them between the words, so they move with the paragraph or the next word reads as a show
// drawn straight after a moved one.
func paragraphRunsWithBlanks(l pageLayout, pi int) []textRun {
	p := l.paragraphs[pi]
	var out []textRun
	for _, ln := range p.lines {
		out = append(out, ln.runs...)
	}
	for _, r := range l.loose {
		if r.artifact {
			continue
		}
		for _, ln := range p.lines {
			em := math.Max(ln.size, r.size)
			if math.Abs(ln.y-r.y) <= 0.3*em && r.x >= ln.x0-em && r.x <= ln.x1+em {
				out = append(out, r)
				break
			}
		}
	}
	return out
}
