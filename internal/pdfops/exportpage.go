package pdfops

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
)

// The export model's two remaining inputs (PLAN-import-export D18, P02.S03b): a page's paragraphs as STYLED SPANS,
// and the pictures the page draws joined to their bytes. Both are reads; the document model (P02.S03c) composes them
// with the author's tree. `Paragraphs` (text only, for the reflow editor) is unchanged.
//
// **The text of a paragraph's spans is the paragraph's text.** The spans are cut from the line the grouping built,
// at the offsets `joinRuns` recorded as it joined each run (`textLine.starts`), so nothing here re-derives where a
// space went. A line whose cuts do not reproduce its text (a merge elsewhere changed it) is one plain span of the
// text and is counted in `UnstyledLines`: the words are never wrong, only the style is lost, and it says so.

// ExportSpan is a stretch of a paragraph in one style, and — when a carried link's box holds the centre of the run it
// came from — that link's URI.
type ExportSpan struct {
	Text   string
	Bold   bool
	Italic bool
	Link   string
}

// ExportParagraph is one paragraph in reading order.
type ExportParagraph struct {
	Spans []ExportSpan
	// Rect is the box its lines occupy, in the page map's fractions of the displayed page (ADR-088), and Size the
	// largest size of its lines in points: where a positioned writer (PPTX, ADR-206) sets it.
	Rect MapRect
	Size float64
	// Running says it is a running header or footer.
	Running bool
}

// Text is the paragraph's text.
func (p ExportParagraph) Text() string {
	var b strings.Builder
	for _, s := range p.Spans {
		b.WriteString(s.Text)
	}
	return b.String()
}

// ExportCell is one cell of a ruled table, its paragraphs top to bottom.
type ExportCell struct {
	Paragraphs []ExportParagraph
}

// ExportTable is a ruled grid read as a table (ADR-121).
type ExportTable struct {
	Rows [][]ExportCell
	// Before is how many of the page's paragraphs come before it.
	Before int
}

// Placeholder causes: why a picture the page draws is carried as a placeholder and not as bytes.
const (
	PlaceholderInline   = "inline-image"       // drawn inline (`BI … EI`): it has no object to extract
	PlaceholderTurned   = "turned-or-mirrored" // drawn under a matrix that turns or mirrors it
	PlaceholderStencil  = "stencil-mask"
	PlaceholderMasked   = "masked"
	PlaceholderSoftMask = "soft-masked"
	PlaceholderFilter   = "filter"       // JPX, CCITT, JBIG2: no writer carries them unchanged
	PlaceholderColour   = "colour-space" // CMYK and every space a writer cannot carry unchanged
	PlaceholderNotRead  = "not-extracted"
	PlaceholderTooBig   = "too-big"
	PlaceholderBudget   = "over-budget"
)

// Caps on the pictures an export carries (D18's "per-image and total byte caps are named constants").
const (
	MaxExportImageBytes = 32 << 20
	MaxExportImageTotal = 256 << 20
)

// The caps in force: variables so a test can lower them (the constants are the values).
var exportImageBytes, exportImageTotal, exportImagePixels = int64(MaxExportImageBytes), int64(MaxExportImageTotal), maxImagePixels

// ExportFigure is a picture a page draws, at its place.
type ExportFigure struct {
	userBox [4]float64
	// Rect is the picture as drawn, in the page map's fractions of the displayed page (ADR-088).
	Rect MapRect
	// Before is how many of the page's paragraphs come before it.
	Before int
	// ObjNr is the image object, or 0 for an inline image.
	ObjNr int
	// Ext, Width, Height and Data are the picture when it is carried ("jpg" or "png"); Data is nil for a placeholder.
	Ext           string
	Width, Height int
	Data          []byte
	// Placeholder is the cause Data is nil.
	Placeholder string
}

// ExportBlock is one thing on a page in reading order, as the proposal reads it (ADR-121, ADR-122): a heading, a
// paragraph, a list item, a table or a picture. Role is the proposal's: H1-H6, P, LI, Table or Figure.
type ExportBlock struct {
	Role string
	// Marker is a list item's label as drawn; List numbers the list it belongs to (consecutive items share one).
	Marker string
	List   int
	// Paragraph is the text of a heading, paragraph or list item; Table and Figure index into the page's Tables and
	// Figures for those roles, and are -1 otherwise.
	Paragraph ExportParagraph
	Table     int
	Figure    int
}

// ExportPage is one page read for the document model.
type ExportPage struct {
	Page       int
	Paragraphs []ExportParagraph
	Tables     []ExportTable
	Figures    []ExportFigure
	// Blocks are the page's paragraphs, tables and pictures in the proposal's order and with its roles.
	Blocks []ExportBlock
	// NoText says the page draws no glyph (the walk's fact, not "no line survived": an artifact-only page draws text, /pending 952).
	NoText bool
	// Width and Height are the displayed page in points (`displaySpace`): what a positioned writer scales Rect by.
	Width, Height float64
	// Unsupported is the layout limits the grouping reported (D19), never a refusal.
	Unsupported string
	// UnstyledLines counts lines that fell back to one plain span; DroppedLinks the links not carried, by cause.
	UnstyledLines int
	DroppedLinks  map[string]int
	// Visibility counts the text left out because a reader could not see it (exportvis.go).
	Visibility ExportVisibility
	// SkippedImages counts pictures drawn and not offered as figures: "artifact" (decoration the author marked) and
	// "small" (under figureMinSide).
	SkippedImages map[string]int
}

// ExportDoc is a document read for the model: its pages and what the catalog says of the whole.
type ExportDoc struct {
	// Lang is the catalog's `/Lang`, or "".
	Lang  string
	Pages []ExportPage
}

// ExportPages reads pages of pdf (nil or empty: every page) for the document model. Pictures are extracted in one pass
// over the pages that draw any, each object once however often it is drawn.
func ExportPages(pdf []byte, pages []int) ([]ExportPage, error) {
	d, err := ExportRead(context.Background(), pdf, pages)
	return d.Pages, err
}

// ExportRead is ExportPages with the catalog's language: the document is validated once. ctx is observed between pages and
// after each picture is decoded (/pending 941 #1), and its error is returned as it is.
func ExportRead(ctx context.Context, pdf []byte, pages []int) (ExportDoc, error) {
	if err := ctx.Err(); err != nil {
		return ExportDoc{}, err
	}
	pc, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		return ExportDoc{}, err
	}
	lang := ""
	if cat, cerr := pc.Catalog(); cerr == nil && cat != nil {
		lang = readLang(pc.XRefTable, cat["Lang"])
	}
	out, err := exportPagesIn(ctx, pc, pdf, pages)
	return ExportDoc{Lang: lang, Pages: out}, err
}

func exportPagesIn(cctx context.Context, ctx *model.Context, pdf []byte, pages []int) ([]ExportPage, error) {
	if len(pages) == 0 {
		for p := 1; p <= ctx.PageCount; p++ {
			pages = append(pages, p)
		}
	}
	budget := newFormWalkBudget(len(pages))
	var out []ExportPage
	var reads []pageRead
	var withPictures []int
	for _, p := range pages {
		if err := cctx.Err(); err != nil {
			return nil, err
		}
		if err := pageInDocument(p, ctx.PageCount); err != nil {
			return nil, err
		}
		pg := pageAt(ctx, nil, p)
		ep, pr, err := exportPageIn(ctx, pg, p, budget)
		if err != nil {
			return nil, err
		}
		reads = append(reads, pr)
		for _, f := range ep.Figures {
			if f.Placeholder == "" {
				withPictures = append(withPictures, p)
				break
			}
		}
		out = append(out, ep)
	}
	// The roles are the proposer's, over the document's pages together: a heading's level is set against the body size
	// of them all (ADR-122's rule, `proposeFromLayouts`), and the layouts are the visible ones.
	layouts := make([]pageLayout, len(reads))
	for i := range reads {
		layouts[i] = reads[i].l
	}
	prop := proposeFromLayouts(layouts)
	for _, el := range prop.elements {
		if el.page < 1 || el.page > len(reads) || el.parent != -1 {
			continue // a table's rows and cells are the Table's
		}
		reads[el.page-1].block(&out[el.page-1], el)
	}
	if len(withPictures) > 0 {
		if err := joinPictures(cctx, pdf, withPictures, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// pageRead is what a page's blocks are built from once the proposer has spoken.
type pageRead struct {
	l     pageLayout
	sp    displaySpace
	links PageLinkSet
}

// block adds the proposed element el to ep as an ExportBlock, joined to the layout by position and box.
func (pr pageRead) block(ep *ExportPage, el proposedElement) {
	b := ExportBlock{Role: el.role, Marker: el.marker, List: el.list, Table: -1, Figure: -1}
	switch {
	case el.role == "Table":
		if el.tableIdx < 1 || el.tableIdx > len(ep.Tables) {
			return
		}
		b.Table = el.tableIdx - 1
	case el.role == figureRole:
		for i, f := range ep.Figures {
			if f.userBox == el.box && f.Rect == pr.sp.rect(el.box) {
				b.Figure = i
				break
			}
		}
		if b.Figure < 0 {
			return
		}
	default:
		unstyled := 0
		b.Paragraph = exportParagraph(pr.sp, pr.links, el.lines, el.running, &unstyled)
		ep.UnstyledLines += unstyled
	}
	ep.Blocks = append(ep.Blocks, b)
}

func exportPageIn(ctx *model.Context, pg pdfread.Page, page int, budget *formWalkBudget) (ExportPage, pageRead, error) {
	ep := ExportPage{Page: page}
	l, images, vis, err := readPageExportLayout(ctx, pg, budget)
	if err != nil {
		return ep, pageRead{}, err
	}
	links := pageLinksIn(ctx, pg)
	ep.DroppedLinks = links.Dropped
	sp := newDisplaySpace(pg)
	ep.Width, ep.Height = sp.w, sp.h
	ep.NoText, ep.Unsupported, ep.Visibility = l.noText, l.unsupported, vis
	for _, p := range l.paragraphs {
		ep.Paragraphs = append(ep.Paragraphs, exportParagraph(sp, links, p.lines, p.running, &ep.UnstyledLines))
	}
	for _, t := range l.tables {
		et := ExportTable{Before: t.before, Rows: make([][]ExportCell, len(t.cells))}
		for r, row := range t.cells {
			et.Rows[r] = make([]ExportCell, len(row))
			for c, lines := range row {
				for _, p := range paragraphsOf(lines, 0) {
					et.Rows[r][c].Paragraphs = append(et.Rows[r][c].Paragraphs, exportParagraph(sp, links, p.lines, false, &ep.UnstyledLines))
				}
			}
		}
		ep.Tables = append(ep.Tables, et)
	}
	skip := func(why string) {
		if ep.SkippedImages == nil {
			ep.SkippedImages = map[string]int{}
		}
		ep.SkippedImages[why]++
	}
	for _, im := range images {
		switch {
		case im.artifact:
			skip("artifact")
			continue
		case im.box[2]-im.box[0] < figureMinSide || im.box[3]-im.box[1] < figureMinSide:
			skip("small")
			continue
		}
		f := ExportFigure{Rect: sp.rect(im.box), Before: tablePlace(l.paragraphs, im.box), ObjNr: im.objNr, userBox: im.box}
		l.figures = append(l.figures, pageFigure{box: im.box, span: im.span, before: f.Before})
		switch {
		case im.objNr == 0:
			f.Placeholder = PlaceholderInline
		case !im.plain:
			f.Placeholder = PlaceholderTurned
		default:
			f.Placeholder = im.cause
		}
		ep.Figures = append(ep.Figures, f)
	}
	return ep, pageRead{l: l, sp: sp, links: links}, nil
}

// exportParagraph cuts lines into styled spans, joined by one space as `textParagraph.text` joins them.
func exportParagraph(sp displaySpace, links PageLinkSet, lines []textLine, running bool, unstyled *int) ExportParagraph {
	var spans []ExportSpan
	add := func(s ExportSpan) {
		if s.Text == "" {
			return
		}
		if n := len(spans); n > 0 && spans[n-1].Bold == s.Bold && spans[n-1].Italic == s.Italic && spans[n-1].Link == s.Link {
			spans[n-1].Text += s.Text
			return
		}
		spans = append(spans, s)
	}
	for i, ln := range lines {
		if i > 0 {
			// The joining space belongs to the style of the text before it, so a bold title followed by a plain line
			// does not begin the line with a plain space.
			if n := len(spans); n > 0 {
				spans[n-1].Text += " "
			} else {
				add(ExportSpan{Text: " "})
			}
		}
		cut := lineSpans(sp, links, ln)
		if cut == nil {
			*unstyled++
			cut = []ExportSpan{{Text: ln.text}}
		}
		for _, s := range cut {
			add(s)
		}
	}
	p := ExportParagraph{Spans: spans, Running: running}
	var box [4]float64
	for _, ln := range lines {
		p.Size = math.Max(p.Size, ln.size)
		for _, r := range ln.runs {
			b := runBox(r)
			if box == ([4]float64{}) {
				box = b
				continue
			}
			box = [4]float64{math.Min(box[0], b[0]), math.Min(box[1], b[1]), math.Max(box[2], b[2]), math.Max(box[3], b[3])}
		}
	}
	if box != ([4]float64{}) {
		p.Rect = sp.rect(box)
	}
	return p
}

// lineSpans cuts ln at its runs' recorded starts, or nil when they do not reproduce its text.
func lineSpans(sp displaySpace, links PageLinkSet, ln textLine) []ExportSpan {
	if len(ln.starts) != len(ln.runs) || len(ln.runs) == 0 {
		return nil
	}
	var out []ExportSpan
	for i, r := range ln.runs {
		from, to := ln.starts[i], len(ln.text)
		if i+1 < len(ln.starts) {
			to = ln.starts[i+1]
		}
		if from > to || to > len(ln.text) || !validCut(ln.text, from, to) {
			return nil
		}
		s := ExportSpan{Text: ln.text[from:to], Bold: r.drawsBold(), Italic: r.italic}
		if len(links.Links) > 0 {
			if pieces := linkPieces(sp, links, r, s); pieces != nil {
				out = append(out, pieces...)
				continue
			}
			b := sp.rect(runBox(r))
			if l, ok := links.At((b[0]+b[2])/2, (b[1]+b[3])/2); ok {
				s.Link = l.URI
			}
		}
		out = append(out, s)
	}
	var joined strings.Builder
	for _, s := range out {
		joined.WriteString(s.Text)
	}
	if joined.String() != ln.text {
		return nil
	}
	return out
}

func validCut(s string, from, to int) bool {
	return (from == 0 || from == len(s) || s[from]&0xC0 != 0x80) && (to == len(s) || s[to]&0xC0 != 0x80)
}

// joinPictures extracts the pictures of pages once and gives each figure its bytes, or the cause it has none.
func joinPictures(ctx context.Context, pdf []byte, pages []int, out []ExportPage) error {
	type carried struct {
		img  model.Image
		data []byte
		why  string
	}
	byObj := map[int]*carried{}
	total := int64(0)
	sel := make([]string, len(pages))
	for i, p := range pages {
		sel[i] = fmt.Sprint(p)
	}
	digest := func(img model.Image) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if byObj[img.ObjNr] != nil {
			return nil
		}
		c := &carried{img: img}
		byObj[img.ObjNr] = c
		if c.why = pictureCause(img); c.why != "" {
			return nil
		}
		if img.Reader == nil {
			c.why = PlaceholderNotRead
			return nil
		}
		data, err := io.ReadAll(io.LimitReader(img, exportImageBytes+1))
		switch {
		case err != nil || len(data) == 0:
			c.why = PlaceholderNotRead
		case int64(len(data)) > exportImageBytes:
			c.why = PlaceholderTooBig
		case total+int64(len(data)) > exportImageTotal:
			c.why = PlaceholderBudget
		default:
			// pdfcpu's renderer gives the encoded picture and no size (`collectImages` reads it the same way): a
			// picture whose header does not read is not one a writer can place.
			cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
			switch {
			case err != nil:
				c.why = PlaceholderNotRead
			case cfg.Width*cfg.Height > exportImagePixels:
				c.why = PlaceholderTooBig
			default:
				total += int64(len(data))
				c.data, c.img.Width, c.img.Height = data, cfg.Width, cfg.Height
			}
		}
		return nil
	}
	// One pass over the pages, one image at a time. pdfcpu refuses a page over one picture it cannot render and a generic
	// fault panics; `ExtractImagesEach` skips that picture, so the others keep theirs and the skipped one stays
	// `not-extracted` (the export never fails for a picture). It used to redo a failed pass a page at a time, reading and
	// optimizing the whole file again for each (/pending 941 #3). A pass that cannot start (a budget refusal) fails every
	// figure the same way.
	_, _ = pdfread.ExtractImagesEach(ctx, pdf, sel, int64(exportImagePixels), digest)
	if err := ctx.Err(); err != nil {
		return err // a cancelled pass is not a picture that could not be read
	}
	for pi := range out {
		for fi := range out[pi].Figures {
			f := &out[pi].Figures[fi]
			if f.Placeholder != "" {
				continue
			}
			c := byObj[f.ObjNr]
			switch {
			case c == nil:
				f.Placeholder = PlaceholderNotRead
			case c.why != "":
				f.Placeholder = c.why
			default:
				f.Ext, f.Width, f.Height, f.Data = c.img.FileType, c.img.Width, c.img.Height, c.data
			}
		}
	}
	return nil
}

// pictureCause is why img is not carried, or "" — the allowlist's second half (the first, from the dictionary, is `imageCause`). A picture is carried as stored (JPEG) or as the PNG
// pdfcpu decodes it to, and only when nothing about it changes how the page shows it: a mask, a soft mask, a stencil,
// a filter or colour space no writer carries unchanged is a placeholder with its cause (D18).
func pictureCause(img model.Image) string {
	switch {
	case img.IsImgMask:
		return PlaceholderStencil
	case img.HasImgMask:
		return PlaceholderMasked
	case img.HasSMask:
		return PlaceholderSoftMask
	case img.FileType != "jpg" && img.FileType != "png":
		return PlaceholderColour
	}
	return ""
}

// PlaceholderCounts is how many figures of pages are placeholders, by cause — what an export says it could not carry.
func PlaceholderCounts(pages []ExportPage) map[string]int {
	m := map[string]int{}
	for _, p := range pages {
		for _, f := range p.Figures {
			if f.Placeholder != "" {
				m[f.Placeholder]++
			}
		}
	}
	return m
}
