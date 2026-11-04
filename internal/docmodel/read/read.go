// Package read is the ONE reader of the document model (PLAN-import-export D16, D17, D20, D30, ADR-192): it composes
// `pdfops.ExportRead` (the visible text, grouped, styled, with its links and pictures, and the tagger's roles) with the
// author's structure tree, page by page, and returns a `docmodel.Document` and a Report of what it left out.
//
// **The author's tree wins, one page at a time** (D17): a page takes the tree when the tree accounts for exactly the
// words a reader sees on it (a multiset, running headers and footers aside), and the proposal otherwise, named in
// `Report.FlaggedPages` as `tree-incomplete`. Equality both ways is the safe test: a word the tree holds that the page
// does not show (text beneath a box, a redaction's remains) sends the page to the proposal, which reads only what is seen.
package read

import (
	"context"
	"strings"
	"unicode"

	"nib/internal/docmodel"
	"nib/internal/pdfops"
)

// Options are what a caller supplies. Title is never read from a path.
type Options struct {
	Title string
}

// Read reads pdf into the model. ctx is observed between pages here and inside `pdfops.ExportRead`; a cancelled read
// returns the context's error (/pending 941 #1).
func Read(ctx context.Context, pdf []byte, opts Options) (docmodel.Document, error) {
	ed, err := pdfops.ExportRead(ctx, pdf, nil)
	if err != nil {
		return docmodel.Document{}, err
	}
	if err := ctx.Err(); err != nil {
		return docmodel.Document{}, err
	}
	tree, terr := pdfops.ReadStructure(pdf)
	doc := docmodel.Document{Meta: docmodel.Meta{Title: opts.Title, Lang: ed.Lang}}
	if doc.Meta.Lang == "" {
		doc.Meta.Lang = "und"
	}
	rep := &doc.Report
	rep.Placeholders, rep.DroppedLinks, rep.Layout = map[string]int{}, map[string]int{}, map[int]string{}
	rep.SkippedImages = map[string]int{}
	rep.Tagged = terr == nil && tree.Tagged
	rep.TreeUnread = terr != nil
	var byPage map[int][]docmodel.Block
	var boxes map[*docmodel.Image][4]float64
	if rep.Tagged {
		byPage, boxes = treeBlocks(tree)
	}
	usedTree, usedProposal := false, false
	for _, ep := range ed.Pages {
		if err := ctx.Err(); err != nil {
			return docmodel.Document{}, err
		}
		prop := proposalBlocks(ep)
		blocks := prop
		flag := func(cause string) {
			rep.FlaggedPages = append(rep.FlaggedPages, docmodel.FlaggedPage{Page: ep.Page, Cause: cause})
		}
		if rep.Tagged {
			tb := byPage[ep.Page]
			if sameWords(tb, prop) && (len(tb) > 0 || len(prop) == 0) {
				blocks = withStyles(tb, prop)
				rep.RunningSetAside += countRunning(prop)
				blocks = withFigures(blocks, ep, boxes)
				usedTree = true
			} else if hasText(prop) || len(tb) > 0 {
				flag(docmodel.FlagTreeIncomplete)
				usedProposal = true
			}
		} else {
			usedProposal = true
		}
		blocks = scanned(blocks, ep, rep)
		if ep.Unsupported != "" {
			rep.Layout[ep.Page] = ep.Unsupported
			flag(docmodel.FlagLayout)
		}
		if ep.Visibility.UnreadGlyphs > 0 {
			flag(docmodel.FlagUnread)
		}
		if len(blocks) == 0 {
			flag(docmodel.FlagEmpty)
		}
		v := ep.Visibility
		rep.CoveredText += v.Covered
		rep.MaybeCovered += v.MaybeCovered
		rep.HiddenText += v.HiddenText
		rep.HiddenLayer += v.HiddenLayer
		rep.OffPage += v.OffPage
		rep.CoverUntested += v.CoverUntested
		rep.UnreadGlyphs += v.UnreadGlyphs
		rep.UnstyledLines += ep.UnstyledLines
		if ep.NoText {
			rep.NoTextPages++
		}
		for k, n := range ep.SkippedImages {
			rep.SkippedImages[k] += n
		}
		for k, n := range ep.DroppedLinks {
			rep.DroppedLinks[k] += n
		}
		blocks = boundTables(blocks, rep)
		for i := range blocks {
			blocks[i].Page = ep.Page
			if blocks[i].Dir == "" {
				blocks[i].Dir = dirOf(blocks[i])
			}
			if im := blocks[i].Image; im != nil && im.Placeholder != "" {
				rep.Placeholders[im.Placeholder]++
			}
		}
		doc.Blocks = append(doc.Blocks, blocks...)
	}
	switch {
	case usedTree && usedProposal:
		rep.Source = docmodel.SourceMixed
	case usedTree:
		rep.Source = docmodel.SourceTree
	default:
		rep.Source = docmodel.SourceProposal
	}
	doc.Meta.Dir = majorityDir(doc.Blocks)
	return doc, nil
}

// proposalBlocks turns a page's proposed blocks into model blocks.
func proposalBlocks(ep pdfops.ExportPage) []docmodel.Block {
	var out []docmodel.Block
	for _, b := range ep.Blocks {
		switch {
		case b.Role == "Table":
			t := ep.Tables[b.Table]
			blk := docmodel.Block{Kind: docmodel.Table}
			for _, row := range t.Rows {
				var r []docmodel.Cell
				for _, c := range row {
					cell := docmodel.Cell{ColSpan: 1, RowSpan: 1}
					for _, p := range c.Paragraphs {
						cell.Paragraphs = append(cell.Paragraphs, spansOf(p))
					}
					r = append(r, cell)
				}
				blk.Rows = append(blk.Rows, r)
			}
			if len(blk.Rows) > 1 {
				blk.HeaderRows = 1
				for i := range blk.Rows[0] {
					blk.Rows[0][i].Header = true
				}
			}
			out = append(out, blk)
		case b.Role == "Figure":
			out = append(out, docmodel.Block{Kind: docmodel.Figure, Image: imageOf(ep.Figures[b.Figure])})
		case strings.HasPrefix(b.Role, "H") && len(b.Role) == 2:
			out = append(out, docmodel.Block{Kind: docmodel.Heading, Level: int(b.Role[1] - '0'), Spans: spansOf(b.Paragraph), Running: b.Paragraph.Running})
		case b.Role == "LI":
			sp := spansOf(b.Paragraph)
			if len(sp) > 0 && b.Marker != "" {
				sp[0].Text = strings.TrimLeft(strings.TrimPrefix(sp[0].Text, b.Marker), " ")
			}
			out = append(out, docmodel.Block{Kind: docmodel.ListItem, Level: 1, Marker: b.Marker, List: b.List, Spans: sp})
		default:
			out = append(out, docmodel.Block{Kind: docmodel.Paragraph, Spans: spansOf(b.Paragraph), Running: b.Paragraph.Running})
		}
	}
	return out
}

func spansOf(p pdfops.ExportParagraph) []docmodel.Span {
	out := make([]docmodel.Span, 0, len(p.Spans))
	for _, s := range p.Spans {
		out = append(out, docmodel.Span{Text: s.Text, Bold: s.Bold, Italic: s.Italic, Link: s.Link})
	}
	return out
}

func imageOf(f pdfops.ExportFigure) *docmodel.Image {
	return &docmodel.Image{Data: f.Data, Ext: f.Ext, Width: f.Width, Height: f.Height, Placeholder: f.Placeholder}
}

// scanned gives a page that draws no text and one picture over most of it that picture as its content (D20).
//
// **"Draws no text" is the page's own fact (`ExportPage.NoText`, no glyph in any stream), not "none survived the
// visibility rules"**: a page whose text sits under a later opaque picture, or off the page, draws text, and calling it
// a scan would name it "Scanned page N" and send it to the reader as a picture (/pending 941 #12).
func scanned(blocks []docmodel.Block, ep pdfops.ExportPage, rep *docmodel.Report) []docmodel.Block {
	if hasText(blocks) || !ep.NoText {
		return blocks
	}
	big := -1
	best := 0.5
	for i, f := range ep.Figures {
		if a := (f.Rect[2] - f.Rect[0]) * (f.Rect[3] - f.Rect[1]); a >= best {
			big, best = i, a
		}
	}
	if big < 0 {
		return blocks
	}
	im := imageOf(ep.Figures[big])
	im.Alt = "Scanned page " + itoa(ep.Page)
	rep.ScannedPages = append(rep.ScannedPages, ep.Page)
	out := []docmodel.Block{{Kind: docmodel.Figure, Image: im}}
	for _, b := range blocks {
		if b.Kind == docmodel.Figure && b.Image != nil && b.Image.Placeholder == imageOf(ep.Figures[big]).Placeholder && sameImage(b.Image, ep.Figures[big]) {
			continue
		}
		out = append(out, b)
	}
	return out
}

func sameImage(im *docmodel.Image, f pdfops.ExportFigure) bool {
	return im.Width == f.Width && im.Height == f.Height && len(im.Data) == len(f.Data)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func hasText(blocks []docmodel.Block) bool {
	for _, b := range blocks {
		if b.Kind != docmodel.Figure {
			return true
		}
	}
	return false
}

func countRunning(blocks []docmodel.Block) int {
	n := 0
	for _, b := range blocks {
		if b.Running {
			n++
		}
	}
	return n
}

// words is a block's words, by kind, for the multiset comparison; running furniture is not compared.
func words(blocks []docmodel.Block, into map[string]int) {
	for _, b := range blocks {
		if b.Running {
			continue
		}
		var parts []string
		if b.Marker != "" {
			parts = append(parts, b.Marker)
		}
		parts = append(parts, b.Text())
		for _, r := range b.Rows {
			for _, c := range r {
				for _, p := range c.Paragraphs {
					parts = append(parts, plain(p))
				}
			}
		}
		for _, w := range strings.Fields(strings.Join(parts, " ")) {
			into[w]++
		}
	}
}

func plain(sp []docmodel.Span) string {
	var s string
	for _, x := range sp {
		s += x.Text
	}
	return s
}

func sameWords(a, b []docmodel.Block) bool {
	ma, mb := map[string]int{}, map[string]int{}
	words(a, ma)
	words(b, mb)
	if len(ma) != len(mb) {
		return false
	}
	for w, n := range ma {
		if mb[w] != n {
			return false
		}
	}
	return true
}

// withStyles gives each tree paragraph the spans of the proposal's paragraph with the same words, when there is one:
// the tree holds text and the page holds the style, and identical text is the join.
func withStyles(tree, prop []docmodel.Block) []docmodel.Block {
	pool := map[string][][]docmodel.Span{}
	add := func(sp []docmodel.Span) {
		k := strings.Join(strings.Fields(plain(sp)), " ")
		pool[k] = append(pool[k], sp)
	}
	for _, b := range prop {
		if b.Kind == docmodel.Table {
			for _, r := range b.Rows {
				for _, c := range r {
					for _, p := range c.Paragraphs {
						add(p)
					}
				}
			}
		} else if b.Kind != docmodel.Figure {
			add(b.Spans)
		}
	}
	take := func(sp []docmodel.Span) []docmodel.Span {
		k := strings.Join(strings.Fields(plain(sp)), " ")
		if q := pool[k]; len(q) > 0 {
			pool[k] = q[1:]
			return q[0]
		}
		return sp
	}
	out := make([]docmodel.Block, len(tree))
	copy(out, tree)
	for i := range out {
		b := &out[i]
		if b.Kind == docmodel.Table {
			rows := make([][]docmodel.Cell, len(b.Rows))
			for r := range b.Rows {
				rows[r] = append([]docmodel.Cell(nil), b.Rows[r]...)
				for c := range rows[r] {
					ps := make([][]docmodel.Span, len(rows[r][c].Paragraphs))
					for p, sp := range rows[r][c].Paragraphs {
						ps[p] = take(sp)
					}
					rows[r][c].Paragraphs = ps
				}
			}
			b.Rows = rows
		} else if b.Kind != docmodel.Figure {
			b.Spans = take(b.Spans)
		}
	}
	return out
}

// withFigures joins the tree's pictures to the page's pictures by the largest overlap of their boxes (a tree picture
// with no box takes the first page picture still unused), and then adds the page's own pictures the tree never named:
// a tagged page that leaves an image out of its tree still draws it (/pending 941 #11). Each is placed after as many of
// the blocks that are not pictures as the page drew before it.
func withFigures(blocks []docmodel.Block, ep pdfops.ExportPage, boxes map[*docmodel.Image][4]float64) []docmodel.Block {
	used := map[int]bool{}
	for i := range blocks {
		b := &blocks[i]
		if b.Kind != docmodel.Figure || b.Image == nil {
			continue
		}
		best, bi := 0.0, -1
		box := boxes[b.Image]
		for j, f := range ep.Figures {
			if used[j] {
				continue
			}
			if o := overlap(box, [4]float64(f.Rect)); o > best {
				best, bi = o, j
			}
		}
		if bi < 0 && (box[2] <= box[0] || box[3] <= box[1]) {
			for j := range ep.Figures {
				if !used[j] {
					bi = j
					break
				}
			}
		}
		alt := b.Image.Alt
		if bi >= 0 {
			used[bi] = true
			*b.Image = *imageOf(ep.Figures[bi])
		} else {
			b.Image.Placeholder = pdfops.PlaceholderNotRead
		}
		b.Image.Alt = alt
	}
	for j, f := range ep.Figures {
		if used[j] {
			continue
		}
		at, seen := len(blocks), 0
		for i, b := range blocks {
			if seen >= f.Before {
				at = i
				break
			}
			if b.Kind != docmodel.Figure {
				seen++
			}
		}
		blocks = append(blocks, docmodel.Block{})
		copy(blocks[at+1:], blocks[at:])
		blocks[at] = docmodel.Block{Kind: docmodel.Figure, Image: imageOf(f)}
	}
	return blocks
}

func overlap(a, b [4]float64) float64 {
	w := minf(a[2], b[2]) - maxf(a[0], b[0])
	h := minf(a[3], b[3]) - maxf(a[1], b[1])
	if w <= 0 || h <= 0 {
		return 0
	}
	return w * h
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// letterCounts is how many right-to-left letters and how many other letters the block's text and cells hold.
func letterCounts(b docmodel.Block) (rtl, ltr int) {
	count := func(s string) {
		for _, r := range s {
			switch {
			case unicode.In(r, unicode.Hebrew, unicode.Arabic, unicode.Syriac, unicode.Thaana, unicode.Nko):
				rtl++
			case unicode.IsLetter(r):
				ltr++
			}
		}
	}
	count(b.Text())
	for _, row := range b.Rows {
		for _, c := range row {
			for _, p := range c.Paragraphs {
				count(plain(p))
			}
		}
	}
	return rtl, ltr
}

// dirOf is "rtl" when right-to-left letters outnumber the rest in the block's text.
func dirOf(b docmodel.Block) string {
	if rtl, ltr := letterCounts(b); rtl > ltr {
		return "rtl"
	}
	return "ltr"
}

func hasLetters(b docmodel.Block) bool {
	rtl, ltr := letterCounts(b)
	return rtl+ltr > 0
}

func majorityDir(blocks []docmodel.Block) string {
	n := map[string]int{}
	for _, b := range blocks {
		if !hasLetters(b) {
			continue // a picture, or digits alone, has no direction to vote with (/pending 941 #13)
		}
		n[b.Dir]++
	}
	if n["rtl"] > n["ltr"] {
		return "rtl"
	}
	return "ltr"
}

// boundTables is the ONE place a table meets docmodel's bounds, for the tree's tables and the proposal's alike.
func boundTables(blocks []docmodel.Block, rep *docmodel.Report) []docmodel.Block {
	var out []docmodel.Block
	for _, b := range blocks {
		if b.Kind != docmodel.Table {
			out = append(out, b)
			continue
		}
		bs, asText := docmodel.BoundTable(b)
		if asText {
			rep.TablesAsText++
		}
		out = append(out, bs...)
	}
	return out
}
