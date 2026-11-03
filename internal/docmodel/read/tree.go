package read

import (
	"strings"

	"nib/internal/docmodel"
	"nib/internal/pdfops"
)

// treeBlocks reads the author's structure tree into model blocks, bucketed by the page each begins on, and the box
// (fractions of the page) of each Figure so it can be joined to the picture the page draws.
//
// An element's Text is its own marked content AND its kids' (`structview.go`), so a block-level element is emitted
// whole and not descended into; containers are descended. Declared: the tree route carries text and structure, and
// borrows style and links from the page's own paragraph with identical words (`withStyles`); a paragraph the page
// reads differently keeps plain text.
func treeBlocks(t pdfops.StructureTree) (map[int][]docmodel.Block, map[*docmodel.Image][4]float64) {
	out := map[int][]docmodel.Block{}
	boxes := map[*docmodel.Image][4]float64{}
	els := t.Elements
	std := func(e pdfops.StructureElement) string {
		if e.Standard != "" {
			return e.Standard
		}
		return e.Kind
	}
	emit := func(page int, b docmodel.Block) { out[page] = append(out[page], b) }
	text := func(e pdfops.StructureElement) []docmodel.Span {
		s := strings.TrimSpace(e.Text)
		if s == "" {
			return nil
		}
		return []docmodel.Span{{Text: s}}
	}
	var walk func(i, listDepth int)
	var descendants func(i int, kind string, fn func(int))
	descendants = func(i int, kind string, fn func(int)) {
		for _, k := range els[i].Kids {
			if k < 0 || k >= len(els) {
				continue
			}
			if std(els[k]) == kind {
				fn(k)
			}
			descendants(k, kind, fn)
		}
	}
	cellsOf := func(row int) []docmodel.Cell {
		var cells []docmodel.Cell
		for _, k := range els[row].Kids {
			c := els[k]
			if s := std(c); s != "TH" && s != "TD" {
				continue
			}
			cell := docmodel.Cell{ColSpan: max(c.ColSpan, 1), RowSpan: max(c.RowSpan, 1), Header: std(c) == "TH"}
			if sp := text(c); sp != nil {
				cell.Paragraphs = [][]docmodel.Span{sp}
			}
			cells = append(cells, cell)
		}
		return cells
	}
	table := func(i int) docmodel.Block {
		b := docmodel.Block{Kind: docmodel.Table}
		var rows func(j int)
		rows = func(j int) {
			for _, k := range els[j].Kids {
				switch std(els[k]) {
				case "TR":
					b.Rows = append(b.Rows, cellsOf(k))
				case "THead", "TBody", "TFoot":
					rows(k)
				}
			}
		}
		rows(i)
		for _, r := range b.Rows {
			if len(r) == 0 {
				break
			}
			allTH := true
			for _, c := range r {
				allTH = allTH && c.Header
			}
			if !allTH {
				break
			}
			b.HeaderRows++
		}
		return b
	}
	walk = func(i, listDepth int) {
		e := els[i]
		switch s := std(e); s {
		case "H", "H1", "H2", "H3", "H4", "H5", "H6":
			lvl := 1
			if len(s) == 2 {
				lvl = int(s[1] - '0')
			}
			if sp := text(e); sp != nil {
				emit(e.Page, docmodel.Block{Kind: docmodel.Heading, Level: lvl, Spans: sp})
			}
		case "LI":
			own := e.Text
			var nested []int
			descendants(i, "L", func(k int) { nested = append(nested, k) })
			for _, k := range nested {
				own = strings.Replace(own, els[k].Text, "", 1)
			}
			marker := ""
			for _, k := range e.Kids {
				if std(els[k]) == "Lbl" {
					marker = strings.TrimSpace(els[k].Text)
				}
			}
			body := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(own), marker))
			if body != "" || marker != "" {
				emit(e.Page, docmodel.Block{Kind: docmodel.ListItem, Level: max(listDepth, 1), Marker: marker, List: i,
					Spans: []docmodel.Span{{Text: body}}})
			}
			for _, k := range e.Kids {
				walkNested(els, k, std, func(j int) { walk(j, max(listDepth, 1)) })
			}
		case "L":
			for _, k := range e.Kids {
				walk(k, listDepth+1)
			}
		case "Table":
			emit(e.Page, table(i))
		case "Figure":
			im := &docmodel.Image{Alt: e.Alt}
			boxes[im] = [4]float64(e.Place)
			emit(e.Page, docmodel.Block{Kind: docmodel.Figure, Image: im})
		case "P", "Caption", "BlockQuote", "Quote", "Code", "Note", "BibEntry", "Formula", "Reference", "Span", "Link", "Lbl", "LBody", "TOCI", "Form", "Annot":
			if sp := text(e); sp != nil {
				emit(e.Page, docmodel.Block{Kind: docmodel.Paragraph, Spans: sp})
			}
		default:
			if len(e.Kids) == 0 {
				if sp := text(e); sp != nil {
					emit(e.Page, docmodel.Block{Kind: docmodel.Paragraph, Spans: sp})
				}
				return
			}
			for _, k := range e.Kids {
				walk(k, listDepth)
			}
		}
	}
	for i, e := range els {
		if e.Parent == -1 {
			walk(i, 0)
		}
	}
	return out, boxes
}

// walkNested calls fn for every L at or below k that is not inside another L found first (an LI's nested lists).
func walkNested(els []pdfops.StructureElement, k int, std func(pdfops.StructureElement) string, fn func(int)) {
	if k < 0 || k >= len(els) {
		return
	}
	if std(els[k]) == "L" {
		fn(k)
		return
	}
	for _, c := range els[k].Kids {
		walkNested(els, c, std, fn)
	}
}
