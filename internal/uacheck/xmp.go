package uacheck

import (
	"encoding/xml"
	"io"
	"strings"

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
	// LangAlts is every item of every language alternative (`rdf:Alt`) in the packet, with the
	// `xml:lang` it declares — "" when it declares none. These are the metadata text 7.2 t33 is
	// about, measured: `dc:title`, `dc:description` and `dc:rights` as `rdf:Alt` give the clause a
	// subject, while `dc:creator` (an `rdf:Seq`) and `xmp:CreateDate` do not.
	LangAlts []string
	// Why carries the reason when Readable is false.
	Why string
}

// The namespaces the clauses name. Compared by URI rather than by prefix, because a prefix is the
// document's choice — a packet may bind `dc:` to something else entirely, and a reader trusting the
// prefix would read the wrong property and report a pass.
const (
	nsDC      = "http://purl.org/dc/elements/1.1/"
	nsPDFUAID = "http://www.aiim.org/pdfua/ns/id/"
)

// readXMP gathers what the metadata rules need, in one pass over the packet.
func readXMP(d *Document) xmpFacts {
	raw, has := d.Catalog["Metadata"]
	if !has {
		return xmpFacts{}
	}
	sd, _, err := d.Ctx.DereferenceStreamDict(raw)
	if err != nil || sd == nil {
		return xmpFacts{Present: true, Why: "the catalog's /Metadata does not resolve to a stream"}
	}
	f := xmpFacts{Present: true}
	if n := sd.Dict.NameEntry("Type"); n != nil {
		f.StreamType = *n
	}
	if n := sd.Dict.NameEntry("Subtype"); n != nil {
		f.StreamSubtype = *n
	}
	if derr := sd.Decode(); derr != nil {
		f.Why = "the metadata stream could not be decoded: " + derr.Error()
		return f
	}
	dec := xml.NewDecoder(strings.NewReader(string(sd.Content)))
	// **Namespace-aware but tolerant of a malformed packet.** A packet that is not well-formed XML
	// is Readable=false, and the rules turn that into `CannotCheck` — the document may well carry
	// what the clause wants, and nib is the thing that could not read it.
	var path []xml.Name
	const nsRDF = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	const nsXML = "http://www.w3.org/XML/1998/namespace"
	for {
		tok, terr := dec.Token()
		if terr == io.EOF {
			break
		}
		if terr != nil {
			f.Why = "the metadata packet is not well-formed XML: " + terr.Error()
			return f
		}
		switch t := tok.(type) {
		case xml.StartElement:
			// An `rdf:li` directly inside an `rdf:Alt` is one language alternative.
			if t.Name.Space == nsRDF && t.Name.Local == "li" && len(path) > 0 &&
				path[len(path)-1].Space == nsRDF && path[len(path)-1].Local == "Alt" {
				lang := ""
				for _, a := range t.Attr {
					if a.Name.Space == nsXML && a.Name.Local == "lang" {
						lang = a.Value
					}
				}
				f.LangAlts = append(f.LangAlts, lang)
			}
			path = append(path, t.Name)
		case xml.EndElement:
			if len(path) > 0 {
				path = path[:len(path)-1]
			}
		case xml.CharData:
			if len(path) == 0 {
				continue
			}
			text := strings.TrimSpace(string(t))
			if text == "" {
				continue
			}
			// The property is the nearest ancestor in a namespace we care about — `dc:title`
			// wraps an `rdf:Alt` wrapping an `rdf:li`, so the character data is three levels
			// down from the element that names the property.
			for i := len(path) - 1; i >= 0; i-- {
				switch {
				case path[i].Space == nsDC && path[i].Local == "title" && f.Title == "":
					f.Title = text
				case path[i].Space == nsPDFUAID && path[i].Local == "part" && f.UAPart == "":
					f.UAPart = text
				default:
					continue
				}
				break
			}
		}
	}
	f.Readable = true
	return f
}

// catalogLang returns the catalog's `/Lang`, trimmed.
//
// **The catalog key is the only thing that determines a document's language for `7.2 t33` and
// `7.2 t34`, measured rather than assumed.** P06.S04 put a `/Lang` on the CONTENT via
// `/Span <</Lang (en)>> BDC` and 7.2 t34 still failed; P06.S05 put one on a form FIELD dictionary
// and 7.2 t25 still failed. Both times the catalog key cleared it and nothing else did.
func catalogLang(d *Document) string {
	s, _ := d.Catalog["Lang"].(types.StringLiteral)
	if s == "" {
		return ""
	}
	dec, err := types.StringLiteralToString(s)
	if err != nil {
		return strings.TrimSpace(string(s))
	}
	return strings.TrimSpace(dec)
}
