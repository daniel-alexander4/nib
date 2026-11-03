// Package text writes the document model as plain text (PLAN-import-export D28, ADR-193): paragraphs in reading order
// separated by a blank line, a list item behind its marker, a table row on one line with a tab between cells. It imports
// the model and no PDF reader.
package text

import (
	"strconv"
	"strings"

	"nib/internal/docmodel"
)

// Write is the document as UTF-8 text with a final newline, or nil for a document with no block.
func Write(doc docmodel.Document) []byte {
	var out []string
	for i, b := range doc.Blocks {
		s := block(b)
		if s == "" {
			continue
		}
		// List items stay together; everything else is set apart by a blank line.
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

func plain(sp []docmodel.Span) string {
	var b strings.Builder
	for _, s := range sp {
		b.WriteString(s.Text)
	}
	return docmodel.OneLine(b.String())
}

func block(b docmodel.Block) string {
	switch b.Kind {
	case docmodel.ListItem:
		ind := strings.Repeat("  ", max(b.Level-1, 0))
		m := docmodel.OneLine(b.Marker)
		if m == "" {
			m = "-"
		}
		return ind + m + " " + plain(b.Spans)
	case docmodel.Table:
		var rows []string
		for _, r := range b.Rows {
			cells := make([]string, len(r))
			for i, c := range r {
				var ps []string
				for _, p := range c.Paragraphs {
					if s := plain(p); s != "" {
						ps = append(ps, s)
					}
				}
				cells[i] = strings.Join(ps, " ")
			}
			rows = append(rows, strings.Join(cells, "\t"))
		}
		return strings.Join(rows, "\n")
	case docmodel.Figure:
		if b.Image == nil {
			return ""
		}
		alt := docmodel.OneLine(b.Image.Alt)
		if alt == "" {
			alt = "picture on page " + strconv.Itoa(b.Page)
		}
		return "[Picture: " + alt + "]"
	}
	return plain(b.Spans)
}
