package html

import (
	"bytes"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"nib/internal/docwrite/wtest"
)

func TestHTMLReadsBackAsTheModelAndHoldsNoScript(t *testing.T) {
	d := wtest.Hostile()
	out := Write(d)
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
