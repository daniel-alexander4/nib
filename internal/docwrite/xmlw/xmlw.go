// Package xmlw is what the two word-processor writers (docx, odt) share and nothing more: the ONE door text passes
// through on its way into an XML attribute or element (PLAN-import-export Y9), the link allowlist, the language tag and
// the numbered-marker test. It imports the model and no PDF reader.
package xmlw

import (
	"net/url"
	"strings"

	"nib/internal/docmodel"
)

// Esc is s as XML character data or an attribute value: `docmodel.Clean` first (XML 1.0 forbids what it drops), then
// the five predefined entities. Every text and attribute value either writer emits goes through it.
func Esc(s string) string {
	s = docmodel.Clean(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SafeLink says a link may be written: http, https or mailto (W1). The model already carries only those; a writer
// asks again because the model is data and a hand-built one is hostile in the tests.
func SafeLink(u string) bool {
	p, err := url.Parse(u)
	if err != nil {
		return false
	}
	switch strings.ToLower(p.Scheme) {
	case "http", "https", "mailto":
		return true
	}
	return false
}

// LangTag is a catalog /Lang kept only when it is a BCP 47 shape — subtags of one to eight letters or digits joined by
// single hyphens, the first of them two to eight letters (or the single letter i or x) — anything else is "und".
func LangTag(l string) string {
	if len(l) > 35 {
		return "und"
	}
	for i, part := range strings.Split(l, "-") {
		if part == "" || len(part) > 8 {
			return "und"
		}
		for _, c := range part {
			letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
			if !letter && !(c >= '0' && c <= '9') || i == 0 && !letter {
				return "und"
			}
		}
		if i == 0 && len(part) == 1 && part != "i" && part != "x" && part != "I" && part != "X" {
			return "und"
		}
	}
	return l
}

// Numbered says a list item's marker as drawn is a counter ("1.", "2)"), so the list is numbered, not bulleted.
func Numbered(m string) bool {
	m = strings.TrimSpace(m)
	return len(m) > 1 && (m[len(m)-1] == '.' || m[len(m)-1] == ')') && m[0] >= '0' && m[0] <= '9'
}

// Title is the document's title as one line, else "Document".
func Title(m docmodel.Meta) string {
	if t := docmodel.OneLine(m.Title); t != "" {
		return t
	}
	return "Document"
}

// MIME is the media type of a carried picture's extension, "" for one a writer does not carry.
func MIME(ext string) string { return map[string]string{"jpg": "image/jpeg", "png": "image/png"}[ext] }

// Cell is one table cell placed on the grid: the model's cell, or a covered position under a row span.
type Cell struct {
	docmodel.Cell
	// Covered is a position a row span from above occupies; Cell is empty then.
	Covered bool
	// Restart says this cell begins a row span (RowSpan > 1).
	Restart bool
}

// MaxSpan caps a cell's column and row span, so a hostile tag cannot make a writer emit billions of covered cells.
const MaxSpan = 256

func heldTo[H interface{ ~struct{ rows, span int } }](m map[int]H) int {
	last := 0
	for c, h := range m {
		if r := struct{ rows, span int }(h); r.rows > 0 {
			last = max(last, c+r.span)
		}
	}
	return last
}

// Grid lays a table's rows out on columns: a cell that spans rows leaves one Covered cell in each row below, at the
// column it begins and with its column span, so a format that wants every grid position written (DOCX `vMerge`, ODF
// covered cells) gets them. A cell that spans columns is one cell; its covered positions to the right are the writer's.
func Grid(rows [][]docmodel.Cell) [][]Cell {
	type hold struct{ rows, span int }
	var out [][]Cell
	under := map[int]hold{} // column a span begins at -> what is left of it
	for _, r := range rows {
		var line []Cell
		col := 0
		fill := func() {
			for {
				h, ok := under[col]
				if !ok || h.rows == 0 {
					return
				}
				h.rows--
				under[col] = h
				line = append(line, Cell{Covered: true, Cell: docmodel.Cell{ColSpan: h.span, RowSpan: 1}})
				col += h.span
			}
		}
		for _, c := range r {
			fill()
			c.ColSpan = min(max(c.ColSpan, 1), MaxSpan)
			c.RowSpan = min(max(c.RowSpan, 1), MaxSpan)
			line = append(line, Cell{Cell: c, Restart: c.RowSpan > 1})
			if c.RowSpan > 1 {
				under[col] = hold{rows: c.RowSpan - 1, span: c.ColSpan}
			}
			col += c.ColSpan
		}
		fill()
		// A row that ends before a column a span from above still holds is padded up to it with empty cells, so the
		// covered position lands in the column it belongs to.
		for last := heldTo(under); col < last; {
			if h, ok := under[col]; ok && h.rows > 0 {
				fill()
				continue
			}
			line = append(line, Cell{Cell: docmodel.Cell{ColSpan: 1, RowSpan: 1}})
			col++
		}
		out = append(out, line)
	}
	return out
}
