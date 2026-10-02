package pdfops

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/contentstream"
	"nib/internal/fontcode"
	"nib/internal/pdfread"
	"nib/mdpdf"
)

// Reflow — `PLAN-text-reflow.md` P06. A paragraph is re-broken in its own font, at its own measure, by the one line
// breaker (`mdpdf.BreakGreedy`, law 4), with widths the document itself carries (D2).

// reflowWord is one word of a paragraph: its glyphs, and its width as drawn — every glyph's advance and every kern
// between them, but not the kern before its first glyph, which belongs to the gap in front of it.
type reflowWord struct {
	glyphs []runGlyph
	// spacing is the text state each glyph was drawn under, by index: a word is drawn across runs of different spacing
	// often enough (Acrobat writes a `Tc` per run, mid-word) that one state per word would re-space its glyphs (P08.S01).
	spacing []textSpacing
	width   float64
	face    *runFont
	size    float64
	scale   float64 // text space → user space along the baseline, as `runTextState.scale`
	// font and tfSize are the `Tf` operands of the run the word began in — what re-emitting it selects.
	font   string
	tfSize float64
	// startX is where the word's first glyph begins, in user space: a line's lead is its first word's startX less its
	// first run's origin, which a re-emitted line must keep.
	startX float64
}

// textSpacing is the part of the text state that sets how a glyph advances and where it sits: character and word spacing,
// horizontal scaling (`Tz`, as a fraction), rise and the rendering mode. A re-set glyph is drawn under the spacing it was
// drawn with, so it advances exactly as it did.
type textSpacing struct {
	tc, tw, th, ts float64
	tr             int
}

func spacingOf(st runTextState) textSpacing {
	return textSpacing{tc: st.tc, tw: st.tw, th: st.th, ts: st.ts, tr: st.tr}
}

// operators writes the operators that take a content stream from text state from to s: only what differs.
func (s textSpacing) operators(from textSpacing) string {
	var b strings.Builder
	if s.tc != from.tc {
		fmt.Fprintf(&b, "%s Tc ", num(s.tc))
	}
	if s.tw != from.tw {
		fmt.Fprintf(&b, "%s Tw ", num(s.tw))
	}
	if s.th != from.th {
		fmt.Fprintf(&b, "%s Tz ", tzOperand(s.th))
	}
	if s.ts != from.ts {
		fmt.Fprintf(&b, "%s Ts ", num(s.ts))
	}
	if s.tr != from.tr {
		fmt.Fprintf(&b, "%d Tr ", s.tr)
	}
	return b.String()
}

// allOperators writes all five, whatever was in force.
func (s textSpacing) allOperators() string {
	return fmt.Sprintf("%s Tc %s Tw %s Tz %s Ts %d Tr ", num(s.tc), num(s.tw), tzOperand(s.th), num(s.ts), s.tr)
}

// tzOperand is the `Tz` operand that sets horizontal scaling th: a percentage, rounded where the multiplication left a
// trace (`110 Tz` reads as 1.1, and 1.1 × 100 is 110.00000000000001).
func tzOperand(th float64) string { return num(math.Round(th*100*1e9) / 1e9) }

// style is what of a glyph's spacing the reader SEES as its look rather than its fit: rise (a superscript), rendering mode
// (a stroked synthetic bold) and horizontal scaling (condensed type). Character and word spacing are fit — a producer
// varies them line by line to fill a measure.
func (s textSpacing) style() textSpacing { return textSpacing{th: s.th, ts: s.ts, tr: s.tr} }

func (w reflowWord) text() string {
	var s string
	for _, g := range w.glyphs {
		s += g.text
	}
	return s
}

// wordGapEm is the smallest gap between two runs on a line, in ems of the larger of the line so far and the later run,
// that separates two words. Below it the runs are one word drawn in pieces (a kerned producer, a change of style inside a
// word). Grouping reads a paragraph's text by the same rule (`separatesWords`), so the text the user edits and the
// words reflow keeps are cut in the same places.
const wordGapEm = 0.15

// separatesWords is the one rule for "this gap between two runs on a line is a space": wider than wordGapEm of em.
func separatesWords(gap, em float64) bool { return gap > wordGapEm*em }

// paragraphWords cuts a paragraph read with its glyphs into its words, line by line, and measures the paragraph's space.
//
// A word ends at a space glyph, at a gap between runs wider than wordGapEm, and at the end of a line. A narrower gap
// between runs of the SAME font continues the word, carried as a kern on its next glyph; a word whose runs change font
// or size is `styled-word` (P08's typographic fidelity). The paragraph's space is the MEDIAN of the gaps it draws BETWEEN
// words — a space glyph's own kern and advance, or the gap between runs — never a gap before a line's first word, which
// is an indent (kept per line: see lineMeasures). The cause is non-empty when the paragraph cannot be reflowed, and names
// why (law 3).
func paragraphWords(p textParagraph) (lines [][]reflowWord, space float64, cause string) {
	var gaps []float64
	for _, l := range p.lines {
		var line []reflowWord
		var cur *reflowWord
		lineEm := 0.0 // the largest run so far on the line, as grouping measures it
		end := func() {
			if cur != nil && len(cur.glyphs) > 0 {
				line = append(line, *cur)
			}
			cur = nil
		}
		for ri, r := range l.runs {
			switch {
			case r.inForm:
				return nil, 0, causeTextInForm
			case r.rotated:
				return nil, 0, causeRotated
			case r.widthSrc == widthNone:
				return nil, 0, causeNoWidths
			case !r.decoded:
				return nil, 0, causeUndecoded
			case len(r.glyphs) != r.codes:
				return nil, 0, causeGlyphsNotKept
			case r.face != nil && r.face.vertical:
				return nil, 0, causeVertical
			case r.replaced:
				return nil, 0, causeReplacementText
			case invisibleMode(r.state.tr):
				return nil, 0, causeInvisible
			case clipMode(r.state.tr):
				// The glyphs' outlines join the clip: re-set, the window everything after them is cut by changes.
				return nil, 0, causeClips
			case math.Abs(r.state.scale-p.lines[0].runs[0].state.scale) > 1e-9:
				// Each glyph keeps its own spacing (P08.S01); a text matrix's SCALE — a size or stretch set by `Tm` — converts
				// every kern and lead the rewrite writes, and is not carried.
				return nil, 0, causeMixedState
			}
			joinGap := 0.0
			em := math.Max(lineEm, r.size)
			lineEm = em
			if ri > 0 {
				prev := l.runs[ri-1]
				gap := r.x - (prev.x + prev.width)
				switch {
				case separatesWords(gap, em):
					if cur != nil || len(line) > 0 {
						end()
						gaps = append(gaps, gap)
					}
				case cur != nil && len(r.glyphs) > 0 && r.glyphs[0].text != " " && (r.font != cur.font || r.state.tfSize != cur.tfSize):
					// Only a run that CONTINUES the word: one opening with a space ends the word, and a change of font
					// at a word boundary is an ordinary bold or italic word.
					return nil, 0, causeStyledWord
				default:
					joinGap = gap
				}
			}
			pos := r.x
			for gi, g := range r.glyphs {
				pos += g.kern
				start := pos
				pos += g.advance
				if g.text == " " {
					if cur != nil || len(line) > 0 {
						gaps = append(gaps, g.kern+g.advance)
					}
					end()
					continue
				}
				if cur == nil {
					cur = &reflowWord{face: r.face, size: r.size, scale: r.state.scale, font: r.font, tfSize: r.state.tfSize, startX: start}
					cur.glyphs = append(cur.glyphs, g)
					cur.spacing = append(cur.spacing, spacingOf(r.state))
					cur.width += g.advance
					continue
				}
				if gi == 0 {
					g.kern += joinGap // the gap between two runs of one word, kept inside it
				}
				cur.glyphs = append(cur.glyphs, g)
				cur.spacing = append(cur.spacing, spacingOf(r.state))
				cur.width += g.kern + g.advance
			}
		}
		end()
		if len(line) == 0 {
			return nil, 0, causeEmptyLine
		}
		lines = append(lines, line)
	}
	if len(gaps) == 0 {
		return lines, 0, causeNoSpaceWidth
	}
	sort.Float64s(gaps)
	return lines, gaps[len(gaps)/2], ""
}

// wordSpacer is the space a paragraph sets after a word: by its last glyph's font, size and spacing.
type wordSpacer func(face *runFont, tfSize float64, after textSpacing) float64

// spacerOf is the space a paragraph sets after a word: the font's own space glyph, advanced under the spacing that word
// ends in — so a line under `Tw 2` or `Tz 110` keeps its wider spaces, and a typed word's is the space of the spacing it
// is drawn in — plus the paragraph's MEDIAN of what each gap it drew added to that natural space (a producer that
// positions its words rather than drawing a space, or adjusts the space it draws). One wide gap moves no other: the
// residual is the paragraph's, and a spacing's own gaps are never its only evidence (P08.S01's review, R2-2). Where
// the font draws no space it can measure, the paragraph's median gap, fallback.
func spacerOf(lines [][]reflowWord, fallback float64) wordSpacer {
	scale := lines[0][0].scale
	residual := spaceResiduals(lines)
	if len(residual) == 0 {
		return func(*runFont, float64, textSpacing) float64 { return fallback }
	}
	extra := median(residual)
	return func(f *runFont, tfSize float64, after textSpacing) float64 {
		if nat, ok := naturalSpace(f, tfSize, after, scale); ok {
			return nat + extra
		}
		return fallback
	}
}

// naturalSpace is the user-space advance of font f's own space glyph at tfSize under spacing s, or false when the font
// draws no space it can measure.
func naturalSpace(f *runFont, tfSize float64, s textSpacing, scale float64) (float64, bool) {
	codes := f.codesFor(" ")
	if len(codes) == 0 {
		return 0, false
	}
	w0, src := f.widths.advance(fontcode.Value(codes[0]))
	if src == widthNone {
		return 0, false
	}
	tx := w0/1000*tfSize + s.tc
	if len(codes[0]) == 1 && codes[0][0] == ' ' {
		tx += s.tw // word spacing applies to the single-byte code 32 alone
	}
	return tx * s.th * scale, true
}

// spaceResiduals is what each gap lines draw between words added to the natural space after the word before it.
func spaceResiduals(lines [][]reflowWord) []float64 {
	var out []float64
	for _, l := range lines {
		for i := 1; i < len(l); i++ {
			prev := l[i-1]
			if nat, ok := naturalSpace(prev.face, prev.tfSize, lastSpacing(prev.spacing), prev.scale); ok {
				out = append(out, l[i].startX-prev.startX-prev.width-nat)
			}
		}
	}
	return out
}

// wordAlignment is an edit aligned to its paragraph word by word: the longest common subsequence of the two, kept is one
// such alignment (each kept word of the edit → the paragraph word it is), and the two tables say what EVERY longest one
// does — an edit is often explained equally well by two, and which occurrence a word is then is a guess.
type wordAlignment struct {
	kept     map[int]int
	old, new []string
	f, b     []uint16 // f[i][j] = LCS(old[:i], new[:j]); b[i][j] = LCS(old[i:], new[j:]); rows of len(new)+1
	ok       bool     // false past the size bound: nothing is kept, and nothing can be vouched for
}

// alignment is how a paragraph's lines sit in their measure (P08.S03): justified — every line but the last set from the
// left edge to the flush edge — or not. S04 adds centred and right.
type alignment struct {
	justified bool
	edge      float64 // where a justified paragraph's lines end, in user space
	tol       float64 // how far apart line ends may be and still be the edge: justifyTol em
	continues bool    // its last line ends at the edge too: the paragraph runs on past it, and that line is set flush
	// centred is a paragraph whose every line is centred on axis — its column's centre, or on a single-column page the
	// page's (P08.S04); width is the most a line may take about it.
	centred     bool
	axis, width float64
	// lastDelta is what a justified paragraph's last line adds to each space the spacer gives it: the spacer's residual is
	// the median over every line, stretched ones included, and the last line is the one set at its natural spaces — the
	// residual of the original last line (0, the font's own space, where it has one word).
	lastDelta float64
}

// justifyTol is how far apart, in ems, two line ends may be and still be one edge.
const justifyTol = 0.05

// paragraphAlignment is the one door for how paragraph pi's lines (read by `paragraphWords`) are aligned. Justified when
// two or more lines before the last end within justifyTol em of each other and the lines after the first begin together;
// or — two lines, the most a paragraph of two can show — when the first ends at its column's right edge, the last ends
// short of it, and the first is set looser than the last (by median gap). A paragraph of one line says nothing of
// justification. A paragraph that is not justified may be centred (`centredAlignment`).
func paragraphAlignment(l pageLayout, pi int, lines [][]reflowWord) alignment {
	if len(lines) == 0 {
		return alignment{}
	}
	p := l.paragraphs[pi]
	tol := justifyTol * p.lines[0].size
	var lefts, rights []float64
	for _, ln := range lines {
		end := ln[len(ln)-1]
		lefts, rights = append(lefts, ln[0].startX), append(rights, end.startX+end.width)
	}
	if len(lines) == 1 {
		return centredAlignment(l, pi, lefts, rights) // one line says nothing of justification
	}
	spread := func(v []float64) float64 {
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, x := range v {
			lo, hi = math.Min(lo, x), math.Max(hi, x)
		}
		return hi - lo
	}
	body := len(lines) - 1
	em := p.lines[0].size
	// usual is the gaps of a line that are its ordinary spaces: within 1.5 em of its narrowest — a wider one is a jump (a
	// label to its value, a tab to a page number), and a producer's wider space after a sentence is within it.
	usual := func(g []float64) []float64 {
		if len(g) == 0 {
			return nil
		}
		lo := slices.Min(g)
		var out []float64
		for _, x := range g {
			if x <= lo+1.5*em {
				out = append(out, x)
			}
		}
		return out
	}
	justified := func(edge float64) alignment {
		a := alignment{justified: true, edge: edge, tol: tol}
		// A paragraph whose last line ALSO ends at the edge continues past this page or column: that line is justified too,
		// and set to the edge like the rest (the review of S03, R2-W1).
		a.continues = math.Abs(rights[body]-edge) <= tol && len(lines[body]) > 1
		all := spaceResiduals(lines)
		if len(all) == 0 {
			return a
		}
		// The natural space is the last line's — its usual gaps' median — or, continuing, the tightest line's.
		natural := math.Inf(1)
		for i, ln := range lines {
			if (i == body || a.continues) && len(ln) > 1 {
				if r := usual(spaceResiduals(lines[i : i+1])); len(r) > 0 {
					natural = math.Min(natural, median(r))
				}
			}
		}
		if !math.IsInf(natural, 0) {
			a.lastDelta = natural - median(all)
		} else {
			a.lastDelta = -median(all)
		}
		return a
	}
	// Justification stretches a line's spaces alike; a line that ends at the edge by one wide gap — a tab to a page number,
	// a label to its value — is a table row, not a justified line. A gap after a sentence's or a clause's punctuation is
	// left out: a producer may widen the space there (70 of the corpus's justified paragraphs do; the review of S03, R2-C1),
	// but a leader of dots is not punctuation.
	even := func(ln []reflowWord) bool {
		var g []float64
		for i := 1; i < len(ln); i++ {
			t := ln[i-1].text()
			if strings.ContainsAny(t[len(t)-1:], ".:;?!,)") && !strings.HasSuffix(t, "..") {
				continue
			}
			g = append(g, ln[i].startX-ln[i-1].startX-ln[i-1].width)
		}
		return len(g) < 2 || spread(g) <= 5*tol
	}
	evenLines := true
	for _, ln := range lines[:body] {
		evenLines = evenLines && even(ln)
	}
	if body >= 2 && spread(rights[:body]) <= tol && spread(lefts[1:]) <= tol && evenLines {
		return justified(median(rights[:body]))
	}
	if body != 1 || len(lines[0]) < 2 || len(lines[1]) < 2 {
		return centredAlignment(l, pi, lefts, rights)
	}
	right := math.Inf(-1)
	for _, q := range l.paragraphs {
		if q.column == p.column {
			right = math.Max(right, paragraphBox(q)[2])
		}
	}
	// The MEDIAN gap: one wide gap (a label's jump to its value) does not make a line looser.
	gap := func(ln []reflowWord) float64 {
		var g []float64
		for i := 1; i < len(ln); i++ {
			g = append(g, ln[i].startX-ln[i-1].startX-ln[i-1].width)
		}
		return median(g)
	}
	if math.Abs(rights[0]-right) <= tol && rights[1] < rights[0]-10*tol && gap(lines[0]) > gap(lines[1])+tol {
		return justified(rights[0])
	}
	return centredAlignment(l, pi, lefts, rights)
}

// centredAlignment reads a paragraph as centred when EVERY line's middle lies within half a point of one axis — its
// column's centre, or, on a single-column page, the page's (on a page of several the page's centre is a gutter) — and
// every line takes no more than 70% of the column's width (which keeps it well in from the margin, since its middle is on
// the axis). That is what tells a centred heading from a full line whose middle happens to fall there; measured over the
// real-producer corpus they leave titles, headings, a LaTeX title block and centred table cells. RIGHT alignment is not
// read: a one-line paragraph ending at its column's right edge is, in the corpus, a contents row ending in a page number
// (Antenna House: every one), and lines set flush right with ragged starts are split into paragraphs of their own.
func centredAlignment(l pageLayout, pi int, lefts, rights []float64) alignment {
	p := l.paragraphs[pi]
	cl, cr := math.Inf(1), math.Inf(-1)
	for _, q := range l.paragraphs {
		if q.column == p.column {
			b := paragraphBox(q)
			cl, cr = math.Min(cl, b[0]), math.Max(cr, b[2])
		}
	}
	// A line that begins where other lines of its column begin is set from that edge, whatever its middle: two others
	// starting within half a point is a left edge (a body line on a page grouped as one column read as centred: the
	// review of P08.S04).
	for i := range p.lines {
		shared := 0
		for qi, q := range l.paragraphs {
			for _, ln := range q.lines {
				if qi != pi && q.column == p.column && math.Abs(ln.x0-p.lines[i].x0) <= 0.5 {
					shared++
				}
			}
		}
		if shared >= 2 {
			return alignment{}
		}
	}
	axes := []float64{(cl + cr) / 2}
	if l.columns == 1 && l.box[2] > l.box[0] {
		axes = append(axes, (l.box[0]+l.box[2])/2)
	}
	for _, axis := range axes {
		ok := true
		for i := range lefts {
			ok = ok && math.Abs((lefts[i]+rights[i])/2-axis) <= 0.5 && rights[i]-lefts[i] <= 0.7*(cr-cl)
		}
		if ok {
			// A line may take as much as fits on both sides of the axis: within its column, and short of anything drawn
			// beside it on either side.
			left, right := roomOnItsLine(l, pi)
			return alignment{centred: true, axis: axis, width: 2 * math.Min(axis-left, right-axis)}
		}
	}
	return alignment{}
}

// justifiedLines is the extra space each gap of each broken line takes so the line ends at a justified paragraph's flush
// edge: every line but the last, its slack shared evenly over its spaces, shrinking as well as stretching; the last line
// its natural spaces (`lastDelta`). fits is false when some line cannot be set within the edge — a line of one word wider
// than it, a last line wider than it at its natural spaces, or a line whose spaces would have to fall under a quarter of
// their natural width to reach it. lines are the paragraph as read, for where each line begins (a new line past them
// begins where the last did).
func justifiedLines(a alignment, broken [][]emitWord, lines [][]reflowWord, space wordSpacer) (out []float64, fits bool) {
	out = make([]float64, len(broken))
	if !a.justified {
		return out, true
	}
	last := len(broken) - 1
	if !a.continues {
		out[last] = a.lastDelta
	}
	for i, l := range broken {
		x := lines[min(i, len(lines)-1)][0].startX
		var gaps []float64
		for wi, w := range l {
			if wi > 0 {
				g := space(l[wi-1].face, l[wi-1].tfSize, lastSpacing(l[wi-1].spacing))
				if i == last && !a.continues {
					g += a.lastDelta
				}
				gaps = append(gaps, g)
				x += g
			}
			x += w.width
		}
		slack := a.edge - x
		switch {
		case (i == last && !a.continues) || len(gaps) == 0:
			// Within the paragraph's own tolerance of the edge is AT it: the lines it was read from end that far apart.
			if slack < -a.tol-measureSlack {
				return out, false
			}
		default:
			// A quarter of the NATURAL space — the spacer's gap carries the paragraph's median stretch; lastDelta takes it off.
			per := slack / float64(len(gaps))
			for _, g := range gaps {
				if g+per < (g+a.lastDelta)/4 {
					return out, false
				}
			}
			out[i] = per
		}
	}
	return out, true
}

// withWordSpacing is lines with every glyph's word spacing set to tw — a copy; the lines read are not changed.
func withWordSpacing(lines [][]reflowWord, tw float64) [][]reflowWord {
	out := make([][]reflowWord, len(lines))
	for i, l := range lines {
		out[i] = make([]reflowWord, len(l))
		for j, w := range l {
			sp := append([]textSpacing(nil), w.spacing...)
			for k := range sp {
				sp[k].tw = tw
			}
			w.spacing = sp
			out[i][j] = w
		}
	}
	return out
}

// alignWords aligns an edit's words to the paragraph's. O(old × new) in time and in two tables of uint16s — a page's
// paragraph is hundreds of words; past 4M cells (a paragraph and its edit both past some 2,000 words, measured at 57 ms)
// nothing is aligned, and a word drawn in two looks is refused rather than guessed.
func alignWords(old, edited []string) wordAlignment {
	al := wordAlignment{kept: map[int]int{}, old: old, new: edited}
	n, m := len(old), len(edited)
	if (n+1)*(m+1) > 1<<22 || n >= 1<<16 || m >= 1<<16 {
		return al
	}
	w := m + 1
	al.f, al.b, al.ok = make([]uint16, (n+1)*w), make([]uint16, (n+1)*w), true
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			if old[i-1] == edited[j-1] {
				al.f[i*w+j] = al.f[(i-1)*w+j-1] + 1
			} else {
				al.f[i*w+j] = max(al.f[(i-1)*w+j], al.f[i*w+j-1])
			}
		}
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if old[i] == edited[j] {
				al.b[i*w+j] = al.b[(i+1)*w+j+1] + 1
			} else {
				al.b[i*w+j] = max(al.b[(i+1)*w+j], al.b[i*w+j+1])
			}
		}
	}
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case old[i] == edited[j]:
			al.kept[j] = i
			i, j = i+1, j+1
		case al.b[(i+1)*w+j] >= al.b[i*w+j+1]:
			i++
		default:
			j++
		}
	}
	return al
}

// choices is what every longest alignment says of the edit's word j: the paragraph words some longest alignment makes it,
// and whether some longest alignment leaves it unkept (typed, or moved). Only meaningful when ok.
func (al wordAlignment) choices(j int) (olds []int, unkept bool) {
	w := len(al.new) + 1
	best := al.b[0]
	for i := range al.old {
		if al.old[i] == al.new[j] && al.f[i*w+j]+1+al.b[(i+1)*w+j+1] == best {
			olds = append(olds, i)
		}
	}
	for i := 0; i <= len(al.old); i++ {
		if al.f[i*w+j]+al.b[i*w+j+1] == best {
			unkept = true
			break
		}
	}
	return olds, unkept
}

// lastSpacing is the spacing a word's last glyph was drawn under — the one the space after it is drawn and measured in.
func lastSpacing(sp []textSpacing) textSpacing { return sp[len(sp)-1] }

// lineMeasures is the room each of the paragraph's lines had: from where its first word begins to the paragraph's right
// edge — the furthest any of its lines reaches, set with its own spaces. A breaker that set a line narrower than its
// room would have fitted the next word, so the right edge can have been no nearer; a first-line indent has less room on
// its line, and that is the difference between this and one measure for every line.
func lineMeasures(lines [][]reflowWord, space wordSpacer) []float64 {
	right := math.Inf(-1)
	for _, l := range lines {
		w := l[0].startX
		for i, word := range l {
			if i > 0 {
				w += space(l[i-1].face, l[i-1].tfSize, lastSpacing(l[i-1].spacing))
			}
			w += word.width
		}
		right = math.Max(right, w)
	}
	out := make([]float64, len(lines))
	for i, l := range lines {
		out[i] = right - l[0].startX
	}
	return out
}

// paragraphMeasures is the room each line of paragraph pi of l has — the one measure the rewrite breaks at and every
// check of its breaks re-derives. A paragraph of ONE line carries no record of its measure (its own width is only where its
// words ended), so it was set in its column's: the furthest any line of the column reaches. Wrapped at its own width, a
// grown one-liner would break short, and a short line followed by one whose first word would have fitted reads back as two
// paragraphs. It stops an em short of anything drawn to its right on its line — and of any annotation there: a form label
// re-set at its column's measure ran under the field beside it (P07 phase-close review), and `annotatedOver` asks only
// about the box the paragraph HAD.
func paragraphMeasures(l pageLayout, pi int, lines [][]reflowWord, space wordSpacer) []float64 {
	measures := lineMeasures(lines, space)
	if len(lines) == 1 {
		_, right := roomOnItsLine(l, pi)
		measures[0] = math.Max(measures[0], right-lines[0][0].startX)
	}
	return measures
}

// roomOnItsLine is how far a one-line paragraph may reach on each side: its column's edges, and never into something
// drawn beside it — a form label's line ends where its field or the next label begins, and a table cell where the next
// cell does, and its column's edge says nothing about either. An em short of the nearest thing on its line, each side.
// The one door for "beside" (`paragraphMeasures` asks the right, a centred line both: P08.S04's review).
func roomOnItsLine(l pageLayout, pi int) (left, right float64) {
	p := l.paragraphs[pi]
	left, right = math.Inf(1), math.Inf(-1)
	for _, q := range l.paragraphs {
		if q.column == p.column {
			b := paragraphBox(q)
			left, right = math.Min(left, b[0]), math.Max(right, b[2])
		}
	}
	own, em := paragraphBox(p), p.lines[0].size
	beside := func(b [4]float64) {
		if b[1] < own[3] && own[1] < b[3] {
			switch {
			case b[0] >= own[2]-1e-9:
				right = math.Min(right, b[0]-em)
			case b[2] <= own[0]+1e-9:
				left = math.Max(left, b[2]+em)
			}
		}
	}
	for _, o := range obstaclesOf(l, map[int]bool{pi: true}) {
		beside(o.box)
	}
	for _, a := range l.annots {
		beside(a)
	}
	return left, right
}

// measureSlack absorbs the arithmetic in comparing a sum against the maximum of the same sums.
const measureSlack = 1e-6

// breakAt is BreakGreedy with a line's room by index — past the last measured line, the last line's — and the space after
// an item by the item.
func breakAt[T any](items []T, measures []float64, width func(T) float64, space func(prev T) float64) [][]T {
	return mdpdf.BreakGreedy(items, 0, mdpdf.BreakOps[T]{
		Width: width,
		Space: space,
		LineWidth: func(line int) float64 {
			if line >= len(measures) {
				line = len(measures) - 1
			}
			return measures[line] + measureSlack
		},
	})
}

// reflowWord's emission needs the run a word came from: its font resource name and text state. New words take the
// paragraph's first run's.

// A reflow refusal names its cause (law 3). Each is a fallback to cover-and-replace, never an error.
const (
	reflowNoChange       = ""                 // the text is the paragraph's own: nothing to do (law 1)
	causeMissingGlyph    = "missing-glyph"    // the font carries no code for a character (D8)
	causeMixedState      = "mixed-state"      // runs differ in their text matrix's scale — a size or stretch set by `Tm`, not `Tf`/`Tz` (P08.S06)
	causeMixedContent    = "mixed-content"    // something other than positioning sits among its show operators
	causeTextObjects     = "text-objects"     // its lines are drawn as separate text objects (BT … ET each)
	causeInlineFollower  = "inline-follower"  // text is drawn straight after it with no repositioning
	causeTagged          = "tagged"           // marked content sits among its lines; moving text would mis-tag it (P07)
	causeReplacementText = "replacement-text" // the paragraph sits inside /ActualText or /Alt, which would keep the old words readable
	causeNoSpaceGlyph    = "no-space-glyph"   // the font draws no space, and a gap with no glyph is invisible to every reader of the text
	causePageFull        = "page-full"        // it needs more room than lies free below it on the page (P07.S06 flows it on)
	causeNoPitch         = "no-pitch"         // it grows, and neither it nor its column says how far apart its lines are set
	causeAnchored        = "anchored"         // an annotation, link, widget, flag, bookmark or drawing sits where the text would move
	causeWordTooWide     = "word-too-wide"    // a word is wider than the paragraph's measure
	causeNoParagraph     = "no-such-paragraph"
	causeEmpty           = "empty-text"       // the new text has no words: deleting a paragraph is not a reflow
	causeAmbiguousStyle  = "ambiguous-style"  // a word of the new text is drawn in two styles in the paragraph
	causeVertical        = "vertical"         // the font writes vertically (Identity-V); lines are measured across
	causeInvisible       = "invisible-text"   // drawn invisibly (Tr 3 or 7): a search layer, whose words the page does not show
	causeTextInForm      = "text-in-form"     // drawn inside a form XObject, not in the page's own content
	causeRotated         = "rotated"          // the baseline is turned
	causeNoWidths        = "no-widths"        // a glyph's width has no source (law 2)
	causeUndecoded       = "undecoded"        // a code decodes to no text, so the words cannot be read
	causeGlyphsNotKept   = "glyphs-not-kept"  // the run's glyphs could not be read one by one
	causeStyledWord      = "styled-word"      // a word changes font or size part-way through (P08)
	causeEmptyLine       = "empty-line"       // a line of the paragraph draws no word
	causeNoSpaceWidth    = "no-space-width"   // the paragraph draws no space between words to measure one by
	causeDegenerate      = "degenerate-state" // a zero or infinite scale, size or coordinate
	causeClips           = "text-clips"       // drawn in a clipping mode (Tr 4-7): re-set or moved, the clip it makes changes
)

// ReflowCauses is every cause a reflow can fall back on, the server's included — the list the editor must have a
// sentence for. `TestEveryReflowCauseIsSaidToTheUser` holds `web/app.js`'s REFLOW_CAUSES to it.
var ReflowCauses = []string{
	causeMissingGlyph, causeMixedState, causeMixedContent, causeTextObjects, causeInlineFollower, causeTagged, causeReplacementText,
	causeNoSpaceGlyph, causePageFull, causeNoPitch, causeAnchored, causeWordTooWide, causeNoParagraph, causeEmpty, causeAmbiguousStyle, causeVertical,
	causeInvisible, causeTextInForm, causeRotated, causeNoWidths, causeUndecoded, causeGlyphsNotKept, causeStyledWord,
	causeEmptyLine, causeNoSpaceWidth, causeDegenerate, causeClips, causeStateNotCarried, causeTaggedAcross, ReflowCauseSigned, ReflowCauseInvalidOutput,
}

// editWords cuts text as a user typed it into words: at white space, but never at a no-break space, which is part of
// the word it sits in — exactly as `paragraphWords` ends a word only at a space glyph.
func editWords(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) && r != '\u00a0' && r != '\u2007' && r != '\u202f'
	})
}

// normalizedText is text as a word sequence — the one comparison both the staleness check and "nothing changed" make.
func normalizedText(s string) string { return strings.Join(editWords(s), " ") }

// isSubset reports a subset font by its tag (ISO 32000-1 §9.6.4: six upper-case letters and a plus sign). A subset
// embeds only the glyphs its document drew, so a code its encoding names is no promise the glyph is there.
func isSubset(baseFont string) bool {
	if len(baseFont) < 8 || baseFont[6] != '+' {
		return false
	}
	for i := 0; i < 6; i++ {
		if baseFont[i] < 'A' || baseFont[i] > 'Z' {
			return false
		}
	}
	return true
}

// finite reports whether every value is a real number: an operand of 300 digits parses finite and can multiply to
// infinity, and a content stream has no spelling for either.
func finite(vs ...float64) bool {
	for _, v := range vs {
		if math.IsInf(v, 0) || math.IsNaN(v) {
			return false
		}
	}
	return true
}

// paragraphRefusal is the one door for "why this paragraph cannot be reflowed" before any text is typed: what its
// words say (`paragraphWords`), then replacement text on the structure element its marked content belongs to. Both
// routes ask it, so the dialog refuses up front exactly what the rewrite would.
func paragraphRefusal(ctx *model.Context, pg pdfread.Page, p textParagraph) string {
	if _, _, cause := paragraphWords(p); cause != "" {
		return cause
	}
	if structReplacesText(ctx, pg, p) {
		return causeReplacementText
	}
	return ""
}

// structReplacesText reports whether any of the paragraph's marked content belongs to a structure element — or an
// ancestor of one — carrying `/ActualText`, `/Alt` or `/E`: the tree then reports that text for these glyphs, and
// rewriting the glyphs would leave the old words there. Only the page's own stream is asked (a run in a form is
// refused before this), through the page's `/StructParents` row of the `/ParentTree`.
func structReplacesText(ctx *model.Context, pg pdfread.Page, p textParagraph) bool {
	var mcids []int
	seen := map[int]bool{}
	for _, l := range p.lines {
		for _, r := range l.runs {
			if r.mcid >= 0 && r.stm == 0 && !seen[r.mcid] {
				seen[r.mcid] = true
				mcids = append(mcids, r.mcid)
			}
		}
	}
	if len(mcids) == 0 {
		return false
	}
	xt := ctx.XRefTable
	pd, err := pg.Dict, pg.Err
	if err != nil || pd == nil {
		return false
	}
	key, ok, _ := structParentsOf(xt, pd)
	if !ok {
		return false
	}
	cat, err := xt.Catalog()
	if err != nil {
		return false
	}
	root := derefDict(xt, cat["StructTreeRoot"])
	if root == nil {
		return false
	}
	row, _, found := rowFor(ctx, &structTree{root: root}, key)
	if !found {
		return false
	}
	replaces := func(d types.Dict) bool {
		_, a := d["ActualText"]
		_, b := d["Alt"]
		_, e := d["E"]
		return a || b || e
	}
	for _, m := range mcids {
		if m >= len(row) {
			continue
		}
		// Up the /P chain to the root, bounded: a cycle in a hostile tree must not hang the request.
		o := row[m]
		for depth := 0; depth < 256; depth++ {
			d := derefDict(xt, o)
			if d == nil {
				break
			}
			if t := d.NameEntry("Type"); t != nil && *t == "StructTreeRoot" {
				break
			}
			if replaces(d) {
				return true
			}
			o = d["P"]
		}
	}
	return false
}

// reflowOutcome is what a reflow did: the rewritten page's content, or the cause it fell back on.
type reflowOutcome struct {
	content []byte // the page's new content; nil when nothing was written
	cause   string // why not, when content is nil and the text changed
	// below is the text of the paragraph below that refused to move, when the refusal is its — so the user is told WHICH
	// paragraph stands in the way, not only why (P07.S03).
	below string
	// flow is what else a grown paragraph moves — the pages after this one, and what is anchored on each (P07.S04, S06) —
	// applied to the document by the door. flow[0] is this page, whose content is `content`.
	flow []flowStep
}

// emitWord is a word as it will be drawn: its codes, the kern before each after the first (user space), the spacing each
// is drawn under, its font.
type emitWord struct {
	face    *runFont
	codes   [][]byte
	kerns   []float64
	spacing []textSpacing
	width   float64
	font    string
	tfSize  float64
}

// reflowParagraph re-sets paragraph pi of page pageNr as text, in the paragraph's own font, at its own measure and
// baselines, and returns the page's rewritten content — `PLAN-text-reflow.md` P06.S04. The original show operators are
// DELETED, so the original words are gone from the content rather than covered. Anything the rewrite cannot do exactly
// is a named cause and no content (law 3).
func reflowParagraph(ctx *model.Context, pageNr, pi int, text string) (reflowOutcome, error) {
	pg := pageAt(ctx, nil, pageNr)
	layout, err := readPageGlyphLayout(ctx, pg)
	if err != nil {
		return reflowOutcome{}, err
	}
	return reflowParagraphIn(ctx, layout, pg, pi, text)
}

// reflowParagraphIn is reflowParagraph over a layout the caller already read — the door reads one to check staleness.
func reflowParagraphIn(ctx *model.Context, layout pageLayout, pg pdfread.Page, pi int, text string) (reflowOutcome, error) {
	pageNr := pg.Nr
	d, err := pg.Dict, pg.Err
	if err != nil || d == nil {
		return reflowOutcome{}, fmt.Errorf("pdfops: page %d does not resolve: %w", pageNr, err)
	}
	if pi < 0 || pi >= len(layout.paragraphs) {
		return reflowOutcome{cause: causeNoParagraph}, nil
	}
	para := layout.paragraphs[pi]
	if cause := paragraphRefusal(ctx, pg, para); cause != "" {
		return reflowOutcome{cause: cause}, nil
	}
	lines, medianGap, _ := paragraphWords(para)
	// A justified paragraph's word spacing IS its justification: InDesign sets each line's `Tw` to stretch it to the edge,
	// and `Tw` moves nothing but the space (code 32). Carried with its words it would stretch every space after a word to its
	// OLD line's width (the review of S03, C3); so in a justified paragraph every word is given the word spacing of its last
	// line — the one set naturally — and the re-set lines are stretched to the edge afresh. No glyph moves by it.
	align := paragraphAlignment(layout, pi, lines)
	if align.justified {
		// The natural word spacing is the last line's; a paragraph that continues past its last line has none set
		// naturally, and takes the tightest line's.
		tw := lastSpacing(lines[len(lines)-1][len(lines[len(lines)-1])-1].spacing).tw
		if align.continues {
			for _, l := range lines {
				tw = math.Min(tw, lastSpacing(l[len(l)-1].spacing).tw)
			}
		}
		lines = withWordSpacing(lines, tw)
		align = paragraphAlignment(layout, pi, lines)
	}
	space := spacerOf(lines, medianGap)
	if normalizedText(text) == normalizedText(para.text()) {
		return reflowOutcome{cause: reflowNoChange}, nil
	}
	if len(editWords(text)) == 0 {
		return reflowOutcome{cause: causeEmpty}, nil
	}
	var runs []textRun
	for _, l := range para.lines {
		runs = append(runs, l.runs...)
	}
	first := runs[0]
	// The words the paragraph already draws keep their own codes, kerns, spacing and font — each its OWN occurrence: the
	// edit is aligned to the paragraph word by word (`alignWords`), and a word the alignment keeps is drawn as it was. A
	// word the alignment does not keep (typed, or moved) takes any occurrence of it while they all look alike. A word drawn
	// in two LOOKS (a font, a size, or a style: a stroked synthetic bold, a raised figure) is used only where EVERY longest
	// alignment keeps it and keeps it in one look — an edit two alignments explain equally does not say which copy
	// survived — and refuses everywhere else (law 3 — never guess). A new word is spelled in the first run's font from the
	// codes it carries, preferring a code the paragraph already draws, and kerned as the page kerns its pairs (`kernsOf`).
	known := map[string][]emitWord{}
	var old []emitWord
	var oldText []string
	styles := map[textSpacing]int{} // how many glyphs the paragraph draws in each style
	used := map[string]bool{}
	for _, l := range lines {
		for _, w := range l {
			ew := emitWord{width: w.width, font: w.font, tfSize: w.tfSize, face: w.face, spacing: w.spacing}
			for gi, g := range w.glyphs {
				ew.codes = append(ew.codes, g.code)
				if gi > 0 {
					ew.kerns = append(ew.kerns, g.kern)
				}
				used[string(g.code)] = true
				styles[w.spacing[gi].style()]++
			}
			known[w.text()] = append(known[w.text()], ew)
			old = append(old, ew)
			oldText = append(oldText, w.text())
		}
	}
	sameLook := func(a, b emitWord) bool {
		if a.font != b.font || a.tfSize != b.tfSize || len(a.spacing) != len(b.spacing) {
			return false
		}
		for i := range a.spacing {
			if a.spacing[i].style() != b.spacing[i].style() {
				return false
			}
		}
		return true
	}
	ambiguous := map[string]bool{}
	for t, occ := range known {
		for _, o := range occ[1:] {
			if !sameLook(o, occ[0]) {
				ambiguous[t] = true
			}
		}
	}
	// A typed word takes the paragraph's usual style — the one most of its glyphs are drawn in, never a superscript's or
	// a stroked word's that happens to stand before it — and the fit (character and word spacing) of the glyph before it.
	// A tie goes to the plainer style — unscaled, unraised, filled — and then to the smaller figures, so the choice never
	// rests on a map's order and never makes typed text raised or stroked on an even count.
	plainer := func(a, b textSpacing) bool {
		ka := [5]float64{math.Abs(a.th - 1), math.Abs(a.ts), float64(a.tr), a.th, a.ts}
		kb := [5]float64{math.Abs(b.th - 1), math.Abs(b.ts), float64(b.tr), b.th, b.ts}
		for i := range ka {
			if ka[i] != kb[i] {
				return ka[i] < kb[i]
			}
		}
		return false
	}
	var usual textSpacing
	best := 0
	for st, n := range styles {
		if n > best || (n == best && plainer(st, usual)) {
			usual, best = st, n
		}
	}
	// drawn is every code the page's paragraphs show in each face: a subset font embeds only the glyphs its document
	// drew, so a code it names in its encoding but never draws may have no glyph behind it (D8). Conservative by
	// construction: a glyph drawn only in an artifact, a blank run or on another page is not counted, and reads as
	// missing — a refusal, never a wrong glyph.
	drawn := map[*runFont]map[string]bool{}
	for _, p := range layout.paragraphs {
		for _, l := range p.lines {
			for _, r := range l.runs {
				if drawn[r.face] == nil {
					drawn[r.face] = map[string]bool{}
				}
				for _, g := range r.glyphs {
					drawn[r.face][string(g.code)] = true
				}
			}
		}
	}
	// pick chooses the code a new character is drawn with — one the paragraph already draws, else the first the font
	// offers — or reports it has none it can vouch for: no code, no width, a zero width (a subset producer's mark for
	// a glyph it left out), or a subset that never draws it.
	pick := func(f *runFont, text string, allowZero bool) (code []byte, w0 float64, cause string) {
		codes := f.codesFor(text)
		if len(codes) == 0 {
			return nil, 0, causeMissingGlyph
		}
		// A code the paragraph draws, else one the page draws in this face, else the font's first.
		code = codes[0]
		for _, c := range codes {
			if drawn[f][string(c)] {
				code = c
				break
			}
		}
		for _, c := range codes {
			if used[string(c)] {
				code = c
				break
			}
		}
		w0, src := f.widths.advance(fontcode.Value(code))
		switch {
		case src == widthNone:
			return nil, 0, causeNoWidths
		case w0 <= 0 && !allowZero:
			return nil, 0, causeMissingGlyph
		case isSubset(f.baseFont) && !drawn[f][string(code)]:
			return nil, 0, causeMissingGlyph
		}
		return code, w0, ""
	}
	var kerns pageKerns // read on the first typed word: an edit that types none pays nothing for it
	var words []emitWord
	edited := editWords(text)
	al := alignWords(oldText, edited)
	for ni, t := range edited {
		if ambiguous[t] {
			// Drawn in two looks: kept only where every longest alignment keeps it, and keeps it in one look.
			if !al.ok {
				return reflowOutcome{cause: causeAmbiguousStyle}, nil
			}
			olds, unkept := al.choices(ni)
			if unkept || len(olds) == 0 {
				return reflowOutcome{cause: causeAmbiguousStyle}, nil
			}
			for _, oi := range olds[1:] {
				if !sameLook(old[oi], old[olds[0]]) {
					return reflowOutcome{cause: causeAmbiguousStyle}, nil
				}
			}
		}
		if oi, ok := al.kept[ni]; ok {
			words = append(words, old[oi])
			continue
		}
		if occ, ok := known[t]; ok {
			words = append(words, occ[0]) // every occurrence looks alike, or the word refused above
			continue
		}
		fit := lines[0][0].spacing[0] // the paragraph's first glyph — its word spacing normalized when justified
		if n := len(words); n > 0 {
			fit = lastSpacing(words[n-1].spacing)
		}
		sp := textSpacing{tc: fit.tc, tw: fit.tw, th: usual.th, ts: usual.ts, tr: usual.tr}
		ew := emitWord{font: first.font, tfSize: first.state.tfSize, face: first.face}
		for _, r := range t {
			code, w0, why := pick(first.face, string(r), false)
			if why != "" {
				return reflowOutcome{cause: why}, nil
			}
			ew.width += (w0/1000*first.state.tfSize + sp.tc) * sp.th * first.state.scale
			if n := len(ew.codes); n > 0 { // by code, not by the string's byte offset: a letter can take several bytes
				if kerns == nil {
					kerns = kernsOf(layout)
				}
				// The kern the page draws for this pair, where it draws it one way (P08.S05), under this word's scaling.
				k := -kerns.lend(first.face, first.size, ew.codes[n-1], code) / 1000 * first.state.tfSize * sp.th * first.state.scale
				ew.kerns = append(ew.kerns, k)
				ew.width += k
			}
			ew.codes = append(ew.codes, code)
			ew.spacing = append(ew.spacing, sp)
		}
		words = append(words, ew)
	}
	measures := paragraphMeasures(layout, pi, lines, space)
	if align.centred {
		// Centred: every line may take the width that fits about the axis, whichever line it is.
		for i := range measures {
			measures[i] = align.width
		}
	}
	broken := breakAt(words, measures, func(w emitWord) float64 { return w.width },
		func(w emitWord) float64 { return space(w.face, w.tfSize, lastSpacing(w.spacing)) })
	// A justified paragraph sets every line but its last to the flush edge (P08.S03). Broken at its own measure — which
	// re-breaks the corpus's justified paragraphs in place — a line can still need more than the edge when the paragraph's
	// median space carries stretch; then it is broken again AT the edge, at its natural spaces, and a word that does not fit
	// even so is too wide.
	justify, fits := justifiedLines(align, broken, lines, space)
	if !fits {
		edgeMeasures := make([]float64, len(lines))
		for i, l := range lines {
			edgeMeasures[i] = align.edge - l[0].startX
		}
		broken = breakAt(words, edgeMeasures, func(w emitWord) float64 { return w.width },
			func(w emitWord) float64 { return space(w.face, w.tfSize, lastSpacing(w.spacing)) + align.lastDelta })
		if justify, fits = justifiedLines(align, broken, lines, space); !fits {
			return reflowOutcome{cause: causeWordTooWide}, nil
		}
	}
	// A paragraph that needs more lines grows DOWN into the room below it (P07.S03): its new lines at its own pitch, and
	// the paragraphs below it moved by the growth. What it cannot move with them, it refuses.
	n := len(para.lines)
	extra := len(broken) - n
	var region flowRegion
	pitch := 0.0
	if extra > 0 {
		if pitch = paragraphPitch(layout, pi); !(pitch > 0) || !finite(pitch) {
			return reflowOutcome{cause: causeNoPitch}, nil
		}
		region = regionOf(layout, pi, visibleBoxOf(pg))
		if len(region.marks) > 0 {
			return reflowOutcome{cause: causeAnchored}, nil
		}
		// The paragraph itself must fit: what lies below it can leave for the next page (P07.S06), it cannot.
		if para.bottom()-float64(extra)*pitch < region.floor-measureSlack {
			return reflowOutcome{cause: causePageFull}, nil
		}
	}
	// A re-wrap moves the paragraph's words inside its box, so an annotation laid over them — a link on a word, a form
	// widget — would point at other words after ANY edit; when it grows, anything anchored in the band below moves too.
	if annotatedOver(ctx, pg, para) {
		return reflowOutcome{cause: causeAnchored}, nil
	}

	// lineAt is the text state a broken line is set from: its own original line's, and past the last of them the last
	// line's, shifted down by the pitch once for each line beyond it.
	lineAt := func(i int) (runMatrix, bool) {
		if i < n {
			return para.lines[i].runs[0].state.tm, true
		}
		return shiftedTm(para.lines[n-1].runs[0].state, float64(i-n+1)*pitch)
	}
	// A centred line is MOVED to its axis — its text matrix shifted, not a lead inside it — so the line's origin is where
	// its ink begins, as every reader of the page's geometry takes it (`textLine.x0`; the review of P08.S04: a lead left
	// the layout reading the old place, and a field beside the moved ink went unseen).
	if align.centred {
		plain := lineAt
		lineAt = func(i int) (runMatrix, bool) {
			li := min(i, n-1)
			w := 0.0
			for wi, word := range broken[i] {
				if wi > 0 {
					w += space(broken[i][wi-1].face, broken[i][wi-1].tfSize, lastSpacing(broken[i][wi-1].spacing))
				}
				w += word.width
			}
			dx := align.axis - w/2 - lines[li][0].startX
			if math.Abs(dx) <= 1e-9 {
				return plain(i)
			}
			return shiftedTmBy(para.lines[li].runs[0].state, dx, float64(max(0, i-n+1))*pitch)
		}
	}
	for i, l := range broken {
		if len(l) == 1 && l[0].width > measures[min(i, len(measures)-1)]+measureSlack {
			return reflowOutcome{cause: causeWordTooWide}, nil
		}
	}
	src, err := pdfread.PageContent(ctx, d, pageNr)
	if err != nil {
		return reflowOutcome{}, err
	}
	spans := make([]opSpan, len(runs))
	for i, r := range runs {
		spans[i] = r.span
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	// A producer that shows each space as its own `( ) Tj` draws it between the words; no line holds it (grouping drops a
	// blank run), so it is the paragraph's to delete with it (P08.S02, /pending 732). Only a blank between the paragraph's
	// own first and last show, or straight after its last with nothing but text state between (Acrobat ends a paragraph on
	// one: left in place, it is a show relying on where the deleted text ended) — one elsewhere may belong to whatever is
	// drawn there — and only one in the page's own stream, since a form's offsets index another stream.
	lo, hi := spans[0].start, spans[len(spans)-1].end
	// spaceCodes is every code the paragraph's own lines draw as a space, in each face: a blank show is deleted only if every
	// code it draws is the single-byte 32 — the code PDF itself treats as the word space — or one of these. Decoding to
	// white space is no promise of a blank glyph: a ToUnicode can map a check mark to U+0020.
	spaceCodes := map[*runFont]map[string]bool{}
	for _, r := range runs {
		for _, g := range r.glyphs {
			if g.text == " " {
				if spaceCodes[r.face] == nil {
					spaceCodes[r.face] = map[string]bool{}
				}
				spaceCodes[r.face][string(g.code)] = true
			}
		}
	}
	drawsOnlySpaces := func(r textRun) bool {
		if !r.decoded || len(r.glyphs) == 0 || len(r.glyphs) != r.codes {
			return false
		}
		for _, g := range r.glyphs {
			if !(len(g.code) == 1 && g.code[0] == ' ') && !spaceCodes[r.face][string(g.code)] {
				return false
			}
		}
		return true
	}
	var after []opSpan
	for _, r := range paragraphRunsWithBlanks(layout, pi)[len(runs):] {
		switch {
		// A run no line holds is not always a space: a glyph that decodes to nothing or to white space can be ink (a
		// Dingbats check mark), and a space in a clipping mode cuts what follows (`text-clips`, as the paragraph's own).
		case r.inForm || !drawsOnlySpaces(r) || clipMode(r.state.tr):
		case r.span.start >= lo && r.span.end <= hi:
			spans = append(spans, r.span)
		case r.span.start >= hi:
			after = append(after, r.span)
		}
	}
	sort.Slice(after, func(i, j int) bool { return after[i].start < after[j].start })
	for _, b := range after {
		// A trailing `'` or `"` moves to a new line and `"` sets the spacing everything after it is drawn in: deleted, the
		// restore (the paragraph's own last state) would undo it. Only a plain `Tj`/`TJ` ends a paragraph.
		if op := showOperator(src[b.start:b.end]); (op != "Tj" && op != "TJ") || !onlyTextState(src[hi:b.start]) {
			break
		}
		spans, hi = append(spans, b), b.end
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	if c := contentAround(src, spans); c != "" {
		return reflowOutcome{cause: c}, nil
	}
	last := runs[0]
	for _, r := range runs {
		if r.span.start > last.span.start {
			last = r
		}
	}
	// Words are separated by the font's own SPACE GLYPH, not by a gap: a gap draws the same but reads as one word to
	// every extractor, search and screen reader. Any difference between the glyph's advance and the paragraph's space
	// goes in a TJ adjustment beside it.
	type spaceGlyph struct {
		code []byte
		w0   float64 // its width in glyph space; its advance depends on the spacing it is drawn under
	}
	// Keyed by face AND size: the space's advance scales with the `Tf` size it is drawn at.
	type spaceKey struct {
		face *runFont
		size float64
	}
	spaces := map[spaceKey]spaceGlyph{}
	for _, l := range broken {
		for wi, w := range l {
			if wi == len(l)-1 {
				continue
			}
			k := spaceKey{w.face, w.tfSize}
			if _, ok := spaces[k]; ok {
				continue
			}
			code, w0, why := pick(w.face, " ", true)
			switch why {
			case "":
			case causeMissingGlyph:
				return reflowOutcome{cause: causeNoSpaceGlyph}, nil
			default:
				return reflowOutcome{cause: why}, nil
			}
			spaces[k] = spaceGlyph{code: code, w0: w0}
		}
	}
	// spaceAdvance is the user-space advance of space glyph sp drawn at size under spacing s.
	spaceAdvance := func(sp spaceGlyph, size float64, s textSpacing) float64 {
		tx := sp.w0/1000*size + s.tc
		if len(sp.code) == 1 && sp.code[0] == ' ' {
			tx += s.tw
		}
		return tx * s.th * first.state.scale
	}
	st0 := first.state
	if !(st0.scale > 0) || !finite(st0.scale, medianGap) {
		return reflowOutcome{cause: causeDegenerate}, nil
	}
	for _, w := range words {
		for _, s := range w.spacing {
			if s.th == 0 || !finite(s.tc, s.tw, s.th, s.ts) {
				return reflowOutcome{cause: causeDegenerate}, nil
			}
		}
	}
	for i, l := range broken {
		tm, ok := lineAt(i)
		if !ok || !finite(tm[:]...) || !finite(last.state.tlm[:]...) {
			return reflowOutcome{cause: causeDegenerate}, nil
		}
		for _, w := range l {
			if w.tfSize == 0 || !finite(w.tfSize, w.width) {
				return reflowOutcome{cause: causeDegenerate}, nil
			}
		}
	}
	// adj writes a user-space horizontal displacement as a `TJ` adjustment: in thousandths of the size, under the `Tz` in
	// force where it is written.
	adj := func(user, size float64, s textSpacing) string { return num(-user / st0.scale / (size * s.th) * 1000) }
	var buf strings.Builder
	// The spacing the first glyph was drawn under, stated before the lines — a deleted `"` set it as it showed, and the
	// state at this point is whatever preceded the paragraph's first show — and from there only what changes, before the
	// glyph that needs it: an operator cannot sit inside a `TJ` array, so a change closes the array and opens another.
	cur := words[0].spacing[0]
	fmt.Fprintf(&buf, "\n%s\n", cur.allOperators())
	var font string
	var size float64
	open := false
	closeArray := func() {
		if open {
			buf.WriteString("] TJ\n")
			open = false
		}
	}
	openArray := func() {
		if !open {
			buf.WriteString("[")
			open = true
		}
	}
	setFont := func(f string, sz float64) {
		if f != font || sz != size {
			closeArray()
			font, size = f, sz
			fmt.Fprintf(&buf, "%s %s Tf\n", pdfName(font), num(size))
		}
	}
	setSpacing := func(s textSpacing) {
		if s != cur {
			closeArray()
			buf.WriteString(s.operators(cur) + "\n")
			cur = s
		}
	}
	for i, l := range broken {
		tm, _ := lineAt(i)
		closeArray()
		fmt.Fprintf(&buf, "%s %s %s %s %s %s Tm\n", num(tm[0]), num(tm[1]), num(tm[2]), num(tm[3]), num(tm[4]), num(tm[5]))
		font = "" // every line states its font
		setFont(l[0].font, l[0].tfSize)
		setSpacing(l[0].spacing[0])
		openArray()
		// The line's lead: where its first word began, past the origin of its first run (a leading kern or space). A new
		// line takes the last line's.
		li := min(i, n-1)
		if lead := lines[li][0].startX - para.lines[li].runs[0].x; math.Abs(lead) > 1e-9 {
			fmt.Fprintf(&buf, "%s ", adj(lead, size, cur))
		}
		for wi, w := range l {
			if wi > 0 {
				// The space is drawn in the font and spacing of the glyph before it, and only then does either change.
				sp := spaces[spaceKey{l[wi-1].face, l[wi-1].tfSize}]
				openArray()
				fmt.Fprintf(&buf, "<%X>", sp.code)
				prev := l[wi-1]
				if extra := space(prev.face, prev.tfSize, cur) + justify[i] - spaceAdvance(sp, size, cur); math.Abs(extra) > 1e-9 {
					fmt.Fprintf(&buf, " %s ", adj(extra, size, cur))
				}
			}
			setFont(w.font, w.tfSize)
			for ci, c := range w.codes {
				setSpacing(w.spacing[ci])
				openArray()
				if ci > 0 && math.Abs(w.kerns[ci-1]) > 1e-9 { // past arithmetic noise, as every other adjustment
					fmt.Fprintf(&buf, " %s ", adj(w.kerns[ci-1], size, cur))
				}
				fmt.Fprintf(&buf, "<%X>", c)
			}
		}
	}
	closeArray()
	// Restore what the deleted operators left behind: the font, the spacing, and the line matrix (a `Tm` sets both the
	// text and the line matrix), so everything after the paragraph in this text object lands where it did, as it was.
	st := last.state
	fmt.Fprintf(&buf, "%s %s Tf %s%s %s %s %s %s %s Tm\n", pdfName(last.font), num(st.tfSize), spacingOf(st).allOperators(),
		num(st.tlm[0]), num(st.tlm[1]), num(st.tlm[2]), num(st.tlm[3]), num(st.tlm[4]), num(st.tlm[5]))
	e := contentstream.NewEdit(src)
	for _, sp := range spans {
		if sp == last.span {
			e.Replace(sp.start, sp.end, []byte(buf.String()))
		} else {
			e.Replace(sp.start, sp.end, nil)
		}
	}
	// The paragraphs below move down with it (P07.S03), what is anchored among them with them (P07.S04), and what no
	// longer fits leaves for the next page (P07.S06) — one rule, `pushDown`, from this page on.
	var flow []flowStep
	if extra > 0 {
		if layout.columns != 1 && len(region.paragraphs) > 0 && region.room < float64(extra)*pitch-measureSlack {
			return reflowOutcome{cause: causePageFull}, nil // several columns: nothing flows to another page
		}
		steps, cause, below, err := pushDown(ctx, pg, layout, src, region.paragraphs, float64(extra)*pitch, region.floor, region.bound,
			para.bottom(), region.step, anchorXs(layout, region), nil)
		if err != nil {
			return reflowOutcome{}, err
		}
		if cause != "" {
			return reflowOutcome{cause: cause, below: below}, nil
		}
		for _, m := range steps[0].moves {
			e.Replace(m.span.start, m.span.end, m.with)
		}
		flow = steps
	}
	out, err := e.Apply()
	if err != nil {
		return reflowOutcome{}, err
	}
	return reflowOutcome{content: out, flow: flow}, nil
}

// anchorShift is what a grown paragraph moves besides its text: everything anchored inside zone, down by dy.
type anchorShift struct {
	zone [4]float64
	dy   float64
	set  anchorSet // what is anchored inside zone, resolved when the step was planned (`anchorsIn`)
}

// anchorXs is the horizontal extent of what moves when a paragraph grows over region: the whole page's width when the page
// has one column, so a margin note or a signing flag beside a moved paragraph moves with it rather than staying behind; the
// column's on a page of several, where anything beside it may belong to another column and refuses instead (P07.S04).
func anchorXs(l pageLayout, region flowRegion) [2]float64 {
	if l.columns == 1 {
		return [2]float64{math.Inf(-1), math.Inf(1)}
	}
	return [2]float64{region.x0, region.x1}
}

// contentAround checks the stretch between a paragraph's first and last show operator holds only positioning, `Tf`, the
// text state and the paragraph's own shows, and that what follows it repositions before it draws: the rewrite restores the
// line matrix, and a show relying on the text matrix the last deleted show left would land at the line's start instead.
// It returns the cause, or "". Over the stretch it names the most specific thing there, not the first: marked content is
// `tagged`, another show, a paint or a colour `mixed-content`, and lines drawn as separate text objects `text-objects` — a
// producer that tags line by line also closes a text object per line, and "a colour change or a drawing" is not what
// stands in its way (/pending 732).
//
// Replacement text around the paragraph is not asked here: the walker records it per run (`textRun.replaced`, inline
// and named property lists alike), and `paragraphWords` refuses it — one reader of a property list, not two.
func contentAround(src []byte, spans []opSpan) string {
	toks := contentstream.Tokenize(src)
	// spans are sorted and disjoint: the last span starting at or before at is the only one that can hold it.
	inSpan := func(at int) bool {
		i := sort.Search(len(spans), func(i int) bool { return spans[i].start > at }) - 1
		return i >= 0 && at < spans[i].end
	}
	lo, hi := spans[0].start, spans[len(spans)-1].end
	tagged, foreign, objects := false, false, false
	within := func() string {
		switch {
		case tagged:
			return causeTagged
		case foreign:
			return causeMixedContent
		case objects:
			return causeTextObjects
		}
		return ""
	}
	for _, tk := range toks {
		if tk.Kind != contentstream.Operator || tk.End <= lo {
			continue
		}
		op := string(tk.Bytes(src))
		if tk.End <= hi {
			if inSpan(tk.Start) {
				continue
			}
			switch op {
			case "Td", "TD", "Tm", "T*", "TL", "Tf", "Tc", "Tw", "Tz", "Ts", "Tr":
				// Positioning, and the text state each glyph carries for itself (P08.S01) — the rewrite states it again.
			case "BDC", "BMC", "EMC":
				tagged = true
			case "BT", "ET":
				objects = true
			default:
				foreign = true
			}
			continue
		}
		if c := within(); c != "" {
			return c
		}
		switch op {
		case "Tj", "TJ":
			return causeInlineFollower
		case "Td", "TD", "Tm", "T*", "'", "\"", "ET":
			return ""
		}
	}
	return within()
}

// showOperator is the operator a show's span ends in — `Tj`, `TJ`, `'` or `"`.
func showOperator(span []byte) string {
	op := ""
	for _, tk := range contentstream.Tokenize(span) {
		if tk.Kind == contentstream.Operator {
			op = string(tk.Bytes(span))
		}
	}
	return op
}

// onlyTextState reports whether src holds no operator but the text state's — `Tf`, `Tc`, `Tw`, `Tz`, `Ts`, `Tr`, `TL` — so
// a show after it draws where the show before it ended.
func onlyTextState(src []byte) bool {
	for _, tk := range contentstream.Tokenize(src) {
		if tk.Kind != contentstream.Operator {
			continue
		}
		switch string(tk.Bytes(src)) {
		case "Tf", "Tc", "Tw", "Tz", "Ts", "Tr", "TL":
		default:
			return false
		}
	}
	return true
}

// num writes a number the way a content stream wants it: fixed-point with no exponent, as few digits as represent it
// exactly (a coordinate is never rounded), and never "-0".
func num(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if s == "-0" {
		return "0"
	}
	return s
}

// pdfName writes a resource name as a PDF name token: `/` and the name, with every byte that is not a regular character
// — and `#` itself — written as #xx (ISO 32000-1 §7.3.5). The reader decodes `#xx` (`fontcode.Name`), so a name read
// back must be escaped again or `/F#20a` is written as the two tokens `/F` and `a`.
func pdfName(name string) string {
	var b strings.Builder
	b.WriteByte('/')
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '#' || c < 0x21 || c > 0x7e || !regularByte(c) {
			fmt.Fprintf(&b, "#%02X", c)
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// regularByte is ISO 32000-1 §7.2.2's regular character: neither white-space nor a delimiter.
func regularByte(c byte) bool {
	switch c {
	case 0, '\t', '\n', '\f', '\r', ' ', '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return false
	}
	return true
}

// minPitchEm is the smallest baseline step, in ems of the smaller of the two lines' sizes, that is a leading. Over the real-producer
// corpus 7,083 of 7,875 steps inside a paragraph lie at 1.1 em or more and the rest scatter down to zero; set tighter than
// 0.8 em, a line's descenders cross the next line's capitals.
const minPitchEm = 0.8

// paragraphPitch is how far apart paragraph pi's lines are set, baseline to baseline in user space: the median of its own
// steps, or for a paragraph of one line the median step of its column's paragraphs set at its size — or 0 when neither
// says, and a grown paragraph is refused rather than set at an invented leading.
//
// A step under minPitchEm of its paragraph's size is not a leading: it is two "lines" that grouping cut from one baseline
// (a table row's cells drawn a fraction of a point apart, a raised figure). Counted, it set a one-line paragraph's second
// line a quarter of a point under its first — on top of it — in 1,809 of the corpus's 17,936 one-line paragraphs (found
// by P08.S05's census, which typed a word that grew one).
func paragraphPitch(l pageLayout, pi int) float64 {
	p := l.paragraphs[pi]
	stepsOf := func(q textParagraph) []float64 {
		var out []float64
		for i := 1; i < len(q.lines); i++ {
			// The smaller of the two lines' sizes: a line's size is its largest run, and one large word on it must not
			// raise the floor past the paragraph's real leading (S05's review, W2).
			if step := q.lines[i-1].y - q.lines[i].y; step >= minPitchEm*math.Min(q.lines[i-1].size, q.lines[i].size) {
				out = append(out, step)
			}
		}
		return out
	}
	if len(p.lines) > 1 {
		return median(stepsOf(p))
	}
	size := p.lines[0].size
	var steps []float64
	for _, q := range l.paragraphs {
		if q.column == p.column && len(q.lines) > 1 && math.Abs(q.lines[0].size-size) <= 0.1*size {
			steps = append(steps, stepsOf(q)...)
		}
	}
	return median(steps)
}
