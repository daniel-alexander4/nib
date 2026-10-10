package uacheck

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"nib/internal/pdfread"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Reading the XMP packet — `PLAN-accessibility.md` P07.S02.
//
// # Why this is parsed and not searched
//
// Three clauses ask about the metadata packet's CONTENT: `7.1 t9` wants a `dc:title`, `5 t1` wants
// the PDF/UA identification schema, and `7.2 t33` wants the metadata's language determinable. Every
// one of them is a question about an XML element, and `bytes.Contains(packet, "dc:title")` answers a
// different question — it matches the string inside a comment, inside an `rdf:about` attribute,
// inside another element's character data, or inside a property whose name merely ends that way.
//
// This repo has already paid for that mistake once in the other direction: `bytes.Count(pdf,
// "/StructElem")` was 0 for every pdfcpu output because the tree lives in a compressed object
// stream, and four slices of conclusions rested on it. A byte search answers a question about the
// FILE while being read as one about the DOCUMENT.
//
// # What this reader does NOT cover, stated rather than discovered later
//
// It walks elements and namespaces and reads character data. It does not resolve
// `rdf:parseType="Resource"` shorthand in general — only where a clause was measured against it (the
// identification's `rdf:value`, a `dc:title` item's language) — and a packet using it elsewhere reads as
// *absent* here, which is the safe direction for a checker — it yields `Fail` or `CannotCheck` rather than a
// pass nib did not establish — but it is a disagreement with veraPDF that law 5's guard will surface, and it
// is written down so the surfacing is expected.

// xmpFacts is what the rules need to know about a document's metadata packet.
type xmpFacts struct {
	// Present is whether the catalog has a /Metadata stream that could be read at all. When false
	// every clause about the packet's content has no subject.
	Present bool
	// Readable is whether the stream decoded and parsed as XML. A present-but-unreadable packet is
	// `CannotCheck` territory, not `Fail`: nib could not evaluate it, which is a different fact
	// from the document lacking what the clause wants.
	Readable bool
	// StreamType and StreamSubtype are the stream dictionary's own `/Type` and `/Subtype`. 7.1 t8
	// names them: veraPDF's error is "doesn't contain metadata key OR metadata stream dictionary
	// does not contain either entry Type with…".
	StreamType    string
	StreamSubtype string
	// TitledInALanguage is whether a `dc:title` holds an ITEM carrying a language — an `rdf:li` with an `xml:lang`,
	// written on the item, on its `rdf:value`, or as an `xml:lang` qualifier element (/pending 694). It is 7.1 t9's
	// whole subject, and it is not "the title has text".
	//
	// **Measured on veraPDF 1.30.2**, by mutating the dc:title of `ghostscript/pdflatex-article.pdf` (Ghostscript's
	// re-distil, which writes an `rdf:Alt` holding ONE EMPTY `x-default` item): PASSED with an empty item, a
	// whitespace one, a self-closed one, an `en`-only one, an `rdf:Seq` or `rdf:Bag` item with a language, an item
	// whose `rdf:value` carries it (as `parseType="Resource"` or as an inner `rdf:Description`), and an item with an
	// `xml:lang` qualifier element; FAILED with no dc:title, an empty `rdf:Alt`, an `rdf:Alt` or `rdf:Bag` item with
	// no language, a simple-valued `<dc:title>T</dc:title>` (with or without `xml:lang` on the property), an
	// attribute-form `dc:title="T"`, a language on the `rdf:Alt` or the property rather than the item, and a
	// `parseType="Resource"` title whose `rdf:value` carries one. veraPDF asks for the title's language
	// alternative (`VeraPDFXMPNode.getLanguageAlternative`), and the value is never read. Reading "the first
	// non-empty text" made the empty item a false FAIL and the last five shapes with text false PASSES.
	TitledInALanguage bool
	// UAPart is the `pdfuaid:part` value, empty when the identification schema is absent.
	UAPart string
	// UAProps is every property in the PDF/UA identification namespace, keyed by its local name and
	// carrying the prefix the packet actually wrote it under. It is the subject gate for the whole
	// clause-5 family: `5 t2`, `5 t3`, `5 t4` and `5 t5` have a subject exactly when this is non-empty.
	//
	// **Measured on veraPDF 1.30.2, because the obvious reading is wrong** (P06.S01). veraPDF's
	// `containsPDFUAIdentification` is not "the packet declares a part" and not "the pdfaExtension
	// schema describes the pdfuaid namespace". Three length-preserving mutations of the corpus file
	// `5-t03-pass-a.pdf` settle it: a packet whose only pdfuaid property is `corr` passes `5 t1` and
	// fails `5 t2`; so does one whose only property is an unknown `zzzz`; and one keeping the extension
	// schema while carrying NO pdfuaid property fails `5 t1` and leaves `5 t2`-`5 t5` with no subject
	// at all (`passedChecks="0" failedChecks="0"`). So any property in the namespace makes the
	// identification exist, whatever it is called.
	UAProps map[string]xmpProp
	// LangAlts is every language alternative in the packet that is 7.2 t33's subject, each as the languages of
	// its items in order. These are the metadata text the clause is about, measured: `dc:title`,
	// `dc:description` and `dc:rights` as `rdf:Alt` give the clause a subject, while `dc:creator` (an `rdf:Seq`)
	// and `xmp:CreateDate` do not.
	//
	// **Grouped per Alt, because the rule is about an Alt** (`/pending 489`): an Alt holding `x-default`
	// AND a real language is determined, and a flat list could not tell that Alt from two Alts.
	//
	// **Which `rdf:Alt` is one, measured on veraPDF 1.30.2 (`/pending 654`,
	// `TestWhichAlternativeIsTheLanguageClausesSubject`).** The value of a property written directly in an
	// `rdf:Description` directly in `rdf:RDF`, in any namespace and in any of the packet's descriptions — never an
	// Alt nested in an item, in a structure's field or in a Bag's or Seq's item. It holds at least one item, and
	// EVERY item carries a language: one item without (`[en, —]`, an item that is itself an Alt, a child that is
	// not an `rdf:li`) and the whole Alt is no subject. An empty `xml:lang=""` is a language. Every `rdf:Alt` used
	// to be listed, with "" for an item declaring none, and an item landed on the Alt opened LAST rather than the
	// one it sits in — so an outer Alt's items joined a nested one's and a group naming no language appeared.
	LangAlts [][]string
	// Why carries the reason when Readable is false.
	Why string
}

// hasXMLLang reports whether attrs carry an `xml:lang`, its prefix resolved rather than trusted (the const block's
// contract). Its value is not read: an empty language is still a language to veraPDF's 7.1 t9 (measured, /pending 694).
func hasXMLLang(attrs []xml.Attr, resolve func(string) string) bool {
	_, ok := xmlLangOf(attrs, resolve)
	return ok
}

// xmlLangOf is the `xml:lang` attrs carry, found by the namespace its prefix resolves to and never by the prefix
// as written (the const block's contract), and whether they carry one.
func xmlLangOf(attrs []xml.Attr, resolve func(string) string) (string, bool) {
	for _, a := range attrs {
		if a.Name.Local == "lang" && a.Name.Space != "" && a.Name.Space != "xmlns" && resolve(a.Name.Space) == nsXML {
			return a.Value, true
		}
	}
	return "", false
}

// Where an element sits in RDF/XML's alternation — a node holds properties, a property holds text or one node.
const (
	xkOutside = iota // not inside `rdf:RDF`
	xkRDF            // `rdf:RDF` itself
	xkNode           // a node: `rdf:Description`, a typed node, a container, or a `parseType="Resource"` property
	xkProp           // a property
	xkOpaque         // under a `rdf:parseType` nib has no measurement for: nothing below it is judged
)

// xmpAltItem is one item of a language alternative being read, and the languages written for it.
type xmpAltItem struct {
	// direct is an `xml:lang` on the item or on its `rdf:value`.
	direct []string
	// qualified is one on the node inside the item, or an `xml:lang` element — counted only beside an `rdf:value`.
	qualified []string
	hasValue  bool
	// elem is the open `xml:lang` element's text.
	elem *strings.Builder
}

// xmpAlt is a language alternative being read: the frame it opened at, and what its items came to.
type xmpAlt struct {
	at         int
	langs      []string
	unlabelled bool
	item       *xmpAltItem
}

// xmpProp is one property as the packet wrote it: the namespace prefix it used, and its character data.
//
// **The prefix is the point** (`5 t3`/`t4`/`t5`). A property is found by NAMESPACE — veraPDF looks it up
// as `getProperty(nsPDFUAID, "part")` — and then judged by the PREFIX that namespace was bound to at that
// element, which is the document's free choice and is what the clause refuses.
type xmpProp struct {
	// Prefix is the prefix as written, with no colon, and "" when the property used the default namespace.
	Prefix string
	// Value is the property's character data.
	Value string
	// own and rdfValue are the raw text directly inside the property and inside its first `rdf:value`;
	// `Value` is settled from them when the packet has been read (a qualified property's value IS its
	// `rdf:value`, and its other qualifiers are not part of it).
	own, rdfValue string
	hasRDFValue   bool
}

// The namespaces the clauses name. Compared by URI rather than by prefix, because a prefix is the
// document's choice — a packet may bind `dc:` to something else entirely, and a reader trusting the
// prefix would read the wrong property and report a pass.
const (
	nsDC      = "http://purl.org/dc/elements/1.1/"
	nsPDFUAID = "http://www.aiim.org/pdfua/ns/id/"
	// nsXML is bound by the XML specification rather than by any packet, so `resolve` answers it
	// directly — no document declares it and a reader waiting for a declaration would never see one.
	nsXML = "http://www.w3.org/XML/1998/namespace"
)

// maxXMPDepth bounds how deeply `parseXMP` will descend before refusing the packet.
//
// **A bound is needed because the packet's size is the document's choice, not nib's.** The metadata
// stream is FlateDecode'd, so a few kilobytes on disk decompress to whatever the producer wrote, and
// `sd.Decode()` hands back all of it. Real XMP nests under ten elements deep — `x:xmpmeta` /
// `rdf:RDF` / `rdf:Description` / a property / `rdf:Alt` / `rdf:li` is six — so this is three orders
// of magnitude of headroom over any packet a producer emits, and a packet past it is a packet nib
// declines to read rather than one it reads wrongly. The refusal reaches the rules as `CannotCheck`.
const maxXMPDepth = 1000

// maxXMPBytes bounds the metadata packet's DECODED size, for the reason `maxXMPDepth` gives: the stream is
// FlateDecode'd, pdfcpu's own ceiling is 512 MiB, and a producer's XMP is kilobytes (a packet carrying an
// embedded thumbnail, the largest real shape, is a few hundred). A packet past it is refused, not read short.
const maxXMPBytes = 16 << 20

// decodeWithin decodes a stream nib is about to parse, refusing it once its DECODED size passes max, and
// answers why when it would not decode or was refused ("" when sd.Content holds the whole stream).
//
// **The cap is applied DURING the decode, not after it** (the P07 phase-close review, R4-5): checking
// `len(sd.Content)` after `sd.Decode()` bounded what nib parsed and nothing about what it inflated, which is
// the half the document chooses. `DecodeWithLimit` stops the filter at the ceiling; the length check after it
// covers what that cannot see — a stream decoded earlier by another reader, and an unfiltered stream, whose
// raw bytes pdfcpu hands back without a limit.
//
// **It bounds nib's decode, not the document's open.** pdfcpu's validator decodes the catalog's /Metadata itself
// at open (`catalogMetaData`, under its own 512 MiB ceiling, into a copy nib never sees), so a packet between this
// cap and pdfcpu's is inflated once there and then refused here (the P07 phase-close re-review; read from pdfcpu's
// source, unmeasured).
func decodeWithin(sd *types.StreamDict, max int, what string) string {
	if err := pdfread.DecodeWithin(sd, int64(max)); err != nil {
		if errors.Is(err, pdfread.ErrDecodeLimit) {
			return fmt.Sprintf("%s exceeds %d bytes decoded, and nib stopped decoding it there", what, max)
		}
		return what + " could not be decoded: " + err.Error()
	}
	return ""
}

// readXMP gathers what the metadata rules need, in one pass over the packet.
func readXMP(d *Document) xmpFacts {
	if !d.xmpDone {
		d.xmp, d.xmpDone = parseXMP(d), true
	}
	return d.xmp
}

// parseXMP is readXMP's one parse; five rules read the packet, and it is decoded once.
func parseXMP(d *Document) xmpFacts {
	raw, has := d.Catalog["Metadata"]
	if !has {
		return xmpFacts{}
	}
	sd, _, err := d.Ctx.DereferenceStreamDict(raw)
	if err != nil || sd == nil {
		return xmpFacts{Present: true, Why: "the catalog's /Metadata does not resolve to a stream"}
	}
	f := xmpFacts{Present: true, StreamType: d.name(sd.Dict["Type"]), StreamSubtype: d.name(sd.Dict["Subtype"])}
	if why := decodeWithin(sd, maxXMPBytes, "the metadata stream"); why != "" {
		f.Why = why
		return f
	}
	dec := xml.NewDecoder(strings.NewReader(string(sd.Content)))
	// **Namespace-aware but tolerant of a malformed packet.** A packet that is not well-formed XML
	// is Readable=false, and the rules turn that into `CannotCheck` — the document may well carry
	// what the clause wants, and nib is the thing that could not read it.
	//
	// # Why RawToken and a scope stack nib maintains itself (P06.S01)
	//
	// `Token()` resolves a prefix to its namespace URI and then **discards the prefix**, and `5 t3`,
	// `5 t4` and `5 t5` are clauses about the prefix. The tempting inversion — ask which in-scope
	// prefix maps to this URI — is ambiguous on precisely the document that matters: veraPDF's own
	// `5-t04-fail-a.pdf` binds BOTH `pdfuaia` and `pdfuaid` to the identification namespace and writes
	// `pdfuaid:part` beside `pdfuaia:amd`. So the raw qualified name is needed, `RawToken` is what
	// supplies it (it performs no namespace translation, putting the prefix in `Name.Space`), and the
	// binding scope is resolved here instead.
	//
	// **That moves the well-formedness check onto nib**, because `RawToken` does not verify that start
	// and end elements match. It is re-implemented below rather than dropped: without it a truncated or
	// mismatched packet would read as merely *empty*, and `7.1 t9`, `5 t1`, `5 t2` and `7.2 t33` would
	// quietly answer Fail over a packet nib failed to read instead of `CannotCheck`.
	//
	// # Why the bindings live in ONE map and the stack depth is bounded
	//
	// The first version of this rewrite resolved a prefix by walking the open-element stack, which is
	// O(depth) per element and therefore O(depth²) over a packet. `Token()` kept a flat map and was
	// linear. Measured end to end through `Check` on a packet of nested elements: 5,000 deep 57 ms,
	// 20,000 deep 667 ms, 80,000 deep **11.1 s** — four times the depth for seventeen times the work.
	// The packet is a FlateDecode stream inside a PDF the user opened, so that is an attacker-supplied
	// quadratic. A prefix now maps to a stack of URIs in one map, popped at each end tag, which restores
	// O(1) resolution, and the depth carries a ceiling the way every other walk in this package does
	// (`overBudget`, `maxWalkDepth`).
	//
	// # And no other step may be per-token × something the packet chooses (the P07 phase close, R4-1)
	//
	// **This comment used to call the parse linear, and it was not.** Two more steps scaled with the packet
	// twice: a property's text grew by `p.own += raw` once per character-data RUN, and a comment splits one
	// value into as many runs as the packet likes (`1<!---->1<!---->…`), so each run copied the whole value so
	// far — measured 200,000 runs 3.8 s and 400,000 runs 20.4 s, on a 3.2 MB packet. And every run walked the
	// open-element stack to find its property, O(depth) per run. The text now grows in a builder (`bufs`), each
	// frame carries the index of its nearest property ancestor (`anchor`, `uaAnchor`), computed once when it
	// opens, so a run and an `rdf:value` attribute find their property in O(1), and the packet's decoded size
	// is capped (`maxXMPBytes`), so the linear parse is a bounded one too.
	type frame struct {
		raw      xml.Name // as written: Space is the prefix, "" when the element carried none
		space    string   // the namespace URI that prefix resolved to
		declared []string // the prefixes this element bound, so the same ones are unbound at its end tag
		owns     string   // the UAProps local name this element introduced, so its chardata is its own
		// anchor is the index in path of the nearest frame — this one included — that is a `dc:title` or an
		// identification property, -1 when none is open: the element a character-data run belongs to.
		anchor int
		// uaAnchor is the index of the nearest identification-property frame, this one included, or -1: the
		// property an `rdf:value` attribute on this element belongs to. It does not stop at `dc:title`.
		uaAnchor int
		// titleItem is the index of the open `dc:title` item (`rdf:li`) this element is in, this one included, or -1.
		titleItem int
		// kind is where the element sits in RDF's alternation of nodes and properties (`xk…`), and the rest is
		// what `refuse` below is decided from. list: a node whose element children are properties, each named
		// once (not a container, whose children are items). top: a node directly in `rdf:RDF` — all of them are
		// ONE property list. seen: the properties this node has. text, elems, fields: a property's own
		// non-whitespace text, its element children, and whether it writes a field as an attribute.
		kind      uint8
		list, top bool
		seen      map[xml.Name]bool
		text      bool
		elems     int
		fields    bool
		// altItem is the index of the open item of the language alternative being read (`alt`) this element is
		// in, this one included, or -1; langElem marks an `xml:lang` element that is that item's qualifier.
		altItem  int
		langElem bool
	}
	var path []frame
	// # The RDF a packet must be, as veraPDF's reader holds it (`/pending 654`)
	//
	// veraPDF does not read a packet its XMP library refuses: `5 t1` and `7.1 t9` FAIL and `5 t2` and `7.2 t33`
	// have no subject, whatever the packet holds. Go's decoder is not that library, and nib read each of these
	// and answered from it — a pass on all four over a packet veraPDF reads nothing from. Each is measured on
	// veraPDF 1.30.2 against a packet that passes all four without it
	// (`TestAPacketVeraPDFsReaderRefusesIsNotRead`), and each is `Readable == false` here, which the rules
	// answer `CannotCheck` — as they do for a packet that is not well-formed XML:
	//
	//   - an attribute written twice on one element, by its name as written or by namespace and local name;
	//   - a property named twice in one node — and every node directly in `rdf:RDF` is one node between them,
	//     whether the property is written as an element or as an attribute;
	//   - text other than white space directly in `rdf:RDF`, in a node, or in a `parseType="Resource"` property;
	//   - a property holding text beside an element, two elements, or text beside a field written as an attribute;
	//   - a property in no namespace, or under a prefix nothing binds;
	//   - `rdf:parseType` `Literal` or `Collection`;
	//   - two nodes directly in `rdf:RDF` whose `rdf:about` differ;
	//   - an item of a language alternative given its language twice.
	//
	// **This is the measured part of that library's grammar, not all of it.** A shape outside the list is read
	// as before.
	refuse := func(what string) xmpFacts {
		f.Why = "the metadata packet is not RDF veraPDF's reader accepts, and it reads no packet there: " + what
		return f
	}
	var (
		sawRDF   bool
		topSeen  = map[xml.Name]bool{}
		topAbout string
		// alt is the language alternative being read. One at a time: an Alt nested in another is no subject.
		alt *xmpAlt
	)
	// bufs holds each identification property's text as it grows; `p.own += raw` per run was the quadratic.
	type propBuf struct{ own, rdf strings.Builder }
	bufs := map[string]*propBuf{}
	buf := func(name string) *propBuf {
		b := bufs[name]
		if b == nil {
			b = &propBuf{}
			bufs[name] = b
		}
		return b
	}
	ns := map[string][]string{}
	const nsRDF = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	// resolve maps a prefix to the URI currently bound to it. The `xml` prefix is bound by the XML
	// specification itself and need never be declared, so it is answered here.
	resolve := func(prefix string) string {
		if prefix == "xml" {
			return nsXML
		}
		if u := ns[prefix]; len(u) > 0 {
			return u[len(u)-1]
		}
		return ""
	}
	// inUANamespace reports whether an ATTRIBUTE names a property of the identification. An unprefixed
	// attribute is in no namespace at all under XML Namespaces, so it can never be one.
	uaAttr := func(a xml.Attr) bool {
		return a.Name.Space != "" && a.Name.Space != "xmlns" && resolve(a.Name.Space) == nsPDFUAID
	}
	for {
		tok, terr := dec.RawToken()
		if terr == io.EOF {
			break
		}
		if terr != nil {
			f.Why = "the metadata packet is not well-formed XML: " + terr.Error()
			return f
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if len(path) >= maxXMPDepth {
				f.Why = fmt.Sprintf("the metadata packet nests more than %d elements deep, so nib "+
					"stopped reading it rather than spend the whole document's budget on one packet", maxXMPDepth)
				return f
			}
			fr := frame{raw: t.Name, anchor: -1, uaAnchor: -1, titleItem: -1, altItem: -1}
			if len(path) > 0 {
				top := path[len(path)-1]
				fr.anchor, fr.uaAnchor, fr.titleItem, fr.altItem = top.anchor, top.uaAnchor, top.titleItem, top.altItem
			}
			for _, a := range t.Attr {
				// `xmlns="U"` arrives as a bare local name; `xmlns:p="U"` as Space "xmlns", Local "p".
				prefix, isDecl := "", false
				switch {
				case a.Name.Space == "" && a.Name.Local == "xmlns":
					prefix, isDecl = "", true
				case a.Name.Space == "xmlns":
					prefix, isDecl = a.Name.Local, true
				}
				if isDecl {
					ns[prefix] = append(ns[prefix], a.Value)
					fr.declared = append(fr.declared, prefix)
				}
			}
			// Bound BEFORE resolving, so an element's own declarations bind its own name.
			fr.space = resolve(t.Name.Space)

			// One attribute, once — as written, and by the namespace its prefix resolves to (two prefixes bound
			// to one URI name the same attribute).
			if len(t.Attr) > 1 {
				written := make(map[xml.Name]bool, 2*len(t.Attr))
				for _, a := range t.Attr {
					keys := []xml.Name{a.Name}
					if a.Name.Space != "" && a.Name.Space != "xmlns" {
						keys = append(keys, xml.Name{Space: "\x00" + resolve(a.Name.Space), Local: a.Name.Local})
					}
					for _, k := range keys {
						if written[k] {
							return refuse("an element writes the attribute " + a.Name.Local + " twice")
						}
						written[k] = true
					}
				}
			}
			// propertyAttr reports whether an attribute writes a property of the element's node: one in a
			// namespace that is neither RDF's nor XML's own.
			propertyAttr := func(a xml.Attr) (xml.Name, bool) {
				if a.Name.Space == "" || a.Name.Space == "xmlns" {
					return xml.Name{}, false
				}
				uri := resolve(a.Name.Space)
				return xml.Name{Space: uri, Local: a.Name.Local}, uri != nsRDF && uri != nsXML
			}
			rdfAttr := func(local string) (string, bool) {
				for _, a := range t.Attr {
					if a.Name.Local == local && a.Name.Space != "" && a.Name.Space != "xmlns" && resolve(a.Name.Space) == nsRDF {
						return a.Value, true
					}
				}
				return "", false
			}
			parentKind := uint8(xkOutside)
			if len(path) > 0 {
				parentKind = path[len(path)-1].kind
			}
			switch parentKind {
			case xkOutside:
				if fr.space == nsRDF && t.Name.Local == "RDF" && !sawRDF {
					fr.kind, sawRDF = xkRDF, true
				}
			case xkRDF:
				fr.kind, fr.list, fr.top = xkNode, true, true
				if about, _ := rdfAttr("about"); about != "" {
					if topAbout != "" && about != topAbout {
						return refuse("two descriptions directly in rdf:RDF carry different rdf:about values")
					}
					topAbout = about
				}
				for _, a := range t.Attr {
					if name, is := propertyAttr(a); is {
						if topSeen[name] {
							return refuse("the property " + a.Name.Local + " is written twice")
						}
						topSeen[name] = true
					}
				}
			case xkNode:
				fr.kind = xkProp
				if fr.space == "" {
					return refuse("the property " + t.Name.Local + " is in no namespace")
				}
				if parent := &path[len(path)-1]; parent.list {
					seen := topSeen
					if !parent.top {
						if parent.seen == nil {
							parent.seen = map[xml.Name]bool{}
						}
						seen = parent.seen
					}
					name := xml.Name{Space: fr.space, Local: t.Name.Local}
					if seen[name] {
						return refuse("the property " + t.Name.Local + " is written twice in one description")
					}
					seen[name] = true
				}
				for _, a := range t.Attr {
					if _, is := propertyAttr(a); is {
						fr.fields = true
					}
				}
				if pt, has := rdfAttr("parseType"); has {
					switch pt {
					case "Resource":
						// The property is its own node: its children are its fields.
						fr.kind, fr.list = xkNode, true
					case "Literal", "Collection":
						return refuse("the property " + t.Name.Local + " is written with rdf:parseType " + pt)
					default:
						fr.kind = xkOpaque
					}
				}
			case xkProp:
				parent := &path[len(path)-1]
				parent.elems++
				if parent.elems > 1 || parent.text {
					return refuse("the property " + parent.raw.Local + " holds more than one value")
				}
				fr.kind = xkNode
				fr.list = !(fr.space == nsRDF && (t.Name.Local == "Alt" || t.Name.Local == "Bag" || t.Name.Local == "Seq"))
			case xkOpaque:
				fr.kind = xkOpaque
			}

			// **7.2 t33's subject: a language alternative and its items' languages** (`LangAlts`). An item's
			// language is its own `xml:lang`, or its `rdf:value`'s, or — beside an `rdf:value` only — an
			// `xml:lang` on the node inside the item or an `xml:lang` element there. Measured: each of the four
			// writes `x-default` and FAILS the clause alone, and an `xml:lang` element with no `rdf:value` beside
			// it is a field, not a language (the Alt is then no subject).
			switch i := fr.altItem; {
			case alt != nil && alt.item != nil && i >= 0:
				it, parent := alt.item, path[len(path)-1]
				// field: this element is written where the item's value and qualifiers are — directly in an
				// item that is its own node, or in the one node inside a plain item.
				field := (len(path) == i+1 && path[i].kind == xkNode) ||
					(len(path) == i+2 && path[i].kind == xkProp && parent.kind == xkNode && parent.list)
				lang, has := xmlLangOf(t.Attr, resolve)
				switch {
				case len(path) == i+1 && path[i].kind == xkProp && fr.list:
					if has {
						it.qualified = append(it.qualified, lang)
					}
				case field && fr.space == nsRDF && t.Name.Local == "value":
					it.hasValue = true
					if has {
						it.direct = append(it.direct, lang)
					}
				case field && fr.space == nsXML && t.Name.Local == "lang":
					fr.langElem, it.elem = true, &strings.Builder{}
				}
			case alt != nil && len(path) == alt.at+1:
				// Every element directly in the Alt is an item, `rdf:li` or not (measured: a child of another
				// name with an `xml:lang` is judged as an item, and one without leaves no subject).
				fr.altItem, alt.item = len(path), &xmpAltItem{}
				if lang, has := xmlLangOf(t.Attr, resolve); has {
					alt.item.direct = append(alt.item.direct, lang)
				}
			case alt == nil && fr.kind == xkNode && fr.space == nsRDF && t.Name.Local == "Alt" && len(path) >= 2:
				prop, desc := path[len(path)-1], path[len(path)-2]
				if prop.kind == xkProp && desc.top && desc.space == nsRDF && desc.raw.Local == "Description" {
					alt = &xmpAlt{at: len(path)}
				}
			}

			// **Attribute-form properties, which RDF/XML calls the abbreviated syntax.** A simple-valued
			// property may be written as an attribute on `rdf:Description` — `pdfuaid:part="1"` — and
			// that is ordinary XMP rather than an exotic shape. veraPDF reads it: measured on a
			// length-preserving mutation of `5-t03-pass-a.pdf`, veraPDF passes all five clause-5 tests
			// on an attribute-form identification while nib, reading start elements only, reported
			// `5 t1` Fail and the other four NotApplicable — a live false fail on a whole serialisation.
			//
			// An attribute-form `dc:title="T"` is NOT read: it is simple-valued, it has no item to carry a
			// language, and veraPDF fails 7.1 t9 on it (measured, /pending 694 — reading it was a false pass).
			for _, a := range t.Attr {
				if !uaAttr(a) {
					continue
				}
				if f.UAProps == nil {
					f.UAProps = map[string]xmpProp{}
				}
				if _, seen := f.UAProps[a.Name.Local]; !seen {
					f.UAProps[a.Name.Local] = xmpProp{Prefix: a.Name.Space, Value: a.Value}
				}
			}

			// **7.1 t9's subject: an item of a `dc:title` carrying a language** (`TitledInALanguage`). The item is an
			// `rdf:li` in a container directly inside the title; its language is an `xml:lang` on the item, on an
			// `rdf:value` inside it (directly, or through one `rdf:Description`), or an `xml:lang` qualifier element
			// in the same places — the shapes measured to pass. A language anywhere else is not the item's.
			if len(path) >= 2 && fr.space == nsRDF && t.Name.Local == "li" && path[len(path)-1].space == nsRDF {
				if p := path[len(path)-2]; p.space == nsDC && p.raw.Local == "title" {
					fr.titleItem = len(path)
				}
			}
			if i := fr.titleItem; i >= 0 && !f.TitledInALanguage {
				// inItem is whether this element sits where the item's value or qualifiers are written: the item
				// itself, or its child, or a child of the one `rdf:Description` inside it.
				parent := path[len(path)-1]
				inItem := len(path) == i+1 ||
					(len(path) == i+2 && parent.space == nsRDF && parent.raw.Local == "Description")
				switch {
				case len(path) == i && hasXMLLang(t.Attr, resolve):
					f.TitledInALanguage = true
				case inItem && fr.space == nsRDF && t.Name.Local == "value" && hasXMLLang(t.Attr, resolve):
					f.TitledInALanguage = true
				case inItem && fr.space == nsXML && t.Name.Local == "lang":
					f.TitledInALanguage = true
				}
			}

			// Every property in the identification namespace, under the prefix the packet chose. First
			// occurrence wins: a property repeated in one description is refused above, as veraPDF's
			// reader refuses it, and one met again deeper in the packet does not overwrite the first.
			//
			// **The element that introduces a property OWNS its value.** Recording the prefix here and
			// letting any later chardata fill the value let the two come from DIFFERENT elements: a
			// packet writing an empty `<pdfuaia:part/>` and then `<pdfuaid:part>1</pdfuaid:part>`
			// synthesised `{pdfuaia, "1"}`, a pairing no element in the document has, and the prefix
			// and value clauses then judged a property that was never written.
			if fr.space == nsPDFUAID {
				if f.UAProps == nil {
					f.UAProps = map[string]xmpProp{}
				}
				if _, seen := f.UAProps[t.Name.Local]; !seen {
					f.UAProps[t.Name.Local] = xmpProp{Prefix: t.Name.Space}
					fr.owns = t.Name.Local
				}
			}
			// **`rdf:value` written as an ATTRIBUTE is the value too** — on the property itself
			// (`<pdfuaid:part rdf:value="1"/>`) or on its `rdf:Description`; veraPDF passes both, measured by
			// the R1 re-review's third round, and reading `rdf:value` only as an element failed them.
			// The frame's own index once pushed: an anchor naming it is this element.
			if fr.space == nsPDFUAID {
				fr.uaAnchor, fr.anchor = len(path), len(path)
			} else if fr.space == nsDC && t.Name.Local == "title" {
				fr.anchor = len(path)
			}
			for _, a := range t.Attr {
				if a.Name.Local != "value" || a.Name.Space == "" || resolve(a.Name.Space) != nsRDF {
					continue
				}
				owner := ""
				switch {
				case fr.space == nsPDFUAID:
					owner = fr.owns
				case fr.uaAnchor >= 0:
					owner = path[fr.uaAnchor].owns
				}
				if owner != "" {
					p := f.UAProps[owner]
					if !p.hasRDFValue {
						b := buf(owner)
						b.rdf.Reset()
						b.rdf.WriteString(a.Value)
						p.hasRDFValue = true
						f.UAProps[owner] = p
					}
				}
			}

			path = append(path, fr)
		case xml.EndElement:
			// The check `Token()` used to perform. A packet whose tags do not nest is unreadable, not empty.
			if len(path) == 0 {
				f.Why = "the metadata packet is not well-formed XML: an end tag with no open element"
				return f
			}
			last := path[len(path)-1]
			if last.raw != t.Name {
				f.Why = "the metadata packet is not well-formed XML: element closed by a mismatched end tag"
				return f
			}
			for _, prefix := range last.declared {
				if u := ns[prefix]; len(u) > 0 {
					ns[prefix] = u[:len(u)-1]
				}
			}
			if at := len(path) - 1; alt != nil {
				switch it := alt.item; {
				case last.langElem && it != nil && it.elem != nil:
					it.qualified, it.elem = append(it.qualified, it.elem.String()), nil
				case it != nil && last.altItem == at:
					langs := it.direct
					if it.hasValue {
						langs = append(langs, it.qualified...)
					}
					switch len(langs) {
					case 0:
						alt.unlabelled = true
					case 1:
						alt.langs = append(alt.langs, langs[0])
					default:
						return refuse("an item of a language alternative is given its language more than once")
					}
					alt.item = nil
				case at == alt.at:
					if len(alt.langs) > 0 && !alt.unlabelled {
						f.LangAlts = append(f.LangAlts, alt.langs)
					}
					alt = nil
				}
			}
			path = path[:len(path)-1]
		case xml.CharData:
			if len(path) == 0 {
				continue
			}
			// The identification property keeps its character data RAW and WHOLE; `dc:title`'s is not read at all
			// (`TitledInALanguage`, /pending 694). The rawness is measured, not stylistic (the P06 phase-close
			// review): veraPDF reads `pdfuaid:part` as all of the element's text, untrimmed, parsed as an
			// integer — `01` and `+1` pass `5 t2`, while ` 1 `, a newline-wrapped `1` and `1<!---->1` (two runs,
			// "11") fail it. Trimming made the second group false PASSES and the string compare made the first
			// false FAILS.
			raw := string(t)
			// White space is XML's four characters and nothing else: a no-break space is text (measured).
			if here := &path[len(path)-1]; strings.Trim(raw, " \t\r\n") != "" {
				switch here.kind {
				case xkRDF, xkNode:
					return refuse("text is written directly in " + here.raw.Local + ", where only elements may be")
				case xkProp:
					if here.elems > 0 || here.fields {
						return refuse("the property " + here.raw.Local + " holds more than one value")
					}
					here.text = true
				}
			}
			if here := path[len(path)-1]; here.langElem && alt != nil && alt.item != nil && alt.item.elem != nil {
				alt.item.elem.WriteString(raw)
			}
			// The property is the nearest ancestor in a namespace we care about — `dc:title`
			// wraps an `rdf:Alt` wrapping an `rdf:li`, so the character data is three levels
			// down from the element that names the property.
			//
			// **The walk STOPS at the nearest identification property**, filled or not. Continuing
			// outward let a second chardata run under an already-filled property — which a comment or
			// a processing instruction inside the text is enough to produce — land on an ENCLOSING
			// property instead, giving it a value from an element that is not it.
			//
			// That nearest ancestor is the top frame's `anchor`, computed once when each element opened.
			top := path[len(path)-1]
			switch i := top.anchor; {
			case i < 0:
			case path[i].space == nsDC && path[i].raw.Local == "title":
				// A title's text is no part of 7.1 t9's subject; the frame stays an anchor so a run inside it
				// never lands on an identification property enclosing it.
			default: // an identification property
				// **The property's OWN text, or — if it is qualified — its `rdf:value`'s, and nothing else.**
				// Measured by the R1 re-review on veraPDF: the qualified form pretty-printed, and one with a
				// second qualifier, both PASS; reading every descendant's text made them `"\n  \n 1\n"` and
				// `"12"` and failed them. The unqualified form stays raw — ` 1 ` fails there, measured.
				if owns := path[i].owns; owns != "" {
					switch {
					case i == len(path)-1:
						buf(owns).own.WriteString(raw)
					case top.space == nsRDF && top.raw.Local == "value":
						buf(owns).rdf.WriteString(raw)
						p := f.UAProps[owns]
						p.hasRDFValue = true
						f.UAProps[owns] = p
					}
				}
			}
		}
	}
	if len(path) != 0 {
		f.Why = "the metadata packet is not well-formed XML: an element was never closed"
		return f
	}
	for k, p := range f.UAProps {
		if b := bufs[k]; b != nil {
			p.own, p.rdfValue = b.own.String(), b.rdf.String()
		}
		switch {
		case p.hasRDFValue:
			p.Value = p.rdfValue
		case p.own != "":
			p.Value = p.own // an attribute-form value, set when the element opened, is kept otherwise
		}
		f.UAProps[k] = p
	}
	f.UAPart = f.UAProps["part"].Value
	f.Readable = true
	return f
}

// catalogDeclaresLang reports whether the catalog declares a `/Lang` — present counts, even empty
// (`Document.declaresLang` says why).
//
// **What else determines a language, measured on veraPDF's own corpus (`/pending 489`).** For 7.2 t33
// only the catalog key or an `rdf:Alt` item naming a real language does. For 7.2 t34 a marked-content
// `/Lang` does too (`7.2-t34-pass-c.pdf`). This comment used to say P06.S04 had measured the content
// route failing; that section records no such measurement, and the corpus says the opposite. P06.S05's
// measurement stands: a `/Lang` on a form FIELD dictionary does not clear 7.2 t25.
func catalogDeclaresLang(d *Document) bool {
	_, ok := d.catalogLang()
	return ok
}
