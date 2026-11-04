// Package docx writes the document model as a Word (.docx) package (PLAN-import-export P03.S01, ADR-194): flowing text
// in Word's own styles (Heading 1-6, List Paragraph, Table Grid), real numbering, real tables, hyperlinks and pictures.
// Every text and attribute value goes through `xmlw.Esc` — the one door for XML (Y9) — part and image names are fixed
// (`media/img-N.ext`) and never derived from the PDF, and the package carries no author, thumbnail, revision id or
// timestamp. It imports the model, the zip door and `xmlw`, and no PDF reader.
package docx

import (
	"context"
	"strconv"
	"strings"

	"nib/internal/docmodel"
	"nib/internal/docwrite/xmlw"
	"nib/internal/ziputil"
)

const (
	nsW   = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"`
	nsR   = `xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
	nsWP  = `xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"`
	nsA   = `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"`
	nsPic = `xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"`
	hdr   = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

	emuPerPx  = 9525
	maxWidth  = 6 * 914400 // six inches
	fallbackW = 3 * 914400
	fallbackH = 2 * 914400
)

// Parts is every part name Write can emit besides the pictures, which are `word/media/img-N.png|jpg`. A test holds the
// package to this list (Y9).
var Parts = []string{
	"[Content_Types].xml", "_rels/.rels", "docProps/core.xml", "docProps/app.xml",
	"word/document.xml", "word/styles.xml", "word/numbering.xml", "word/_rels/document.xml.rels",
}

type rel struct {
	id, typ, target string
	external        bool
}

type writer struct {
	body    strings.Builder
	rels    []rel
	media   []ziputil.Entry
	nums    map[int]int // model list id -> numId of its decimal list
	numIDs  []int       // decimal numIds in order, for numbering.xml
	picID   int
	hasJPG  bool
	hasPNG  bool
	linkIDs map[string]string
}

// Write is the document as a .docx package, or nil when the model holds nothing to write.
func Write(ctx context.Context, doc docmodel.Document) ([]byte, error) {
	if len(doc.Blocks) == 0 {
		return nil, nil
	}
	w := &writer{nums: map[int]int{}, linkIDs: map[string]string{}}
	if err := w.blocks(ctx, doc); err != nil {
		return nil, err
	}
	if w.body.Len() == 0 {
		return nil, nil
	}
	body := hdr + `<w:document ` + nsW + ` ` + nsR + ` ` + nsWP + ` ` + nsA + ` ` + nsPic + `><w:body>` + w.body.String() +
		`<w:sectPr><w:pgSz w:w="12240" w:h="15840"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440" w:header="720" w:footer="720" w:gutter="0"/></w:sectPr></w:body></w:document>`

	ct := hdr + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>`
	if w.hasPNG {
		ct += `<Default Extension="png" ContentType="image/png"/>` // ADR-194 writes
	}
	if w.hasJPG {
		ct += `<Default Extension="jpg" ContentType="image/jpeg"/>` // ADR-194 writes
	}
	ct += `<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` + // ADR-194 writes
		`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
		`<Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/>` +
		`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>` +
		`<Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/></Types>`

	rootRels := hdr + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>` +
		`<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/></Relationships>`

	docRels := hdr + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rIdStyles" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
		`<Relationship Id="rIdNumbering" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml"/>`
	for _, r := range w.rels {
		docRels += `<Relationship Id="` + r.id + `" Type="` + r.typ + `" Target="` + xmlw.Esc(r.target) + `"`
		if r.external {
			docRels += ` TargetMode="External"`
		}
		docRels += `/>`
	}
	docRels += `</Relationships>`

	core := hdr + `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" ` +
		`xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>` + xmlw.Esc(xmlw.Title(doc.Meta)) + `</dc:title><dc:language>` +
		xmlw.Esc(xmlw.LangTag(doc.Meta.Lang)) + `</dc:language></cp:coreProperties>`
	app := hdr + `<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"><Application>Nib</Application></Properties>`

	entries := []ziputil.Entry{
		{Name: "[Content_Types].xml", Data: []byte(ct)},
		{Name: "_rels/.rels", Data: []byte(rootRels)},
		{Name: "docProps/core.xml", Data: []byte(core)},
		{Name: "docProps/app.xml", Data: []byte(app)},
		{Name: "word/document.xml", Data: []byte(body)},
		{Name: "word/styles.xml", Data: []byte(styles)},
		{Name: "word/numbering.xml", Data: []byte(w.numbering())},
		{Name: "word/_rels/document.xml.rels", Data: []byte(docRels)},
	}
	entries = append(entries, w.media...)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ziputil.Write(entries)
}

func dirOf(d string) bool { return d == "rtl" }

// text is one run's text: tabs and line breaks become their elements, the rest is `w:t` through the XML door.
func text(t string, rtl bool) string {
	t = docmodel.Clean(t)
	var b strings.Builder
	flush := func(s string) {
		if s != "" {
			b.WriteString(`<w:t xml:space="preserve">` + xmlw.Esc(s) + `</w:t>`)
		}
	}
	start := 0
	for i, r := range t {
		if r == '\t' || r == '\n' {
			flush(t[start:i])
			if r == '\t' {
				b.WriteString(`<w:tab/>`)
			} else {
				b.WriteString(`<w:br/>`)
			}
			start = i + 1
		}
	}
	flush(t[start:])
	return b.String()
}

func (w *writer) link(u string) string {
	if id, ok := w.linkIDs[u]; ok {
		return id
	}
	id := "rIdL" + strconv.Itoa(len(w.linkIDs)+1)
	w.linkIDs[u] = id
	w.rels = append(w.rels, rel{id: id, typ: "http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink", target: u, external: true})
	return id
}

func (w *writer) spans(sp []docmodel.Span, rtl, bold bool) string {
	var b strings.Builder
	for _, s := range sp {
		t := text(s.Text, rtl)
		if t == "" {
			continue
		}
		rpr := ""
		if bold || s.Bold {
			rpr += `<w:b/>`
		}
		if s.Italic {
			rpr += `<w:i/>`
		}
		linked := s.Link != "" && xmlw.SafeLink(s.Link)
		if linked {
			rpr = `<w:rStyle w:val="Hyperlink"/>` + rpr
		}
		if rtl {
			rpr += `<w:rtl/>`
		}
		run := `<w:r>`
		if rpr != "" {
			run += `<w:rPr>` + rpr + `</w:rPr>`
		}
		run += t + `</w:r>`
		if linked {
			run = `<w:hyperlink r:id="` + w.link(s.Link) + `" w:history="1">` + run + `</w:hyperlink>`
		}
		b.WriteString(run)
	}
	return b.String()
}

func (w *writer) para(ppr, runs string) string {
	if ppr != "" {
		ppr = `<w:pPr>` + ppr + `</w:pPr>`
	}
	return `<w:p>` + ppr + runs + `</w:p>`
}

func (w *writer) blocks(ctx context.Context, doc docmodel.Document) error {
	for _, blk := range doc.Blocks {
		if err := ctx.Err(); err != nil {
			return err
		}
		rtl := dirOf(blk.Dir)
		bidi := ""
		if rtl {
			bidi = `<w:bidi/>`
		}
		switch blk.Kind {
		case docmodel.Heading:
			l := strconv.Itoa(min(max(blk.Level, 1), 6))
			w.body.WriteString(w.para(`<w:pStyle w:val="Heading`+l+`"/>`+bidi, w.spans(blk.Spans, rtl, false)))
		case docmodel.ListItem:
			lvl := min(max(blk.Level, 1), 9) - 1
			num := 1
			if xmlw.Numbered(blk.Marker) {
				num = w.decimal(blk.List)
			}
			w.body.WriteString(w.para(`<w:pStyle w:val="ListParagraph"/><w:numPr><w:ilvl w:val="`+strconv.Itoa(lvl)+`"/><w:numId w:val="`+strconv.Itoa(num)+`"/></w:numPr>`+bidi, w.spans(blk.Spans, rtl, false)))
		case docmodel.Table:
			w.table(blk)
		case docmodel.Figure:
			w.figure(blk)
		default:
			if s := w.spans(blk.Spans, rtl, false); s != "" {
				w.body.WriteString(w.para(bidi, s))
			}
		}
	}
	return nil
}

// decimal is the numId of the decimal list a model list is, drawn from 2 up so each list restarts at 1.
func (w *writer) decimal(list int) int {
	if n, ok := w.nums[list]; ok {
		return n
	}
	n := len(w.numIDs) + 2
	w.nums[list] = n
	w.numIDs = append(w.numIDs, n)
	return n
}

func (w *writer) table(blk docmodel.Block) {
	if len(blk.Rows) == 0 {
		return
	}
	rtl := dirOf(blk.Dir)
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
	b.WriteString(`<w:tbl><w:tblPr><w:tblStyle w:val="TableGrid"/>`)
	if rtl {
		b.WriteString(`<w:bidiVisual/>`)
	}
	b.WriteString(`<w:tblW w:w="0" w:type="auto"/>`)
	b.WriteString(`</w:tblPr><w:tblGrid>`)
	for i := 0; i < cols; i++ {
		b.WriteString(`<w:gridCol w:w="` + strconv.Itoa(9360/cols) + `"/>`)
	}
	b.WriteString(`</w:tblGrid>`)
	for i, r := range grid {
		b.WriteString(`<w:tr>`)
		if i < blk.HeaderRows {
			b.WriteString(`<w:trPr><w:tblHeader/></w:trPr>`)
		}
		used := 0
		for _, c := range r {
			used += c.ColSpan
			b.WriteString(`<w:tc><w:tcPr>`)
			if c.ColSpan > 1 {
				b.WriteString(`<w:gridSpan w:val="` + strconv.Itoa(c.ColSpan) + `"/>`)
			}
			switch {
			case c.Covered:
				b.WriteString(`<w:vMerge/>`)
			case c.Restart:
				b.WriteString(`<w:vMerge w:val="restart"/>`)
			}
			b.WriteString(`</w:tcPr>`)
			bold := c.Header || i < blk.HeaderRows
			wrote := false
			for _, p := range c.Paragraphs {
				if s := w.spans(p, rtl, bold); s != "" {
					bidi := ""
					if rtl {
						bidi = `<w:bidi/>`
					}
					b.WriteString(w.para(bidi, s))
					wrote = true
				}
			}
			if !wrote {
				b.WriteString(`<w:p/>`)
			}
			b.WriteString(`</w:tc>`)
		}
		for ; used < cols; used++ {
			b.WriteString(`<w:tc><w:p/></w:tc>`)
		}
		b.WriteString(`</w:tr>`)
	}
	b.WriteString(`</w:tbl><w:p/>`)
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
	if len(im.Data) == 0 || xmlw.MIME(im.Ext) == "" {
		w.body.WriteString(w.para("", `<w:r><w:rPr><w:i/></w:rPr><w:t xml:space="preserve">`+xmlw.Esc("[Picture: "+alt+"]")+`</w:t></w:r>`))
		return
	}
	w.picID++
	name := "img-" + strconv.Itoa(w.picID) + "." + im.Ext
	w.media = append(w.media, ziputil.Entry{Name: "word/media/" + name, Data: im.Data})
	if im.Ext == "png" {
		w.hasPNG = true
	} else {
		w.hasJPG = true
	}
	id := "rIdP" + strconv.Itoa(w.picID)
	w.rels = append(w.rels, rel{id: id, typ: "http://schemas.openxmlformats.org/officeDocument/2006/relationships/image", target: "media/" + name})
	cx, cy := fallbackW, fallbackH
	if im.Width > 0 && im.Height > 0 {
		iw, ih := min(im.Width, xmlw.MaxPx), min(im.Height, xmlw.MaxPx) // Word's own extent limit is 22 inches each way
		cx, cy = iw*emuPerPx, ih*emuPerPx
		if cx > maxWidth {
			cy = int(int64(cy) * maxWidth / int64(cx))
			cx = maxWidth
		}
		cy = max(cy, 1)
	}
	sx, sy := strconv.Itoa(cx), strconv.Itoa(cy)
	n := strconv.Itoa(w.picID)
	w.body.WriteString(w.para("", `<w:r><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0"><wp:extent cx="`+sx+`" cy="`+sy+`"/>`+
		`<wp:docPr id="`+n+`" name="Picture `+n+`" descr="`+xmlw.Esc(alt)+`"/><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">`+
		`<pic:pic><pic:nvPicPr><pic:cNvPr id="`+n+`" name="`+name+`"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="`+id+`"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>`+
		`<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="`+sx+`" cy="`+sy+`"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic>`+
		`</a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`))
}

func (w *writer) numbering() string {
	var b strings.Builder
	b.WriteString(hdr + `<w:numbering ` + nsW + `>`)
	level := func(i int, fmtName, txt string) string {
		return `<w:lvl w:ilvl="` + strconv.Itoa(i) + `"><w:start w:val="1"/><w:numFmt w:val="` + fmtName + `"/><w:lvlText w:val="` + txt + `"/><w:lvlJc w:val="left"/>` +
			`<w:pPr><w:ind w:left="` + strconv.Itoa(720*(i+1)) + `" w:hanging="360"/></w:pPr></w:lvl>`
	}
	b.WriteString(`<w:abstractNum w:abstractNumId="1"><w:multiLevelType w:val="hybridMultilevel"/>`)
	for i := 0; i < 9; i++ {
		b.WriteString(level(i, "bullet", "•"))
	}
	b.WriteString(`</w:abstractNum><w:abstractNum w:abstractNumId="2"><w:multiLevelType w:val="hybridMultilevel"/>`)
	for i := 0; i < 9; i++ {
		b.WriteString(level(i, "decimal", "%"+strconv.Itoa(i+1)+"."))
	}
	b.WriteString(`</w:abstractNum><w:num w:numId="1"><w:abstractNumId w:val="1"/></w:num>`)
	for _, n := range w.numIDs {
		b.WriteString(`<w:num w:numId="` + strconv.Itoa(n) + `"><w:abstractNumId w:val="2"/><w:lvlOverride w:ilvl="0"><w:startOverride w:val="1"/></w:lvlOverride></w:num>`)
	}
	b.WriteString(`</w:numbering>`)
	return b.String()
}

var styles = func() string {
	var b strings.Builder
	b.WriteString(hdr + `<w:styles ` + nsW + `><w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:cs="Calibri" w:eastAsia="Calibri"/><w:sz w:val="22"/><w:szCs w:val="22"/></w:rPr></w:rPrDefault>` +
		`<w:pPrDefault><w:pPr><w:spacing w:after="160" w:line="259" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults>` +
		`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:qFormat/></w:style>`)
	sizes := []int{40, 32, 28, 26, 24, 22}
	for i, sz := range sizes {
		l := strconv.Itoa(i + 1)
		b.WriteString(`<w:style w:type="paragraph" w:styleId="Heading` + l + `"><w:name w:val="heading ` + l + `"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/>` +
			`<w:pPr><w:keepNext/><w:spacing w:before="240" w:after="80"/><w:outlineLvl w:val="` + strconv.Itoa(i) + `"/></w:pPr><w:rPr><w:b/><w:sz w:val="` + strconv.Itoa(sz) + `"/></w:rPr></w:style>`)
	}
	b.WriteString(`<w:style w:type="paragraph" w:styleId="ListParagraph"><w:name w:val="List Paragraph"/><w:basedOn w:val="Normal"/><w:qFormat/><w:pPr><w:spacing w:after="60"/><w:ind w:left="720"/></w:pPr></w:style>` +
		`<w:style w:type="character" w:styleId="Hyperlink"><w:name w:val="Hyperlink"/><w:rPr><w:color w:val="0563C1"/><w:u w:val="single"/></w:rPr></w:style>` +
		`<w:style w:type="table" w:default="1" w:styleId="TableNormal"><w:name w:val="Normal Table"/><w:tblPr><w:tblCellMar><w:left w:w="108" w:type="dxa"/><w:right w:w="108" w:type="dxa"/></w:tblCellMar></w:tblPr></w:style>` +
		`<w:style w:type="table" w:styleId="TableGrid"><w:name w:val="Table Grid"/><w:basedOn w:val="TableNormal"/><w:tblPr><w:tblBorders>` +
		`<w:top w:val="single" w:sz="4" w:space="0" w:color="888888"/><w:left w:val="single" w:sz="4" w:space="0" w:color="888888"/><w:bottom w:val="single" w:sz="4" w:space="0" w:color="888888"/>` +
		`<w:right w:val="single" w:sz="4" w:space="0" w:color="888888"/><w:insideH w:val="single" w:sz="4" w:space="0" w:color="888888"/><w:insideV w:val="single" w:sz="4" w:space="0" w:color="888888"/></w:tblBorders></w:tblPr></w:style></w:styles>`)
	return b.String()
}()
