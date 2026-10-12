package uacheck

import (
	"strings"
	"testing"
	"unicode/utf16"

	"nib/internal/pdfops"
)

// **7.15 t1's XML reader refuses what veraPDF's parser refuses, constraint by constraint** (`/pending 673`).
//
// `dynamicRenderIn` reads the config packet with Go's `encoding/xml`, which is not a conforming XML parser:
// it leaves out a good part of XML 1.0's and Namespaces in XML's well-formedness, and veraPDF's parser does
// not. A packet veraPDF cannot parse PASSES the clause, so every omission is a false FAIL — and an encoding
// veraPDF reads and Go does not (UTF-16) is a false PASS. Three reviews had found eleven such shapes by
// guessing; this table walks the productions instead, one row per way of breaking one, with the rows that
// must still be READ beside them (marked `ok`) so that a check cannot be made to pass by refusing everything.
//
// **Every row's `vera` is veraPDF 1.30.2's measured answer** — `F`, it fails the clause, so it read the packet
// and found `required`; `P`, it passes, so its parser refused the packet — and where veraPDF is present the
// table is asked again. `R` marks what nib declares it does not read (XML 1.1, an encoding outside the few
// measured): `CannotCheck`, whichever way veraPDF answers.
//
// Of the 282 rows measured before the fix, 57 were false FAILS and 13 false PASSES.
func TestTheXFAReaderRefusesWhatVeraPDFsParserRefuses(t *testing.T) {
	base, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	const (
		P = 0 // veraPDF passes the clause: its parser refused the packet
		F = 1 // veraPDF fails it: the packet was read
		R = 2 // nib refuses to read it
	)
	onPath := "<config><acrobat><acrobat7><dynamicRender>required</dynamicRender></acrobat7></acrobat></config>"
	// in puts a violation inside the root, BEFORE the value: the whole packet is parsed before any of it is read.
	in := func(v string) string { return "<config>" + v + onPath[len("<config>"):] }
	u16 := func(s string, be, bom bool) string {
		if bom {
			s = string(rune(0xFEFF)) + s
		}
		var b []byte
		for _, c := range utf16.Encode([]rune(s)) {
			if be {
				b = append(b, byte(c>>8), byte(c))
			} else {
				b = append(b, byte(c), byte(c>>8))
			}
		}
		return string(b)
	}
	l1 := "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?>"
	d := func(enc string) string { return "<?xml version=\"1.0\" encoding=\"" + enc + "\"?>" }
	rows := []struct {
		name, packet string
		vera         int
	}{
		{"baseline", in(""), F},
		{"char: raw U+0001 in content", in("<p>\x01</p>"), P},
		{"char: raw U+000B in content", in("<p>\x0b</p>"), P},
		{"char: raw U+007F in content", in("<p>\x7f</p>"), F},
		{"char: raw U+0085 C1 in content", in("<p>\xc2\x85</p>"), F},
		{"char: raw U+FFFE in content", in("<p>\xef\xbf\xbe</p>"), P},
		{"char: raw U+FFFF in content", in("<p>\xef\xbf\xbf</p>"), P},
		{"char: raw surrogate bytes", in("<p>\xed\xa0\x80</p>"), P},
		{"char: invalid UTF-8 byte ff", in("<p>\xff</p>"), P},
		{"char: overlong UTF-8", in("<p>\xc0\xaf</p>"), P},
		{"char: truncated UTF-8 seq", in("<p>\xe2\x82</p>"), P},
		{"char: ref &#1;", in("<p>&#1;</p>"), P},
		{"char: ref &#0;", in("<p>&#0;</p>"), P},
		{"char: ref &#xFFFE;", in("<p>&#xFFFE;</p>"), P},
		{"char: ref &#xD800;", in("<p>&#xD800;</p>"), P},
		{"char: ref &#x110000;", in("<p>&#x110000;</p>"), P},
		{"char: ref &#9; ok", in("<p>&#9;</p>"), F},
		{"char: ref &#X41; capital X", in("<p>&#X41;</p>"), P},
		{"char: ref &#;", in("<p>&#;</p>"), P},
		{"char: ref &#x;", in("<p>&#x;</p>"), P},
		{"char: ref &#12a;", in("<p>&#12a;</p>"), P},
		{"char: U+0001 in attribute", in("<p a=\"\x01\"/>"), P},
		{"char: U+0001 in comment", in("<!--\x01-->"), P},
		{"char: U+0001 in PI", in("<?pi \x01?>"), P},
		{"char: U+0001 in CDATA", in("<p><![CDATA[\x01]]></p>"), P},
		{"char: ref &#1; in attribute", in("<p a=\"&#1;\"/>"), P},
		{"char: NUL raw", in("<p>\x00</p>"), P},
		{"comment: -- inside", in("<!-- a -- b -->"), P},
		{"comment: ends --->", in("<!-- a --->"), P},
		{"comment: ends ->", in("<!-- a -><p/>-->"), F},
		{"comment: unclosed", in("<!-- a "), P},
		{"comment: <!- one dash", in("<!- a -->"), P},
		{"comment: empty <!---->", in("<!---->"), F},
		{"comment: <!--->", in("<!--->"), P},
		{"comment: single dash inside ok", in("<!-- a - b -->"), F},
		{"attr: < in value", in("<p a=\"<\"/>"), P},
		{"attr: > in value ok", in("<p a=\">\"/>"), F},
		{"attr: undeclared entity", in("<p a=\"&nbsp;\"/>"), P},
		{"attr: bare &", in("<p a=\"a&b\"/>"), P},
		{"attr: &amp without ;", in("<p a=\"&amp\"/>"), P},
		{"attr: predefined &lt; ok", in("<p a=\"&lt;\"/>"), F},
		{"attr: no quotes", in("<p a=1/>"), P},
		{"attr: no value", in("<p a/>"), P},
		{"attr: no space between", in("<p a=\"1\"b=\"2\"/>"), P},
		{"attr: mismatched quotes", in("<p a=\"1'/>"), P},
		{"attr: spaces around = ok", in("<p a = \"1\"/>"), F},
		{"attr: single quotes ok", in("<p a='1'/>"), F},
		{"attr: on end tag", in("<p></p a=\"1\">"), P},
		{"attr: name starts with digit", in("<p 1a=\"1\"/>"), P},
		{"attr: empty name", in("<p =\"1\"/>"), P},
		{"content: undeclared entity", in("<p>&nbsp;</p>"), P},
		{"content: bare &", in("<p>a & b</p>"), P},
		{"content: &amp without ;", in("<p>&amp</p>"), P},
		{"content: predefined ok", in("<p>&lt;&gt;&amp;&apos;&quot;</p>"), F},
		{"content: ]]> literal", in("<p>a]]>b</p>"), P},
		{"content: ]]&gt; ok", in("<p>a]]&gt;b</p>"), F},
		{"content: > literal ok", in("<p>a>b</p>"), F},
		{"content: ]] > ok", in("<p>a]] >b</p>"), F},
		{"pi: unclosed", in("<?pi "), P},
		{"pi: space before target", in("<? pi?>"), P},
		{"pi: no target", in("<??>"), P},
		{"pi: target starts with digit", in("<?1pi?>"), P},
		{"pi: target with colon", in("<?a:b x?>"), F},
		{"pi: xml mid-document", in("<?xml version=\"1.0\"?>"), P},
		{"pi: xml-stylesheet ok", in("<?xml-stylesheet x?>"), F},
		{"pi: xmlfoo ok", in("<?xmlfoo x?>"), F},
		{"pi: Xml mid", in("<?Xml x?>"), P},
		{"pi: no space after target", in("<?pi?x?>"), P},
		{"pi: ? inside ok", in("<?pi a?b?>"), F},
		{"tag: end tag other case", in("<p></P>"), P},
		{"tag: name starts with digit", in("<1p/>"), P},
		{"tag: name starts with dash", in("<-p/>"), P},
		{"tag: name starts with dot", in("<.p/>"), P},
		{"tag: space before name", in("< p/>"), P},
		{"tag: space in </ p>", in("<p></ p>"), P},
		{"tag: space before > in end ok", in("<p></p >"), F},
		{"tag: / > split", in("<p/ >"), P},
		{"tag: non-ASCII letter name ok", in("<\xc3\xa9/>"), F},
		{"tag: U+00D7 in name", in("<a\xc3\x97b/>"), P},
		{"tag: middle dot in name ok", in("<a\xc2\xb7b/>"), F},
		{"tag: name with $", in("<a$b/>"), P},
		{"tag: unclosed start", in("<p"), P},
		{"tag: empty <>", in("<>"), P},
		{"tag: </>", in("<p></>"), P},
		{"tag: unclosed element", "<config><p>" + onPath[len("<config>"):], P},
		{"qname: colon only", in("<:/>"), F},
		{"qname: leading colon", in("<:a/>"), F},
		{"qname: trailing colon", in("<a:/>"), P},
		{"qname: two colons bound", in("<a:b:c xmlns:a=\"u:x\"/>"), P},
		{"qname: attr two colons", in("<p xmlns:a=\"u:x\" a:b:c=\"1\"/>"), P},
		{"qname: attr leading colon", in("<p :a=\"1\"/>"), F},
		{"qname: attr trailing colon", in("<p xmlns:a=\"u:x\" a:=\"1\"/>"), P},
		{"qname: attr colon only", in("<p :=\"1\"/>"), F},
		{"qname: xmlns: with empty local", in("<p xmlns:=\"u:x\"/>"), P},
		{"qname: unbound attr prefix", in("<p q:a=\"1\"/>"), P},
		{"qname: prefix declared on a sibling only", in("<p xmlns:q=\"u:x\"/><q:r/>"), P},
		{"ns: xmlns:xmlns declared", in("<p xmlns:xmlns=\"u:x\"/>"), P},
		{"ns: xmlns:xmlns to its own URI", in("<p xmlns:xmlns=\"http://www.w3.org/2000/xmlns/\"/>"), P},
		{"ns: xmlns:xml wrong URI", in("<p xmlns:xml=\"u:x\"/>"), P},
		{"ns: xmlns:xml right URI ok", in("<p xmlns:xml=\"http://www.w3.org/XML/1998/namespace\"/>"), F},
		{"ns: other prefix to xml URI", in("<p xmlns:a=\"http://www.w3.org/XML/1998/namespace\"/>"), P},
		{"ns: other prefix to xmlns URI", in("<p xmlns:a=\"http://www.w3.org/2000/xmlns/\"/>"), P},
		{"ns: default to xml URI", in("<p xmlns=\"http://www.w3.org/XML/1998/namespace\"/>"), P},
		{"ns: default to xmlns URI", in("<p xmlns=\"http://www.w3.org/2000/xmlns/\"/>"), P},
		{"ns: default unbound ok", in("<p xmlns=\"\"/>"), F},
		{"ns: element with xmlns prefix", in("<xmlns:p/>"), P},
		{"ns: element with xml prefix ok", in("<xml:p/>"), F},
		{"ns: attribute xml:lang ok", in("<p xml:lang=\"en\"/>"), F},
		{"ns: attribute xmlns:a twice", in("<p xmlns:a=\"u:x\" xmlns:a=\"u:y\"/>"), P},
		{"ns: xmlns twice", in("<p xmlns=\"u:x\" xmlns=\"u:y\"/>"), P},
		{"ns: element named xmlns", in("<xmlns/>"), F},
		{"ns: element named xmlfoo", in("<xmlfoo/>"), F},
		{"ns: prefix xmlfoo bound", in("<xmlfoo:p xmlns:xmlfoo=\"u:x\"/>"), F},
		{"ns: relative URI ok", in("<p xmlns:a=\"rel\"/>"), F},
		{"root: text after", onPath + "text", P},
		{"root: whitespace after ok", onPath + " \n", F},
		{"root: comment after ok", onPath + "<!--c-->", F},
		{"root: PI after ok", onPath + "<?pi x?>", F},
		{"root: CDATA after", onPath + "<![CDATA[x]]>", P},
		{"root: empty CDATA after", onPath + "<![CDATA[]]>", P},
		{"root: char ref after", onPath + "&#32;", P},
		{"root: entity after", onPath + "&amp;", P},
		{"root: NUL after", onPath + "\x00", P},
		{"root: text before", "text" + onPath, P},
		{"root: whitespace before ok", " \n" + onPath, F},
		{"root: comment before ok", "<!--c-->" + onPath, F},
		{"root: CDATA before", "<![CDATA[x]]>" + onPath, P},
		{"root: char ref before", "&#32;" + onPath, P},
		{"root: end tag first", "</config>" + onPath, P},
		{"root: DOCTYPE after root", onPath + "<!DOCTYPE config>", P},
		{"root: empty document", "", P},
		{"root: whitespace only", "  ", P},
		{"root: comment only", "<!--c-->", P},
		{"decl: no version", "<?xml encoding=\"UTF-8\"?>" + onPath, P},
		{"decl: empty", "<?xml?>" + onPath, P},
		{"decl: version 2.0", "<?xml version=\"2.0\"?>" + onPath, P},
		{"decl: version 1.1", "<?xml version=\"1.1\"?>" + onPath, F | R},
		{"decl: version 1.01", "<?xml version=\"1.01\"?>" + onPath, P},
		{"decl: version empty", "<?xml version=\"\"?>" + onPath, P},
		{"decl: version abc", "<?xml version=\"abc\"?>" + onPath, P},
		{"decl: single quotes ok", "<?xml version='1.0'?>" + onPath, F},
		{"decl: encoding before version", "<?xml encoding=\"UTF-8\" version=\"1.0\"?>" + onPath, P},
		{"decl: standalone maybe", "<?xml version=\"1.0\" standalone=\"maybe\"?>" + onPath, P},
		{"decl: standalone yes ok", "<?xml version=\"1.0\" standalone=\"yes\"?>" + onPath, F},
		{"decl: standalone before encoding", "<?xml version=\"1.0\" standalone=\"yes\" encoding=\"UTF-8\"?>" + onPath, P},
		{"decl: unknown pseudo-attr", "<?xml version=\"1.0\" foo=\"bar\"?>" + onPath, P},
		{"decl: VERSION upper", "<?xml VERSION=\"1.0\"?>" + onPath, P},
		{"decl: no quotes", "<?xml version=1.0?>" + onPath, P},
		{"decl: no space before version", "<?xmlversion=\"1.0\"?>" + onPath, P},
		{"decl: version twice", "<?xml version=\"1.0\" version=\"1.0\"?>" + onPath, P},
		{"decl: encoding empty", "<?xml version=\"1.0\" encoding=\"\"?>" + onPath, P},
		{"decl: encoding bad name", "<?xml version=\"1.0\" encoding=\"8utf\"?>" + onPath, P},
		{"decl: encoding unknown", "<?xml version=\"1.0\" encoding=\"x-nope\"?>" + onPath, P | R},
		{"decl: encoding utf-8 lower", "<?xml version=\"1.0\" encoding=\"utf-8\"?>" + onPath, F},
		{"decl: encoding UTF8 no dash", "<?xml version=\"1.0\" encoding=\"UTF8\"?>" + onPath, F},
		{"decl: encoding US-ASCII plain", "<?xml version=\"1.0\" encoding=\"US-ASCII\"?>" + onPath, F},
		{"decl: encoding US-ASCII with high byte", "<?xml version=\"1.0\" encoding=\"US-ASCII\"?>" + in("<p>\xe9</p>"), P},
		{"decl: encoding latin1 with high byte", "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?>" + in("<p>\xe9</p>"), F},
		{"decl: encoding latin1 C1 byte 0x85", "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?>" + in("<p>\x85</p>"), F},
		{"decl: encoding latin1 byte 0x01", "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?>" + in("<p>\x01</p>"), P},
		{"decl: encoding label latin1", "<?xml version=\"1.0\" encoding=\"latin1\"?>" + onPath, F},
		{"decl: encoding label latin-1", "<?xml version=\"1.0\" encoding=\"latin-1\"?>" + onPath, P | R},
		{"decl: encoding ISO_8859-1", "<?xml version=\"1.0\" encoding=\"ISO_8859-1\"?>" + onPath, F},
		{"decl: encoding windows-1252", "<?xml version=\"1.0\" encoding=\"windows-1252\"?>" + onPath, F | R},
		{"decl: encoding UTF-16 declared, bytes ASCII", "<?xml version=\"1.0\" encoding=\"UTF-16\"?>" + onPath, P | R},
		{"decl: trailing space before ?> ok", "<?xml version=\"1.0\" ?>" + onPath, F},
		{"decl: newline after ok", "<?xml version=\"1.0\"?>\n" + onPath, F},
		{"decl: BOM then decl ok", "\xef\xbb\xbf<?xml version=\"1.0\"?>" + onPath, F},
		{"decl: two BOMs", "\xef\xbb\xbf\xef\xbb\xbf" + onPath, P},
		{"decl: BOM mid-content", in("<p>\xef\xbb\xbf</p>"), F},
		{"enc: UTF-16LE with BOM", u16(onPath, false, true), F},
		{"enc: UTF-16BE with BOM", u16(onPath, true, true), F},
		{"cdata: unclosed", in("<p><![CDATA[x</p>"), P},
		{"cdata: lower case", in("<p><![cdata[x]]></p>"), P},
		{"cdata: nested-looking ok", in("<p><![CDATA[<![CDATA[x]]></p>"), F},
		{"cdata: holds markup ok", in("<p><![CDATA[<a>&nbsp;</b>]]></p>"), F},
		{"directive: <!ELEMENT in content", in("<!ELEMENT p ANY>"), P},
		{"directive: <![INCLUDE[ in content", in("<![INCLUDE[x]]>"), P},
		{"directive: <!x> in content", in("<!x>"), P},
		{"char: invalid UTF-8 in comment", in("<!--\xff-->"), P},
		{"char: invalid UTF-8 in PI", in("<?pi \xff?>"), P},
		{"char: U+FFFE in comment", in("<!--\xef\xbf\xbe-->"), P},
		{"char: U+FFFF in PI", in("<?pi \xef\xbf\xbf?>"), P},
		{"char: surrogate bytes in comment", in("<!--\xed\xa0\x80-->"), P},
		{"char: U+007F in comment ok", in("<!--\x7f-->"), F},
		{"char: tab in comment ok", in("<!--\t-->"), F},
		{"char: astral in comment ok", in("<!--\xf0\x9f\x98\x80-->"), F},
		{"char: astral in content ok", in("<p>\xf0\x9f\x98\x80</p>"), F},
		{"char: ref &#xD800; in attr", in("<p a=\"&#xD800;\"/>"), P},
		{"char: ref &#xDFFF;", in("<p>&#xDFFF;</p>"), P},
		{"char: ref &#55296; decimal surrogate", in("<p>&#55296;</p>"), P},
		{"char: ref &#xFFFF;", in("<p>&#xFFFF;</p>"), P},
		{"char: ref &#x10FFFF; ok", in("<p>&#x10FFFF;</p>"), F},
		{"char: ref &#xE000; ok", in("<p>&#xE000;</p>"), F},
		{"char: ref &#x0000000041; ok", in("<p>&#x0000000041;</p>"), F},
		{"char: ref &#xD800; inside CDATA ok", in("<p><![CDATA[&#xD800;]]></p>"), F},
		{"char: ref &#xD800; inside comment ok", in("<!--&#xD800;-->"), F},
		{"char: ref &#xD800; in the value", strings.Replace(onPath, "required", "required&#xD800;", 1), P},
		{"qname: :a:", in("<:a:/>"), P},
		{"qname: a::b", in("<a::b xmlns:a=\"u:x\"/>"), P},
		{"qname: ::", in("<::/>"), P},
		{"qname: :a:b", in("<:a:b/>"), P},
		{"qname: attr :a:b", in("<p :a:b=\"1\"/>"), P},
		{"qname: attr ::", in("<p ::=\"1\"/>"), P},
		{"qname: end tag for :a", in("<:a></:a>"), F},
		{"ns: xmlns:XML case ok", in("<p xmlns:XML=\"u:x\"/>"), F},
		{"ns: xml:foo attribute ok", in("<p xml:foo=\"1\"/>"), F},
		{"ns: xmlns prefix on root", "<xmlns:config>" + onPath[len("<config>"):len(onPath)-len("</config>")] + "</xmlns:config>", P},
		{"ns: xml URI on root default", "<config xmlns=\"http://www.w3.org/XML/1998/namespace\">" + onPath[len("<config>"):], P},
		{"ns: xmlns:xml right URI plus use", in("<p xmlns:xml=\"http://www.w3.org/XML/1998/namespace\" xml:lang=\"en\"/>"), F},
		{"ns: xmlns:xmlns empty", in("<p xmlns:xmlns=\"\"/>"), P},
		{"ns: xmlns:xml empty", in("<p xmlns:xml=\"\"/>"), P},
		{"ns: URI via char ref equal to xml URI", in("<p xmlns:a=\"http://www.w3.org/XML/1998/namespac&#101;\"/>"), P},
		{"root: ws char ref after decl", "<?xml version=\"1.0\"?>&#x20;" + onPath, P},
		{"root: ws CDATA before", "<![CDATA[ ]]>" + onPath, P},
		{"root: tab char ref after", onPath + "&#9;", P},
		{"tag: no space, single quotes", in("<p a='1'b='2'/>"), P},
		{"tag: tab between attrs ok", in("<p a=\"1\"\tb=\"2\"/>"), F},
		{"tag: newline before attr ok", in("<p\n a=\"1\"\r\n b=\"2\"\n/>"), F},
		{"tag: no space before / ok", in("<p a=\"1\"/>"), F},
		{"tag: value holds quote of other kind ok", in("<p a=\"it's\" b='say \"x\"'/>"), F},
		{"tag: value holds > and = ok", in("<p a=\"x>y=z\" b=\"2\"/>"), F},
		{"tag: no space after value with > inside", in("<p a=\"x>y\"b=\"2\"/>"), P},
		{"tag: no space before xmlns", in("<p a=\"1\"xmlns:q=\"u:x\"/>"), P},
		{"pi: tab after target ok", in("<?pi\tx?>"), F},
		{"pi: newline after target ok", in("<?pi\nx?>"), F},
		{"pi: bare target ok", in("<?pi?>"), F},
		{"pi: target then space only ok", in("<?pi ?>"), F},
		{"pi: no space, = after target", in("<?pi=x?>"), P},
		{"pi: xml-foo no space", in("<?xml-foo=x?>"), P},
		{"pi: target xml:foo", in("<?xml:foo x?>"), F},
		{"decl: standalone 'no' single ok", "<?xml version='1.0' standalone='no'?>" + onPath, F},
		{"decl: spaces around = ok", "<?xml version = \"1.0\" encoding = \"UTF-8\" ?>" + onPath, F},
		{"decl: newlines as separators ok", "<?xml\nversion=\"1.0\"\nencoding=\"UTF-8\"\n?>" + onPath, F},
		{"decl: no space before encoding", "<?xml version=\"1.0\"encoding=\"UTF-8\"?>" + onPath, P},
		{"decl: standalone YES", "<?xml version=\"1.0\" standalone=\"YES\"?>" + onPath, P},
		{"decl: mismatched quotes", "<?xml version=\"1.0'?>" + onPath, P},
		{"decl: version 1.0 with space", "<?xml version=\" 1.0\"?>" + onPath, P},
		{"decl: version 1.2", "<?xml version=\"1.2\"?>" + onPath, P},
		{"decl: version 1.10", "<?xml version=\"1.10\"?>" + onPath, P},
		{"decl: just <?xml ?>", "<?xml ?>" + onPath, P},
		{"decl: tab-led <?xml\\t", "<?xml\tversion=\"1.0\"?>" + onPath, F},
		{"decl: encoding with space inside", "<?xml version=\"1.0\" encoding=\"UTF 8\"?>" + onPath, P},
		{"decl: encoding UTF-8 upper mixed Utf-8", "<?xml version=\"1.0\" encoding=\"Utf-8\"?>" + onPath, F},
		{"decl: encoding ASCII", d("ASCII") + onPath, F},
		{"decl: encoding ascii with high byte", d("ascii") + in("<p>\xe9</p>"), P},
		{"decl: encoding l1", d("l1") + onPath, F},
		{"decl: encoding ISO8859_1", d("ISO8859_1") + onPath, F},
		{"decl: encoding cp819", d("cp819") + onPath, F},
		{"decl: encoding ISO-8859-1 lower", d("iso-8859-1") + onPath, F},
		{"decl: encoding UTF-8 with latin1 byte", d("UTF-8") + in("<p>\xe9</p>"), P},
		{"decl: latin1, high byte, then split CDATA value", l1 + "<config><p>\xe9</p><acrobat><acrobat7><dynamicRender>requi<![CDATA[red]]></dynamicRender></acrobat7></acrobat></config>", P},
		{"decl: latin1, high byte, then CDATA value", l1 + "<config><p>\xe9</p><acrobat><acrobat7><dynamicRender><![CDATA[required]]></dynamicRender></acrobat7></acrobat></config>", F},
		{"decl: latin1, high byte in comment", l1 + in("<!--\xe9-->"), F},
		{"decl: latin1, C1 0x81 in comment", l1 + in("<!--\x81-->"), F},
		{"decl: latin1, 0x01 in comment", l1 + in("<!--\x01-->"), P},
		{"decl: UTF-8 BOM, declared latin1", "\xef\xbb\xbf" + l1 + onPath, F},
		{"decl: UTF-8 BOM, declared UTF-16", "\xef\xbb\xbf" + d("UTF-16") + onPath, P | R},
		{"decl: UTF-8 BOM, declared US-ASCII", "\xef\xbb\xbf" + d("US-ASCII") + onPath, F},
		{"enc: UTF-16LE no BOM no decl", u16(onPath, false, false), P},
		{"enc: UTF-16BE no BOM no decl", u16(onPath, true, false), P},
		{"enc: UTF-16LE no BOM, decl UTF-16", u16(d("UTF-16")+onPath, false, false), F},
		{"enc: UTF-16BE no BOM, decl UTF-16", u16(d("UTF-16")+onPath, true, false), F},
		{"enc: UTF-16LE no BOM, decl UTF-16LE", u16(d("UTF-16LE")+onPath, false, false), F},
		{"enc: UTF-16BE no BOM, decl UTF-16BE", u16(d("UTF-16BE")+onPath, true, false), F},
		{"enc: UTF-16LE no BOM, plain decl", u16("<?xml version=\"1.0\"?>"+onPath, false, false), F},
		{"enc: UTF-16LE BOM, decl UTF-16", u16(d("UTF-16")+onPath, false, true), F},
		{"enc: UTF-16BE BOM, decl UTF-16", u16(d("UTF-16")+onPath, true, true), F},
		{"enc: UTF-16LE BOM, decl utf-16 lower", u16(d("utf-16")+onPath, false, true), F},
		{"enc: UTF-16LE BOM, decl UTF-16LE", u16(d("UTF-16LE")+onPath, false, true), F},
		{"enc: UTF-16LE BOM, decl UTF-16BE", u16(d("UTF-16BE")+onPath, false, true), P},
		{"enc: UTF-16LE BOM, decl UTF-8", u16(d("UTF-8")+onPath, false, true), P},
		{"enc: UTF-16LE BOM, decl ISO-8859-1", u16(d("ISO-8859-1")+onPath, false, true), P},
		{"enc: UTF-16LE BOM, plain decl", u16("<?xml version=\"1.0\"?>"+onPath, false, true), F},
		{"enc: UTF-16LE BOM, odd trailing byte", u16(onPath, false, true) + "\x20", P},
		{"enc: UTF-16LE BOM, lone high surrogate in p", u16(in("<p>X</p>"), false, true)[:0] + strings.Replace(u16(in("<p>\u4e00</p>"), false, true), "\x00\x4e", "\x00\xd8", 1), P},
		{"enc: UTF-16LE BOM, astral in p ok", u16(in("<p>\U0001F600</p>"), false, true), F},
		{"enc: UTF-16LE BOM, U+FFFE in p", u16(in("<p>X</p>"), false, true)[:0] + strings.Replace(u16(in("<p>\u4e00</p>"), false, true), "\x00\x4e", "\xfe\xff", 1), P},
		{"enc: UTF-16LE BOM, split CDATA value", u16(strings.Replace(onPath, "required", "requi<![CDATA[red]]>", 1), false, true), P},
		{"enc: UTF-16LE BOM, non-ASCII value", u16(strings.Replace(onPath, "required", "requir\u00e9d", 1), false, true), P},
		{"enc: UTF-32LE BOM", "\xff\xfe\x00\x00<\x00\x00\x00c\x00\x00\x00/\x00\x00\x00>\x00\x00\x00", P},
		{"enc: UTF-16 BOM only", "\xff\xfe", P},
		{"root: no-break space after", onPath + "\xc2\xa0", P},
		{"root: U+0085 after", onPath + "\xc2\x85", P},
		{"root: U+2028 after", onPath + "\xe2\x80\xa8", P},
		{"root: bad character in a comment after", onPath + "<!--\x01-->", P},
		{"char: invalid UTF-8 in element name", in("<p\xff/>"), P},
		{"char: invalid UTF-8 in attribute value", in("<p a=\"\xff\"/>"), P},
		{"char: ref-like text in PI ok", in("<?pi &#xD800;?>"), F},
		{"enc: UTF-16LE no BOM, decl UTF-8", u16(d("UTF-8")+onPath, false, false), P},
		{"enc: UTF-16LE no BOM, decl UTF-16BE", u16(d("UTF-16BE")+onPath, false, false), P},
		{"enc: UTF-16BE no BOM, decl UTF-16LE", u16(d("UTF-16LE")+onPath, true, false), P},
		{"enc: UTF-16LE no BOM, opens with another PI", u16("<?pi x?>"+onPath, false, false), F},
		{"enc: UTF-16BE BOM, decl UTF-16BE", u16(d("UTF-16BE")+onPath, true, true), F},
		{"enc: UTF-16BE BOM, decl UTF-16LE", u16(d("UTF-16LE")+onPath, true, true), P},
		{"enc: UTF-16BE BOM, astral in p ok", u16(in("<p>\U0001F600</p>"), true, true), F},
		{"enc: UTF-16LE BOM, decl UCS-2", u16(d("ISO-10646-UCS-2")+onPath, false, true), F | R},
		{"decl: UTF-8 BOM, declared latin1, high byte", "\xef\xbb\xbf" + l1 + in("<p>\xc3\xa9</p>"), F | R},
		{"decl: encoding UTF8 with UTF-8 text", d("UTF8") + in("<p>\xc3\xa9</p>"), F},
		{"decl: encoding cp819 with high byte", d("cp819") + in("<p>\xe9</p>"), F},
		// Valid UTF-8, so only the declared US-ASCII refuses it: the row above it in kind would pass on Go's own
		// refusal of a stray high byte (a mutation that read the packet as written left the table green).
		{"decl: encoding US-ASCII with UTF-8 text", d("US-ASCII") + in("<p>\xc3\xa9</p>"), P},
		{"decl: version 1.1 with a C1 control", "<?xml version=\"1.1\"?>" + in("<p>\xc2\x85</p>"), F | R},
	}
	var docs [][]byte
	read := 0
	for _, r := range rows {
		doc := withXFAConfig(t, base, r.packet)
		docs = append(docs, doc)
		want := Pass
		switch {
		case r.vera&R != 0:
			want = CannotCheck
		case r.vera&F != 0:
			want = Fail
			read++
		}
		if got := verdictOf(t, doc, "7.15 t1"); got.Verdict != want {
			t.Errorf("%s: config packet %q reports %v (%s), want %v", r.name, r.packet, got.Verdict, got.Why, want)
		}
	}
	// A table of refusals alone would pass a reader that refuses everything.
	if read < 80 {
		t.Errorf("only %d rows are packets the reader must READ, so the table no longer holds it to both sides", read)
	}
	vera := veraAsk(t, docs)
	for i, r := range rows {
		if vera == nil {
			break
		}
		if vera[i] == nil {
			t.Errorf("%s: veraPDF gave no report for the document", r.name)
			continue
		}
		if got := vera[i]["7.15 t1"] == "failed"; got != (r.vera&F != 0) {
			t.Errorf("%s: veraPDF now answers %q for config packet %q, and the row records the other answer",
				r.name, vera[i]["7.15 t1"], r.packet)
		}
	}
	if !strings.Contains(onPath, "required") {
		t.Fatal("setup: the packets no longer carry the value the clause fails on")
	}
}
