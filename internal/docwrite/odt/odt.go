// Package odt writes the document model as an OpenDocument Text (.odt) package (PLAN-import-export P03.S01, ADR-194):
// flowing text with real headings, lists, tables, links and pictures. The `mimetype` entry is first and stored, so it
// sits at byte 38 (R8); every text and attribute value goes through `xmlw.Esc`; picture names are fixed
// (`Pictures/img-N.ext`); and the package carries no creator, generator version, thumbnail or timestamp. It imports the
// model, the zip door and `xmlw`, and no PDF reader.
package odt

import (
	"strconv"
	"strings"

	"nib/internal/docmodel"
	"nib/internal/docwrite/xmlw"
	"nib/internal/ziputil"
)

const (
	mime    = "application/vnd.oasis.opendocument.text"
	hdr     = `<?xml version="1.0" encoding="UTF-8"?>` + "\n"
	pxPerIn = 96.0
	maxInch = 6.0
)

const nsAll = `xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" ` +
	`xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0" xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0" ` +
	`xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0" xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0" ` +
	`xmlns:xlink="http://www.w3.org/1999/xlink" xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0" ` +
	`xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:meta="urn:oasis:names:tc:opendocument:xmlns:meta:1.0" xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0"`

// Parts is every part name Write can emit besides the pictures (`Pictures/img-N.png|jpg`). A test holds the package to it.
var Parts = []string{"mimetype", "META-INF/manifest.xml", "content.xml", "styles.xml", "meta.xml"}

type openList struct {
	id       int
	itemOpen bool
}

type writer struct {
	body   strings.Builder
	media  []ziputil.Entry
	pic    int
	picMan []string
	stack  []openList
}

// Write is the document as an .odt package, or nil when the model holds nothing to write.
func Write(doc docmodel.Document) ([]byte, error) {
	if len(doc.Blocks) == 0 {
		return nil, nil
	}
	w := &writer{}
	w.blocks(doc)
	if w.body.Len() == 0 {
		return nil, nil
	}
	content := hdr + `<office:document-content ` + nsAll + ` office:version="1.3"><office:automatic-styles>` + autoStyles +
		`</office:automatic-styles><office:body><office:text>` + w.body.String() + `</office:text></office:body></office:document-content>`
	man := hdr + `<manifest:manifest ` + nsAll + ` manifest:version="1.3"><manifest:file-entry manifest:full-path="/" manifest:media-type="` + mime + `"/>` +
		`<manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/><manifest:file-entry manifest:full-path="styles.xml" manifest:media-type="text/xml"/>` +
		`<manifest:file-entry manifest:full-path="meta.xml" manifest:media-type="text/xml"/>`
	for _, p := range w.picMan {
		man += p
	}
	man += `</manifest:manifest>`
	meta := hdr + `<office:document-meta ` + nsAll + ` office:version="1.3"><office:meta><dc:title>` + xmlw.Esc(xmlw.Title(doc.Meta)) + `</dc:title><dc:language>` +
		xmlw.Esc(xmlw.LangTag(doc.Meta.Lang)) + `</dc:language><meta:generator>Nib</meta:generator></office:meta></office:document-meta>`
	styles := hdr + `<office:document-styles ` + nsAll + ` office:version="1.3"><office:styles>` + namedStyles + `</office:styles></office:document-styles>`
	entries := []ziputil.Entry{
		{Name: "mimetype", Data: []byte(mime), Store: true},
		{Name: "META-INF/manifest.xml", Data: []byte(man)},
		{Name: "content.xml", Data: []byte(content)},
		{Name: "styles.xml", Data: []byte(styles)},
		{Name: "meta.xml", Data: []byte(meta)},
	}
	return ziputil.Write(append(entries, w.media...))
}

// text is a run's text: a tab and a line break are elements, and a space that would collapse is `text:s`.
func text(t string) string {
	t = docmodel.Clean(t)
	var b strings.Builder
	i := 0
	rs := []rune(t)
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			b.WriteString(xmlw.Esc(cur.String()))
			cur.Reset()
		}
	}
	for i < len(rs) {
		r := rs[i]
		switch {
		case r == '\t':
			flush()
			b.WriteString(`<text:tab/>`)
		case r == '\n':
			flush()
			b.WriteString(`<text:line-break/>`)
		case r == ' ':
			j := i
			for j < len(rs) && rs[j] == ' ' {
				j++
			}
			n := j - i
			// the first space of a run inside text is literal; the rest, and a leading one, are counted
			if i == 0 || n > 1 {
				if i != 0 {
					cur.WriteRune(' ')
					n--
				}
				if n > 0 {
					flush()
					b.WriteString(`<text:s text:c="` + strconv.Itoa(n) + `"/>`)
				}
			} else {
				cur.WriteRune(' ')
			}
			i = j
			continue
		default:
			cur.WriteRune(r)
		}
		i++
	}
	flush()
	return b.String()
}

func spans(sp []docmodel.Span, bold bool) string {
	var b strings.Builder
	for _, s := range sp {
		t := text(s.Text)
		if t == "" {
			continue
		}
		style := ""
		switch {
		case (bold || s.Bold) && s.Italic:
			style = "TBI"
		case bold || s.Bold:
			style = "TB"
		case s.Italic:
			style = "TI"
		}
		if style != "" {
			t = `<text:span text:style-name="` + style + `">` + t + `</text:span>`
		}
		if s.Link != "" && xmlw.SafeLink(s.Link) {
			t = `<text:a xlink:type="simple" xlink:href="` + xmlw.Esc(s.Link) + `">` + t + `</text:a>`
		}
		b.WriteString(t)
	}
	return b.String()
}

func pstyle(dir, base string) string {
	if dir == "rtl" {
		return base + "R"
	}
	return base
}

// closeTo closes open lists until n remain; the item that holds a closed sub-list stays open.
func (w *writer) closeTo(n int) {
	for len(w.stack) > n {
		top := w.stack[len(w.stack)-1]
		if top.itemOpen {
			w.body.WriteString(`</text:list-item>`)
		}
		w.body.WriteString(`</text:list>`)
		w.stack = w.stack[:len(w.stack)-1]
	}
}

func (w *writer) listItem(blk docmodel.Block) {
	d := min(max(blk.Level, 1), 9)
	w.closeTo(d)
	if len(w.stack) == d {
		top := &w.stack[d-1]
		if top.id != blk.List { // a different list at this depth restarts numbering
			w.closeTo(d - 1)
		} else if top.itemOpen {
			w.body.WriteString(`</text:list-item>`)
			top.itemOpen = false
		}
	}
	for len(w.stack) < d {
		if n := len(w.stack); n > 0 && !w.stack[n-1].itemOpen {
			w.body.WriteString(`<text:list-item>`)
			w.stack[n-1].itemOpen = true
		}
		style := "L1"
		if xmlw.Numbered(blk.Marker) && len(w.stack) == d-1 {
			style = "L2"
		}
		w.body.WriteString(`<text:list text:style-name="` + style + `">`)
		w.stack = append(w.stack, openList{id: blk.List})
	}
	w.body.WriteString(`<text:list-item><text:p text:style-name="` + pstyle(blk.Dir, "LP") + `">` + spans(blk.Spans, false) + `</text:p>`)
	w.stack[len(w.stack)-1].itemOpen = true
}

func (w *writer) blocks(doc docmodel.Document) {
	for _, blk := range doc.Blocks {
		if blk.Kind != docmodel.ListItem {
			w.closeTo(0)
		}
		switch blk.Kind {
		case docmodel.Heading:
			l := strconv.Itoa(min(max(blk.Level, 1), 6))
			w.body.WriteString(`<text:h text:style-name="` + pstyle(blk.Dir, "H") + l + `" text:outline-level="` + l + `">` + spans(blk.Spans, false) + `</text:h>`)
		case docmodel.ListItem:
			w.listItem(blk)
		case docmodel.Table:
			w.table(blk)
		case docmodel.Figure:
			w.figure(blk)
		default:
			if s := spans(blk.Spans, false); s != "" {
				w.body.WriteString(`<text:p text:style-name="` + pstyle(blk.Dir, "P") + `">` + s + `</text:p>`)
			}
		}
	}
	w.closeTo(0)
}

func (w *writer) table(blk docmodel.Block) {
	if len(blk.Rows) == 0 {
		return
	}
	grid := xmlw.Grid(blk.Rows)
	cols := 1
	for _, r := range grid {
		n := 0
		for _, c := range r {
			n += c.ColSpan
		}
		cols = max(cols, n)
	}
	var b strings.Builder
	tbl := "Tbl"
	if blk.Dir == "rtl" {
		tbl = "TblR"
	}
	b.WriteString(`<table:table table:style-name="` + tbl + `"><table:table-column table:number-columns-repeated="` + strconv.Itoa(cols) + `"/>`)
	inHead := false
	for i, r := range grid {
		if i < blk.HeaderRows && !inHead {
			b.WriteString(`<table:table-header-rows>`)
			inHead = true
		}
		if i >= blk.HeaderRows && inHead {
			b.WriteString(`</table:table-header-rows>`)
			inHead = false
		}
		b.WriteString(`<table:table-row>`)
		used := 0
		for _, c := range r {
			used += c.ColSpan
			if c.Covered {
				for k := 0; k < c.ColSpan; k++ {
					b.WriteString(`<table:covered-table-cell/>`)
				}
				continue
			}
			attrs := ` table:style-name="Cell" office:value-type="string"`
			if c.ColSpan > 1 {
				attrs += ` table:number-columns-spanned="` + strconv.Itoa(c.ColSpan) + `"`
			}
			if c.RowSpan > 1 {
				attrs += ` table:number-rows-spanned="` + strconv.Itoa(c.RowSpan) + `"`
			}
			b.WriteString(`<table:table-cell` + attrs + `>`)
			bold := c.Header || i < blk.HeaderRows
			wrote := false
			for _, p := range c.Paragraphs {
				if s := spans(p, bold); s != "" {
					b.WriteString(`<text:p text:style-name="` + pstyle(blk.Dir, "P") + `">` + s + `</text:p>`)
					wrote = true
				}
			}
			if !wrote {
				b.WriteString(`<text:p/>`)
			}
			b.WriteString(`</table:table-cell>`)
			for k := 1; k < c.ColSpan; k++ {
				b.WriteString(`<table:covered-table-cell/>`)
			}
		}
		for ; used < cols; used++ {
			b.WriteString(`<table:table-cell office:value-type="string" table:style-name="Cell"><text:p/></table:table-cell>`)
		}
		b.WriteString(`</table:table-row>`)
	}
	if inHead {
		b.WriteString(`</table:table-header-rows>`)
	}
	b.WriteString(`</table:table>`)
	w.body.WriteString(b.String())
}

func (w *writer) figure(blk docmodel.Block) {
	im := blk.Image
	if im == nil {
		return
	}
	alt := docmodel.OneLine(im.Alt)
	if alt == "" {
		alt = "picture on page " + strconv.Itoa(blk.Page)
	}
	mt := xmlw.MIME(im.Ext)
	if len(im.Data) == 0 || mt == "" {
		w.body.WriteString(`<text:p text:style-name="P"><text:span text:style-name="TI">` + xmlw.Esc("[Picture: "+alt+"]") + `</text:span></text:p>`)
		return
	}
	w.pic++
	name := "Pictures/img-" + strconv.Itoa(w.pic) + "." + im.Ext
	w.media = append(w.media, ziputil.Entry{Name: name, Data: im.Data})
	w.picMan = append(w.picMan, `<manifest:file-entry manifest:full-path="`+name+`" manifest:media-type="`+mt+`"/>`)
	wd, ht := 3.0, 2.0
	if im.Width > 0 && im.Height > 0 {
		wd, ht = float64(min(im.Width, 20000))/pxPerIn, float64(min(im.Height, 20000))/pxPerIn
		if wd > maxInch {
			ht, wd = ht*maxInch/wd, maxInch
		}
		ht = max(ht, 0.01)
	}
	n := strconv.Itoa(w.pic)
	w.body.WriteString(`<text:p text:style-name="P"><draw:frame draw:name="Picture ` + n + `" text:anchor-type="as-char" svg:width="` +
		strconv.FormatFloat(wd, 'f', 3, 64) + `in" svg:height="` + strconv.FormatFloat(ht, 'f', 3, 64) + `in"><draw:image xlink:href="` + name +
		`" xlink:type="simple" xlink:show="embed" xlink:actuate="onLoad"/><svg:desc>` + xmlw.Esc(alt) + `</svg:desc></draw:frame></text:p>`)
}

var autoStyles = func() string {
	var b strings.Builder
	rl := `<style:paragraph-properties style:writing-mode="rl-tb" fo:text-align="start"/>`
	b.WriteString(`<style:style style:name="TB" style:family="text"><style:text-properties fo:font-weight="bold" style:font-weight-asian="bold" style:font-weight-complex="bold"/></style:style>` +
		`<style:style style:name="TI" style:family="text"><style:text-properties fo:font-style="italic" style:font-style-asian="italic" style:font-style-complex="italic"/></style:style>` +
		`<style:style style:name="TBI" style:family="text"><style:text-properties fo:font-weight="bold" fo:font-style="italic" style:font-weight-asian="bold" style:font-weight-complex="bold" style:font-style-asian="italic" style:font-style-complex="italic"/></style:style>` +
		`<style:style style:name="P" style:family="paragraph" style:parent-style-name="Standard"/>` +
		`<style:style style:name="PR" style:family="paragraph" style:parent-style-name="Standard">` + rl + `</style:style>` +
		`<style:style style:name="LP" style:family="paragraph" style:parent-style-name="Standard"/>` +
		`<style:style style:name="LPR" style:family="paragraph" style:parent-style-name="Standard">` + rl + `</style:style>`)
	for i := 1; i <= 6; i++ {
		n := strconv.Itoa(i)
		b.WriteString(`<style:style style:name="H` + n + `" style:family="paragraph" style:parent-style-name="Heading_20_` + n + `"/>` +
			`<style:style style:name="HR` + n + `" style:family="paragraph" style:parent-style-name="Heading_20_` + n + `">` + rl + `</style:style>`)
	}
	b.WriteString(`<style:style style:name="Tbl" style:family="table"><style:table-properties table:align="margins" style:width="6.5in"/></style:style>` +
		`<style:style style:name="TblR" style:family="table"><style:table-properties table:align="margins" style:width="6.5in" style:writing-mode="rl-tb"/></style:style>` +
		`<style:style style:name="Cell" style:family="table-cell"><style:table-cell-properties fo:padding="0.04in" fo:border="0.5pt solid #888888"/></style:style>` +
		`<text:list-style style:name="L1">`)
	for i := 1; i <= 9; i++ {
		n := strconv.Itoa(i)
		b.WriteString(`<text:list-level-style-bullet text:level="` + n + `" text:bullet-char="•"><style:list-level-properties text:list-level-position-and-space-mode="label-alignment"><style:list-level-label-alignment text:label-followed-by="listtab" text:list-tab-stop-position="` +
			strconv.FormatFloat(0.25*float64(i)+0.25, 'f', 2, 64) + `in" fo:text-indent="-0.25in" fo:margin-left="` + strconv.FormatFloat(0.25*float64(i)+0.25, 'f', 2, 64) + `in"/></style:list-level-properties></text:list-level-style-bullet>`)
	}
	b.WriteString(`</text:list-style><text:list-style style:name="L2">`)
	for i := 1; i <= 9; i++ {
		n := strconv.Itoa(i)
		b.WriteString(`<text:list-level-style-number text:level="` + n + `" style:num-suffix="." style:num-format="1"><style:list-level-properties text:list-level-position-and-space-mode="label-alignment"><style:list-level-label-alignment text:label-followed-by="listtab" text:list-tab-stop-position="` +
			strconv.FormatFloat(0.25*float64(i)+0.25, 'f', 2, 64) + `in" fo:text-indent="-0.25in" fo:margin-left="` + strconv.FormatFloat(0.25*float64(i)+0.25, 'f', 2, 64) + `in"/></style:list-level-properties></text:list-level-style-number>`)
	}
	b.WriteString(`</text:list-style>`)
	return b.String()
}()

var namedStyles = func() string {
	var b strings.Builder
	b.WriteString(`<style:default-style style:family="paragraph"><style:paragraph-properties fo:margin-bottom="0.1in"/><style:text-properties style:font-name="Liberation Sans" fo:font-size="11pt"/></style:default-style>` +
		`<style:style style:name="Standard" style:family="paragraph" style:class="text"/>`)
	sizes := []string{"20pt", "16pt", "14pt", "13pt", "12pt", "11pt"}
	for i, sz := range sizes {
		n := strconv.Itoa(i + 1)
		b.WriteString(`<style:style style:name="Heading_20_` + n + `" style:display-name="Heading ` + n + `" style:family="paragraph" style:parent-style-name="Standard" style:next-style-name="Standard" style:default-outline-level="` + n + `" style:class="text">` +
			`<style:paragraph-properties fo:margin-top="0.17in" fo:margin-bottom="0.08in" fo:keep-with-next="always"/><style:text-properties fo:font-size="` + sz + `" fo:font-weight="bold"/></style:style>`)
	}
	return b.String()
}()
