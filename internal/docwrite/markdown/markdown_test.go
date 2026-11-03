package markdown

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
	xhtml "golang.org/x/net/html"

	"nib/internal/docwrite/wtest"
)

// TestMarkdownReadsBackAsTheModel renders the output with goldmark (GFM, raw HTML ALLOWED so a word that became markup
// shows as an element) and judges the HTML it makes: the model's words in order, the one https link, one heading, no
// element a word could have conjured.
func TestMarkdownReadsBackAsTheModel(t *testing.T) {
	d := wtest.Hostile()
	out := Write(d)
	var buf bytes.Buffer
	if err := goldmark.New(goldmark.WithExtensions(extension.GFM), goldmark.WithRendererOptions(html.WithUnsafe())).Convert(out, &buf); err != nil {
		t.Fatal(err)
	}
	root, err := xhtml.Parse(&buf)
	if err != nil {
		t.Fatal(err)
	}
	var words, hrefs []string
	tags := map[string]int{}
	var walk func(n *xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode {
			tags[n.Data]++
			for _, a := range n.Attr {
				if a.Key == "href" {
					hrefs = append(hrefs, a.Val)
				}
			}
		}
		if n.Type == xhtml.TextNode {
			words = append(words, strings.Fields(n.Data)...)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	for _, bad := range []string{"script", "b", "img"} {
		if tags[bad] != 0 {
			t.Errorf("a word became <%s>:\n%s", bad, out)
		}
	}
	if tags["h1"] != 1 || tags["table"] != 1 || tags["br"] != 1 || tags["strong"] != 1 || tags["em"] != 3 || tags["ol"] != 1 || tags["ul"] != 1 {
		t.Errorf("structure = %v\n%s", tags, out)
	}
	if len(hrefs) != 1 || hrefs[0] != "https://example.com/a%28b%29c?x=1&y=2" {
		t.Errorf("hrefs = %v, want only the https link\n%s", hrefs, out)
	}
	i := 0
	for _, w := range wtest.Words(d) {
		for i < len(words) && words[i] != w {
			i++
		}
		if i == len(words) {
			t.Fatalf("word %q missing or out of order:\n%s", w, out)
		}
		i++
	}
	s := string(out)
	for _, want := range []string{"**bold**", "*italic*", "\\- not a bullet", "1\\. not a list", "| Name | Value \\| v |", "| --- | --- |", "1. first", "    - nested", "2. second"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	if strings.ContainsAny(s, "\x00\x01\x08\u0085￾") {
		t.Errorf("a control character survived")
	}
}
