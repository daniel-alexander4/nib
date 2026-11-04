package pdfops

import (
	"bytes"
	"encoding/base64"
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// The saved-web-page sanitiser (PLAN-import-export P04.S04, Y8, D29, ADR-200). A page a stranger wrote is
// parsed with x/net/html and REBUILT from an allowlist of tags and attributes — what is not named is not
// copied. The CSP header and the host rule (internal/browserrender) are the walls; this is the rule, so a
// page that would have reached outside is not even handed to the browser. No filesystem path is ever
// derived from the page's content: a picture is a `data:` URL carried in the page or nothing.

// htmlKeep are the elements copied. Anything else with children is unwrapped (its children are kept) unless
// it is in htmlDropWhole.
var htmlKeep = map[atom.Atom]bool{
	atom.A: true, atom.Abbr: true, atom.Address: true, atom.Article: true, atom.Aside: true, atom.B: true,
	atom.Bdi: true, atom.Bdo: true, atom.Blockquote: true, atom.Br: true, atom.Caption: true, atom.Center: true,
	atom.Cite: true, atom.Code: true, atom.Col: true, atom.Colgroup: true, atom.Dd: true, atom.Del: true,
	atom.Details: true, atom.Dfn: true, atom.Div: true, atom.Dl: true, atom.Dt: true, atom.Em: true,
	atom.Figcaption: true, atom.Figure: true, atom.Footer: true, atom.H1: true, atom.H2: true, atom.H3: true,
	atom.H4: true, atom.H5: true, atom.H6: true, atom.Header: true, atom.Hr: true, atom.I: true, atom.Img: true,
	atom.Ins: true, atom.Kbd: true, atom.Li: true, atom.Main: true, atom.Mark: true, atom.Nav: true, atom.Ol: true,
	atom.P: true, atom.Pre: true, atom.Q: true, atom.S: true, atom.Samp: true, atom.Section: true, atom.Small: true,
	atom.Span: true, atom.Strong: true, atom.Sub: true, atom.Summary: true, atom.Sup: true, atom.Table: true,
	atom.Tbody: true, atom.Td: true, atom.Tfoot: true, atom.Th: true, atom.Thead: true, atom.Time: true,
	atom.Tr: true, atom.U: true, atom.Ul: true, atom.Var: true, atom.Wbr: true,
}

// htmlDropWhole are elements (by name, as parsed) removed with everything inside them: they run code, load something, take input
// or navigate. (`style` is handled apart: kept only when it is free of anything that fetches.)
var htmlDropWhole = map[string]bool{
	"script": true, "noscript": true, "iframe": true, "frame": true, "frameset": true,
	"object": true, "embed": true, "applet": true, "svg": true, "math": true, "template": true,
	"audio": true, "video": true, "source": true, "track": true, "canvas": true, "form": true,
	"input": true, "button": true, "select": true, "textarea": true, "link": true, "meta": true,
	"base": true, "title": true, "map": true, "area": true, "picture": true, "dialog": true,
}

var htmlKeepAttr = map[string]bool{
	"lang": true, "dir": true, "title": true, "class": true, "id": true, "colspan": true, "rowspan": true,
	"start": true, "reversed": true, "type": true, "align": true, "valign": true, "width": true, "height": true,
	"scope": true, "headers": true, "open": true, "datetime": true,
}

const maxDataImage = 8 << 20 // one embedded picture, decoded

// maxDataImagesAll bounds all embedded pictures together; a variable so a test can lower it.
var maxDataImagesAll = 64 << 20

// dataImageTypeOK: a picture a browser draws from a data URL. The MIME types come from the format table
// (ADR-184), by key, so this is not a second list of them.
func dataImageTypeOK(typ string) bool {
	for _, f := range Formats() {
		if !f.Image {
			continue
		}
		switch f.Key {
		case "png", "jpeg", "gif", "webp":
			for _, m := range f.MIME {
				if m == typ {
					return true
				}
			}
		}
	}
	return false
}

// cssIsInert reports whether a style sheet or declaration is free of anything that could fetch or run.
// The CSP header already stops all of it; refusing the text is the rule the header backs up.
func cssIsInert(css string) bool {
	l := strings.ToLower(css)
	for _, bad := range []string{"@import", "url(", "image-set(", "image(", "src:", "expression", "behavior", "-moz-binding", "\\", "@font-face", "javascript:", "<"} {
		if strings.Contains(l, bad) {
			return false
		}
	}
	return true
}

// safeHref admits an address a click may follow: http, https and mailto. Nothing is fetched by it.
func safeHref(v string) bool {
	u, err := url.Parse(strings.TrimSpace(v))
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "mailto":
		return true
	}
	return false
}

// dataImage admits a base64 picture in one of four types and returns it as given. The body is decoded to
// bound its size; its pixels are the browser's to decode, inside its own limits.
func dataImage(v *string, total *int) bool {
	s := strings.TrimSpace(*v)
	if !strings.HasPrefix(strings.ToLower(s), "data:") {
		return false
	}
	meta, body, ok := strings.Cut(s[5:], ",")
	if !ok {
		return false
	}
	meta = strings.ToLower(strings.ReplaceAll(meta, " ", ""))
	if !strings.HasSuffix(meta, ";base64") {
		return false
	}
	typ := strings.TrimSuffix(meta, ";base64")
	okType := dataImageTypeOK(typ)
	if !okType {
		return false
	}
	n := base64.StdEncoding.DecodedLen(len(body))
	if n > maxDataImage || *total+n > maxDataImagesAll {
		return false
	}
	if _, err := base64.StdEncoding.DecodeString(strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, body)); err != nil {
		return false
	}
	*total += n
	*v = "data:" + typ + ";base64," + body
	return true
}

// htmlStats is what the sanitiser left out, counted for a test and the notice.
type htmlStats struct {
	DroppedElements int
	DroppedImages   int
}

// sanitizeHTML returns a new document built from the allowlist.
func sanitizeHTML(data []byte) ([]byte, htmlStats, error) {
	return sanitizeHTMLWith(data, nil)
}

// sanitizeHTMLWith is sanitizeHTML for a page whose pictures live in the same package (an MHTML message, an
// EPUB): resolve maps a picture's `src` to a `data:` URL built from a member of that package, or says it
// cannot. It is the ONLY way a picture that is not already a data URL survives, and it never touches a path.
func sanitizeHTMLWith(data []byte, resolve func(src string) (string, bool)) ([]byte, htmlStats, error) {
	var st htmlStats
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, st, err
	}
	var head, body *html.Node
	var find func(*html.Node)
	find = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.Head:
				head = n
			case atom.Body:
				body = n
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			find(c)
		}
	}
	find(doc)

	out := &html.Node{Type: html.DocumentNode}
	out.AppendChild(&html.Node{Type: html.DoctypeNode, Data: "html"})
	root := &html.Node{Type: html.ElementNode, DataAtom: atom.Html, Data: "html"}
	out.AppendChild(root)
	oh := &html.Node{Type: html.ElementNode, DataAtom: atom.Head, Data: "head"}
	root.AppendChild(oh)
	meta := &html.Node{Type: html.ElementNode, DataAtom: atom.Meta, Data: "meta", Attr: []html.Attribute{{Key: "charset", Val: "utf-8"}}}
	oh.AppendChild(meta)
	ob := &html.Node{Type: html.ElementNode, DataAtom: atom.Body, Data: "body"}
	root.AppendChild(ob)

	total := 0
	// Style sheets in the head or the body are copied only when inert.
	var styles func(*html.Node)
	styles = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.DataAtom == atom.Style {
				var sb strings.Builder
				for t := c.FirstChild; t != nil; t = t.NextSibling {
					if t.Type == html.TextNode {
						sb.WriteString(t.Data)
					}
				}
				if cssIsInert(sb.String()) {
					s := &html.Node{Type: html.ElementNode, DataAtom: atom.Style, Data: "style"}
					s.AppendChild(&html.Node{Type: html.RawNode, Data: sb.String()})
					oh.AppendChild(s)
				} else {
					st.DroppedElements++
				}
				continue
			}
			styles(c)
		}
	}
	if head != nil {
		styles(head)
	}
	if body != nil {
		styles(body)
	}

	var copyInto func(dst, src *html.Node)
	copyInto = func(dst, src *html.Node) {
		for c := src.FirstChild; c != nil; c = c.NextSibling {
			switch c.Type {
			case html.TextNode:
				dst.AppendChild(&html.Node{Type: html.TextNode, Data: c.Data})
			case html.ElementNode:
				switch {
				case c.DataAtom == atom.Style:
					// handled above
				case htmlDropWhole[c.Data]:
					st.DroppedElements++
				case c.DataAtom == atom.Img:
					el := &html.Node{Type: html.ElementNode, DataAtom: atom.Img, Data: "img"}
					alt := ""
					haveSrc := false
					for _, a := range c.Attr {
						switch strings.ToLower(a.Key) {
						case "alt":
							alt = a.Val
							el.Attr = append(el.Attr, html.Attribute{Key: "alt", Val: a.Val})
						case "src":
							v := a.Val
							if resolve != nil {
								if r, ok := resolve(strings.TrimSpace(v)); ok {
									v = r
								}
							}
							if dataImage(&v, &total) {
								haveSrc = true
								el.Attr = append(el.Attr, html.Attribute{Key: "src", Val: v})
							}
						case "width", "height", "title":
							el.Attr = append(el.Attr, html.Attribute{Key: strings.ToLower(a.Key), Val: a.Val})
						}
					}
					if haveSrc {
						dst.AppendChild(el)
					} else {
						st.DroppedImages++
						if alt != "" {
							dst.AppendChild(&html.Node{Type: html.TextNode, Data: alt})
						}
					}
				case htmlKeep[c.DataAtom]:
					el := &html.Node{Type: html.ElementNode, DataAtom: c.DataAtom, Data: c.Data}
					for _, a := range c.Attr {
						k := strings.ToLower(a.Key)
						switch {
						case k == "href" && c.DataAtom == atom.A:
							if safeHref(a.Val) {
								el.Attr = append(el.Attr, html.Attribute{Key: "href", Val: a.Val})
							}
						case k == "style":
							if cssIsInert(a.Val) {
								el.Attr = append(el.Attr, html.Attribute{Key: "style", Val: a.Val})
							}
						case htmlKeepAttr[k]:
							el.Attr = append(el.Attr, html.Attribute{Key: k, Val: a.Val})
						}
					}
					dst.AppendChild(el)
					copyInto(el, c)
				default:
					// Unknown or structural (html, head, body, font, label, …): its words stay, its tag goes.
					copyInto(dst, c)
				}
			}
		}
	}
	if body != nil {
		copyInto(ob, body)
	}
	var buf bytes.Buffer
	if err := html.Render(&buf, out); err != nil {
		return nil, st, err
	}
	return buf.Bytes(), st, nil
}
