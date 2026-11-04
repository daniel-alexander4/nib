package pdfops

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/url"
	"path"
	"strings"

	"golang.org/x/net/html/charset"

	"nib/internal/ziputil"
)

// Saved pages that are PACKAGES: MHTML (one MIME message) and EPUB (a zip) — PLAN-import-export P04.S04b,
// W34, Y8, ADR-201. Both are unpacked IN MEMORY and become one HTML document whose pictures are `data:` URLs;
// that document goes through the same sanitiser and the same origin as a saved page. No file is written under
// a name the package chose, and no filesystem path is derived from its content: a picture is found by the
// name the package gives it for a part of itself, and by nothing else.

var (
	// ErrPackageTooBig: a saved-page package past a count or size cap.
	ErrPackageTooBig = errors.New("that saved page is packaged in a way Nib will not open: too many parts, or too large")
	// ErrPackageUnreadable: the package does not hold a page.
	ErrPackageUnreadable = errors.New("Nib could not find a page in that package")
)

const (
	packageParts     = 500      // MIME parts / EPUB spine items and pictures read
	packagePartBytes = 8 << 20  // one decoded part
	packageDepth     = 4        // nested multipart
	mimeHeaderBytes  = 64 << 10 // the message's own header block
)

// Package-wide caps: variables so a test can lower them.
var (
	packageAllBytes = 96 << 20 // all decoded parts together
	epubChapters    = 300
)

// packagePictures holds a package's pictures by the names it gives them.
type packagePictures struct {
	byName map[string]string // exact name → data URL
	total  int
}

func (p *packagePictures) add(names []string, typ string, body []byte) {
	if len(body) == 0 || len(body) > maxDataImage || p.total+len(body) > packageAllBytes {
		return
	}
	// The type is decided by the bytes, never by what the part says it is.
	k, ok := sniffImage(body)
	if !ok {
		return
	}
	mt := ""
	for _, f := range Formats() {
		if f.Image && f.Key == string(k) && len(f.MIME) > 0 {
			mt = f.MIME[0]
		}
	}
	if !dataImageTypeOK(mt) {
		return
	}
	p.total += len(body)
	u := "data:" + mt + ";base64," + base64.StdEncoding.EncodeToString(body)
	for _, n := range names {
		if n != "" {
			p.byName[n] = u
		}
	}
}

func (p *packagePictures) resolver(base string) func(string) (string, bool) {
	return func(src string) (string, bool) {
		if u, ok := p.byName[src]; ok {
			return u, true
		}
		if base != "" {
			if u, ok := p.byName[path.Clean(path.Join(base, src))]; ok {
				return u, true
			}
		}
		if rest, ok := strings.CutPrefix(src, "cid:"); ok {
			if u, ok := p.byName["cid:"+rest]; ok {
				return u, true
			}
		}
		return "", false
	}
}

// ---- MHTML ----

type mimePart struct {
	typ      string
	location string
	cid      string
	disp     string // the Content-Disposition media type, lower-cased ("attachment", "inline" or "")
	body     []byte
}

func decodePart(r io.Reader, cte string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(cte)) {
	case "base64":
		r = base64.NewDecoder(base64.StdEncoding, &stripSpace{r})
	case "quoted-printable":
		r = quotedprintable.NewReader(r)
	}
	b, err := io.ReadAll(io.LimitReader(r, packagePartBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > packagePartBytes {
		return nil, ErrPackageTooBig
	}
	return b, nil
}

// stripSpace removes the line breaks and spaces base64 in a MIME body is wrapped with.
type stripSpace struct{ r io.Reader }

func (s *stripSpace) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	w := 0
	for _, c := range p[:n] {
		if c != '\r' && c != '\n' && c != ' ' && c != '\t' {
			p[w] = c
			w++
		}
	}
	return w, err
}

// walkMIME reads parts to the caps. depth counts nested multipart.
//
// decodeIf, when not nil, is asked before a leaf part is decoded: a part it refuses is recorded with no body
// (mail attachments are counted and never read — Y8).
func walkMIME(h func(string) string, body io.Reader, depth int, parts *[]mimePart, total *int, decodeIf func(typ, disp string) bool) error {
	typ, params, err := mime.ParseMediaType(h("Content-Type"))
	if err != nil {
		typ = "text/plain"
	}
	if strings.HasPrefix(typ, "multipart/") {
		if depth >= packageDepth || params["boundary"] == "" {
			return ErrPackageTooBig
		}
		mr := multipart.NewReader(body, params["boundary"])
		for {
			p, err := mr.NextRawPart()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return nil // a damaged tail ends the walk; what was read stays
			}
			if len(*parts) >= packageParts {
				return ErrPackageTooBig
			}
			if err := walkMIME(func(k string) string { return p.Header.Get(k) }, p, depth+1, parts, total, decodeIf); err != nil {
				return err
			}
		}
	}
	disp, _, _ := mime.ParseMediaType(h("Content-Disposition"))
	disp = strings.ToLower(disp)
	if decodeIf != nil && !decodeIf(typ, disp) {
		*parts = append(*parts, mimePart{typ: typ, disp: disp, cid: strings.Trim(strings.TrimSpace(h("Content-ID")), "<>")})
		return nil
	}
	data, err := decodePart(body, h("Content-Transfer-Encoding"))
	if err != nil {
		return err
	}
	if *total += len(data); *total > packageAllBytes {
		return ErrPackageTooBig
	}
	if typ == "text/html" || typ == "text/plain" {
		if cs := params["charset"]; cs != "" {
			if r, cerr := charset.NewReaderLabel(cs, bytes.NewReader(data)); cerr == nil {
				if conv, rerr := io.ReadAll(io.LimitReader(r, packagePartBytes*2)); rerr == nil {
					data = conv
				}
			}
		}
	}
	*parts = append(*parts, mimePart{
		typ:      typ,
		location: strings.TrimSpace(h("Content-Location")),
		cid:      strings.Trim(strings.TrimSpace(h("Content-ID")), "<>"),
		disp:     disp,
		body:     data,
	})
	return nil
}

// mhtmlToHTML turns an MHTML message into one HTML document (sanitised later) and its picture resolver.
func mhtmlToHTML(data []byte) ([]byte, func(string) (string, bool), error) {
	end := len(data)
	for _, sep := range []string{"\r\n\r\n", "\n\n"} {
		if i := bytes.Index(data, []byte(sep)); i >= 0 && i < end {
			end = i
		}
	}
	if end > mimeHeaderBytes {
		return nil, nil, ErrPackageTooBig
	}
	msg, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrPackageUnreadable, err)
	}
	var parts []mimePart
	total := 0
	if err := walkMIME(func(k string) string { return msg.Header.Get(k) }, msg.Body, 0, &parts, &total, nil); err != nil {
		return nil, nil, err
	}
	pics := &packagePictures{byName: map[string]string{}}
	var page []byte
	for _, p := range parts {
		switch {
		case p.typ == "text/html" && page == nil:
			page = p.body
		case strings.HasPrefix(p.typ, "image/"):
			names := []string{p.location}
			if p.cid != "" {
				names = append(names, "cid:"+p.cid)
			}
			pics.add(names, p.typ, p.body)
		}
	}
	if page == nil {
		return nil, nil, ErrPackageUnreadable
	}
	return page, pics.resolver(""), nil
}

// ---- EPUB ----

type epubContainer struct {
	Rootfiles []struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}

type epubPackage struct {
	Manifest []struct {
		ID   string `xml:"id,attr"`
		Href string `xml:"href,attr"`
		Type string `xml:"media-type,attr"`
	} `xml:"manifest>item"`
	Spine []struct {
		IDRef string `xml:"idref,attr"`
	} `xml:"spine>itemref"`
}

func readXML(b []byte, v any) error {
	d := xml.NewDecoder(bytes.NewReader(b))
	d.Strict = false
	d.CharsetReader = charset.NewReaderLabel
	return d.Decode(v) // encoding/xml expands no external entity
}

// epubToHTML reads an EPUB's spine in order and joins the chapters into one sanitised document.
func epubToHTML(data []byte) ([]byte, error) {
	a, err := ziputil.Open(bytes.NewReader(data), int64(len(data)), packageLimits)
	if err != nil {
		return nil, ErrPackageTooBig
	}
	members := map[string]int{}
	files := a.Files()
	for i, f := range files {
		members[f.Name] = i
	}
	read := func(name string, max int64) ([]byte, bool) {
		i, ok := members[name]
		if !ok {
			return nil, false
		}
		b, err := a.Read(files[i], max)
		return b, err == nil
	}
	cbytes, ok := read("META-INF/container.xml", 1<<20)
	if !ok {
		return nil, ErrPackageUnreadable
	}
	var c epubContainer
	if readXML(cbytes, &c) != nil || len(c.Rootfiles) == 0 || c.Rootfiles[0].FullPath == "" {
		return nil, ErrPackageUnreadable
	}
	opfPath := path.Clean(c.Rootfiles[0].FullPath)
	obytes, ok := read(opfPath, 4<<20)
	if !ok {
		return nil, ErrPackageUnreadable
	}
	var opf epubPackage
	if readXML(obytes, &opf) != nil || len(opf.Spine) == 0 {
		return nil, ErrPackageUnreadable
	}
	opfDir := path.Dir(opfPath)
	if opfDir == "." {
		opfDir = ""
	}
	href := func(base, h string) string {
		h = strings.SplitN(h, "#", 2)[0]
		if u, err := url.PathUnescape(h); err == nil {
			h = u
		}
		return path.Clean(path.Join(base, h))
	}
	byID := map[string]string{}
	pics := &packagePictures{byName: map[string]string{}}
	for _, it := range opf.Manifest {
		byID[it.ID] = href(opfDir, it.Href)
		if strings.HasPrefix(it.Type, "image/") && it.Type != "image/svg+xml" && len(pics.byName) < packageParts {
			if b, ok := read(href(opfDir, it.Href), packagePartBytes); ok {
				pics.add([]string{href(opfDir, it.Href)}, it.Type, b)
			}
		}
	}
	var heads, bodies bytes.Buffer
	n := 0
	seen := map[string]bool{}
	for _, ref := range opf.Spine {
		if n >= epubChapters || heads.Len()+bodies.Len() > epubOutputBytes {
			return nil, ErrPackageTooBig
		}
		name, ok := byID[ref.IDRef]
		if !ok || seen[name] {
			continue // a spine that names one chapter again is read once: each read charges its pictures again
		}
		seen[name] = true
		b, ok := read(name, packagePartBytes)
		if !ok {
			continue
		}
		dir := path.Dir(name)
		if dir == "." {
			dir = ""
		}
		out, _, err := sanitizeHTMLWith(b, pics.resolver(dir))
		if err != nil {
			continue
		}
		hd, bd := splitOwnDocument(out)
		heads.Write(hd)
		if n > 0 {
			bodies.WriteString(`<div style="break-before:page"></div>`)
		}
		bodies.Write(bd)
		n++
	}
	if n == 0 {
		return nil, ErrPackageUnreadable
	}
	return []byte("<!doctype html><html><head><meta charset=utf-8>" + heads.String() + "</head><body>" + bodies.String() + "</body></html>"), nil
}

// splitOwnDocument cuts a document sanitizeHTML wrote into its style elements and its body. The input is
// Nib's own output, whose shape is fixed (one head holding a meta and style elements, one body), so a
// string cut is exact; a stranger's text cannot contain a literal `</body>` that the renderer did not escape.
func splitOwnDocument(doc []byte) (head, body []byte) {
	s := string(doc)
	hs := strings.Index(s, "<head>")
	he := strings.Index(s, "</head>")
	bs := strings.Index(s, "<body>")
	be := strings.LastIndex(s, "</body>")
	if hs < 0 || he < hs || bs < he || be < bs {
		return nil, nil
	}
	h := strings.Replace(s[hs+len("<head>"):he], `<meta charset="utf-8"/>`, "", 1)
	h = strings.Replace(h, `<meta charset="utf-8">`, "", 1)
	return []byte(h), []byte(s[bs+len("<body>") : be])
}

// epubOutputBytes bounds the chapters' sanitised output together. Each chapter's picture budget is its own,
// so without this one 300-chapter book could write 300 budgets (a variable so a test can lower it).
var epubOutputBytes = packageAllBytes
