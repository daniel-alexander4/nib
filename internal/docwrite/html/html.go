// Package html writes the document model as one self-contained HTML file (PLAN-import-export P02.S04, ADR-193): no
// script, no outside reference, a Content-Security-Policy that allows nothing but inline style and `data:` pictures, and
// every value through `html.EscapeString` — the one door for HTML syntax (Y9). It imports the model and no PDF reader.
package html

import (
	"context"
	"encoding/base64"
	stdhtml "html"
	"strconv"
	"strings"

	"nib/internal/docmodel"
	"nib/internal/docwrite/xmlw"
)

const style = `body{font-family:system-ui,sans-serif;line-height:1.5;max-width:50rem;margin:2rem auto;padding:0 1rem}` +
	`table{border-collapse:collapse}td,th{border:1px solid #888;padding:.25rem .5rem;vertical-align:top}img{max-width:100%;height:auto}`

func esc(s string) string { return stdhtml.EscapeString(docmodel.Clean(s)) }

func dirAttr(d string) string {
	if d == "rtl" {
		return ` dir="rtl"`
	}
	return ""
}

// Picture is a picture the XHTML body names by file, so a package can carry it as a member instead of a `data:` URL.
type Picture struct {
	Name string // "images/img-N.ext", fixed and never derived from the PDF (Y9)
	MIME string
	Data []byte
}

// Heading is one heading of the XHTML body: its id in the body, its level and its text as one line.
type Heading struct {
	ID    string
	Level int
	Text  string
}

// XHTML is the model's body as well-formed XML — the same blocks, escapes and link allowlist as Write, through the same
// code — for a package that wraps it (EPUB, P03.S03): void elements self-close, each heading carries an id (`h-N`) for a
// table of contents, and a picture is a separate member (`images/img-N.ext`) the body refers to.
func XHTML(ctx context.Context, doc docmodel.Document) (body string, pics []Picture, heads []Heading, err error) {
	r := &render{xhtml: true}
	if err := r.blocks(ctx, doc.Blocks); err != nil {
		return "", nil, nil, err
	}
	return r.b.String(), r.pics, r.heads, nil
}

// render holds one pass: the output, and in XHTML mode the pictures and headings it met.
type render struct {
	b     strings.Builder
	xhtml bool
	pics  []Picture
	heads []Heading
}

func (r *render) void(tag string) string {
	if r.xhtml {
		return "<" + tag + "/>"
	}
	return "<" + tag + ">"
}

// Write is the document as HTML5.
func Write(ctx context.Context, doc docmodel.Document) ([]byte, error) {
	title := xmlw.Title(doc.Meta)
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"" + esc(xmlw.LangTag(doc.Meta.Lang)) + "\"" + dirAttr(doc.Meta.Dir) + ">\n<head>\n")
	b.WriteString("<meta charset=\"utf-8\">\n")
	b.WriteString("<meta http-equiv=\"Content-Security-Policy\" content=\"default-src 'none'; img-src data:; style-src 'unsafe-inline'\">\n")
	b.WriteString("<meta name=\"generator\" content=\"Nib\">\n")
	b.WriteString("<title>" + esc(title) + "</title>\n<style>" + style + "</style>\n</head>\n<body>\n")
	r := &render{}
	if err := r.blocks(ctx, doc.Blocks); err != nil {
		return nil, err
	}
	if r.b.Len() == 0 {
		return nil, nil // nothing to write: the route answers "empty" for every format alike
	}
	b.WriteString(r.b.String())
	b.WriteString("</body>\n</html>\n")
	return []byte(b.String()), nil
}

func spans(sp []docmodel.Span) string {
	var b strings.Builder
	for _, s := range sp {
		t := esc(s.Text)
		if t == "" {
			continue
		}
		if s.Bold {
			t = "<strong>" + t + "</strong>"
		}
		if s.Italic {
			t = "<em>" + t + "</em>"
		}
		if s.Link != "" && xmlw.SafeLink(s.Link) {
			t = "<a href=\"" + esc(s.Link) + "\" rel=\"noopener noreferrer\">" + t + "</a>"
		}
		b.WriteString(t)
	}
	return b.String()
}

func (r *render) blocks(ctx context.Context, bs []docmodel.Block) error {
	b := &r.b
	// A list nested in a list sits INSIDE the parent's open <li> (a <ul> holds only <li>), so an item's <li> is left open
	// until the next item at its depth or a shallower one closes it.
	type level struct {
		tag    string
		liOpen bool
	}
	var open []level
	closeTo := func(n int) {
		for len(open) > n {
			l := open[len(open)-1]
			if l.liOpen {
				b.WriteString("</li>\n")
			}
			b.WriteString("</" + l.tag + ">\n")
			open = open[:len(open)-1]
		}
	}
	for _, blk := range bs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if blk.Kind != docmodel.ListItem {
			closeTo(0)
		}
		switch blk.Kind {
		case docmodel.Heading:
			l := strconv.Itoa(min(max(blk.Level, 1), 6))
			id := ""
			if r.xhtml {
				id = " id=\"h-" + strconv.Itoa(len(r.heads)+1) + "\""
				r.heads = append(r.heads, Heading{ID: "h-" + strconv.Itoa(len(r.heads)+1), Level: min(max(blk.Level, 1), 6), Text: plainLine(blk.Spans)})
			}
			b.WriteString("<h" + l + id + dirAttr(blk.Dir) + ">" + spans(blk.Spans) + "</h" + l + ">\n")
		case docmodel.ListItem:
			depth := max(blk.Level, 1)
			closeTo(depth)
			if depth == len(open) && open[depth-1].liOpen {
				b.WriteString("</li>\n")
				open[depth-1].liOpen = false
			}
			for len(open) < depth {
				if len(open) > 0 && !open[len(open)-1].liOpen {
					b.WriteString("<li>") // a level was skipped: the parent item is empty
					open[len(open)-1].liOpen = true
				}
				tag := "ul"
				if len(open) == depth-1 && xmlw.Numbered(blk.Marker) {
					tag = "ol"
				}
				b.WriteString("<" + tag + dirAttr(blk.Dir) + ">\n")
				open = append(open, level{tag: tag})
			}
			b.WriteString("<li>" + spans(blk.Spans))
			open[depth-1].liOpen = true
		case docmodel.Table:
			r.table(blk)
		case docmodel.Figure:
			r.figure(blk)
		default:
			if s := spans(blk.Spans); s != "" {
				b.WriteString("<p" + dirAttr(blk.Dir) + ">" + s + "</p>\n")
			}
		}
	}
	closeTo(0)
	return nil
}

// plainLine is a heading's spans as one line of text.
func plainLine(sp []docmodel.Span) string {
	var t strings.Builder
	for _, s := range sp {
		t.WriteString(s.Text)
	}
	return docmodel.OneLine(t.String())
}

func (r *render) figure(blk docmodel.Block) {
	b := &r.b
	im := blk.Image
	if im == nil {
		return
	}
	alt := docmodel.OneLine(im.Alt)
	if alt == "" {
		alt = "picture on page " + strconv.Itoa(blk.Page)
	}
	mime := xmlw.MIME(im.Ext)
	if len(im.Data) == 0 || mime == "" {
		b.WriteString("<p><em>[Picture: " + esc(alt) + "]</em></p>\n")
		return
	}
	if r.xhtml {
		name := "images/img-" + strconv.Itoa(len(r.pics)+1) + "." + im.Ext
		r.pics = append(r.pics, Picture{Name: name, MIME: mime, Data: im.Data})
		b.WriteString("<figure><img alt=\"" + esc(alt) + "\" src=\"" + name + "\"/></figure>\n")
		return
	}
	b.WriteString("<figure><img alt=\"" + esc(alt) + "\" src=\"data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(im.Data) + "\"></figure>\n")
}

func (r *render) table(blk docmodel.Block) {
	b := &r.b
	if len(blk.Rows) == 0 {
		return
	}
	b.WriteString("<table" + dirAttr(blk.Dir) + ">\n")
	for i, row := range blk.Rows {
		b.WriteString("<tr>")
		for _, c := range row {
			tag := "td"
			if c.Header || i < blk.HeaderRows {
				tag = "th"
			}
			attrs := ""
			if c.ColSpan > 1 {
				attrs += " colspan=\"" + strconv.Itoa(c.ColSpan) + "\""
			}
			if c.RowSpan > 1 {
				attrs += " rowspan=\"" + strconv.Itoa(c.RowSpan) + "\""
			}
			var ps []string
			for _, p := range c.Paragraphs {
				if s := spans(p); s != "" {
					ps = append(ps, s)
				}
			}
			b.WriteString("<" + tag + attrs + ">" + strings.Join(ps, r.void("br")) + "</" + tag + ">")
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</table>\n")
}
