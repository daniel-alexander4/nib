package docmodel

import (
	"reflect"
	"testing"
)

var bidiRunes = []rune{0x202a, 0x202b, 0x202c, 0x202d, 0x202e, 0x2066, 0x2067, 0x2068, 0x2069}

// TestBidiControlsCountWhatCleanDrops holds /pending 947's contract at the door: the count of a string is exactly the
// number of runes Clean removes of this class, for each of the nine, and the kept marks count nothing.
func TestBidiControlsCountWhatCleanDrops(t *testing.T) {
	for _, r := range bidiRunes {
		in := "a" + string(r) + "b" + string(r)
		if n := BidiControls(in); n != 2 {
			t.Errorf("U+%04X: counted %d, want 2", r, n)
		}
		if got := Clean(in); got != "ab" {
			t.Errorf("U+%04X: Clean = %q, the count describes a drop that did not happen", r, got)
		}
	}
	for _, r := range []rune{0x200e, 0x200f, 0x061c, 0xfeff, 0x2028} {
		if n := BidiControls("x" + string(r)); n != 0 {
			t.Errorf("U+%04X is not an embedding, override or isolate control but counted %d", r, n)
		}
	}
}

// TestBidiControlsInReadsEveryTextFieldAWriterReads: one control in each counted place is one in the sum.
func TestBidiControlsInReadsEveryTextFieldAWriterReads(t *testing.T) {
	x := "‮"
	d := Document{
		Meta: Meta{Title: x},
		Blocks: []Block{
			{Marker: x, Spans: []Span{{Text: x}, {Text: "a" + x + x}}},
			{Kind: Table, Rows: [][]Cell{{{Paragraphs: [][]Span{{{Text: x}}}}}}},
			{Kind: Figure, Image: &Image{Alt: x}},
		},
	}
	if n := BidiControlsIn(d); n != 7 {
		t.Errorf("BidiControlsIn = %d, want 7 (title, marker, two spans = 3, one cell, one alt)", n)
	}
	if n := BidiControlsIn(Document{Blocks: []Block{{Spans: []Span{{Text: "plain ‏ mark"}}}}}); n != 0 {
		t.Errorf("a plain direction mark counted %d", n)
	}
}

// TestEveryModelStringIsCountedOrNamed is the census: a string field added to the model is either one BidiControlsIn
// counts or a row here with its reason, so a new text field cannot be dropped by a writer's Clean uncounted.
func TestEveryModelStringIsCountedOrNamed(t *testing.T) {
	counted := map[string]bool{"Meta.Title": true, "Block.Marker": true, "Span.Text": true, "Image.Alt": true}
	exempt := map[string]string{
		"Meta.Lang": "a language tag the reader sets", "Meta.Dir": "ltr or rtl", "Block.Kind": "an enum", "Block.Dir": "ltr or rtl",
		"Span.Link": "a destination, judged by xmlw.SafeLink and not by Clean", "Image.Ext": "jpg or png", "Image.Placeholder": "an enum word",
	}
	seen := map[string]bool{}
	var walk func(reflect.Type)
	walk = func(ty reflect.Type) {
		for ty.Kind() == reflect.Slice || ty.Kind() == reflect.Ptr {
			ty = ty.Elem()
		}
		if ty.Kind() != reflect.Struct || ty.Name() == "Report" {
			return
		}
		for i := 0; i < ty.NumField(); i++ {
			f := ty.Field(i)
			name := ty.Name() + "." + f.Name
			ft := f.Type
			for ft.Kind() == reflect.Slice || ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			switch {
			case ft.Kind() == reflect.String:
				seen[name] = true
				if !counted[name] && exempt[name] == "" {
					t.Errorf("%s is a string the model carries that BidiControlsIn does not count and no exemption names", name)
				}
			case ft.Kind() == reflect.Struct:
				walk(ft)
			}
		}
	}
	walk(reflect.TypeOf(Document{}))
	for name := range counted {
		if !seen[name] {
			t.Errorf("%s is counted but is not a string field of the model any more", name)
		}
	}
}
