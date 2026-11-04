package html

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"nib/internal/docmodel"
	"nib/internal/docwrite/wtest"
)

func TestHTMLReadsBackAsTheModelAndHoldsNoScript(t *testing.T) {
	d := wtest.Hostile()
	out := writeOK(d)
	root, err := html.Parse(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	var words, hrefs []string
	var tags = map[string]int{}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.ElementNode:
			tags[n.Data]++
			for _, a := range n.Attr {
				if strings.HasPrefix(a.Key, "on") {
					t.Errorf("event attribute %s on <%s>", a.Key, n.Data)
				}
				if a.Key == "href" {
					hrefs = append(hrefs, a.Val)
				}
				if a.Key == "src" && !strings.HasPrefix(a.Val, "data:") {
					t.Errorf("an outside reference: %s", a.Val)
				}
			}
		case html.TextNode:
			if n.Parent != nil && n.Parent.Data != "style" {
				words = append(words, strings.Fields(n.Data)...)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	if tags["script"] != 0 || tags["b"] != 0 || tags["img"] != 1 {
		t.Errorf("tags = %v: a word became an element, or a picture went missing\n%s", tags, out)
	}
	if len(hrefs) != 1 || hrefs[0] != "https://example.com/a(b)c?x=1&y=2" {
		t.Errorf("hrefs = %v, want only the https link", hrefs)
	}
	if tags["ul"] != 1 || tags["ol"] != 1 || tags["table"] != 1 || tags["th"] != 2 || tags["h1"] != 1 {
		t.Errorf("structure lost: %v", tags)
	}
	// The words, in order, from the output's text nodes (title excluded: it is the document's name).
	var body []string
	skip := true
	for _, w := range words {
		if skip && w == "#" {
			skip = false
		}
		if !skip {
			body = append(body, w)
		}
	}
	i := 0
	for _, w := range wtest.Words(d) {
		for i < len(body) && body[i] != w {
			i++
		}
		if i == len(body) {
			t.Fatalf("word %q missing or out of order", w)
		}
		i++
	}
	for _, want := range []string{`lang="und"`, `dir="rtl"`, "Content-Security-Policy", "<strong>bold</strong>", "<em>italic</em>", `rel="noopener noreferrer"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %s", want)
		}
	}
}

func TestXHTMLIsWellFormedAndTheSameBodyAsTheWebPage(t *testing.T) {
	d := wtest.Hostile()
	body, pics, heads := xhtmlOK(d)
	dec := xml.NewDecoder(strings.NewReader("<r>" + body + "</r>"))
	for {
		if _, err := dec.Token(); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("the XHTML body is not well-formed: %v\n%s", err, body)
		}
	}
	if len(heads) != 1 || heads[0].ID != "h-1" || heads[0].Level != 1 || !strings.Contains(body, `<h1 id="h-1">`) {
		t.Errorf("headings = %+v", heads)
	}
	if len(pics) != 1 || pics[0].Name != "images/img-1.png" || pics[0].MIME != "image/png" || !strings.Contains(body, `src="images/img-1.png"/>`) || strings.Contains(body, "data:") {
		t.Errorf("pictures = %+v", pics)
	}
	if strings.Contains(body, "<br>") || strings.Contains(body, "<img alt=\"a dot\" src=\"images/img-1.png\">") {
		t.Error("a void element is not self-closed")
	}
	// The web page is unchanged by the option: no ids, no files, `data:` pictures.
	page := string(writeOK(d))
	if strings.Contains(page, ` id="h-`) || !strings.Contains(page, "data:image/png;base64,") {
		t.Error("the web page took the XHTML mode")
	}
}

func TestNestedListsSitInsideTheirParentItem(t *testing.T) {
	li := func(level int, text string) docmodel.Block {
		return docmodel.Block{Kind: docmodel.ListItem, Level: level, List: 1, Marker: "•", Page: 1, Spans: []docmodel.Span{{Text: text}}}
	}
	for name, blocks := range map[string][]docmodel.Block{
		"1-2-1":         {li(1, "a"), li(2, "b"), li(1, "c")},
		"1-2-3-1":       {li(1, "a"), li(2, "b"), li(3, "c"), li(1, "d")},
		"skips a level": {li(2, "x"), li(1, "y")},
		"opens at 3":    {li(3, "x")},
	} {
		body, _, _ := xhtmlOK(docmodel.Document{Blocks: blocks})
		dec := xml.NewDecoder(strings.NewReader("<r>" + body + "</r>"))
		var stack []string
		items := 0
		for {
			tok, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("%s: %v\n%s", name, err, body)
			}
			switch e := tok.(type) {
			case xml.StartElement:
				if (e.Name.Local == "ul" || e.Name.Local == "ol") && len(stack) > 0 && stack[len(stack)-1] != "li" && stack[len(stack)-1] != "r" {
					t.Errorf("%s: a <%s> is a child of <%s>, not of an <li>:\n%s", name, e.Name.Local, stack[len(stack)-1], body)
				}
				if e.Name.Local == "li" {
					items++
					if len(stack) == 0 || (stack[len(stack)-1] != "ul" && stack[len(stack)-1] != "ol") {
						t.Errorf("%s: an <li> outside a list:\n%s", name, body)
					}
				}
				stack = append(stack, e.Name.Local)
			case xml.EndElement:
				stack = stack[:len(stack)-1]
			}
		}
		if items < len(blocks) {
			t.Errorf("%s: %d <li> for at least %d items", name, items, len(blocks))
		}
	}
	// Every item's text survives, in order.
	body, _, _ := xhtmlOK(docmodel.Document{Blocks: []docmodel.Block{li(1, "a"), li(2, "b"), li(1, "c")}})
	if ia, ib, ic := strings.Index(body, ">a<"), strings.Index(body, "b</li>"), strings.Index(body, "c</li>"); !(ia >= 0 && ia < ib && ib < ic) {
		t.Errorf("items out of order or missing:\n%s", body)
	}
	if page := string(writeOK(docmodel.Document{Blocks: []docmodel.Block{li(1, "a"), li(2, "b")}})); strings.Contains(page, "</ul>\n</ul>") || !strings.Contains(page, "<li>a<ul>") {
		t.Errorf("the web page nests its lists wrongly:\n%s", page)
	}
}

func TestHTMLWithNothingToWriteIsNilAndTakesTheSharedLanguageRule(t *testing.T) {
	if out := writeOK(docmodel.Document{Blocks: []docmodel.Block{{Kind: docmodel.Paragraph, Page: 1}}}); out != nil {
		t.Errorf("a model of one blank paragraph wrote %d bytes, want nil", len(out))
	}
	page := string(writeOK(docmodel.Document{Meta: docmodel.Meta{Lang: "a--b"}, Blocks: []docmodel.Block{{Kind: docmodel.Paragraph, Page: 1, Spans: []docmodel.Span{{Text: "x"}}}}}))
	if !strings.Contains(page, `<html lang="und"`) {
		t.Errorf("a malformed language reached the page:\n%s", page)
	}
}

// The page's policy is exactly the one the ADR names, once, in the head, before anything that could load (/pending 941
// #21): a substring match passed a policy with `script-src *` appended, a second weaker policy, and a meta in the body
// (where a browser ignores it).
func TestTheContentSecurityPolicyIsExactlyTheOneWeMean(t *testing.T) {
	const want = "default-src 'none'; img-src data:; style-src 'unsafe-inline'"
	root, err := html.Parse(bytes.NewReader(writeOK(wtest.Hostile())))
	if err != nil {
		t.Fatal(err)
	}
	var policies []string
	var walk func(x *html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.ElementNode {
			if x.Data == "meta" {
				for _, a := range x.Attr {
					if strings.EqualFold(a.Key, "http-equiv") {
						for _, c := range x.Attr {
							if c.Key == "content" {
								policies = append(policies, c.Val)
							}
						}
					}
				}
			}
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	if len(policies) != 1 || policies[0] != want {
		t.Fatalf("policies = %q, want exactly one: %q", policies, want)
	}
	// Nothing that can fetch (style, link, img, script, base, a second meta) comes before it.
	seen := false
	var order func(x *html.Node)
	order = func(x *html.Node) {
		if x.Type == html.ElementNode && !seen {
			switch x.Data {
			case "style", "link", "img", "script", "base", "title":
				t.Errorf("<%s> comes before the policy", x.Data)
			case "meta":
				for _, a := range x.Attr {
					if strings.EqualFold(a.Key, "http-equiv") {
						seen = true
					}
				}
			}
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			order(c)
		}
	}
	order(root)
}
