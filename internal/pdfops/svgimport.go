package pdfops

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// An SVG in (PLAN-import-export P05.S01a, X11(c), R3, ADR-204). The SVG is parsed strictly and REBUILT from
// an allowlist of elements, with every attribute that could run or fetch removed; only that rebuilt text
// is embedded, and only as `<img src="data:image/svg+xml;base64,…">` in a wrapper page Nib writes itself,
// so the browser draws it in its image mode (no script, no external fetch) behind the same origin, host
// rule and CSP header as every other page (ADR-200). The PDF page is the SVG's declared size, which is
// the stranger's number, so it is capped here and written into the wrapper by Nib, never copied.

const (
	svgNS   = "http://www.w3.org/2000/svg"
	xlinkNS = "http://www.w3.org/1999/xlink"
	xmlNS   = "http://www.w3.org/XML/1998/namespace"

	// svgMaxBytes caps the source (provisional, R5); svgMaxElements and svgMaxDepth bound the walk;
	// svgMaxSide caps one side of the page in CSS pixels (14,400 px = 200 in, the PDF page ceiling).
	svgMaxBytes    = 8 << 20
	svgMaxElements = 200000
	svgMaxDepth    = 128
	svgMaxSide     = 14400.0
)

// svgKeep are the SVG elements copied. Anything else is removed WITH its children: script, foreignObject,
// a, animate, set, feImage, audio, video, iframe and everything unknown.
var svgKeep = map[string]bool{
	"svg": true, "g": true, "defs": true, "symbol": true, "use": true, "path": true, "rect": true, "circle": true,
	"ellipse": true, "line": true, "polyline": true, "polygon": true, "text": true, "tspan": true, "textPath": true,
	"title": true, "desc": true, "linearGradient": true, "radialGradient": true, "stop": true, "pattern": true,
	"clipPath": true, "mask": true, "marker": true, "style": true, "image": true, "filter": true,
	"feGaussianBlur": true, "feOffset": true, "feBlend": true, "feColorMatrix": true, "feMerge": true,
	"feMergeNode": true, "feFlood": true, "feComposite": true, "feDropShadow": true, "feMorphology": true,
	"feComponentTransfer": true, "feFuncR": true, "feFuncG": true, "feFuncB": true, "feFuncA": true,
}

// sanitizeSVG returns the rebuilt SVG and its page size in CSS pixels.
func sanitizeSVG(data []byte) ([]byte, float64, float64, error) {
	bad := func(format string, a ...any) ([]byte, float64, float64, error) {
		return nil, 0, 0, fmt.Errorf("%w: %s", ErrHTMLUnreadable, fmt.Sprintf(format, a...))
	}
	if len(data) > svgMaxBytes {
		return bad("that SVG is larger than Nib reads (%d MB)", svgMaxBytes>>20)
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true // an undefined entity is an error: no DOCTYPE is read, so none can expand
	var out bytes.Buffer
	var w, h float64
	depth, elements, skip := 0, 0, 0 // skip: depth inside a removed subtree
	var openNames []string
	roots := 0
	var styleText strings.Builder // a <style>'s text is judged whole: a comment or CDATA splits it into tokens
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return bad("it is not well-formed (%v)", err)
		}
		switch t := tok.(type) {
		case xml.Directive:
			return bad("it has a document type declaration, which Nib does not read")
		case xml.ProcInst, xml.Comment:
		case xml.StartElement:
			depth++
			if depth > svgMaxDepth {
				return bad("it is nested deeper than %d", svgMaxDepth)
			}
			if skip > 0 {
				skip++
				continue
			}
			elements++
			if elements > svgMaxElements {
				return bad("it has more than %d elements", svgMaxElements)
			}
			name := t.Name.Local
			if depth == 1 {
				if roots++; roots > 1 {
					return bad("it has more than one picture in it")
				}
			}
			if depth == 1 && (t.Name.Space != svgNS || name != "svg") {
				return bad("it is not an SVG picture")
			}
			if t.Name.Space == svgNS && depth > 1 && (name == "a" || name == "switch") {
				// A link is not followed from a picture, and a switch picks by features: the children stay.
				openNames = append(openNames, "")
				continue
			}
			if t.Name.Space != svgNS || !svgKeep[name] {
				skip = 1
				continue
			}
			attrs := keptSVGAttrs(t, name)
			if depth == 1 {
				var ok bool
				w, h, ok = svgSize(t)
				if !ok {
					return bad("it does not say how big it is (no width and height, no viewBox)")
				}
				attrs = rootAttrs(attrs, w, h)
			}
			out.WriteString("<" + name)
			if depth == 1 {
				out.WriteString(` xmlns="` + svgNS + `" xmlns:xlink="` + xlinkNS + `"`)
			}
			out.WriteString(attrs)
			out.WriteString(">")
			openNames = append(openNames, name)
		case xml.EndElement:
			depth--
			if skip > 0 {
				skip--
				continue
			}
			n := openNames[len(openNames)-1]
			openNames = openNames[:len(openNames)-1]
			if n == "style" {
				if cssIsInert(styleText.String()) {
					_ = xml.EscapeText(&out, []byte(styleText.String()))
				}
				styleText.Reset()
			}
			if n != "" {
				out.WriteString("</" + n + ">")
			}
		case xml.CharData:
			if skip > 0 || len(openNames) == 0 {
				continue
			}
			if openNames[len(openNames)-1] == "style" {
				styleText.Write(t)
				continue
			}
			_ = xml.EscapeText(&out, t)
		}
	}
	if out.Len() == 0 || len(openNames) != 0 {
		return bad("it is not an SVG picture")
	}
	return out.Bytes(), w, h, nil
}

// keptSVGAttrs writes the attributes of a kept element that cannot run or fetch.
func keptSVGAttrs(t xml.StartElement, el string) string {
	var b strings.Builder
	seen := map[string]bool{} // href and xlink:href, or a:x and x, can land on one name; the second is dropped
	for _, a := range t.Attr {
		sp, local := a.Name.Space, a.Name.Local
		var out string
		switch {
		case sp == "xmlns" || (sp == "" && local == "xmlns"):
			continue
		case sp == "" || sp == svgNS:
			out = local
		case sp == xlinkNS:
			out = "xlink:" + local
		case sp == xmlNS || sp == "xml":
			out = "xml:" + local
		default:
			continue
		}
		low := strings.ToLower(local)
		if strings.HasPrefix(low, "on") || low == "src" {
			continue
		}
		v := a.Value
		if low == "href" {
			// A fragment of this picture, or (on an image) a picture carried inside it — nothing else.
			s := strings.TrimSpace(v)
			if strings.HasPrefix(s, "#") {
				v = s
			} else if el == "image" && dataImageSVG(&s) {
				v = s
			} else {
				continue
			}
		}
		lv := strings.ToLower(v)
		if strings.Contains(lv, "javascript:") || strings.Contains(lv, "@import") || strings.Contains(lv, "<") || strings.Contains(lv, "&#") {
			continue
		}
		if i := strings.Index(lv, "url("); i >= 0 && !onlyFragmentURLs(lv) {
			continue
		}
		if low == "style" && !cssIsInertAllowingFragments(v) {
			continue
		}
		if seen[out] {
			continue
		}
		seen[out] = true
		b.WriteString(" " + out + `="`)
		var esc bytes.Buffer
		_ = xml.EscapeText(&esc, []byte(v))
		b.WriteString(strings.ReplaceAll(esc.String(), `"`, "&quot;"))
		b.WriteString(`"`)
	}
	return b.String()
}

// onlyFragmentURLs: every `url(` in the value is `url(#id)` (a paint server or a clip), which names a part
// of this same picture.
func onlyFragmentURLs(lv string) bool {
	for rest := lv; ; {
		i := strings.Index(rest, "url(")
		if i < 0 {
			return true
		}
		rest = strings.TrimLeft(rest[i+4:], " '\"")
		if !strings.HasPrefix(rest, "#") {
			return false
		}
	}
}

func cssIsInertAllowingFragments(css string) bool {
	if !onlyFragmentURLs(strings.ToLower(css)) {
		return false
	}
	return cssIsInert(strings.ReplaceAll(strings.ToLower(css), "url(#", "u("))
}

// dataImageSVG admits a base64 raster picture inside an <image>: the same four types as a web page's.
func dataImageSVG(v *string) bool {
	total := 0
	return dataImage(v, &total)
}

// svgSize reads the page size in CSS pixels from the root's width and height, falling back to its viewBox.
func svgSize(t xml.StartElement) (float64, float64, bool) {
	var wv, hv, vb string
	for _, a := range t.Attr {
		if a.Name.Space != "" {
			continue
		}
		switch a.Name.Local {
		case "width":
			wv = a.Value
		case "height":
			hv = a.Value
		case "viewBox":
			vb = a.Value
		}
	}
	w, wok := svgLength(wv)
	h, hok := svgLength(hv)
	var vw, vh float64
	vok := false
	if f := strings.FieldsFunc(vb, func(r rune) bool { return r == ' ' || r == ',' || r == '\t' || r == '\n' || r == '\r' }); len(f) == 4 {
		a, e1 := strconv.ParseFloat(f[2], 64)
		b, e2 := strconv.ParseFloat(f[3], 64)
		vok = e1 == nil && e2 == nil && a > 0 && b > 0 && !math.IsInf(a, 0) && !math.IsInf(b, 0)
		vw, vh = a, b
	}
	switch {
	case wok && hok:
	case wok && vok:
		h = w * vh / vw
	case hok && vok:
		w = h * vw / vh
	case vok:
		w, h = vw, vh
	default:
		return 0, 0, false
	}
	if !(w >= 1 && h >= 1 && w <= svgMaxSide && h <= svgMaxSide) {
		return 0, 0, false
	}
	return w, h, true
}

// svgLength reads a number with an absolute unit (or none) as CSS pixels. A percentage or a font-relative
// length is not a size.
func svgLength(s string) (float64, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, false
	}
	mult := 1.0
	for _, u := range []struct {
		suf string
		m   float64
	}{{"px", 1}, {"pt", 96.0 / 72}, {"pc", 16}, {"mm", 96 / 25.4}, {"cm", 96 / 2.54}, {"in", 96}} {
		if strings.HasSuffix(s, u.suf) {
			s, mult = strings.TrimSpace(strings.TrimSuffix(s, u.suf)), u.m
			break
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
		return 0, false
	}
	return v * mult, true
}

// rootAttrs replaces the root's width and height with the capped pixel size Nib will use, and gives it a
// viewBox when it had none so the drawing scales to that size.
func rootAttrs(attrs string, w, h float64) string {
	var keep []string
	hasVB := false
	for _, a := range splitAttrs(attrs) {
		switch {
		case strings.HasPrefix(a, " width="), strings.HasPrefix(a, " height="):
		default:
			if strings.HasPrefix(a, " viewBox=") {
				hasVB = true
			}
			keep = append(keep, a)
		}
	}
	s := strings.Join(keep, "") + fmt.Sprintf(` width="%s" height="%s"`, fnum(w), fnum(h))
	if !hasVB {
		s += fmt.Sprintf(` viewBox="0 0 %s %s"`, fnum(w), fnum(h))
	}
	return s
}

// splitAttrs cuts the attribute text keptSVGAttrs wrote back into ` name="value"` pieces (values carry no
// unescaped quote).
func splitAttrs(s string) []string {
	var out []string
	for len(s) > 0 {
		i := strings.Index(s, `="`)
		if i < 0 {
			break
		}
		j := strings.Index(s[i+2:], `"`)
		if j < 0 {
			break
		}
		end := i + 2 + j + 1
		out = append(out, s[:end])
		s = s[end:]
	}
	return out
}

func fnum(v float64) string { return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64) }

// svgWrapper is the page Nib writes itself: the picture as a data image, at a page of its own size. It holds
// nothing the stranger wrote but the base64 of the rebuilt picture.
func svgWrapper(svg []byte, w, h float64) []byte {
	ws, hs := fnum(w), fnum(h)
	return []byte(`<!doctype html><html><head><meta charset="utf-8"><style>@page{size:` + ws + `px ` + hs + `px;margin:0}` +
		`html,body{margin:0;padding:0}img{display:block;width:` + ws + `px;height:` + hs + `px}</style></head><body>` +
		`<img alt="" src="data:image/svg+xml;base64,` + base64.StdEncoding.EncodeToString(svg) + `"></body></html>`)
}

// ConvertSVGToPDF opens an SVG picture as a one-page PDF of its declared size.
func ConvertSVGToPDF(ctx context.Context, data []byte) ([]byte, error) {
	chromium := browserPath()
	if chromium == "" {
		return nil, ErrBrowserMissing
	}
	svg, w, h, err := sanitizeSVG(data)
	if err != nil {
		return nil, err
	}
	if err := renderProbe(ctx, chromium); err != nil {
		return nil, err
	}
	pdf, err := renderDocument(ctx, chromium, svgWrapper(svg, w, h))
	if err != nil {
		return nil, err
	}
	n, perr := PageCount(pdf)
	if perr != nil || n == 0 {
		return nil, ErrHTMLUnreadable
	}
	if n > 1 {
		return nil, fmt.Errorf("%w: it printed on %d pages", ErrHTMLUnreadable, n)
	}
	return pdf, nil
}

// isSVG: a file whose FIRST element is `<svg`, after an XML declaration, comments and a doctype. A web page
// or a fragment that merely holds an inline svg opens another tag first and is not one.
func isSVG(head []byte) bool {
	h := bytes.TrimPrefix(head, []byte{0xEF, 0xBB, 0xBF})
	if bytes.IndexByte(h, 0) >= 0 {
		return false
	}
	for {
		h = bytes.TrimLeft(h, " \t\r\n\f")
		var end []byte
		switch l := bytes.ToLower(h); {
		case bytes.HasPrefix(l, []byte("<?")):
			end = []byte("?>")
		case bytes.HasPrefix(l, []byte("<!--")):
			end = []byte("-->")
		case bytes.HasPrefix(l, []byte("<!doctype svg")):
			end = []byte(">")
		default:
			return len(l) >= 5 && bytes.HasPrefix(l, []byte("<svg")) && bytes.IndexByte([]byte(" \t\r\n/>"), l[4]) >= 0
		}
		i := bytes.Index(h, end)
		if i < 0 {
			return false
		}
		h = h[i+len(end):]
	}
}
