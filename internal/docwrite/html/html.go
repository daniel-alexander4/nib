// Package html writes the document model as one self-contained HTML file (PLAN-import-export P02.S04, ADR-193): no
// script, no outside reference, a Content-Security-Policy that allows nothing but inline style and `data:` pictures, and
// every value through `html.EscapeString` — the one door for HTML syntax (Y9). It imports the model and no PDF reader.
package html

import (
	"encoding/base64"
	stdhtml "html"
	"net/url"
	"strconv"
	"strings"

	"nib/internal/docmodel"
)

const style = `body{font-family:system-ui,sans-serif;line-height:1.5;max-width:50rem;margin:2rem auto;padding:0 1rem}` +
	`table{border-collapse:collapse}td,th{border:1px solid #888;padding:.25rem .5rem;vertical-align:top}img{max-width:100%;height:auto}`

func esc(s string) string { return stdhtml.EscapeString(docmodel.Clean(s)) }

func safeLink(u string) bool {
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

// langTag is a catalog /Lang kept only when it is a plain BCP 47 shape; anything else is "und".
func langTag(l string) string {
	if l == "" || len(l) > 35 {
		return "und"
	}
	for _, c := range l {
		if !(c == '-' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return "und"
		}
	}
	return l
}

func dirAttr(d string) string {
	if d == "rtl" {
		return ` dir="rtl"`
	}
	return ""
}

// Write is the document as HTML5.
func Write(doc docmodel.Document) []byte {
	title := docmodel.OneLine(doc.Meta.Title)
	if title == "" {
		title = "Document"
	}
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"" + esc(langTag(doc.Meta.Lang)) + "\"" + dirAttr(doc.Meta.Dir) + ">\n<head>\n")
	b.WriteString("<meta charset=\"utf-8\">\n")
	b.WriteString("<meta http-equiv=\"Content-Security-Policy\" content=\"default-src 'none'; img-src data:; style-src 'unsafe-inline'\">\n")
	b.WriteString("<meta name=\"generator\" content=\"Nib\">\n")
	b.WriteString("<title>" + esc(title) + "</title>\n<style>" + style + "</style>\n</head>\n<body>\n")
	blocks(&b, doc.Blocks)
	b.WriteString("</body>\n</html>\n")
	return []byte(b.String())
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
		if s.Link != "" && safeLink(s.Link) {
			t = "<a href=\"" + esc(s.Link) + "\" rel=\"noopener noreferrer\">" + t + "</a>"
		}
		b.WriteString(t)
	}
	return b.String()
}

func blocks(b *strings.Builder, bs []docmodel.Block) {
	var open []string // closing tags of the lists open, innermost last
	closeTo := func(n int) {
		for len(open) > n {
			b.WriteString(open[len(open)-1])
			open = open[:len(open)-1]
		}
	}
	for _, blk := range bs {
		if blk.Kind != docmodel.ListItem {
			closeTo(0)
		}
		switch blk.Kind {
		case docmodel.Heading:
			l := strconv.Itoa(min(max(blk.Level, 1), 6))
			b.WriteString("<h" + l + dirAttr(blk.Dir) + ">" + spans(blk.Spans) + "</h" + l + ">\n")
		case docmodel.ListItem:
			depth := max(blk.Level, 1)
			closeTo(depth)
			for len(open) < depth {
				tag := "ul"
				if len(open) == depth-1 && numbered(blk.Marker) {
					tag = "ol"
				}
				b.WriteString("<" + tag + dirAttr(blk.Dir) + ">\n")
				open = append(open, "</"+tag+">\n")
			}
			b.WriteString("<li>" + spans(blk.Spans) + "</li>\n")
		case docmodel.Table:
			table(b, blk)
		case docmodel.Figure:
			figure(b, blk)
		default:
			if s := spans(blk.Spans); s != "" {
				b.WriteString("<p" + dirAttr(blk.Dir) + ">" + s + "</p>\n")
			}
		}
	}
	closeTo(0)
}

func numbered(m string) bool {
	m = strings.TrimSpace(m)
	return len(m) > 1 && (m[len(m)-1] == '.' || m[len(m)-1] == ')') && m[0] >= '0' && m[0] <= '9'
}

func figure(b *strings.Builder, blk docmodel.Block) {
	im := blk.Image
	if im == nil {
		return
	}
	alt := docmodel.OneLine(im.Alt)
	if alt == "" {
		alt = "picture on page " + strconv.Itoa(blk.Page)
	}
	mime := map[string]string{"jpg": "image/jpeg", "png": "image/png"}[im.Ext]
	if len(im.Data) == 0 || mime == "" {
		b.WriteString("<p><em>[Picture: " + esc(alt) + "]</em></p>\n")
		return
	}
	b.WriteString("<figure><img alt=\"" + esc(alt) + "\" src=\"data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(im.Data) + "\"></figure>\n")
}

func table(b *strings.Builder, blk docmodel.Block) {
	if len(blk.Rows) == 0 {
		return
	}
	b.WriteString("<table" + dirAttr(blk.Dir) + ">\n")
	for i, r := range blk.Rows {
		b.WriteString("<tr>")
		for _, c := range r {
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
			b.WriteString("<" + tag + attrs + ">" + strings.Join(ps, "<br>") + "</" + tag + ">")
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</table>\n")
}
