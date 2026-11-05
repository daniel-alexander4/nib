// Package pptxexport writes a PowerPoint file from the page reader: one slide per page, a text box where the page
// sets each paragraph and each table cell's paragraph, and a picture where the page draws one (PLAN-import-export
// P05.S03, ADR-206).
//
// It is NOT a docwrite package. The docwrite writers take the flowing model and import no PDF reader (R8); a
// positioned slide needs the page's boxes, which only the reader has, so this is the census's one named exemption
// (Y3). The visibility rule is the reader's own: `pdfops.ExportRead` applies `visibleRuns` before it groups, so a
// covered, hidden or off-page run reaches no slide, exactly as it reaches no flowing export.
package pptxexport

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"nib/internal/docmodel"
	"nib/internal/docwrite/xmlw"
	"nib/internal/pdfops"
	"nib/internal/ziputil"
)

const (
	emuPerPt = 12700
	// maxSlideEMU is PowerPoint's own ceiling on a slide side (56 in); a larger page is scaled down to fit.
	maxSlideEMU = 51206400
	minSlideEMU = 914400 // 1 in
	// MaxSlides bounds the package: a slide is a part, and an office reader holds them all.
	MaxSlides = 2000
	// MaxMediaBytes bounds the pictures the package holds, each distinct picture once.
	MaxMediaBytes = 256 << 20
)

const (
	nsA = `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"`
	nsR = `xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
	nsP = `xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"`
	hdr = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"
	rel = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
)

// Counts is what the file carries, for the caller's header: nothing here is text of the document.
type Counts struct {
	Slides, TextBoxes, Pictures, Placeholders, DroppedLinks int
	// BidiControls counts the text-direction controls Clean removed from the text boxes (/pending 947).
	BidiControls int
}

// ErrTooLarge is the pictures, each distinct one counted once, past MaxMediaBytes.
var ErrTooLarge = errors.New("pptxexport: the pictures are more than a presentation Nib writes")

// ErrTooManySlides names the refusal the writer makes of a document with more than MaxSlides pages.
var ErrTooManySlides = fmt.Errorf("pptxexport: more than %d pages", MaxSlides)

type slide struct {
	xml  strings.Builder
	rels []string
	id   int
}

func ff(v float64) string { return strconv.FormatInt(int64(v+0.5), 10) }

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Write builds the presentation. title is the name the caller confirmed.
func Write(ctx context.Context, doc pdfops.ExportDoc, title string) ([]byte, Counts, error) {
	var c Counts
	if len(doc.Pages) > MaxSlides {
		return nil, c, ErrTooManySlides
	}
	lang := xmlw.LangTag(doc.Lang) // the catalog's /Lang, else "und" — never a language nobody said (/pending 948)
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	// One slide size for the deck: the first page's, because a presentation has one. A later page of another size is
	// scaled to fit it, never cropped.
	w0, h0 := 612.0, 792.0
	if len(doc.Pages) > 0 && doc.Pages[0].Width > 0 && doc.Pages[0].Height > 0 && finite(doc.Pages[0].Width) && finite(doc.Pages[0].Height) {
		w0, h0 = doc.Pages[0].Width, doc.Pages[0].Height
	}
	k := 1.0
	if m := max(w0, h0) * emuPerPt; m > maxSlideEMU {
		k = maxSlideEMU / m
	}
	sw := clamp(w0*emuPerPt*k, minSlideEMU, maxSlideEMU)
	sh := clamp(h0*emuPerPt*k, minSlideEMU, maxSlideEMU)

	var entries []ziputil.Entry
	var slides []*slide
	media := 0
	mediaBytes := 0
	byContent := map[[32]byte]string{} // a picture drawn on many pages is one part with many relationships
	exts := map[string]bool{}
	for _, pg := range doc.Pages {
		if err := ctx.Err(); err != nil {
			return nil, c, err
		}
		s := &slide{id: 2}
		s.xml.WriteString(hdr + `<p:sld ` + nsA + ` ` + nsR + ` ` + nsP + `><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
			`<p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>`)
		s.rels = append(s.rels, `<Relationship Id="rId1" Type="`+rel+`/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>`)
		// A page of another size than the deck's is fitted inside it.
		pw, ph := pg.Width, pg.Height
		if pw <= 0 || ph <= 0 || !finite(pw) || !finite(ph) {
			pw, ph = w0, h0
		}
		fit := min(sw/(pw*emuPerPt), sh/(ph*emuPerPt))
		fw, fh := pw*emuPerPt*fit, ph*emuPerPt*fit
		ox, oy := (sw-fw)/2, (sh-fh)/2
		place := func(r pdfops.MapRect) (x, y, cx, cy float64) {
			return ox + clamp(r[0], 0, 1)*fw, oy + clamp(r[1], 0, 1)*fh,
				(clamp(r[2], 0, 1) - clamp(r[0], 0, 1)) * fw, (clamp(r[3], 0, 1) - clamp(r[1], 0, 1)) * fh
		}
		box := func(p pdfops.ExportParagraph) {
			if strings.TrimSpace(p.Text()) == "" {
				return
			}
			x, y, cx, cy := place(p.Rect)
			if cx <= 0 || cy <= 0 {
				return
			}
			cx = min(cx*1.06+emuPerPt, sw-x)              // a substitute face is a little wider; the box is not the page
			sz := int(clamp(p.Size*fit*100, 100, 400000)) // fit already carries the deck's shrink (k)
			if p.Size <= 0 {
				sz = 1200
			}
			s.id++
			c.TextBoxes++
			// A right-to-left paragraph is set right to left and against the box's right edge: without `rtl` its words
			// are laid out left to right in whatever face the program substitutes (/pending 948). The direction is
			// docmodel's one count of letters, the reader's rule for a block.
			pPr := ""
			if docmodel.DirOf(p.Text()) == "rtl" {
				pPr = `<a:pPr algn="r" rtl="1"/>`
			}
			fmt.Fprintf(&s.xml, `<p:sp><p:nvSpPr><p:cNvPr id="%d" name="Text %d"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="%s" y="%s"/><a:ext cx="%s" cy="%s"/></a:xfrm>`+
				`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/></p:spPr><p:txBody><a:bodyPr wrap="square" lIns="0" tIns="0" rIns="0" bIns="0" rtlCol="0"><a:noAutofit/></a:bodyPr><a:lstStyle/><a:p>%s`,
				s.id, s.id, ff(x), ff(y), ff(cx), ff(cy), pPr)
			for _, sp := range p.Spans {
				if sp.Link != "" {
					c.DroppedLinks++
				}
				c.BidiControls += docmodel.BidiControls(sp.Text)
				t := docmodel.Clean(sp.Text)
				if t == "" {
					continue
				}
				attr := ` lang="` + lang + `" sz="` + strconv.Itoa(sz) + `"`
				if sp.Bold {
					attr += ` b="1"`
				}
				if sp.Italic {
					attr += ` i="1"`
				}
				// A line break in the text (U+2028 becomes one in Clean) is a break in the paragraph; a newline in <a:t> is a space.
				for i, part := range strings.Split(t, "\n") {
					if i > 0 {
						s.xml.WriteString(`<a:br><a:rPr` + attr + ` dirty="0"/></a:br>`)
					}
					if part != "" {
						s.xml.WriteString(`<a:r><a:rPr` + attr + ` dirty="0"/><a:t>` + xmlw.Esc(part) + `</a:t></a:r>`)
					}
				}
			}
			s.xml.WriteString(`</a:p></p:txBody></p:sp>`)
		}
		// Text is written after the pictures: the order in the tree is the stacking order, and a picture drawn over
		// text would hide it.
		texts := func() {
			for _, p := range pg.Paragraphs {
				box(p)
			}
			for _, t := range pg.Tables {
				for _, row := range t.Rows {
					for _, cell := range row {
						for _, p := range cell.Paragraphs {
							box(p)
						}
					}
				}
			}
		}
		for fi, f := range pg.Figures {
			x, y, cx, cy := place(f.Rect)
			if cx <= 0 || cy <= 0 {
				continue
			}
			alt := "Picture on page " + strconv.Itoa(pg.Page)
			mt := xmlw.MIME(f.Ext)
			s.id++
			if len(f.Data) == 0 || mt == "" {
				c.Placeholders++
				fmt.Fprintf(&s.xml, `<p:sp><p:nvSpPr><p:cNvPr id="%d" name="Picture %d" descr="%s"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="%s" y="%s"/><a:ext cx="%s" cy="%s"/></a:xfrm>`+
					`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/><a:ln w="9525"><a:solidFill><a:srgbClr val="808080"/></a:solidFill></a:ln></p:spPr><p:txBody><a:bodyPr anchor="ctr"/><a:lstStyle/><a:p><a:pPr algn="ctr"/><a:r><a:rPr lang="%s" sz="1200" dirty="0"/><a:t>%s</a:t></a:r></a:p></p:txBody></p:sp>`,
					s.id, fi+1, xmlw.Esc(alt), ff(x), ff(y), ff(cx), ff(cy), lang, xmlw.Esc("[Picture: not carried]"))
				continue
			}
			c.Pictures++
			exts[f.Ext] = true
			key := sha256.Sum256(append([]byte(f.Ext+"\x00"), f.Data...))
			target, seen := byContent[key]
			if !seen {
				mediaBytes += len(f.Data)
				if mediaBytes > maxMediaBytes {
					return nil, c, ErrTooLarge
				}
				media++
				target = "image" + strconv.Itoa(media) + "." + f.Ext
				byContent[key] = target
				entries = append(entries, ziputil.Entry{Name: "ppt/media/" + target, Data: f.Data})
			}
			rid := "rId" + strconv.Itoa(len(s.rels)+1)
			s.rels = append(s.rels, `<Relationship Id="`+rid+`" Type="`+rel+`/image" Target="../media/`+target+`"/>`)
			fmt.Fprintf(&s.xml, `<p:pic><p:nvPicPr><p:cNvPr id="%d" name="Picture %d" descr="%s"/><p:cNvPicPr><a:picLocks noChangeAspect="1"/></p:cNvPicPr><p:nvPr/></p:nvPicPr>`+
				`<p:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></p:blipFill><p:spPr><a:xfrm><a:off x="%s" y="%s"/><a:ext cx="%s" cy="%s"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr></p:pic>`,
				s.id, fi+1, xmlw.Esc(alt), rid, ff(x), ff(y), ff(cx), ff(cy))
		}
		texts()
		s.xml.WriteString(`</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`)
		slides = append(slides, s)
	}
	c.Slides = len(slides)

	ct := hdr + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/>`
	for _, e := range []string{"jpg", "png"} {
		if exts[e] {
			ct += `<Default Extension="` + e + `" ContentType="` + xmlw.MIME(e) + `"/>`
		}
	}
	const pml = "application/vnd.openxmlformats-officedocument.presentationml."
	ct += `<Override PartName="/ppt/presentation.xml" ContentType="` + pml + `presentation.main+xml"/>` +
		`<Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="` + pml + `slideMaster+xml"/>` +
		`<Override PartName="/ppt/slideLayouts/slideLayout1.xml" ContentType="` + pml + `slideLayout+xml"/>` +
		`<Override PartName="/ppt/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>` +
		`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>`
	for i := range slides {
		ct += `<Override PartName="/ppt/slides/slide` + strconv.Itoa(i+1) + `.xml" ContentType="` + pml + `slide+xml"/>`
	}
	ct += `</Types>`

	prels := hdr + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="` + rel + `/slideMaster" Target="slideMasters/slideMaster1.xml"/>` +
		`<Relationship Id="rId2" Type="` + rel + `/theme" Target="theme/theme1.xml"/>`
	ids := ""
	for i := range slides {
		n := strconv.Itoa(i + 1)
		prels += `<Relationship Id="rId` + strconv.Itoa(i+3) + `" Type="` + rel + `/slide" Target="slides/slide` + n + `.xml"/>`
		ids += `<p:sldId id="` + strconv.Itoa(256+i) + `" r:id="rId` + strconv.Itoa(i+3) + `"/>`
	}
	prels += `</Relationships>`
	pres := hdr + `<p:presentation ` + nsA + ` ` + nsR + ` ` + nsP + `><p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>`
	if ids != "" {
		pres += `<p:sldIdLst>` + ids + `</p:sldIdLst>`
	}
	pres += `<p:sldSz cx="` + ff(sw) + `" cy="` + ff(sh) + `"/><p:notesSz cx="6858000" cy="9144000"/></p:presentation>`

	rootRels := hdr + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="` + rel + `/officeDocument" Target="ppt/presentation.xml"/>` +
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/></Relationships>`
	core := hdr + `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>` +
		xmlw.Esc(docmodel.OneLine(title)) + `</dc:title></cp:coreProperties>`

	master := hdr + `<p:sldMaster ` + nsA + ` ` + nsR + ` ` + nsP + `><p:cSld><p:bg><p:bgRef idx="1001"><a:schemeClr val="bg1"/></p:bgRef></p:bg><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr></p:spTree></p:cSld>` +
		`<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>` +
		`<p:sldLayoutIdLst><p:sldLayoutId id="2147483649" r:id="rId1"/></p:sldLayoutIdLst><p:txStyles><p:titleStyle><a:lvl1pPr><a:defRPr sz="4400"/></a:lvl1pPr></p:titleStyle><p:bodyStyle><a:lvl1pPr><a:defRPr sz="1800"/></a:lvl1pPr></p:bodyStyle><p:otherStyle><a:lvl1pPr><a:defRPr sz="1800"/></a:lvl1pPr></p:otherStyle></p:txStyles></p:sldMaster>`
	layout := hdr + `<p:sldLayout ` + nsA + ` ` + nsR + ` ` + nsP + ` type="blank" preserve="1"><p:cSld name="Blank"><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr></p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sldLayout>`
	one := func(target, typ string) string {
		return hdr + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="` + rel + `/` + typ + `" Target="` + target + `"/></Relationships>`
	}
	out := []ziputil.Entry{
		{Name: "[Content_Types].xml", Data: []byte(ct)},
		{Name: "_rels/.rels", Data: []byte(rootRels)},
		{Name: "docProps/core.xml", Data: []byte(core)},
		{Name: "ppt/presentation.xml", Data: []byte(pres)},
		{Name: "ppt/_rels/presentation.xml.rels", Data: []byte(prels)},
		{Name: "ppt/slideMasters/slideMaster1.xml", Data: []byte(master)},
		{Name: "ppt/slideMasters/_rels/slideMaster1.xml.rels", Data: []byte(hdr + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="` + rel + `/slideLayout" Target="../slideLayouts/slideLayout1.xml"/><Relationship Id="rId2" Type="` + rel + `/theme" Target="../theme/theme1.xml"/></Relationships>`)},
		{Name: "ppt/slideLayouts/slideLayout1.xml", Data: []byte(layout)},
		{Name: "ppt/slideLayouts/_rels/slideLayout1.xml.rels", Data: []byte(one("../slideMasters/slideMaster1.xml", "slideMaster"))},
		{Name: "ppt/theme/theme1.xml", Data: []byte(theme)},
	}
	for i, s := range slides {
		n := strconv.Itoa(i + 1)
		out = append(out,
			ziputil.Entry{Name: "ppt/slides/slide" + n + ".xml", Data: []byte(s.xml.String())},
			ziputil.Entry{Name: "ppt/slides/_rels/slide" + n + ".xml.rels", Data: []byte(hdr + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + strings.Join(s.rels, "") + `</Relationships>`)})
	}
	if err := ctx.Err(); err != nil {
		return nil, c, err
	}
	body, err := ziputil.Write(append(out, entries...))
	return body, c, err
}

const theme = hdr + `<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="Nib"><a:themeElements><a:clrScheme name="Nib">` +
	`<a:dk1><a:srgbClr val="000000"/></a:dk1><a:lt1><a:srgbClr val="FFFFFF"/></a:lt1><a:dk2><a:srgbClr val="1F2937"/></a:dk2><a:lt2><a:srgbClr val="F3F4F6"/></a:lt2>` +
	`<a:accent1><a:srgbClr val="2563EB"/></a:accent1><a:accent2><a:srgbClr val="DC2626"/></a:accent2><a:accent3><a:srgbClr val="16A34A"/></a:accent3><a:accent4><a:srgbClr val="CA8A04"/></a:accent4>` +
	`<a:accent5><a:srgbClr val="7C3AED"/></a:accent5><a:accent6><a:srgbClr val="0891B2"/></a:accent6><a:hlink><a:srgbClr val="2563EB"/></a:hlink><a:folHlink><a:srgbClr val="7C3AED"/></a:folHlink></a:clrScheme>` +
	`<a:fontScheme name="Nib"><a:majorFont><a:latin typeface="Arial"/><a:ea typeface=""/><a:cs typeface=""/></a:majorFont><a:minorFont><a:latin typeface="Arial"/><a:ea typeface=""/><a:cs typeface=""/></a:minorFont></a:fontScheme>` +
	`<a:fmtScheme name="Nib"><a:fillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:fillStyleLst>` +
	`<a:lnStyleLst><a:ln w="9525"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln><a:ln w="9525"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln><a:ln w="9525"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln></a:lnStyleLst>` +
	`<a:effectStyleLst><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle></a:effectStyleLst>` +
	`<a:bgFillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:bgFillStyleLst></a:fmtScheme></a:themeElements></a:theme>`

// maxMediaBytes is MaxMediaBytes as a variable so a test can lower it.
var maxMediaBytes = MaxMediaBytes
