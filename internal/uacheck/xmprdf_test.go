package uacheck

import (
	"strings"
	"testing"
)

// The packet reader against veraPDF's — `/pending 654`. Every row below was asked of veraPDF 1.30.2 before the
// reader was touched, and is asked again here wherever veraPDF is present.

const (
	rdfHead = `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?><x:xmpmeta xmlns:x="adobe:ns:meta/">` +
		`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">`
	rdfTail = `</rdf:RDF></x:xmpmeta><?xpacket end="w"?>`
	rdfNS   = ` xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:xmp="http://ns.adobe.com/xap/1.0/"` +
		` xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/" xmlns:foo="http://example.com/foo/"`
)

// rdfDescription is one top-level description carrying attrs and holding body.
func rdfDescription(about, attrs, body string) string {
	return `<rdf:Description rdf:about="` + about + `"` + rdfNS + attrs + `>` + body + `</rdf:Description>`
}

// rdfPacket is a packet of one description.
func rdfPacket(attrs, body string) string {
	return rdfHead + rdfDescription("", attrs, body) + rdfTail
}

func rdfItem(lang, text string) string {
	if lang == "-" {
		return `<rdf:li>` + text + `</rdf:li>`
	}
	return `<rdf:li xml:lang="` + lang + `">` + text + `</rdf:li>`
}

func rdfAlt(prop string, items ...string) string {
	return `<` + prop + `><rdf:Alt>` + strings.Join(items, "") + `</rdf:Alt></` + prop + `>`
}

// TestWhichAlternativeIsTheLanguageClausesSubject — 7.2 t33 on a document whose catalog declares no /Lang.
//
// Items 1 and 3 of the entry. Item 1's own packet (an Alt whose first item holds another Alt) FAILED here and has no
// subject in veraPDF, and grouping the items rightly was not enough to agree: the measurement is that a nested Alt
// is never the subject, and that one item with no language takes the subject away. Item 3 (a `dc:` property written
// as an attribute) is overturned — veraPDF has no subject there either, and nib already said so; its rows stay.
func TestWhichAlternativeIsTheLanguageClausesSubject(t *testing.T) {
	base := plainDoc(t)
	d := "dc:description"
	nested := func(lang string) string { return `<rdf:Alt>` + rdfItem(lang, "x") + `</rdf:Alt>` }
	cases := []struct {
		name, packet string
		want         Verdict
	}{
		// What decides the answer once there is a subject.
		{"[x-default]", rdfPacket("", rdfAlt(d, rdfItem("x-default", "a"))), Fail},
		{"[en]", rdfPacket("", rdfAlt(d, rdfItem("en", "a"))), Pass},
		{"[x-default, en]", rdfPacket("", rdfAlt(d, rdfItem("x-default", "a"), rdfItem("en", "b"))), Pass},
		{"[en, en]", rdfPacket("", rdfAlt(d, rdfItem("en", "a"), rdfItem("en", "b"))), Pass},
		{"[x-default, x-default]", rdfPacket("", rdfAlt(d, rdfItem("x-default", "a"), rdfItem("x-default", "b"))), Pass},
		{"[\"\"], an empty language", rdfPacket("", rdfAlt(d, rdfItem("", "a"))), Pass},
		{"[X-DEFAULT]", rdfPacket("", rdfAlt(d, rdfItem("X-DEFAULT", "a"))), Pass},
		{"[x-Default]", rdfPacket("", rdfAlt(d, rdfItem("x-Default", "a"))), Pass},
		{"[ x-default ], padded", rdfPacket("", rdfAlt(d, rdfItem(" x-default ", "a"))), Pass},
		{"a determined Alt beside [x-default]", rdfPacket("", rdfAlt(d, rdfItem("en", "a"))+rdfAlt("dc:rights", rdfItem("x-default", "b"))), Fail},
		{"[x-default] in a second description", rdfHead + rdfDescription("", "", rdfAlt("dc:title", rdfItem("en", "T"))) +
			rdfDescription("", "", rdfAlt("dc:rights", rdfItem("x-default", "b"))) + rdfTail, Fail},
		{"[x-default] in a property outside dc:", rdfPacket("", rdfAlt("xmp:Foo", rdfItem("x-default", "a"))), Fail},
		{"[x-default] with white space round the items", rdfPacket("", `<dc:description> <rdf:Alt> `+rdfItem("x-default", "a")+` </rdf:Alt> </dc:description>`), Fail},

		// One item with no language, and the Alt is no subject.
		{"[—]", rdfPacket("", rdfAlt(d, rdfItem("-", "a"))), NotApplicable},
		{"[en, —]", rdfPacket("", rdfAlt(d, rdfItem("en", "a"), rdfItem("-", "b"))), NotApplicable},
		{"[—, en]", rdfPacket("", rdfAlt(d, rdfItem("-", "a"), rdfItem("en", "b"))), NotApplicable},
		{"[x-default, —]", rdfPacket("", rdfAlt(d, rdfItem("x-default", "a"), rdfItem("-", "b"))), NotApplicable},
		{"[—, x-default]", rdfPacket("", rdfAlt(d, rdfItem("-", "a"), rdfItem("x-default", "b"))), NotApplicable},
		{"an empty Alt", rdfPacket("", rdfAlt(d)), NotApplicable},
		{"[en, a child that is not rdf:li]", rdfPacket("", rdfAlt(d, rdfItem("en", "a"), `<foo:x>1</foo:x>`)), NotApplicable},
		{"[a child that is not rdf:li, x-default]", rdfPacket("", `<dc:description><rdf:Alt><foo:x xml:lang="x-default">a</foo:x></rdf:Alt></dc:description>`), Fail},
		{"[—] beside [x-default]", rdfPacket("", rdfAlt(d, rdfItem("-", "a"))+rdfAlt("dc:rights", rdfItem("x-default", "b"))), Fail},

		// The entry's packet and its neighbours: an Alt inside an item is not a subject, and its items are its own.
		{"[— holding [—], en] — the entry's packet", rdfPacket("", rdfAlt("dc:title", rdfItem("-", nested("-")), rdfItem("en", "T"))), NotApplicable},
		{"[— holding [x-default], en]", rdfPacket("", rdfAlt("dc:title", rdfItem("-", nested("x-default")), rdfItem("en", "T"))), NotApplicable},
		{"[— holding [en], x-default]", rdfPacket("", rdfAlt("dc:title", rdfItem("-", nested("en")), rdfItem("x-default", "T"))), NotApplicable},
		{"[x-default, en holding [en]]", rdfPacket("", rdfAlt("dc:title", rdfItem("x-default", "T"), rdfItem("en", nested("en")))), Pass},
		{"[x-default holding [en]]", rdfPacket("", rdfAlt("dc:title", rdfItem("x-default", nested("en")))), Fail},
		{"[x-default holding [x-default]]", rdfPacket("", rdfAlt(d, rdfItem("x-default", nested("x-default")))), Fail},
		{"[en holding [x-default]]", rdfPacket("", rdfAlt(d, rdfItem("en", nested("x-default")))), Pass},
		{"[x-default] in a Seq's item", rdfPacket("", `<dc:creator><rdf:Seq><rdf:li>`+nested("x-default")+`</rdf:li></rdf:Seq></dc:creator>`), NotApplicable},
		{"[x-default] in a Bag's item", rdfPacket("", `<dc:subject><rdf:Bag><rdf:li>`+nested("x-default")+`</rdf:li></rdf:Bag></dc:subject>`), NotApplicable},
		{"[x-default] in a structure's field", rdfPacket("", `<xmp:Foo rdf:parseType="Resource"><foo:a>`+nested("x-default")+`</foo:a></xmp:Foo>`), NotApplicable},
		{"[x-default] in a nested description's field", rdfPacket("", `<xmp:Foo><rdf:Description><foo:a>`+nested("x-default")+`</foo:a></rdf:Description></xmp:Foo>`), NotApplicable},
		{"[x-default] under a typed node in rdf:RDF", rdfHead + `<foo:Thing rdf:about=""` + rdfNS + `>` + rdfAlt(d, rdfItem("x-default", "a")) + `</foo:Thing>` + rdfTail, NotApplicable},

		// Where an item's language may be written.
		{"on the item's rdf:value, the item its own node", rdfPacket("", rdfAlt(d, `<rdf:li rdf:parseType="Resource"><rdf:value xml:lang="x-default">a</rdf:value></rdf:li>`)), Fail},
		{"on the item's rdf:value, in a description", rdfPacket("", rdfAlt(d, `<rdf:li><rdf:Description><rdf:value xml:lang="x-default">a</rdf:value></rdf:Description></rdf:li>`)), Fail},
		{"as an xml:lang element, the item its own node", rdfPacket("", rdfAlt(d, `<rdf:li rdf:parseType="Resource"><rdf:value>a</rdf:value><xml:lang>x-default</xml:lang></rdf:li>`)), Fail},
		{"as an xml:lang element, in a description", rdfPacket("", rdfAlt(d, `<rdf:li><rdf:Description><rdf:value>a</rdf:value><xml:lang>x-default</xml:lang></rdf:Description></rdf:li>`)), Fail},
		{"as an xml:lang element, in a typed node", rdfPacket("", rdfAlt(d, `<rdf:li><foo:x><rdf:value>a</rdf:value><xml:lang>x-default</xml:lang></foo:x></rdf:li>`)), Fail},
		{"as an xml:lang element naming en", rdfPacket("", rdfAlt(d, `<rdf:li><rdf:Description><rdf:value>a</rdf:value><xml:lang>en</xml:lang></rdf:Description></rdf:li>`)), Pass},
		{"as an xml:lang element, padded", rdfPacket("", rdfAlt(d, `<rdf:li rdf:parseType="Resource"><rdf:value>a</rdf:value><xml:lang> x-default </xml:lang></rdf:li>`)), Pass},
		{"on the description inside the item", rdfPacket("", rdfAlt(d, `<rdf:li><rdf:Description xml:lang="x-default"><rdf:value>a</rdf:value></rdf:Description></rdf:li>`)), Fail},
		{"an xml:lang element with no rdf:value beside it is a field", rdfPacket("", rdfAlt(d, `<rdf:li><rdf:Description><xml:lang>x-default</xml:lang></rdf:Description></rdf:li>`)), NotApplicable},
		{"[x-default, an item whose value has none]", rdfPacket("", rdfAlt(d, rdfItem("x-default", "a"), `<rdf:li><rdf:Description><rdf:value>a</rdf:value></rdf:Description></rdf:li>`)), NotApplicable},
		{"[x-default, an item whose rdf:value names en]", rdfPacket("", rdfAlt(d, rdfItem("x-default", "a"), `<rdf:li rdf:parseType="Resource"><rdf:value xml:lang="en">a</rdf:value></rdf:li>`)), Pass},

		// Item 3: a dc: property that is not an Alt is no subject, however it is written.
		{"dc:description as an attribute", rdfPacket(` dc:description="d"`, ""), NotApplicable},
		{"dc:description as an attribute, xml:lang on the description", rdfPacket(` dc:description="d" xml:lang="en"`, ""), NotApplicable},
		{"dc:title as an attribute, xml:lang x-default on the description", rdfPacket(` dc:title="T" xml:lang="x-default"`, ""), NotApplicable},
		{"dc:format and dc:creator as attributes", rdfPacket(` dc:format="application/pdf" dc:creator="me"`, ""), NotApplicable},
		{"dc:description as simple text", rdfPacket("", `<dc:description>d</dc:description>`), NotApplicable},
		{"dc:description as simple text with xml:lang x-default", rdfPacket("", `<dc:description xml:lang="x-default">d</dc:description>`), NotApplicable},
		{"dc:rights as an attribute beside [x-default]", rdfPacket(` dc:rights="r"`, rdfAlt("dc:title", rdfItem("x-default", "T"))), Fail},
		{"dc:rights as an attribute beside [en]", rdfPacket(` dc:rights="r"`, rdfAlt("dc:title", rdfItem("en", "T"))), Pass},
	}
	words := map[Verdict]string{Pass: "passed", Fail: "failed", NotApplicable: "none"}
	var docs [][]byte
	for _, c := range cases {
		docs = append(docs, withRawPacket(t, base, c.packet))
	}
	vera := veraAsk(t, docs)
	for i, c := range cases {
		if got := verdictOf(t, docs[i], "7.2 t33"); got.Verdict != c.want {
			t.Errorf("%s: 7.2 t33 reports %v (%s), want %v", c.name, got.Verdict, got.Why, c.want)
		}
		if vera != nil && (vera[i] == nil || vera[i]["7.2 t33"] != words[c.want]) {
			t.Errorf("%s: veraPDF now says %q for 7.2 t33 where %q was measured — the row is stale", c.name, vera[i]["7.2 t33"], words[c.want])
		}
	}
}

// TestAPacketVeraPDFsReaderRefusesIsNotRead — items 2 and 4 of the entry, and the class they belong to.
//
// Every packet here is the control with ONE thing wrong, and the control passes all four clauses in both checkers.
// veraPDF reads no packet from any of them — `5 t1` and `7.1 t9` fail, `5 t2` and `7.2 t33` have no subject — and nib
// read each one and passed all four. nib now refuses the packet, which the rules answer CannotCheck: it is the
// packet nib declines to read, as it declines one that is not well-formed XML.
func TestAPacketVeraPDFsReaderRefusesIsNotRead(t *testing.T) {
	base := plainDoc(t)
	title := rdfAlt("dc:title", rdfItem("en", "T"))
	part := `<pdfuaid:part>1</pdfuaid:part>`
	tp := title + part
	clauses := []string{"5 t1", "5 t2", "7.1 t9", "7.2 t33"}
	const (
		twiceAttr = "writes the attribute"
		twiceProp = "is written twice"
		text      = "text is written directly in"
		values    = "holds more than one value"
		noNS      = "is in no namespace"
		parseType = "rdf:parseType"
		about     = "different rdf:about"
		langTwice = "its language more than once"
	)
	cases := []struct {
		name, packet string
		why          string // what the refusal names; "" for a packet both checkers read
		ambiguous    bool   // the packet carries no identification, so veraPDF's refusal cannot be told from no subject
	}{
		{"control", rdfPacket("", tp), "", false},
		// An attribute twice (item 2).
		{"xmlns:pdfuaid declared twice — the entry's packet", rdfHead + `<rdf:Description rdf:about=""` + rdfNS + ` xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/">` + tp + `</rdf:Description>` + rdfTail, twiceAttr, false},
		{"rdf:about twice", rdfHead + `<rdf:Description rdf:about="" rdf:about=""` + rdfNS + `>` + tp + `</rdf:Description>` + rdfTail, twiceAttr, false},
		{"xml:lang twice on an item", rdfPacket("", `<dc:title><rdf:Alt><rdf:li xml:lang="en" xml:lang="en">T</rdf:li></rdf:Alt></dc:title>`+part), twiceAttr, false},
		{"an unprefixed attribute twice", rdfPacket("", tp+`<xmp:Foo a="1" a="2">v</xmp:Foo>`), twiceAttr, false},
		{"an attribute twice on x:xmpmeta", strings.Replace(rdfPacket("", tp), `<x:xmpmeta `, `<x:xmpmeta a="1" a="2" `, 1), twiceAttr, false},
		{"one attribute under two prefixes of one namespace", rdfPacket(` xmlns:p2="http://www.aiim.org/pdfua/ns/id/" pdfuaid:part="1" p2:part="1"`, title), twiceAttr, false},
		{"one field attribute under two prefixes of one namespace", rdfPacket(` xmlns:f2="http://example.com/foo/"`, tp+`<xmp:Foo foo:a="1" f2:a="1"/>`), twiceAttr, false},
		// A property twice (item 4's "a packet repeating pdfuaid:part").
		{"pdfuaid:part twice", rdfPacket("", tp+part), twiceProp, false},
		{"pdfuaid:part twice with different values", rdfPacket("", tp+`<pdfuaid:part>2</pdfuaid:part>`), twiceProp, false},
		{"pdfuaid:part as an attribute and as an element", rdfPacket(` pdfuaid:part="1"`, tp), twiceProp, false},
		{"pdfuaid:part in two descriptions", rdfHead + rdfDescription("", "", tp) + rdfDescription("", "", part) + rdfTail, twiceProp, false},
		{"pdfuaid:amd twice", rdfPacket("", tp+`<pdfuaid:amd>a</pdfuaid:amd><pdfuaid:amd>a</pdfuaid:amd>`), twiceProp, false},
		{"dc:title twice", rdfPacket("", title+tp), twiceProp, false},
		{"dc:title as an attribute and as an element", rdfPacket(` dc:title="T"`, tp), twiceProp, false},
		{"another property twice", rdfPacket("", tp+`<xmp:Foo>1</xmp:Foo><xmp:Foo>1</xmp:Foo>`), twiceProp, false},
		{"a property as an attribute and as an element", rdfPacket(` xmp:Foo="1"`, tp+`<xmp:Foo>2</xmp:Foo>`), twiceProp, false},
		{"a property as an attribute of two descriptions", rdfHead + rdfDescription("", ` xmp:Foo="1"`, tp) + rdfDescription("", ` xmp:Foo="2"`, "") + rdfTail, twiceProp, false},
		{"one property under two prefixes of one namespace", rdfPacket(` xmlns:p2="http://ns.adobe.com/xap/1.0/"`, tp+`<xmp:Foo>1</xmp:Foo><p2:Foo>2</p2:Foo>`), twiceProp, false},
		{"a structure's field twice", rdfPacket("", tp+`<xmp:Foo rdf:parseType="Resource"><foo:a>1</foo:a><foo:a>2</foo:a></xmp:Foo>`), twiceProp, false},
		{"a nested description's field twice", rdfPacket("", tp+`<xmp:Foo><rdf:Description><foo:a>1</foo:a><foo:a>2</foo:a></rdf:Description></xmp:Foo>`), twiceProp, false},
		{"a typed node's field twice", rdfPacket("", tp+`<xmp:Foo><foo:x><foo:y>1</foo:y><foo:y>2</foo:y></foo:x></xmp:Foo>`), twiceProp, false},
		{"rdf:value twice in a qualified pdfuaid:part", rdfPacket("", title+`<pdfuaid:part><rdf:Description><rdf:value>1</rdf:value><rdf:value>1</rdf:value></rdf:Description></pdfuaid:part>`), twiceProp, false},
		{"a property in a typed node and in a description, both in rdf:RDF", rdfHead + rdfDescription("", "", tp) + `<foo:Thing rdf:about=""` + rdfNS + `>` + part + `</foo:Thing>` + rdfTail, twiceProp, false},
		// Text where only elements may be (item 4's "a NON-RDF child element" is this: the child is a node, and
		// its text is written directly in it).
		{"pdfuaid:part holding a non-RDF child with text — the entry's packet", rdfPacket("", title+`<pdfuaid:part><foo:x>1</foo:x></pdfuaid:part>`), text, false},
		{"pdfuaid:amd holding a non-RDF child with text", rdfPacket("", tp+`<pdfuaid:amd><foo:x>1</foo:x></pdfuaid:amd>`), text, false},
		{"another property holding a non-RDF child with text", rdfPacket("", tp+`<xmp:Foo><foo:x>1</foo:x></xmp:Foo>`), text, false},
		{"text in a nested description", rdfPacket("", tp+`<xmp:Foo><rdf:Description>1</rdf:Description></xmp:Foo>`), text, false},
		{"text in the top description", rdfPacket("", `junk`+tp), text, false},
		{"a no-break space in the top description", rdfPacket("", "&#160;"+tp), text, false},
		{"text in rdf:RDF", rdfHead + `junk` + rdfDescription("", "", tp) + rdfTail, text, false},
		{"text in an Alt between its items", rdfPacket("", `<dc:title><rdf:Alt>junk`+rdfItem("en", "T")+`</rdf:Alt></dc:title>`+part), text, false},
		{"text in a Bag", rdfPacket("", tp+`<dc:subject><rdf:Bag>junk<rdf:li>a</rdf:li></rdf:Bag></dc:subject>`), text, false},
		{"text in a Seq inside a structure", rdfPacket("", tp+`<xmp:Foo rdf:parseType="Resource"><foo:a><rdf:Seq>junk<rdf:li>1</rdf:li></rdf:Seq></foo:a></xmp:Foo>`), text, false},
		{"text in a parseType Resource pdfuaid:part", rdfPacket("", title+`<pdfuaid:part rdf:parseType="Resource">1</pdfuaid:part>`), text, false},
		{"text in a typed node in rdf:RDF", rdfHead + rdfDescription("", "", tp) + `<foo:Other xmlns:foo="http://example.com/foo/">x</foo:Other>` + rdfTail, text, false},
		// A property with more than one value.
		{"pdfuaid:part holding text and then an element", rdfPacket("", title+`<pdfuaid:part>1<foo:x/></pdfuaid:part>`), values, false},
		{"pdfuaid:part holding text and then a description", rdfPacket("", title+`<pdfuaid:part>1<rdf:Description/></pdfuaid:part>`), values, false},
		{"a property holding an element and then text", rdfPacket("", tp+`<xmp:Foo><rdf:Description/>1</xmp:Foo>`), values, false},
		{"a property holding CDATA and then an element", rdfPacket("", tp+`<xmp:Foo><![CDATA[1]]><rdf:Description/></xmp:Foo>`), values, false},
		{"a property holding two descriptions", rdfPacket("", tp+`<xmp:Foo><rdf:Description/><rdf:Description/></xmp:Foo>`), values, false},
		{"pdfuaid:part holding two descriptions", rdfPacket("", title+`<pdfuaid:part><rdf:Description><rdf:value>1</rdf:value></rdf:Description><rdf:Description/></pdfuaid:part>`), values, false},
		{"an item holding text and an element", rdfPacket("", `<dc:title><rdf:Alt><rdf:li xml:lang="en">T<foo:x/></rdf:li></rdf:Alt></dc:title>`+part), values, false},
		{"a property with a field as an attribute, and text", rdfPacket("", tp+`<xmp:Foo foo:a="1">text</xmp:Foo>`), values, false},
		// A property in no namespace.
		{"a property with no prefix and no default namespace", rdfPacket("", tp+`<bar>1</bar>`), noNS, false},
		{"a property under a prefix nothing binds", rdfPacket("", tp+`<zzz:p>1</zzz:p>`), noNS, false},
		{"a structure's field with no namespace", rdfPacket("", tp+`<xmp:Foo rdf:parseType="Resource"><bar>1</bar></xmp:Foo>`), noNS, false},
		// The rest.
		{"rdf:parseType Literal", rdfPacket("", tp+`<xmp:Foo rdf:parseType="Literal">a<foo:b/>c</xmp:Foo>`), parseType, false},
		{"rdf:parseType Collection", rdfPacket("", tp+`<xmp:Foo rdf:parseType="Collection"><rdf:Description/></xmp:Foo>`), parseType, false},
		{"two descriptions whose rdf:about differ", rdfHead + rdfDescription("a", "", title) + rdfDescription("b", "", part) + rdfTail, about, false},
		{"an item's language on its rdf:value and as an element", rdfPacket("", rdfAlt("dc:description", `<rdf:li rdf:parseType="Resource"><rdf:value xml:lang="x-default">a</rdf:value><xml:lang>en</xml:lang></rdf:li>`)), langTwice, true},
		{"an item's language on the item and on its rdf:value", rdfPacket("", rdfAlt("dc:description", `<rdf:li xml:lang="x-default"><rdf:Description><rdf:value xml:lang="en">a</rdf:value></rdf:Description></rdf:li>`)), langTwice, true},

		// What both checkers READ: each sits beside a refusal above, and is why the refusal is no wider.
		{"read: two descriptions with one rdf:about", rdfHead + rdfDescription("", "", title) + rdfDescription("", "", part) + rdfTail, "", false},
		{"read: one local name in two namespaces", rdfPacket("", tp+`<xmp:part>1</xmp:part><foo:part>1</foo:part>`), "", false},
		{"read: a Bag's items repeated", rdfPacket("", tp+`<dc:subject><rdf:Bag><rdf:li>a</rdf:li><rdf:li>a</rdf:li></rdf:Bag></dc:subject>`), "", false},
		{"read: a comment and a description in one property", rdfPacket("", tp+`<xmp:Foo><!--c--><rdf:Description/></xmp:Foo>`), "", false},
		{"read: white space round a description in one property", rdfPacket("", tp+`<xmp:Foo> <rdf:Description/> </xmp:Foo>`), "", false},
		{"read: an empty typed node", rdfPacket("", tp+`<xmp:Foo><foo:x/></xmp:Foo>`), "", false},
		{"read: a typed node with a field", rdfPacket("", tp+`<xmp:Foo><foo:x><foo:y>1</foo:y></foo:x></xmp:Foo>`), "", false},
		{"read: pdfuaid:part as a typed node's rdf:value", rdfPacket("", title+`<pdfuaid:part><foo:x><rdf:value>1</rdf:value></foo:x></pdfuaid:part>`), "", false},
		{"read: pdfuaid:part as a description's rdf:value", rdfPacket("", title+`<pdfuaid:part><rdf:Description><rdf:value>1</rdf:value></rdf:Description></pdfuaid:part>`), "", false},
		{"read: pdfuaid:part as a parseType Resource rdf:value", rdfPacket("", title+`<pdfuaid:part rdf:parseType="Resource"><rdf:value>1</rdf:value></pdfuaid:part>`), "", false},
		{"read: a property whose fields are attributes", rdfPacket("", tp+`<xmp:Foo foo:a="1" foo:a2="2"/>`), "", false},
		{"read: two field attributes under two prefixes of one namespace", rdfPacket(` xmlns:f2="http://example.com/foo/"`, tp+`<xmp:Foo foo:a="1" f2:b="1"/>`), "", false},
		{"read: text after rdf:RDF", strings.Replace(rdfPacket("", tp), `</rdf:RDF>`, `</rdf:RDF>junk`, 1), "", false},
		{"read: structures nested in a Bag and a Seq", rdfPacket(` xmlns:pe="http://www.aiim.org/pdfa/ns/extension/" xmlns:ps="http://www.aiim.org/pdfa/ns/schema#" xmlns:pp="http://www.aiim.org/pdfa/ns/property#"`,
			tp+`<pe:schemas><rdf:Bag><rdf:li rdf:parseType="Resource"><ps:schema>s</ps:schema><ps:property><rdf:Seq>`+
				`<rdf:li rdf:parseType="Resource"><pp:name>part</pp:name></rdf:li><rdf:li rdf:parseType="Resource"><pp:name>amd</pp:name></rdf:li>`+
				`</rdf:Seq></ps:property></rdf:li></rdf:Bag></pe:schemas>`), "", false},
	}
	var docs [][]byte
	for _, c := range cases {
		docs = append(docs, withRawPacket(t, base, c.packet))
	}
	vera := veraAsk(t, docs)
	refused := map[string]string{"5 t1": "failed", "5 t2": "none", "7.1 t9": "failed", "7.2 t33": "none"}
	for i, c := range cases {
		for _, clause := range clauses {
			got := verdictOf(t, docs[i], clause)
			switch {
			case c.why == "" && got.Verdict != Pass:
				t.Errorf("%s: %s reports %v (%s), want Pass — veraPDF reads this packet", c.name, clause, got.Verdict, got.Why)
			case c.why != "" && (got.Verdict != CannotCheck || !strings.Contains(got.Why, c.why)):
				t.Errorf("%s: %s reports %v (%s), want CannotCheck naming %q — veraPDF reads no packet here", c.name, clause, got.Verdict, got.Why, c.why)
			}
			if vera == nil || (c.ambiguous && clause != "7.2 t33") {
				continue
			}
			want := "passed"
			if c.why != "" {
				want = refused[clause]
			}
			if vera[i] == nil || vera[i][clause] != want {
				t.Errorf("%s: veraPDF now says %q for %s where %q was measured — the row is stale", c.name, vera[i][clause], clause, want)
			}
		}
	}
}
