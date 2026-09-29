package pdfops

import (
	"fmt"
	"math"
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
	width  float64
	face   *runFont
	size   float64
	// font and tfSize are the `Tf` operands of the run the word began in — what re-emitting it selects.
	font   string
	tfSize float64
	// startX is where the word's first glyph begins, in user space: a line's lead is its first word's startX less its
	// first run's origin, which a re-emitted line must keep.
	startX float64
}

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
			case r.state.tr == 3 || r.state.tr == 7:
				return nil, 0, causeInvisible
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
					cur = &reflowWord{face: r.face, size: r.size, font: r.font, tfSize: r.state.tfSize, startX: start}
					cur.glyphs = append(cur.glyphs, g)
					cur.width += g.advance
					continue
				}
				if gi == 0 {
					g.kern += joinGap // the gap between two runs of one word, kept inside it
				}
				cur.glyphs = append(cur.glyphs, g)
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

// lineMeasures is the room each of the paragraph's lines had: from where its first word begins to the paragraph's right
// edge — the furthest any of its lines reaches, set with its own space. A breaker that set a line narrower than its
// room would have fitted the next word, so the right edge can have been no nearer; a first-line indent has less room on
// its line, and that is the difference between this and one measure for every line.
func lineMeasures(lines [][]reflowWord, space float64) []float64 {
	right := math.Inf(-1)
	for _, l := range lines {
		w := l[0].startX
		for i, word := range l {
			if i > 0 {
				w += space
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

// measureSlack absorbs the arithmetic in comparing a sum against the maximum of the same sums.
const measureSlack = 1e-6

// breakAt is BreakGreedy with a line's room by index — past the last measured line, the last line's.
func breakAt[T any](items []T, measures []float64, width func(T) float64, space float64) [][]T {
	return mdpdf.BreakGreedy(items, 0, mdpdf.BreakOps[T]{
		Width: width,
		Space: func(T) float64 { return space },
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
	causeMixedState      = "mixed-state"      // the paragraph's runs differ in spacing, scaling, rise or scale
	causeMixedContent    = "mixed-content"    // something other than positioning sits among its show operators
	causeInlineFollower  = "inline-follower"  // text is drawn straight after it with no repositioning
	causeTagged          = "tagged"           // marked content sits among its lines; moving text would mis-tag it (P07)
	causeReplacementText = "replacement-text" // the paragraph sits inside /ActualText or /Alt, which would keep the old words readable
	causeNoSpaceGlyph    = "no-space-glyph"   // the font draws no space, and a gap with no glyph is invisible to every reader of the text
	causeGrows           = "paragraph-grows"  // it needs more lines than it had — P07's flow
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
)

// ReflowCauses is every cause a reflow can fall back on, the server's included — the list the editor must have a
// sentence for. `TestEveryReflowCauseIsSaidToTheUser` holds `web/app.js`'s REFLOW_CAUSES to it.
var ReflowCauses = []string{
	causeMissingGlyph, causeMixedState, causeMixedContent, causeInlineFollower, causeTagged, causeReplacementText,
	causeNoSpaceGlyph, causeGrows, causeWordTooWide, causeNoParagraph, causeEmpty, causeAmbiguousStyle, causeVertical,
	causeInvisible, causeTextInForm, causeRotated, causeNoWidths, causeUndecoded, causeGlyphsNotKept, causeStyledWord,
	causeEmptyLine, causeNoSpaceWidth, causeDegenerate, ReflowCauseSigned, ReflowCauseInvalidOutput,
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
func paragraphRefusal(ctx *model.Context, pageNr int, p textParagraph) string {
	if _, _, cause := paragraphWords(p); cause != "" {
		return cause
	}
	if structReplacesText(ctx, pageNr, p) {
		return causeReplacementText
	}
	return ""
}

// structReplacesText reports whether any of the paragraph's marked content belongs to a structure element — or an
// ancestor of one — carrying `/ActualText`, `/Alt` or `/E`: the tree then reports that text for these glyphs, and
// rewriting the glyphs would leave the old words there. Only the page's own stream is asked (a run in a form is
// refused before this), through the page's `/StructParents` row of the `/ParentTree`.
func structReplacesText(ctx *model.Context, pageNr int, p textParagraph) bool {
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
	pd, _, _, err := ctx.PageDict(pageNr, false)
	if err != nil || pd == nil {
		return false
	}
	key, ok := pdfNumber(xt, pd["StructParents"])
	if !ok || key < 0 {
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
	row, _, found := rowFor(ctx, &structTree{root: root}, int(key))
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
}

// emitWord is a word as it will be drawn: its codes, the kern before each after the first (user space), its font.
type emitWord struct {
	face   *runFont
	codes  [][]byte
	kerns  []float64
	width  float64
	font   string
	tfSize float64
}

// reflowParagraph re-sets paragraph pi of page pageNr as text, in the paragraph's own font, at its own measure and
// baselines, and returns the page's rewritten content — `PLAN-text-reflow.md` P06.S04. The original show operators are
// DELETED, so the original words are gone from the content rather than covered. Anything the rewrite cannot do exactly
// is a named cause and no content (law 3).
func reflowParagraph(ctx *model.Context, pageNr, pi int, text string) (reflowOutcome, error) {
	layout, err := readPageGlyphLayout(ctx, pageNr)
	if err != nil {
		return reflowOutcome{}, err
	}
	return reflowParagraphIn(ctx, layout, pageNr, pi, text)
}

// reflowParagraphIn is reflowParagraph over a layout the caller already read — the door reads one to check staleness.
func reflowParagraphIn(ctx *model.Context, layout pageLayout, pageNr, pi int, text string) (reflowOutcome, error) {
	d, _, _, err := ctx.PageDict(pageNr, false)
	if err != nil || d == nil {
		return reflowOutcome{}, fmt.Errorf("pdfops: page %d does not resolve: %w", pageNr, err)
	}
	if pi < 0 || pi >= len(layout.paragraphs) {
		return reflowOutcome{cause: causeNoParagraph}, nil
	}
	para := layout.paragraphs[pi]
	if cause := paragraphRefusal(ctx, pageNr, para); cause != "" {
		return reflowOutcome{cause: cause}, nil
	}
	lines, space, _ := paragraphWords(para)
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
	for _, r := range runs[1:] {
		a, b := r.state, first.state
		if a.tc != b.tc || a.tw != b.tw || a.th != b.th || a.ts != b.ts || a.tr != b.tr || math.Abs(a.scale-b.scale) > 1e-9 {
			return reflowOutcome{cause: causeMixedState}, nil
		}
	}
	// The words the paragraph already draws keep their own codes, kerns and font; a new word is spelled in the first
	// run's font from the codes it carries, preferring a code the paragraph already draws. A word drawn in two styles
	// has no one identity to keep, so the new text may not use it (law 3 — never guess which was meant).
	known := map[string]emitWord{}
	ambiguous := map[string]bool{}
	used := map[string]bool{}
	for _, l := range lines {
		for _, w := range l {
			ew := emitWord{width: w.width, font: w.font, tfSize: w.tfSize, face: w.face}
			for gi, g := range w.glyphs {
				ew.codes = append(ew.codes, g.code)
				if gi > 0 {
					ew.kerns = append(ew.kerns, g.kern)
				}
				used[string(g.code)] = true
			}
			if prev, dup := known[w.text()]; !dup {
				known[w.text()] = ew
			} else if prev.font != ew.font || prev.tfSize != ew.tfSize {
				ambiguous[w.text()] = true
			}
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
	var words []emitWord
	for _, t := range editWords(text) {
		if ambiguous[t] {
			return reflowOutcome{cause: causeAmbiguousStyle}, nil
		}
		if ew, ok := known[t]; ok {
			words = append(words, ew)
			continue
		}
		ew := emitWord{font: first.font, tfSize: first.state.tfSize, face: first.face}
		for ri, r := range t {
			code, w0, why := pick(first.face, string(r), false)
			if why != "" {
				return reflowOutcome{cause: why}, nil
			}
			st := first.state
			ew.width += (w0/1000*st.tfSize + st.tc) * st.th * st.scale
			ew.codes = append(ew.codes, code)
			if ri > 0 {
				ew.kerns = append(ew.kerns, 0)
			}
		}
		words = append(words, ew)
	}
	measures := lineMeasures(lines, space)
	broken := breakAt(words, measures, func(w emitWord) float64 { return w.width }, space)
	if len(broken) > len(para.lines) {
		return reflowOutcome{cause: causeGrows}, nil
	}
	for i, l := range broken {
		if len(l) == 1 && l[0].width > measures[i]+measureSlack {
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
		adv  float64
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
			st := first.state
			tx := w0/1000*w.tfSize + st.tc
			if len(code) == 1 && code[0] == ' ' {
				tx += st.tw
			}
			spaces[k] = spaceGlyph{code: code, adv: tx * st.th * st.scale}
		}
	}
	st0 := first.state
	if !(st0.scale > 0) || st0.th == 0 || !finite(st0.scale, st0.th, st0.tc, st0.tw, space) {
		return reflowOutcome{cause: causeDegenerate}, nil
	}
	for i, l := range broken {
		tm := para.lines[i].runs[0].state.tm
		if !finite(tm[:]...) || !finite(last.state.tlm[:]...) {
			return reflowOutcome{cause: causeDegenerate}, nil
		}
		for _, w := range l {
			if w.tfSize == 0 || !finite(w.tfSize, w.width) {
				return reflowOutcome{cause: causeDegenerate}, nil
			}
		}
	}
	adj := func(user, size float64) string { return num(-user / st0.scale / (size * st0.th) * 1000) }
	var buf strings.Builder
	// The spacing the lines were measured under, stated before them: a deleted `"` set it as it showed, and the state
	// at this point is whatever preceded the paragraph's first show.
	fmt.Fprintf(&buf, "\n%s Tc %s Tw\n", num(st0.tc), num(st0.tw))
	for i, l := range broken {
		tm := para.lines[i].runs[0].state.tm
		fmt.Fprintf(&buf, "%s %s %s %s %s %s Tm\n", num(tm[0]), num(tm[1]), num(tm[2]), num(tm[3]), num(tm[4]), num(tm[5]))
		font, size := l[0].font, l[0].tfSize
		fmt.Fprintf(&buf, "%s %s Tf\n[", pdfName(font), num(size))
		// The line's lead: where its first word began, past the origin of its first run (a leading kern or space).
		if lead := lines[i][0].startX - para.lines[i].runs[0].x; math.Abs(lead) > 1e-9 {
			fmt.Fprintf(&buf, "%s ", adj(lead, size))
		}
		for wi, w := range l {
			if wi > 0 {
				// The space is drawn in the font of the word before it, and only then does the font change: an
				// operator cannot sit inside a TJ array, so a change of font closes the array and opens another.
				sp := spaces[spaceKey{l[wi-1].face, l[wi-1].tfSize}]
				fmt.Fprintf(&buf, "<%X>", sp.code)
				if extra := space - sp.adv; math.Abs(extra) > 1e-9 {
					fmt.Fprintf(&buf, " %s ", adj(extra, size))
				}
				if w.font != font || w.tfSize != size {
					font, size = w.font, w.tfSize
					fmt.Fprintf(&buf, "] TJ\n%s %s Tf\n[", pdfName(font), num(size))
				}
			}
			for ci, c := range w.codes {
				if ci > 0 && w.kerns[ci-1] != 0 {
					fmt.Fprintf(&buf, " %s ", adj(w.kerns[ci-1], size))
				}
				fmt.Fprintf(&buf, "<%X>", c)
			}
		}
		buf.WriteString("] TJ\n")
	}
	// Restore what the deleted operators left behind: the font, the spacing, and the line matrix (a `Tm` sets both the
	// text and the line matrix), so everything after the paragraph in this text object lands where it did.
	st := last.state
	fmt.Fprintf(&buf, "%s %s Tf %s Tc %s Tw %s %s %s %s %s %s Tm\n", pdfName(last.font), num(st.tfSize), num(st.tc), num(st.tw),
		num(st.tlm[0]), num(st.tlm[1]), num(st.tlm[2]), num(st.tlm[3]), num(st.tlm[4]), num(st.tlm[5]))
	e := contentstream.NewEdit(src)
	for _, sp := range spans {
		if sp == last.span {
			e.Replace(sp.start, sp.end, []byte(buf.String()))
		} else {
			e.Replace(sp.start, sp.end, nil)
		}
	}
	out, err := e.Apply()
	if err != nil {
		return reflowOutcome{}, err
	}
	return reflowOutcome{content: out}, nil
}

// contentAround checks the stretch between a paragraph's first and last show operator holds only positioning, `Tf` and
// the paragraph's own shows — marked content there is `tagged`, anything else `mixed-content` — and that what follows it
// repositions before it draws: the rewrite restores the line matrix, and a show relying on the text matrix the last
// deleted show left would land at the line's start instead. It returns the cause, or "".
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
			case "Td", "TD", "Tm", "T*", "TL", "Tf":
			case "BDC", "BMC", "EMC":
				return causeTagged
			default:
				return causeMixedContent
			}
			continue
		}
		switch op {
		case "Tj", "TJ":
			return causeInlineFollower
		case "Td", "TD", "Tm", "T*", "'", "\"", "ET":
			return ""
		}
	}
	return ""
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
