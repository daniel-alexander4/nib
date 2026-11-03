// Package markdown writes the document model as CommonMark with GFM tables (PLAN-import-export P02.S04, ADR-193). Every
// text value goes through `escape`, the one door for Markdown syntax (Y9): a PDF's words can hold `*`, `[`, `|`, `<`, a
// line that starts with `#` — none of which may become structure. It imports the model and no PDF reader.
package markdown

import (
	"strconv"
	"strings"

	"nib/internal/docmodel"
	"nib/internal/docwrite/xmlw"
)

// Write is the document as Markdown, or nil for a document with no block. A picture is a bracketed line naming it: a
// single .md file carries no bytes, and writing a file beside it under a name the PDF chose is exactly what Y9 forbids.
func Write(doc docmodel.Document) []byte {
	var out []string
	for i, b := range doc.Blocks {
		s := block(b)
		if s == "" {
			continue
		}
		if i > 0 && !(b.Kind == docmodel.ListItem && doc.Blocks[i-1].Kind == docmodel.ListItem) {
			out = append(out, "")
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return []byte(strings.Join(out, "\n") + "\n")
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
	for _, c := range []byte(u) {
		switch c {
		case ' ', '(', ')', '<', '>', '\\', '"', '`', '[', ']', '*', '_', '|':
			b.WriteString("%" + strings.ToUpper(strconv.FormatInt(int64(c)+256, 16)[1:]))
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// spans renders styled text. A marker hugs its text, so the whitespace at either edge of a styled span moves outside.
func spans(sp []docmodel.Span) string {
	var b strings.Builder
	for _, s := range sp {
		t := docmodel.OneLine(s.Text)
		if strings.TrimSpace(s.Text) == "" {
			if s.Text != "" {
				b.WriteByte(' ')
			}
			continue
		}
		// OneLine trims; restore a single edge space where the span began or ended with whitespace.
		lead, trail := startsSpace(s.Text), endsSpace(s.Text)
		e := escape(t)
		if s.Bold && s.Italic {
			e = "***" + e + "***"
		} else if s.Bold {
			e = "**" + e + "**"
		} else if s.Italic {
			e = "*" + e + "*"
		}
		if s.Link != "" && xmlw.SafeLink(s.Link) {
			e = "[" + e + "](" + linkTarget(s.Link) + ")"
		}
		if lead {
			b.WriteByte(' ')
		}
		b.WriteString(e)
		if trail {
			b.WriteByte(' ')
		}
	}
	return strings.TrimSpace(collapse(b.String()))
}

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
	case docmodel.ListItem:
		t := spans(b.Spans)
		if t == "" {
			return ""
		}
		m := "-"
		if n := docmodel.OneLine(b.Marker); xmlw.Numbered(n) {
			m = n
		}
		return strings.Repeat("    ", max(b.Level-1, 0)) + m + " " + lineStart(t)
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
