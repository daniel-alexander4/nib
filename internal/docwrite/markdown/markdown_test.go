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
	out := writeOK(d)
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
	for _, want := range []string{"**bold**", "*italic*", "\\- not a bullet", "1\\. not a list", "| Name | Value \\| v |", "| --- | --- |", "1. first", "   - nested", "2. second"} {
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
	out := string(writeOK(d))
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

// readHTML is the text nodes of rendered HTML joined, with its element counts and every href.
func readHTML(t *testing.T, h string) (text string, tags map[string]int, hrefs []string) {
	t.Helper()
	root, err := xhtml.Parse(strings.NewReader(h))
	if err != nil {
		t.Fatal(err)
	}
	tags = map[string]int{}
	var b strings.Builder
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
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return b.String(), tags, hrefs
}

// Emphasis is written where CommonMark reads it and nowhere else (/pending 941 #16): a style around punctuation that
// touches a word is not a delimiter pair, and a pair of literal asterisks in the reader's text is the failure.
func TestEmphasisIsWrittenOnlyWhereMarkdownReadsIt(t *testing.T) {
	b := func(s string) docmodel.Span { return docmodel.Span{Text: s, Bold: true} }
	i := func(s string) docmodel.Span { return docmodel.Span{Text: s, Italic: true} }
	p := func(s string) docmodel.Span { return docmodel.Span{Text: s} }
	for _, c := range []struct {
		name   string
		sp     []docmodel.Span
		strong int
		em     int
	}{
		{"a word between spaces", []docmodel.Span{p("a "), b("bold"), p(" b")}, 1, 0},
		{"closing punctuation then a word", []docmodel.Span{p("say "), b("\u201chi.\u201d"), p("x")}, 0, 0},
		{"opening punctuation after a word", []docmodel.Span{p("x"), b("(hi)"), p(" y")}, 0, 0},
		{"punctuation inside, a space outside", []docmodel.Span{p("say "), b("\u201chi.\u201d"), p(" x")}, 1, 0},
		{"a word then punctuation", []docmodel.Span{b("foo"), p(". next")}, 1, 0},
		{"two styles touching", []docmodel.Span{b("a"), i("b"), p(" c")}, 1, 0},
		{"one style split across runs", []docmodel.Span{b("fo"), b("o"), p(" c")}, 1, 0},
		{"one style split after punctuation", []docmodel.Span{b("a."), b("b"), p(" c")}, 1, 0},
		{"italic in a word", []docmodel.Span{p("un"), i("believ"), p("able")}, 0, 1},
	} {
		out := writeOK(docmodel.Document{Blocks: []docmodel.Block{{Kind: docmodel.Paragraph, Page: 1, Spans: c.sp}}})
		text, tags, _ := readHTML(t, render(t, out))
		if strings.Contains(text, "*") {
			t.Errorf("%s: a literal asterisk reached the reader's text: %q\nfrom %q", c.name, text, out)
		}
		var want strings.Builder
		for _, s := range c.sp {
			want.WriteString(s.Text)
		}
		if strings.Join(strings.Fields(text), "") != strings.Join(strings.Fields(want.String()), "") {
			t.Errorf("%s: text %q, want %q", c.name, text, want.String())
		}
		if tags["strong"] != c.strong || tags["em"] != c.em {
			t.Errorf("%s: %d strong, %d em; want %d, %d\nfrom %q", c.name, tags["strong"], tags["em"], c.strong, c.em, out)
		}
	}
}

// A nested item is indented by its parent's content offset (/pending 941 #17), not by four: under "100." four spaces do not nest.
func TestANestedItemIsIndentedByItsParentsMarker(t *testing.T) {
	item := func(level int, marker, s string) docmodel.Block {
		return docmodel.Block{Kind: docmodel.ListItem, List: 1, Level: level, Marker: marker, Page: 1, Spans: []docmodel.Span{{Text: s}}}
	}
	d := docmodel.Document{Blocks: []docmodel.Block{
		item(1, "100.", "outer"), item(2, "-", "inner"), item(3, "1.", "deep"), item(1, "101.", "next"),
	}}
	out := writeOK(d)
	h := render(t, out)
	_, tags, _ := readHTML(t, h)
	if tags["ol"] != 2 || tags["ul"] != 1 || strings.Count(h, "<li>") != 4 {
		t.Errorf("structure %v:\n%s\nfrom\n%s", tags, h, out)
	}
	if !strings.Contains(h, "<li>outer\n<ul>\n<li>inner\n<ol") {
		t.Errorf("the inner items did not nest under the outer one:\n%s\nfrom\n%s", h, out)
	}
	if strings.Contains(h, "<pre>") {
		t.Errorf("an indented item became a code block:\n%s", h)
	}
}

// Two lists in a row are two lists (/pending 941 #18): one marker would merge them into one.
func TestAdjacentListsStayApart(t *testing.T) {
	item := func(list int, marker, s string) docmodel.Block {
		return docmodel.Block{Kind: docmodel.ListItem, List: list, Level: 1, Marker: marker, Page: 1, Spans: []docmodel.Span{{Text: s}}}
	}
	for name, c := range map[string]struct {
		d    docmodel.Document
		want int
	}{
		"bullets": {docmodel.Document{Blocks: []docmodel.Block{item(1, "\u2022", "a"), item(1, "\u2022", "b"), item(2, "\u2022", "c"), item(2, "\u2022", "d")}}, 2},
		"ordered": {docmodel.Document{Blocks: []docmodel.Block{item(1, "1.", "a"), item(1, "2.", "b"), item(2, "1.", "c"), item(2, "2.", "d")}}, 2},
		"three":   {docmodel.Document{Blocks: []docmodel.Block{item(1, "-", "a"), item(2, "-", "b"), item(3, "-", "c")}}, 3},
	} {
		out := writeOK(c.d)
		h := render(t, out)
		_, tags, _ := readHTML(t, h)
		if tags["ul"]+tags["ol"] != c.want {
			t.Errorf("%s: %d lists, want %d:\n%s\nfrom\n%s", name, tags["ul"]+tags["ol"], c.want, h, out)
		}
	}
	two := string(writeOK(docmodel.Document{Blocks: []docmodel.Block{item(1, "-", "a"), item(2, "-", "b")}}))
	if two != "- a\n\n* b\n" {
		t.Errorf("two lists are parted by a blank line and a second marker: %q", two)
	}
	out := writeOK(docmodel.Document{Blocks: []docmodel.Block{item(1, "-", "a"), item(1, "-", "b")}})
	if _, tags, _ := readHTML(t, render(t, out)); tags["ul"] != 1 {
		t.Errorf("one list became %d", tags["ul"])
	}
}

// A destination reads character references (/pending 941 #19): the address that was in the PDF is the one the link opens.
func TestALinkDestinationKeepsTheAddressItWasGiven(t *testing.T) {
	for _, u := range []string{
		"https://example.com/?a=1&amp;b=2",
		"https://example.com/?x=1&copy;=3&y=2",
		"https://example.com/?q=&#65;&#x41;",
		"https://example.com/a?x=1&y=2",
		"https://example.com/&notanentity",
	} {
		out := writeOK(docmodel.Document{Blocks: []docmodel.Block{{Kind: docmodel.Paragraph, Page: 1, Spans: []docmodel.Span{{Text: "go", Link: u}}}}})
		_, _, hrefs := readHTML(t, render(t, out))
		if len(hrefs) != 1 || hrefs[0] != u {
			t.Errorf("link %q came back as %v\nfrom %q", u, hrefs, out)
		}
	}
}

// A marker CommonMark would not read as a number is not written as one: ten digits are a paragraph, not a list.
func TestAMarkerThatIsNotANumberIsABullet(t *testing.T) {
	for _, m := range []string{"1234567890.", "1a."} {
		out := writeOK(docmodel.Document{Blocks: []docmodel.Block{{Kind: docmodel.ListItem, List: 1, Level: 1, Marker: m, Page: 1, Spans: []docmodel.Span{{Text: "x"}}}}})
		if string(out) != "- x\n" {
			t.Errorf("marker %q written as %q", m, out)
		}
	}
}

// Runs of one style are one span: a PDF splits a styled word into runs, and each would otherwise stand alone.
func TestRunsOfOneStyleAreOneSpan(t *testing.T) {
	b := func(s string) docmodel.Span { return docmodel.Span{Text: s, Bold: true} }
	if got := spans([]docmodel.Span{b("fo"), b("o"), {Text: " c"}}); got != "**foo** c" {
		t.Errorf("spans = %q, want %q", got, "**foo** c")
	}
}
