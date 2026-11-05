// Package markdown writes the document model as CommonMark with GFM tables (PLAN-import-export P02.S04, ADR-193). Every
// text value goes through `escape`, the one door for Markdown syntax (Y9): a PDF's words can hold `*`, `[`, `|`, `<`, a
// line that starts with `#` — none of which may become structure. It imports the model and no PDF reader.
package markdown

import (
	"context"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"nib/internal/docmodel"
	"nib/internal/docwrite/xmlw"
)

// Write is the document as Markdown, or nil for a document with no block. A picture is a bracketed line naming it: a
// single .md file carries no bytes, and writing a file beside it under a name the PDF chose is exactly what Y9 forbids.
func Write(ctx context.Context, doc docmodel.Document) ([]byte, error) {
	var out []string
	var ls listState
	for i, b := range doc.Blocks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var s string
		if b.Kind == docmodel.ListItem {
			s = ls.item(b)
		} else {
			ls = listState{}
			s = block(b)
		}
		if s == "" {
			continue
		}
		prevItem := i > 0 && doc.Blocks[i-1].Kind == docmodel.ListItem
		// Items of one list sit together; a new list at the top level is parted by a blank line, and by a different
		// marker (listState), because two lists in a row with one marker are ONE list in CommonMark.
		if i > 0 && !(b.Kind == docmodel.ListItem && prevItem && !ls.fresh) {
			out = append(out, "")
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return []byte(strings.Join(out, "\n") + "\n"), nil
}

// listState remembers what a run of list items needs from the item before it: where an item's content begins at each
// depth (a nested item is indented by its parent's content offset, not by a fixed four — "100. x" starts its content at
// column 5, and four spaces would not nest under it), and which list each depth is in, so a new list can take the other
// marker (/pending 941 #17, #18).
type listState struct {
	width [8]int // width[d] is the content offset of the latest item at depth d+1
	id    [8]int // the List id of the latest item at depth d+1
	flip  [8]bool
	fresh bool // the item just written began a list at the top level
}

func (ls *listState) item(b docmodel.Block) string {
	t := spans(b.Spans)
	if t == "" {
		return ""
	}
	d := min(max(b.Level, 1), len(ls.width)) - 1
	ls.fresh = false
	if ls.id[d] != 0 && b.List != ls.id[d] {
		ls.flip[d] = !ls.flip[d]
		ls.fresh = d == 0
	}
	ls.id[d] = b.List
	for k := d + 1; k < len(ls.width); k++ { // a shallower item ends every deeper list
		ls.width[k], ls.id[k], ls.flip[k] = 0, 0, false
	}
	m := "-"
	if ls.flip[d] {
		m = "*"
	}
	if n := docmodel.OneLine(b.Marker); numbered(n) {
		m = n[:len(n)-1] + "."
		if ls.flip[d] {
			m = n[:len(n)-1] + ")"
		}
	}
	ls.width[d] = len(m) + 1
	indent := 0
	for k := 0; k < d; k++ {
		indent += max(ls.width[k], 2) // a jump over a depth has no parent item: a bullet's width stands in
	}
	return strings.Repeat(" ", indent) + m + " " + lineStart(t)
}

// numbered is a marker CommonMark reads as an ordered list: one to nine digits, then "." or ")". A longer number would be
// a paragraph, and "1a." is not a number.
func numbered(m string) bool {
	if !xmlw.Numbered(m) {
		return false
	}
	n := len(m) - 1
	if n > 9 {
		return false
	}
	for i := 0; i < n; i++ {
		if m[i] < '0' || m[i] > '9' {
			return false
		}
	}
	return true
}

// escape backslash-escapes every character Markdown reads as syntax inside a line.
func escape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\', '`', '*', '_', '[', ']', '<', '>', '|', '~', '&':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// lineStart escapes what only means something at the start of a line: a heading, a quote, a bullet, "1." and a rule.
func lineStart(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '#', '>', '+', '-', '=':
		return "\\" + s
	}
	n := 0
	for n < len(s) && s[n] >= '0' && s[n] <= '9' {
		n++
	}
	if n > 0 && n < len(s) && (s[n] == '.' || s[n] == ')') {
		return s[:n] + "\\" + s[n:]
	}
	return s
}

// linkTarget percent-encodes what would end or break a Markdown destination. Only http, https and mailto get here.
func linkTarget(u string) string {
	var b strings.Builder
	for i, c := range []byte(u) {
		switch c {
		case ' ', '(', ')', '<', '>', '\\', '"', '`', '[', ']', '*', '_', '|':
			b.WriteString("%" + strings.ToUpper(strconv.FormatInt(int64(c)+256, 16)[1:]))
		case '&':
			// A destination reads character references: "&amp;" would arrive as "&" and "&copy;" as "©". Only a "&" that
			// begins one is written as a reference to itself; a query's own "&y=2" stays as it is (/pending 941 #19).
			if entityAt(u[i:]) {
				b.WriteString("&amp;")
			} else {
				b.WriteByte(c)
			}
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// entityAt is whether s, which begins with "&", begins a character reference: "&" then a name or "#" number, then ";".
func entityAt(s string) bool {
	n := 1
	for n < len(s) && n < 34 && (s[n] == '#' || s[n] >= '0' && s[n] <= '9' || s[n] >= 'a' && s[n] <= 'z' || s[n] >= 'A' && s[n] <= 'Z') {
		n++
	}
	return n > 1 && n < len(s) && s[n] == ';'
}

// spans renders styled text. A marker hugs its text, so the whitespace at either edge of a styled span moves outside.
// Neighbouring runs of one style are one span first (a PDF splits a word into runs), and a marker is written only where
// CommonMark will read it as emphasis (flanks): a delimiter beside punctuation needs whitespace or punctuation on its
// other side, and where that does not hold the span is written plain — the words are right and the style is lost, which
// is better than a pair of literal asterisks (/pending 941 #16).
func spans(sp []docmodel.Span) string {
	var merged []docmodel.Span
	for _, s := range sp {
		if n := len(merged); n > 0 && merged[n-1].Bold == s.Bold && merged[n-1].Italic == s.Italic && merged[n-1].Link == s.Link {
			merged[n-1].Text += s.Text
			continue
		}
		merged = append(merged, s)
	}
	type piece struct {
		e            string // the escaped text, one line
		lead, trail  bool   // whitespace began or ended the run
		style, blank bool   // it is emphasised; it is only whitespace
		sp           docmodel.Span
	}
	ps := make([]piece, len(merged))
	for i, s := range merged {
		if strings.TrimSpace(docmodel.Clean(s.Text)) == "" {
			// blank is whitespace that SURVIVES Clean: a span of only a byte-order mark or a bidi control is nothing, and must not
			// become the space between two styled runs (`foo`, U+FEFF, `bar`) (/pending 946 I4).
			ps[i] = piece{blank: docmodel.Clean(s.Text) != "", sp: s}
			continue
		}
		p := piece{sp: s}
		// OneLine trims; restore a single edge space where the span began or ended with whitespace.
		p.lead, p.trail = startsSpace(docmodel.Clean(s.Text)), endsSpace(docmodel.Clean(s.Text))
		p.e = escape(docmodel.OneLine(s.Text))
		p.style = s.Bold || s.Italic
		ps[i] = p
	}
	// nextIsLink is whether the next piece with text, from index j, is written as a link with no space before it.
	nextIsLink := func(j int) bool {
		for _, q := range ps[j:] {
			if q.e != "" {
				return !q.lead && q.sp.Link != "" && xmlw.SafeLink(q.sp.Link)
			}
			if q.blank {
				return false
			}
		}
		return false
	}
	var b strings.Builder
	prev := rune(' ') // the character before the next piece: the start of a block counts as whitespace
	closed := false   // the previous piece ended in an emphasis delimiter, which a second one beside it would run into
	for i, p := range ps {
		if p.e == "" {
			if p.blank {
				b.WriteByte(' ')
				prev, closed = ' ', false
			}
			continue
		}
		e := p.e
		s := p.sp
		linked := s.Link != "" && xmlw.SafeLink(s.Link)
		if p.style && !linked {
			before := prev
			if p.lead {
				before = ' '
			}
			after := ' '
			if !p.trail {
				for _, q := range ps[i+1:] {
					if q.e != "" {
						if !q.lead {
							after, _ = utf8.DecodeRuneInString(q.e)
						}
						break
					}
					if q.blank {
						break
					}
				}
			}
			if !flanks(e, before, after) || (closed && !p.lead) {
				p.style = false
			}
		}
		closed = p.style && !linked
		// A "!" ending the text, with a link written straight after it, is the start of image syntax: `Hello![x](u)` is a
		// picture. Escaped only there (/pending 946 I5); a "!" anywhere else is a plain character.
		if !p.style && !p.trail && strings.HasSuffix(e, "!") && nextIsLink(i+1) {
			e = e[:len(e)-1] + `\!`
		}
		if p.style {
			switch {
			case s.Bold && s.Italic:
				e = "***" + e + "***"
			case s.Bold:
				e = "**" + e + "**"
			default:
				e = "*" + e + "*"
			}
		}
		if linked {
			e = "[" + e + "](" + linkTarget(s.Link) + ")"
		}
		if p.lead {
			b.WriteByte(' ')
		}
		b.WriteString(e)
		prev, _ = utf8.DecodeLastRuneInString(e)
		if p.trail {
			b.WriteByte(' ')
			prev, closed = ' ', false
		}
	}
	return strings.TrimSpace(collapse(b.String()))
}

// flanks is whether an emphasis around e, with before and after the characters just outside it, is read as emphasis:
// the opening delimiter is left-flanking and the closing one right-flanking (CommonMark 6.2). e is already escaped, so
// its edge is a backslash when the text begins or ends in syntax, and a backslash is punctuation too. A symbol counts as
// punctuation: a parser that does not treat it so reads fewer spans as emphasis than this, never more.
func flanks(e string, before, after rune) bool {
	first, _ := utf8.DecodeRuneInString(e)
	last, _ := utf8.DecodeLastRuneInString(e)
	if isPunct(first) && !(unicode.IsSpace(before) || isPunct(before)) {
		return false
	}
	if isPunct(last) && !(unicode.IsSpace(after) || isPunct(after)) {
		return false
	}
	return true
}

func isPunct(r rune) bool { return unicode.IsPunct(r) || unicode.IsSymbol(r) }

func startsSpace(s string) bool { return s != "" && strings.TrimLeft(s, " \t\n") != s }
func endsSpace(s string) bool   { return s != "" && strings.TrimRight(s, " \t\n") != s }

func collapse(s string) string {
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}

func block(b docmodel.Block) string {
	switch b.Kind {
	case docmodel.Heading:
		l := min(max(b.Level, 1), 6)
		t := spans(b.Spans)
		if t == "" {
			return ""
		}
		if strings.HasSuffix(t, "#") && !strings.HasSuffix(t, "\\#") {
			t = t[:len(t)-1] + "\\#" // a trailing "#" would read as a closing sequence and vanish
		}
		return strings.Repeat("#", l) + " " + t
	case docmodel.Table:
		return table(b)
	case docmodel.Figure:
		if b.Image == nil {
			return ""
		}
		alt := docmodel.OneLine(b.Image.Alt)
		if alt == "" {
			alt = "picture on page " + strconv.Itoa(b.Page)
		}
		return "*[Picture: " + escape(alt) + "]*"
	}
	return lineStart(spans(b.Spans))
}

func table(b docmodel.Block) string {
	if len(b.Rows) == 0 {
		return ""
	}
	cols := 0
	for _, r := range b.Rows {
		n := 0
		for _, c := range r {
			n += max(c.ColSpan, 1)
		}
		cols = max(cols, n)
	}
	if cols == 0 {
		return ""
	}
	var grid [][]string
	carry := map[int]int{} // column -> rows still covered by a cell above that spans down
	for _, r := range b.Rows {
		cells := make([]string, 0, cols)
		// skip fills the columns a cell from an earlier row still covers: the row simply has no cell there, and
		// without the filler every later cell would slide one column left under the wrong header.
		skip := func() {
			for carry[len(cells)] > 0 {
				carry[len(cells)]--
				cells = append(cells, "")
			}
		}
		for _, c := range r {
			skip()
			var ps []string
			for _, p := range c.Paragraphs {
				if s := spans(p); s != "" {
					ps = append(ps, s)
				}
			}
			at := len(cells)
			cells = append(cells, strings.Join(ps, "<br>"))
			// A merged cell fills the columns it spans with empty cells: GFM has no span.
			for k := 1; k < c.ColSpan; k++ {
				cells = append(cells, "")
			}
			for k := 0; c.RowSpan > 1 && k < max(c.ColSpan, 1); k++ {
				carry[at+k] = c.RowSpan - 1
			}
		}
		skip()
		cols = max(cols, len(cells))
		grid = append(grid, cells)
	}
	var lines []string
	for i, cells := range grid {
		for len(cells) < cols {
			cells = append(cells, "")
		}
		lines = append(lines, "| "+strings.Join(cells, " | ")+" |")
		if i == 0 {
			sep := make([]string, cols)
			for k := range sep {
				sep[k] = "---"
			}
			lines = append(lines, "| "+strings.Join(sep, " | ")+" |")
		}
	}
	return strings.Join(lines, "\n")
}
