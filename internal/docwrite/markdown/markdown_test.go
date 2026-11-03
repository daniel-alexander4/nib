package markdown

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
	xhtml "golang.org/x/net/html"

	"nib/internal/docmodel"
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

func render(t *testing.T, md []byte) string {
	t.Helper()
	var buf bytes.Buffer
	if err := goldmark.New(goldmark.WithExtensions(extension.GFM)).Convert(md, &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// A list item, a heading and a spanned cell keep their words and their place (phase-close review P02): item text that
// starts like Markdown does not become a nested list or a rule, a trailing "#" is not eaten as a closing sequence, and
// a cell that spans down leaves its column filled in the rows below.
func TestListItemsHeadingsAndSpansKeepWhatThePageSaid(t *testing.T) {
	sp := func(s string) []docmodel.Span { return []docmodel.Span{{Text: s}} }
	cell := func(s string, rs int) docmodel.Cell {
		return docmodel.Cell{Paragraphs: [][]docmodel.Span{sp(s)}, ColSpan: 1, RowSpan: rs}
	}
	d := docmodel.Document{Blocks: []docmodel.Block{
		{Kind: docmodel.Heading, Level: 2, Page: 1, Spans: sp("Appendix #")},
		{Kind: docmodel.ListItem, Level: 1, List: 1, Marker: "-", Page: 1, Spans: sp("- foo")},
		{Kind: docmodel.ListItem, Level: 1, List: 1, Marker: "-", Page: 1, Spans: sp("# bar")},
		{Kind: docmodel.ListItem, Level: 1, List: 1, Marker: "-", Page: 1, Spans: sp("1. baz")},
		{Kind: docmodel.Table, Page: 1, HeaderRows: 1, Rows: [][]docmodel.Cell{
			{cell("H1", 1), cell("H2", 1), cell("H3", 1)},
			{cell("a", 2), cell("b", 1), cell("c", 1)},
			{cell("d", 1), cell("e", 1)},
		}},
	}}
	out := string(Write(d))
	h := render(t, []byte(out))
	for _, want := range []string{"<h2>Appendix #</h2>", "<li>- foo</li>", "<li># bar</li>", "<li>1. baz</li>"} {
		if !strings.Contains(h, want) {
			t.Errorf("missing %q in\n%s\nfrom\n%s", want, h, out)
		}
	}
	if strings.Count(h, "<ul>") != 1 || strings.Count(h, "<ol>") != 0 {
		t.Errorf("an item became a nested list:\n%s", h)
	}
	// Row 3: "a" spans down, so "d" and "e" sit under H2 and H3 — the first column is empty.
	if !strings.Contains(out, "|  | d | e |") {
		t.Errorf("a spanned cell did not hold its column:\n%s", out)
	}
}
