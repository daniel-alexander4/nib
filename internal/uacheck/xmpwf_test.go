package uacheck

import (
	"strings"
	"testing"
)

// **A packet veraPDF's XML parser refuses is not read** (`/pending 673`).
//
// veraPDF reads no metadata packet it cannot parse — `5 t1` and `7.1 t9` fail, whatever the packet holds — and
// Go's `encoding/xml`, which nib reads it with, accepts a good deal that is not XML. Every row here was asked of
// veraPDF 1.30.2 and of nib together, in the positions a packet has, and the letter beside it is what was
// measured: `r`, both read the packet; `x`, veraPDF reads no packet, so nib must answer `CannotCheck`; `c`,
// veraPDF reads it and nib declares that it does not (a control character veraPDF blanks before parsing, an
// encoding or an XML version Go's decoder does not know). Before the fix nib PASSED 115 of the `x` rows.
//
// The rows are the ones `7.15 t1`'s XFA reader was asked (`TestTheXFAReaderRefusesWhatVeraPDFsParserRefuses`),
// and the answers differ: the two readers are two parsers. Where veraPDF is present it is asked again.
func TestAPacketVeraPDFsXMLParserRefusesIsNotRead(t *testing.T) {
	base := plainDoc(t)
	tp := rdfAlt("dc:title", rdfItem("en", "T")) + `<pdfuaid:part>1</pdfuaid:part>`
	ctl := rdfPacket("", tp)
	type row struct {
		name, packet string
		want         byte
	}
	rows := []row{{"control", ctl, 'r'}}
	rep := func(old, new string) string {
		if !strings.Contains(ctl, old) {
			t.Fatalf("setup: %q not in the control", old)
		}
		return strings.Replace(ctl, old, new, 1)
	}
	places := []struct {
		name string
		put  func(v string) string
	}{
		{"before xpacket begin", func(v string) string { return v + ctl }},
		{"before x:xmpmeta", func(v string) string { return rep(`<x:xmpmeta `, v+`<x:xmpmeta `) }},
		{"in x:xmpmeta", func(v string) string { return rep(`</x:xmpmeta>`, v+`</x:xmpmeta>`) }},
		{"in rdf:RDF", func(v string) string { return rep(`</rdf:RDF>`, v+`</rdf:RDF>`) }},
		{"in the description", func(v string) string { return rdfPacket("", tp+v) }},
		{"in a property's text", func(v string) string { return rdfPacket("", tp+`<xmp:Foo>a`+v+`b</xmp:Foo>`) }},
		{"in pdfuaid:part", func(v string) string { return rep(`<pdfuaid:part>1`, `<pdfuaid:part>1`+v) }},
		{"in the title item", func(v string) string { return rep(`>T</rdf:li>`, `>T`+v+`</rdf:li>`) }},
		{"in the Alt", func(v string) string { return rep(`</rdf:Alt>`, v+`</rdf:Alt>`) }},
		{"after x:xmpmeta", func(v string) string { return rep(`</x:xmpmeta>`, `</x:xmpmeta>`+v) }},
		{"after xpacket end", func(v string) string { return ctl + v }},
	}
	things := []struct{ name, v, want string }{
		{"a PI", "<?pi x?>", "rrrxxxxxxrr"},
		{"a bare PI", "<?pi?>", "rrrxxxxxxrr"},
		{"a comment", "<!--c-->", "rrrrrrrrrrr"},
		{"white space", " \n", "rrrrrrrrrrr"},
		{"text", "junk", "xxrxxrrrxxx"},
		{"a space reference", "&#32;", "xxrrrrrrrxx"},
		{"an empty CDATA", "<![CDATA[]]>", "xxrxxxxxxxx"},
		{"a blank CDATA", "<![CDATA[ ]]>", "xxrxxxxxxxx"},
		{"a directive", "<!x>", "xxxxxxxxxxx"},
		{"a DOCTYPE", "<!DOCTYPE x>", "rrxxxxxxxxx"},
		{"a no-break space", "\xc2\xa0", "xxrxxrrrxxx"},
		{"a comment holding U+0001", "<!--\x01-->", "rrrrrrrrrrr"},
		{"a comment holding U+FFFE", "<!--\xef\xbf\xbe-->", "xxxxxxxxxxx"},
		{"a comment holding U+FFFF", "<!--\xef\xbf\xbf-->", "xxxxxxxxxxx"},
		{"a comment holding byte ff", "<!--\xff-->", "rrrrrrrrrrr"},
		{"a comment holding --", "<!-- a -- b -->", "xxxxxxxxxxx"},
	}
	for _, th := range things {
		for i, p := range places {
			rows = append(rows, row{th.name + " " + p.name, p.put(th.v), th.want[i]})
		}
	}
	prop := func(v string) string { return rdfPacket("", tp+`<xmp:Foo>`+v+`</xmp:Foo>`) }
	for _, c := range []struct{ name, v, want string }{
		{"U+0001", "\x01", "ccc"}, {"U+0008", "\x08", "ccc"}, {"U+000B", "\x0b", "ccc"}, {"U+001F", "\x1f", "ccc"}, {"U+007F", "\x7f", "rrr"}, {"U+0085", "\xc2\x85", "rrr"},
		{"U+FFFE", "\xef\xbf\xbe", "xxx"}, {"U+FFFF", "\xef\xbf\xbf", "xxx"}, {"byte ff", "\xff", "ccc"}, {"surrogate bytes", "\xed\xa0\x80", "ccc"}, {"NUL", "\x00", "ccc"},
		{"&#1;", "&#1;", "ccc"}, {"&#xD800;", "&#xD800;", "xxx"}, {"&#xFFFE;", "&#xFFFE;", "xxx"}, {"&#9;", "&#9;", "rrr"}, {"&nbsp;", "&nbsp;", "xxx"}, {"]]>", "]]>", "xrx"}, {"astral", "\xf0\x9f\x98\x80", "rrr"},
	} {
		rows = append(rows, row{"text: " + c.name, prop("a" + c.v + "b"), c.want[0]})
		rows = append(rows, row{"attribute: " + c.name, rdfPacket("", tp+`<xmp:Foo foo:a="a`+c.v+`b"/>`), c.want[1]})
		rows = append(rows, row{"part value: " + c.name, rep(`<pdfuaid:part>1`, `<pdfuaid:part>1`+c.v), c.want[2]})
	}
	for _, c := range []struct{ name, tag, want string }{
		{"ok plain", `<xmp:Foo>v</xmp:Foo>`, "r"},
		{"default to xml URI", `<xmp:Foo xmlns="http://www.w3.org/XML/1998/namespace">v</xmp:Foo>`, "x"},
		{"default to xmlns URI", `<xmp:Foo xmlns="http://www.w3.org/2000/xmlns/">v</xmp:Foo>`, "x"},
		{"default to another URI ok", `<xmp:Foo xmlns="u:x">v</xmp:Foo>`, "r"},
		{"xmlns:xmlns", `<xmp:Foo xmlns:xmlns="u:x">v</xmp:Foo>`, "x"},
		{"xmlns:xml wrong", `<xmp:Foo xmlns:xml="u:x">v</xmp:Foo>`, "x"},
		{"xmlns:xml right", `<xmp:Foo xmlns:xml="http://www.w3.org/XML/1998/namespace">v</xmp:Foo>`, "r"},
		{"prefix to xml URI", `<xmp:Foo xmlns:q="http://www.w3.org/XML/1998/namespace">v</xmp:Foo>`, "x"},
		{"prefix to xmlns URI", `<xmp:Foo xmlns:q="http://www.w3.org/2000/xmlns/">v</xmp:Foo>`, "x"},
		{"prefix to nothing", `<xmp:Foo xmlns:q="">v</xmp:Foo>`, "x"},
		{"attributes unspaced", `<xmp:Foo foo:a="1"foo:b="2"/>`, "x"},
		{"attributes spaced ok", `<xmp:Foo foo:a="1" foo:b="2"/>`, "r"},
		{"unbound prefix", `<zz:Foo>v</zz:Foo>`, "x"},
		{"unbound attribute prefix", `<xmp:Foo zz:a="1"/>`, "x"},
		{"trailing-colon attribute", `<xmp:Foo foo:="1"/>`, "x"},
		{"xmlns-prefixed element", `<xmlns:Foo>v</xmlns:Foo>`, "x"},
		{"end tag other case", `<xmp:Foo>v</xmp:foo>`, "x"},
		{"< in attribute", `<xmp:Foo foo:a="<"/>`, "x"},
		{"empty property ok", `<xmp:Foo/>`, "r"},
	} {
		rows = append(rows, row{"tag: " + c.name, rdfPacket("", tp+c.tag), c.want[0]})
	}
	for _, c := range []struct{ name, decl, want string }{
		{"plain ok", `<?xml version="1.0"?>`, "r"},
		{"encoding UTF-8 ok", `<?xml version="1.0" encoding="UTF-8"?>`, "r"},
		{"no version", `<?xml encoding="UTF-8"?>`, "x"},
		{"empty", `<?xml?>`, "x"},
		{"just a space", `<?xml ?>`, "x"},
		{"version empty", `<?xml version=""?>`, "x"},
		{"version 1.1", `<?xml version="1.1"?>`, "c"},
		{"version 2.0", `<?xml version="2.0"?>`, "x"},
		{"encoding first", `<?xml encoding="UTF-8" version="1.0"?>`, "x"},
		{"standalone maybe", `<?xml version="1.0" standalone="maybe"?>`, "x"},
		{"standalone before encoding", `<?xml version="1.0" standalone="yes" encoding="UTF-8"?>`, "x"},
		{"unknown pseudo-attribute", `<?xml version="1.0" foo="bar"?>`, "x"},
		{"VERSION", `<?xml VERSION="1.0"?>`, "x"},
		{"no quotes", `<?xml version=1.0?>`, "x"},
		{"version twice", `<?xml version="1.0" version="1.0"?>`, "x"},
		{"no space before encoding", `<?xml version="1.0"encoding="UTF-8"?>`, "x"},
		{"mismatched quotes", `<?xml version="1.0'?>`, "x"},
		{"encoding empty", `<?xml version="1.0" encoding=""?>`, "r"},
		{"encoding bad name", `<?xml version="1.0" encoding="8utf"?>`, "c"},
		{"encoding unknown", `<?xml version="1.0" encoding="x-nope"?>`, "c"},
		{"encoding ISO-8859-1", `<?xml version="1.0" encoding="ISO-8859-1"?>`, "c"},
		{"encoding UTF-16 over ASCII", `<?xml version="1.0" encoding="UTF-16"?>`, "c"},
		{"XML upper", `<?XML version="1.0"?>`, "x"},
		{"after a space", ` <?xml version="1.0"?>`, "x"},
		{"after a UTF-8 BOM ok", "\xef\xbb\xbf" + `<?xml version="1.0"?>`, "r"},
		{"two BOMs", "\xef\xbb\xbf\xef\xbb\xbf", "x"},
		{"one BOM, no declaration", "\xef\xbb\xbf", "r"},
	} {
		rows = append(rows, row{"decl: " + c.name, c.decl + ctl, c.want[0]})
	}
	var docs [][]byte
	counts := map[byte]int{}
	for _, r := range rows {
		doc := withRawPacket(t, base, r.packet)
		docs = append(docs, doc)
		counts[r.want]++
		for _, clause := range []string{"5 t1", "7.1 t9"} {
			got := verdictOf(t, doc, clause)
			switch {
			case r.want == 'r' && got.Verdict != Pass:
				t.Errorf("%s: %s reports %v (%s), want Pass — veraPDF reads this packet", r.name, clause, got.Verdict, got.Why)
			case r.want != 'r' && got.Verdict != CannotCheck:
				t.Errorf("%s: %s reports %v (%s), want CannotCheck — packet %q", r.name, clause, got.Verdict, got.Why, r.packet)
			case r.want == 'x' && !strings.Contains(got.Why, "reads no packet there") && !strings.Contains(got.Why, "not well-formed XML"):
				t.Errorf("%s: %s is refused for %q, which does not say the packet is unread", r.name, clause, got.Why)
			}
		}
	}
	// Both sides are held: a reader that refused everything would pass a table of refusals.
	if counts['r'] < 80 || counts['x'] < 100 {
		t.Errorf("the table holds %d packets to read and %d to refuse, so it no longer holds the reader to both", counts['r'], counts['x'])
	}
	vera := veraAsk(t, docs)
	for i, r := range rows {
		if vera == nil {
			break
		}
		for _, clause := range []string{"5 t1", "7.1 t9"} {
			want := "passed"
			if r.want == 'x' {
				want = "failed"
			}
			if vera[i] == nil || vera[i][clause] != want {
				t.Errorf("%s: veraPDF now says %q for %s where %q was measured — the row is stale", r.name, vera[i][clause], clause, want)
			}
		}
	}
}
