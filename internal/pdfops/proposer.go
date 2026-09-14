package pdfops

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// The proposer — `PLAN-accessibility.md` P08.S05.
//
// Law 3 and D5: inferred structure is a PROPOSAL. This file turns a document's paragraphs into
// proposed elements — headings with levels, paragraphs, list items grouped into lists — and writes
// nothing. S06 shows the proposal, lets a person change it, and only a commit writes. A guard asserts
// no function here calls a writer.
//
// # The rules, and what they were cut from
//
// Measured on LibreOffice's own output before a line was written (P08.S05's step zero):
//
//   - **Headings are LARGER.** 18pt and 16pt against a 12pt body, in a bold face. Size is the signal
//     taken; the bold face name is not, because nothing in the corpus needs it and a bold lead-in
//     sentence at body size would be the first thing it got wrong.
//   - **Body size is the size carrying the most characters**, not the most common run: a page of short
//     headings and one long paragraph has more heading runs than body runs.
//   - **A list label is its own token.** LibreOffice draws a bullet as U+F095 in OpenSymbol — a
//     private-use code point, not `•` — and a number as `1.` in the body font, each followed by the
//     item's text. A line whose first token is one of those starts a list item.
//   - **The grouping merges a list that has nothing wider beside it.** Two short numbered items alone
//     in a column are one paragraph to S03, because the column's right edge IS their width and the
//     short-line signal cannot fire. A label at the start of a line splits them here.

// proposedElement is one element the autotagger proposes.
type proposedElement struct {
	// role is a standard structure type: H1–H6, P, or LI.
	role   string
	page   int
	column int
	// marker is a list item's label as drawn (`1.`, U+F095), and text includes it: the label is part
	// of what the item shows, and `LI` holds `Lbl` and `LBody` both.
	marker string
	text   string
	lines  []textLine
	// list numbers the list a list item belongs to — consecutive items share one — or is -1.
	list int
}

// proposal is a document's proposed structure, in reading order.
type proposal struct {
	elements []proposedElement
	bodySize float64
	// unsupported names pages whose layout the grouping could not read, with its reason: those pages'
	// elements are proposed in the order the grouping gave, and the reviewer is told why to look.
	unsupported map[int]string
	// noText lists pages with no text at all — a scan's path is OCR, not a proposal.
	noText []int
}

// headingScale is how much larger than the body a paragraph must be to be proposed as a heading.
const headingScale = 1.15

// maxHeadingLines bounds a heading's length: a long passage set large is large text, not a title.
const maxHeadingLines = 3

// proposeStructure reads every page through the layout door and proposes a structure for the whole
// document. It reads; it never writes.
func proposeStructure(ctx *model.Context) (proposal, error) {
	layouts := make([]pageLayout, 0, ctx.PageCount)
	for pg := 1; pg <= ctx.PageCount; pg++ {
		l, err := readPageLayout(ctx, pg)
		if err != nil {
			return proposal{}, err
		}
		layouts = append(layouts, l)
	}
	return proposeFromLayouts(layouts), nil
}

// proposeFromLayouts is the rule, over layouts already read.
func proposeFromLayouts(layouts []pageLayout) proposal {
	out := proposal{unsupported: map[int]string{}}
	out.bodySize = bodySizeOf(layouts)
	levels := headingLevels(layouts, out.bodySize)

	lists, inList := 0, false
	for i, l := range layouts {
		page := i + 1
		if l.noText {
			out.noText = append(out.noText, page)
			continue
		}
		if l.unsupported != "" {
			out.unsupported[page] = l.unsupported
		}
		for _, par := range l.paragraphs {
			for _, piece := range splitAtListMarkers(par) {
				first := piece.lines[0]
				el := proposedElement{page: page, column: piece.column, text: piece.text(), lines: piece.lines, list: -1}
				switch m := listMarker(first); {
				case m != "":
					if !inList {
						lists++
						inList = true
					}
					el.role, el.marker, el.list = "LI", m, lists-1
				case isHeadingPiece(piece, out.bodySize):
					inList = false
					el.role = fmt.Sprintf("H%d", levels[roundHalf(first.size)])
				default:
					inList = false
					el.role = "P"
				}
				out.elements = append(out.elements, el)
			}
		}
	}
	return out
}

func roundHalf(x float64) float64 { return math.Round(x*2) / 2 }

// bodySizeOf is the size carrying the most characters across the document.
func bodySizeOf(layouts []pageLayout) float64 {
	chars := map[float64]int{}
	for _, l := range layouts {
		for _, par := range l.paragraphs {
			for _, ln := range par.lines {
				for _, r := range ln.runs {
					chars[roundHalf(r.size)] += len([]rune(strings.Join(strings.Fields(r.text), "")))
				}
			}
		}
	}
	best, most := 0.0, -1
	for size, n := range chars {
		if n > most || (n == most && size < best) {
			best, most = size, n
		}
	}
	return best
}

func isHeadingPiece(p textParagraph, body float64) bool {
	return body > 0 && len(p.lines) <= maxHeadingLines && roundHalf(p.lines[0].size) >= body*headingScale
}

// headingLevels numbers the distinct heading sizes, largest first, from 1 to 6.
func headingLevels(layouts []pageLayout, body float64) map[float64]int {
	seen := map[float64]bool{}
	for _, l := range layouts {
		for _, par := range l.paragraphs {
			if isHeadingPiece(par, body) {
				seen[roundHalf(par.lines[0].size)] = true
			}
		}
	}
	sizes := make([]float64, 0, len(seen))
	for s := range seen {
		sizes = append(sizes, s)
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(sizes)))
	out := map[float64]int{}
	for i, s := range sizes {
		out[s] = min(i+1, 6)
	}
	return out
}

// splitAtListMarkers starts a new piece at every line, after the first, that begins with a list label.
func splitAtListMarkers(par textParagraph) []textParagraph {
	var out []textParagraph
	cur := textParagraph{column: par.column}
	for i, ln := range par.lines {
		if i > 0 && listMarker(ln) != "" {
			out = append(out, cur)
			cur = textParagraph{column: par.column}
		}
		cur.lines = append(cur.lines, ln)
	}
	return append(out, cur)
}

// numberedLabel is a list number: `1.`, `12)`, `a.`, `iv)`. Uppercase single letters are excluded —
// `A.` begins ordinary sentences and initials far more often than it labels a list.
var numberedLabel = regexp.MustCompile(`^(\d{1,3}|[a-z]|[ivxlcdm]{1,4})[.)]$`)

// listMarker returns a line's list label, or "". The label must be followed by the item's text.
//
// **The label is read from the first RUN as well as the first word**, because a producer that draws
// the label as its own run need not leave a space before the text: LibreOffice's numbered list draws
// `1.` flush against `Open the document.`, so the line's first word is `1.Open` and matches nothing.
// Found by the truth corpus, where the proposal cleared its score floors and still called all three
// items paragraphs.
func listMarker(ln textLine) string {
	isLabel := func(tok string) bool {
		if numberedLabel.MatchString(tok) {
			return true
		}
		r := []rune(tok)
		return len(r) == 1 && isBulletRune(r[0])
	}
	if len(ln.runs) >= 2 {
		if tok := strings.TrimSpace(ln.runs[0].text); isLabel(tok) {
			return tok
		}
	}
	if fields := strings.Fields(ln.text); len(fields) >= 2 && isLabel(fields[0]) {
		return fields[0]
	}
	return ""
}

// isBulletRune reports the characters documents draw as list bullets — including the private-use
// area, where symbol fonts put theirs (LibreOffice's OpenSymbol bullet is U+F095, measured).
func isBulletRune(r rune) bool {
	switch r {
	case '•', '◦', '▪', '▫', '■', '□', '●', '○', '‣', '⁃', '–', '—', '-', '*', '·', '➢', '✓':
		return true
	}
	return r >= 0xE000 && r <= 0xF8FF && !unicode.IsLetter(r)
}
