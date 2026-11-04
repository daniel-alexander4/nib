// Package epub writes the document model as an EPUB 3 package (PLAN-import-export P03.S03, ADR-195): the HTML writer's
// body as well-formed XHTML (`html.XHTML` — the same blocks, escapes and link allowlist as the web page, through the same
// code), one reflowable content document, a navigation document built from its headings, and the pictures as members.
// The `mimetype` entry is first and stored, so it sits at byte 38 (R8); every text and attribute value goes through
// `xmlw.Esc`; member names are fixed; and the package carries no author, source path, thumbnail, script, or timestamp
// finer than a constant. It imports the model, the zip door, `xmlw` and the HTML writer, and no PDF reader.
package epub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"strconv"
	"strings"

	"nib/internal/docmodel"
	"nib/internal/docwrite/html"
	"nib/internal/docwrite/xmlw"
	"nib/internal/ziputil"
)

const (
	mime = "application/epub+zip"
	hdr  = `<?xml version="1.0" encoding="UTF-8"?>` + "\n"
	// modified is dcterms:modified, which EPUB 3 requires. It is a constant on purpose: a package carries no date of
	// the machine it was made on, and the same model writes the same bytes.
	modified = "2000-01-01T00:00:00Z"
	xhtmlNS  = `xmlns="http://www.w3.org/1999/xhtml"`
	style    = `body{line-height:1.5}table{border-collapse:collapse}td,th{border:1px solid #888;padding:.25em .5em;vertical-align:top}img{max-width:100%;height:auto}`
)

// Parts is every part name Write can emit besides the pictures (`OEBPS/images/img-N.png|jpg`). A test holds the package to it.
var Parts = []string{"mimetype", "META-INF/container.xml", "OEBPS/content.opf", "OEBPS/nav.xhtml", "OEBPS/content.xhtml"}

// identifier is a urn:uuid made from the content, never from the clock or the machine: two exports of one document are
// one publication, and nothing about where it was made can be read from it.
func identifier(title, body string) string {
	h := sha256.Sum256([]byte(title + "\x00" + body))
	x := hex.EncodeToString(h[:16])
	return "urn:uuid:" + x[:8] + "-" + x[8:12] + "-5" + x[13:16] + "-a" + x[17:20] + "-" + x[20:32]
}

// Write is the document as an .epub package, or nil when the model holds nothing to write.
func Write(ctx context.Context, doc docmodel.Document) ([]byte, error) {
	body, pics, heads, err := html.XHTML(ctx, doc)
	if err != nil {
		return nil, err
	}
	if len(doc.Blocks) == 0 || !hasContent(body, len(pics)) {
		return nil, nil
	}
	title := xmlw.Title(doc.Meta)
	lang := xmlw.LangTag(doc.Meta.Lang)
	dir := ""
	if doc.Meta.Dir == "rtl" {
		dir = ` dir="rtl"`
	}

	content := hdr + `<html ` + xhtmlNS + ` lang="` + xmlw.Esc(lang) + `" xml:lang="` + xmlw.Esc(lang) + `"` + dir + `><head><meta charset="utf-8"/><title>` +
		xmlw.Esc(title) + `</title><style>` + style + `</style></head><body>` + "\n" + body + `</body></html>` + "\n"

	var nav strings.Builder
	nav.WriteString(hdr + `<html ` + xhtmlNS + ` xmlns:epub="http://www.idpf.org/2007/ops" lang="` + xmlw.Esc(lang) + `" xml:lang="` + xmlw.Esc(lang) + `"` + dir +
		`><head><meta charset="utf-8"/><title>` + xmlw.Esc(title) + `</title></head><body><nav epub:type="toc" id="toc"><h1>Contents</h1><ol>`)
	if len(heads) == 0 {
		// A navigation document needs one entry; a document with no heading gets its title.
		nav.WriteString(`<li><a href="content.xhtml">` + xmlw.Esc(title) + `</a></li>`)
	}
	for i, h := range heads {
		t := h.Text
		if t == "" {
			t = "Section " + strconv.Itoa(i+1)
		}
		nav.WriteString(`<li><a href="content.xhtml#` + xmlw.Esc(h.ID) + `">` + xmlw.Esc(t) + `</a></li>`)
	}
	nav.WriteString(`</ol></nav></body></html>` + "\n")

	var man strings.Builder
	man.WriteString(`<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/><item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>`)
	var entries []ziputil.Entry
	for i, p := range pics {
		id := "img-" + strconv.Itoa(i+1)
		man.WriteString(`<item id="` + id + `" href="` + xmlw.Esc(p.Name) + `" media-type="` + xmlw.Esc(p.MIME) + `"/>`)
		entries = append(entries, ziputil.Entry{Name: "OEBPS/" + p.Name, Data: p.Data})
	}
	progression := ""
	if doc.Meta.Dir == "rtl" {
		progression = ` page-progression-direction="rtl"`
	}
	opf := hdr + `<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid" xml:lang="` + xmlw.Esc(lang) + `"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/">` +
		`<dc:identifier id="uid">` + identifier(title, body) + `</dc:identifier><dc:title>` + xmlw.Esc(title) + `</dc:title><dc:language>` + xmlw.Esc(lang) + `</dc:language>` +
		`<meta property="dcterms:modified">` + modified + `</meta></metadata><manifest>` + man.String() + `</manifest><spine` + progression + `><itemref idref="content"/></spine></package>` + "\n"
	container := hdr + `<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles>` +
		`<rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>` + "\n"

	all := []ziputil.Entry{
		{Name: "mimetype", Data: []byte(mime), Store: true},
		{Name: "META-INF/container.xml", Data: []byte(container)},
		{Name: "OEBPS/content.opf", Data: []byte(opf)},
		{Name: "OEBPS/nav.xhtml", Data: []byte(nav.String())},
		{Name: "OEBPS/content.xhtml", Data: []byte(content)},
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ziputil.Write(append(all, entries...))
}

// hasContent says the body holds a visible character or a picture: a paragraph of one space is markup and nothing to read.
func hasContent(body string, pics int) bool {
	if pics > 0 {
		return true
	}
	d := xml.NewDecoder(strings.NewReader("<r>" + body + "</r>"))
	for {
		tok, err := d.Token()
		if err != nil {
			return false
		}
		if c, ok := tok.(xml.CharData); ok && strings.TrimSpace(string(c)) != "" {
			return true
		}
	}
}
