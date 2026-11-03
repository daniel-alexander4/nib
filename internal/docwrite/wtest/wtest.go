// Package wtest holds the hand-built models the writers' tests share (PLAN-import-export W22): data and one oracle
// helper, no PDF. Committed code, never a binary fixture.
package wtest

import (
	"strings"

	"nib/internal/docmodel"
)

func sp(t string) []docmodel.Span { return []docmodel.Span{{Text: t}} }

// Hostile is a model whose every text value tries to be structure.
func Hostile() docmodel.Document {
	return docmodel.Document{
		Meta: docmodel.Meta{Title: `</title><script>alert(1)</script>`, Lang: `en"><script>`, Dir: "ltr"},
		Blocks: []docmodel.Block{
			{Kind: docmodel.Heading, Level: 1, Page: 1, Spans: sp("# Title with *stars* and <b>tags</b>")},
			{Kind: docmodel.Paragraph, Page: 1, Spans: []docmodel.Span{
				{Text: "plain "}, {Text: "bold", Bold: true}, {Text: " and "}, {Text: "italic", Italic: true},
				{Text: " then a "}, {Text: "link", Link: "https://example.com/a(b)c?x=1&y=2"},
				{Text: " and "}, {Text: "bad", Link: "javascript:alert(1)"},
				{Text: " <script>alert(2)</script> \"onerror=\"x\" [x](javascript:alert(3)) | pipe \\ back`tick"},
			}},
			{Kind: docmodel.Paragraph, Page: 1, Spans: sp("- not a bullet")},
			{Kind: docmodel.Paragraph, Page: 1, Spans: sp("1. not a list")},
			{Kind: docmodel.Paragraph, Page: 1, Spans: sp("ctl\x00\x01\x08chars\u0085 and ￾ gone")},
			{Kind: docmodel.ListItem, Level: 1, List: 1, Marker: "1.", Page: 2, Spans: sp("first")},
			{Kind: docmodel.ListItem, Level: 2, List: 1, Marker: "•", Page: 2, Spans: sp("nested")},
			{Kind: docmodel.ListItem, Level: 1, List: 1, Marker: "2.", Page: 2, Spans: sp("second")},
			{Kind: docmodel.Table, Page: 2, HeaderRows: 1, Rows: [][]docmodel.Cell{
				{{Paragraphs: [][]docmodel.Span{sp("Name")}, ColSpan: 1, RowSpan: 1, Header: true}, {Paragraphs: [][]docmodel.Span{sp("Value | v")}, ColSpan: 1, RowSpan: 1, Header: true}},
				{{Paragraphs: [][]docmodel.Span{sp("a"), sp("a2")}, ColSpan: 1, RowSpan: 1}, {Paragraphs: [][]docmodel.Span{sp("<img src=x onerror=1>")}, ColSpan: 1, RowSpan: 1}},
			}},
			{Kind: docmodel.Figure, Page: 3, Image: &docmodel.Image{Alt: `Scanned page 3 "quoted" <b>`, Placeholder: "mask"}},
			{Kind: docmodel.Figure, Page: 3, Image: &docmodel.Image{Alt: "a dot", Ext: "png", Data: []byte{0x89, 'P', 'N', 'G'}}},
			{Kind: docmodel.Paragraph, Page: 3, Dir: "rtl", Spans: sp("שלום עולם")},
		},
	}
}

// Words is the model's text as the words a reader of the output should find, in order, from the model's own spans —
// the manifest the writers are judged against (D34: a writer that drops a block must not pass).
func Words(d docmodel.Document) []string {
	var w []string
	add := func(s string) { w = append(w, strings.Fields(docmodel.Clean(s))...) }
	for _, b := range d.Blocks {
		for _, s := range b.Spans {
			add(s.Text)
		}
		for _, r := range b.Rows {
			for _, c := range r {
				for _, p := range c.Paragraphs {
					for _, s := range p {
						add(s.Text)
					}
				}
			}
		}
	}
	return w
}
