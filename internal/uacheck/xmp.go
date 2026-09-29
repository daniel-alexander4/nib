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
// `rdf:parseType="Resource"` shorthand, attribute-form properties (`<rdf:Description dc:title="x"/>`
// rather than `<dc:title>`), or `rdf:Alt` entries beyond taking the first non-empty one. A packet
// using any of those reads as *absent* here, which is the safe direction for a checker — it yields
// `Fail` or `CannotCheck` rather than a pass nib did not establish — but it is a disagreement with
// veraPDF that law 5's guard will surface, and it is written down so the surfacing is expected.

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
	// Title is the first non-empty `dc:title` value found.
	Title string
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
	// LangAlts is every language alternative (`rdf:Alt`) in the packet, each as the `xml:lang` of its
	// items in order — "" for an item that declares none. These are the metadata text 7.2 t33 is
	// about, measured: `dc:title`, `dc:description` and `dc:rights` as `rdf:Alt` give the clause a
	// subject, while `dc:creator` (an `rdf:Seq`) and `xmp:CreateDate` do not.
	//
	// **Grouped per Alt, because the rule is about an Alt** (`/pending 489`): an Alt holding `x-default`
	// AND a real language is determined, and a flat list could not tell that Alt from two Alts.
	LangAlts [][]string
	// Why carries the reason when Readable is false.
	Why string
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
	}
	var path []frame
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
			fr := frame{raw: t.Name, anchor: -1, uaAnchor: -1}
			if len(path) > 0 {
				fr.anchor, fr.uaAnchor = path[len(path)-1].anchor, path[len(path)-1].uaAnchor
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

			// **Attribute-form properties, which RDF/XML calls the abbreviated syntax.** A simple-valued
			// property may be written as an attribute on `rdf:Description` — `pdfuaid:part="1"` — and
			// that is ordinary XMP rather than an exotic shape. veraPDF reads it: measured on a
			// length-preserving mutation of `5-t03-pass-a.pdf`, veraPDF passes all five clause-5 tests
			// on an attribute-form identification while nib, reading start elements only, reported
			// `5 t1` Fail and the other four NotApplicable — a live false fail on a whole serialisation.
			for _, a := range t.Attr {
				switch {
				case uaAttr(a):
					if f.UAProps == nil {
						f.UAProps = map[string]xmpProp{}
					}
					if _, seen := f.UAProps[a.Name.Local]; !seen {
						f.UAProps[a.Name.Local] = xmpProp{Prefix: a.Name.Space, Value: a.Value}
					}
				case a.Name.Space != "" && a.Name.Space != "xmlns" && a.Name.Local == "title" &&
					resolve(a.Name.Space) == nsDC && f.Title == "":
					f.Title = a.Value
				}
			}

			if fr.space == nsRDF && t.Name.Local == "Alt" {
				f.LangAlts = append(f.LangAlts, []string{})
			}
			// An `rdf:li` directly inside an `rdf:Alt` is one item of that alternative.
			if fr.space == nsRDF && t.Name.Local == "li" && len(path) > 0 &&
				path[len(path)-1].space == nsRDF && path[len(path)-1].raw.Local == "Alt" && len(f.LangAlts) > 0 {
				lang := ""
				for _, a := range t.Attr {
					// **By URI, not by prefix.** The const block above states the contract: a prefix is
					// the document's choice. `RawToken` hands back the raw prefix, so the resolution is
					// done here rather than assumed — a packet binding some other prefix to the XML
					// namespace would otherwise lose every language and fail 7.2 t33.
					//
					// **A DECLARED red-proof survivor.** Rewriting this to compare the literal prefix
					// leaves the package green, and that is correct rather than a coverage hole: XML
					// Namespaces forbids binding any prefix but `xml` to this URI, so no conforming
					// document reaches the difference. It is written the contract's way because the
					// contract is what the next reader will trust, and because `Token()` did it this
					// way before the rewrite. veraPDF's own behaviour on the non-conforming shape is
					// unmeasured and is deliberately not asserted here.
					if a.Name.Space != "" && a.Name.Space != "xmlns" &&
						resolve(a.Name.Space) == nsXML && a.Name.Local == "lang" {
						lang = a.Value
					}
				}
				last := len(f.LangAlts) - 1
				f.LangAlts[last] = append(f.LangAlts[last], lang)
			}
			// Every property in the identification namespace, under the prefix the packet chose. First
			// occurrence wins, as `Title` does — a repeated property is the packet's problem, not a
			// reason for the later one to overwrite what the earlier said.
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
			path = path[:len(path)-1]
		case xml.CharData:
			if len(path) == 0 {
				continue
			}
			// The identification property keeps its character data RAW and WHOLE; `dc:title` keeps its first
			// trimmed run, as it always has. The difference is measured, not stylistic (the P06 phase-close
			// review): veraPDF reads `pdfuaid:part` as all of the element's text, untrimmed, parsed as an
			// integer — `01` and `+1` pass `5 t2`, while ` 1 `, a newline-wrapped `1` and `1<!---->1` (two runs,
			// "11") fail it. Trimming made the second group false PASSES and the string compare made the first
			// false FAILS.
			raw := string(t)
			text := strings.TrimSpace(raw)
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
				if f.Title == "" && text != "" {
					f.Title = text
				}
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
