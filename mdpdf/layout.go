package mdpdf

import (
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/font"
)

// Page geometry: US Letter portrait in points, lower-left origin, 1-inch
// margins. Letter (not A4) because the primary consumers produce US
// business/medical correspondence; these constants and the "paper" name in
// spec() must change together or layout math desyncs from the MediaBox.
const (
	pageW    = 612.0
	pageH    = 792.0
	marginX  = 72.0
	marginY  = 72.0
	contentW = pageW - 2*marginX
	pageTop  = pageH - marginY
)

// style is a resolved font face and point size for one fragment of text.
type style struct {
	font string
	size int
	// embedded marks a fallback face (ConvertWithFonts) rather than one of the Base-14
	// core fonts. It exists for width(): the two are measured differently, and using the
	// core rule on an embedded face inflates every multi-byte word.
	embedded bool
}

// width measures text as the PDF will carry it.
//
// pdfcpu counts BYTES for core fonts, so a core-font string must be measured in its
// ENCODED form or every non-ASCII rune inflates the measurement and shifts the rest of
// the line (see encodedWidth). An embedded font is measured by RUNE, so the same
// transformation there would double-count exactly the text the fallback exists to print —
// a Greek or CJK word would reserve twice the space it occupies.
func (s style) width(text string) float64 {
	if s.embedded {
		return font.TextWidth(text, s.font, s.size)
	}
	return font.TextWidth(encodedWidth(text), s.font, s.size)
}

// leading is the baseline-to-baseline distance for lines set in this style.
func (s style) leading() float64 { return float64(s.size) * 1.35 }

// frag is a stretch of characters sharing one style, with no spaces inside.
type frag struct {
	text string
	sty  style
}

// word is a sequence of frags rendered with nothing between them ("bold" +
// "." from `**bold**.`). Lines break only between words, except when a single
// word is wider than the line and must be hard-split.
type word struct {
	frags []frag
}

// isBreak marks a forced line break (Markdown hard break); it carries no text.
func (w word) isBreak() bool { return len(w.frags) == 0 }

func (w word) width() float64 {
	var t float64
	for _, f := range w.frags {
		t += f.sty.width(f.text)
	}
	return t
}

// run is one positioned text draw; box is one positioned filled rectangle.
type run struct {
	x, y float64
	text string
	sty  style
}

type box struct {
	x, y, w, h float64
}

// layout accumulates positioned runs page by page as the cursor descends.
type layout struct {
	runs  [][]run // one slice per page
	boxes [][]box
	y     float64 // current baseline cursor on the last page
}

func newLayout() *layout {
	l := &layout{}
	l.newPage()
	return l
}

func (l *layout) newPage() {
	l.runs = append(l.runs, nil)
	l.boxes = append(l.boxes, nil)
	l.y = pageTop
}

func (l *layout) add(r run) { l.runs[len(l.runs)-1] = append(l.runs[len(l.runs)-1], r) }

// need starts a new page unless h points of vertical room remain.
func (l *layout) need(h float64) {
	if l.y-h < marginY && l.y < pageTop {
		l.newPage()
	}
}

// gap advances the cursor by h, except at the top of a page where block
// spacing would just eat into the margin.
func (l *layout) gap(h float64) {
	if l.y < pageTop {
		l.y -= h
	}
}

// para wraps words to the content width minus indent and emits them line by
// line, paginating as needed. A non-nil marker (list bullet or number) is drawn
// in the gutter markerGutter points left of the first line.
func (l *layout) para(words []word, indent float64, marker *word, lead float64) {
	lines := wrapWords(words, contentW-indent)
	if len(lines) == 0 {
		if marker == nil {
			return
		}
		lines = [][]word{nil} // marker-only list item
	}
	for i, ln := range lines {
		l.need(lead)
		l.y -= lead
		if i == 0 && marker != nil {
			l.line([]word{*marker}, marginX+indent-markerGutter)
		}
		l.line(ln, marginX+indent)
	}
}

// line emits one wrapped line at baseline l.y starting at x, joining words
// with single spaces and merging same-style neighbors into one run.
func (l *layout) line(words []word, x float64) {
	var frs []frag
	for i, w := range words {
		if i > 0 {
			frs = append(frs, frag{" ", frs[len(frs)-1].sty}) // space inherits the preceding style
		}
		frs = append(frs, w.frags...)
	}
	var merged []frag
	for _, f := range frs {
		if f.text == "" {
			continue
		}
		if n := len(merged); n > 0 && merged[n-1].sty == f.sty {
			merged[n-1].text += f.text
			continue
		}
		merged = append(merged, f)
	}
	for _, f := range merged {
		l.add(run{x, l.y, f.text, f.sty})
		x += f.sty.width(f.text)
	}
}

// code emits preformatted lines in the monospace style, hard-wrapping any line
// wider than the content width (Courier is fixed-pitch, so a rune count works).
func (l *layout) code(lines []string, indent float64) {
	sty := style{font: fontCode, size: sizeCode}
	maxChars := int((contentW - indent) / sty.width("M"))
	if maxChars < 1 {
		maxChars = 1
	}
	lead := sty.leading()
	for _, ln := range lines {
		ln = strings.ReplaceAll(ln, "\t", "    ")
		for _, chunk := range chunks(ln, maxChars) {
			l.need(lead)
			l.y -= lead
			if chunk != "" {
				l.add(run{marginX + indent, l.y, chunk, sty})
			}
		}
	}
}

// chunks splits s into rune slices of at most n; an empty s yields one "".
func chunks(s string, n int) []string {
	r := []rune(s)
	if len(r) <= n {
		return []string{s}
	}
	var out []string
	for len(r) > n {
		out = append(out, string(r[:n]))
		r = r[n:]
	}
	return append(out, string(r))
}

// rule draws a thematic break as a thin full-width bar.
func (l *layout) rule(indent float64) {
	l.need(14)
	l.y -= 10
	pg := len(l.boxes) - 1
	l.boxes[pg] = append(l.boxes[pg], box{marginX + indent, l.y, contentW - indent, 0.75})
	l.y -= 2
}

// wrapWords greedily fills lines up to maxW, breaking at word boundaries and
// hard-splitting any single word wider than a whole line.
func wrapWords(words []word, maxW float64) [][]word {
	var lines [][]word
	var cur []word
	var curW float64
	flush := func() {
		if len(cur) > 0 {
			lines = append(lines, cur)
			cur, curW = nil, 0
		}
	}
	for _, w := range words {
		if w.isBreak() {
			flush()
			continue
		}
		ww := w.width()
		if len(cur) > 0 {
			sp := cur[len(cur)-1].frags[len(cur[len(cur)-1].frags)-1].sty.width(" ")
			if curW+sp+ww > maxW {
				flush()
			} else {
				cur = append(cur, w)
				curW += sp + ww
				continue
			}
		}
		if ww > maxW {
			parts := splitWord(w, maxW)
			for _, p := range parts[:len(parts)-1] {
				lines = append(lines, []word{p})
			}
			w = parts[len(parts)-1]
			ww = w.width()
		}
		cur = []word{w}
		curW = ww
	}
	flush()
	return lines
}

// splitWord breaks one over-wide word into pieces no wider than maxW,
// measuring rune by rune (at least one rune per piece).
func splitWord(w word, maxW float64) []word {
	var parts []word
	var cur []frag
	var curW float64
	for _, f := range w.frags {
		for _, r := range f.text {
			rw := f.sty.width(string(r))
			if curW+rw > maxW && curW > 0 {
				parts = append(parts, word{cur})
				cur, curW = nil, 0
			}
			if n := len(cur); n > 0 && cur[n-1].sty == f.sty {
				cur[n-1].text += string(r)
			} else {
				cur = append(cur, frag{string(r), f.sty})
			}
			curW += rw
		}
	}
	if len(cur) > 0 {
		parts = append(parts, word{cur})
	}
	return parts
}
