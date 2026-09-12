package pdfops

import (
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The catalog floor — `PLAN-accessibility.md` P03.S01, PDF/UA clauses 7.1 t8, 7.1 t9 and 7.1 t10.
//
// # What a nib-authored document fails, measured
//
// `CreateFromJSON`'s output through veraPDF ua1 fails **seven** clauses. Four are catalog-only and
// three of those are this slice's:
//
//	7.1 t8   the catalog shall contain a /Metadata key whose value is a metadata stream
//	7.1 t9   that stream shall carry a dc:title which clearly identifies the document
//	7.1 t10  the catalog shall contain ViewerPreferences with DisplayDocTitle true
//
// The fourth, 6.2 t1 (`/MarkInfo /Marked true`), is **deliberately not here**: setting it without a
// `/StructTreeRoot` is exactly `tagState.orphaned()`, which ADR-031's law 1 forbids and P01.S06
// built a door to prevent. It arrives with the tree, in P05. The remaining three clauses need a
// structure tree (P05) or embedded fonts (P04).
//
// # Why the caller supplies the title
//
// PDF/UA asks for a title that *clearly identifies* the document. A generic one — "Document" —
// would satisfy a naive check while being precisely the thing the clause exists to prevent, and the
// primitives cannot do better: `CreateFromJSON` does not know whether it is making a trust
// explainer or a signature page. **Its callers do**, so the title is a parameter and every caller
// passes its own.
//
// # Why this is authoring-only
//
// It is never applied to a document that arrived from somewhere else. Two reasons, and the second
// is the one that would bite: inventing a title for a user's own document is a claim nib has no
// basis for, and rewriting a document to add a catalog key moves every byte after it — which
// invalidates any signature over them. Authored output has no signature yet and no title to
// overwrite.

// xmpPacket is the XMP metadata stream, built by hand because pdfcpu has no writer for one —
// `model/metadata.go` unmarshals XMP and never emits it.
//
// **The `xml:` tags do the escaping.** A title carrying `&` or `<` — a filename can — would
// otherwise produce a packet that is not well-formed, and a malformed XMP stream is worse than an
// absent one: it fails the clause *and* breaks readers that try to parse it.
type xmpPacket struct {
	XMLName xml.Name `xml:"x:xmpmeta"`
	XmlnsX  string   `xml:"xmlns:x,attr"`
	RDF     xmpRDF   `xml:"rdf:RDF"`
}

type xmpRDF struct {
	XmlnsRDF    string         `xml:"xmlns:rdf,attr"`
	Description xmpDescription `xml:"rdf:Description"`
}

type xmpDescription struct {
	About      string `xml:"rdf:about,attr"`
	XmlnsDC    string `xml:"xmlns:dc,attr"`
	XmlnsXMP   string `xml:"xmlns:xmp,attr"`
	Title      xmpAlt `xml:"dc:title"`
	CreateDate string `xml:"xmp:CreateDate,omitempty"`
}

type xmpAlt struct {
	Seq xmpAltSeq `xml:"rdf:Alt"`
}

type xmpAltSeq struct {
	Li xmpAltLi `xml:"rdf:li"`
}

type xmpAltLi struct {
	Lang  string `xml:"xml:lang,attr"`
	Value string `xml:",chardata"`
}

// buildXMP renders the packet for one title.
func buildXMP(title string, when time.Time) ([]byte, error) {
	p := xmpPacket{
		XmlnsX: "adobe:ns:meta/",
		RDF: xmpRDF{
			XmlnsRDF: "http://www.w3.org/1999/02/22-rdf-syntax-ns#",
			Description: xmpDescription{
				About:      "",
				XmlnsDC:    "http://purl.org/dc/elements/1.1/",
				XmlnsXMP:   "http://ns.adobe.com/xap/1.0/",
				Title:      xmpAlt{Seq: xmpAltSeq{Li: xmpAltLi{Lang: "x-default", Value: title}}},
				CreateDate: when.UTC().Format(time.RFC3339),
			},
		},
	}
	body, err := xml.Marshal(p)
	if err != nil {
		return nil, err
	}
	// The packet wrapper is required by XMP (ISO 16684-1) and by every reader that scans for it.
	// **`\ufeff` escaped, not written literally.** The XMP packet header carries a UTF-8 BOM by
	// specification, and a literal one in a Go source file is a compile error ("illegal byte order
	// mark") — which is how this was found rather than reasoned.
	return []byte(fmt.Sprintf("<?xpacket begin=\"\ufeff\" id=\"W5M0MpCehiHzreSzNTczkc9d\"?>\n%s\n<?xpacket end=\"w\"?>", body)), nil
}

// SetTitle gives a document the catalog floor: an XMP `/Metadata` stream carrying `dc:title`, the
// Info dict's `/Title` to match, and `ViewerPreferences /DisplayDocTitle true`.
//
// **One door for all three**, because they are one fact about the document said three ways and a
// document carrying two of them is a worse state than one carrying none — a viewer told to display
// a title it cannot find shows an empty chrome bar.
//
// An empty title is refused rather than written: `dc:title` present and blank satisfies a scan for
// the key and fails the clause it exists for.
func SetTitle(pdf []byte, title string) ([]byte, error) {
	if title == "" {
		return nil, fmt.Errorf("pdfops: SetTitle needs a title — an empty dc:title satisfies a key check and fails the clause")
	}
	return writeMutated(pdf, func(ctx *model.Context) error {
		xt := ctx.XRefTable
		root, err := xt.Catalog()
		if err != nil {
			return err
		}

		packet, err := buildXMP(title, time.Now())
		if err != nil {
			return err
		}
		sd, err := ctx.NewStreamDictForBuf(packet)
		if err != nil {
			return err
		}
		// **`/Type /Metadata /Subtype /XML` is not decoration** — it is how a reader knows this
		// stream is XMP rather than an arbitrary attachment, and veraPDF checks for it.
		sd.Dict["Type"] = types.Name("Metadata")
		sd.Dict["Subtype"] = types.Name("XML")
		if err := sd.Encode(); err != nil {
			return err
		}
		ref, err := ctx.IndRefForNewObject(*sd)
		if err != nil {
			return err
		}
		root["Metadata"] = *ref

		// The Info dict's /Title, so the two places a title can live agree. A reader that prefers
		// Info over XMP — several do — otherwise shows the old title or none.
		if err := setInfoTitle(ctx, title); err != nil {
			return err
		}

		yes := true
		if ctx.ViewerPref == nil {
			ctx.ViewerPref = &model.ViewerPreferences{}
		}
		ctx.ViewerPref.DisplayDocTitle = &yes
		xt.BindViewerPreferences()
		return nil
	})
}

// setInfoTitle writes /Title into the Info dict, creating the dict when the document has none.
func setInfoTitle(ctx *model.Context, title string) error {
	if ctx.Info == nil {
		ref, err := ctx.IndRefForNewObject(types.Dict{})
		if err != nil {
			return err
		}
		ctx.Info = ref
	}
	d, err := ctx.DereferenceDict(*ctx.Info)
	if err != nil {
		return err
	}
	if d == nil {
		return fmt.Errorf("pdfops: the Info reference does not resolve to a dictionary")
	}
	d["Title"] = types.StringLiteral(types.EncodeUTF16String(title))
	return nil
}

// TitleFromFilename derives a document title from a file name: the base name with its extension
// stripped. It is the ONE derivation, shared by the three call sites that have a file name and no
// better title — a per-package copy is how the same rule comes to mean three things (ADR-009).
//
// **It returns "" rather than inventing a fallback.** A document whose name yields nothing has no
// title nib can honestly state, and "Document" is exactly the generic label PDF/UA 7.1 t9 exists to
// refuse. The callers guard on the empty string and write no title at all, which fails the clause
// truthfully instead of passing it falsely.
//
// The separator split is deliberate and is not `filepath.Base`: two of the three callers take the
// name from a multipart header, where the client chooses it, so `C:\docs\report.docx` must yield
// `report` on a machine whose separator is `/`.
func TitleFromFilename(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSuffix(name, filepath.Ext(name))
	return strings.TrimSpace(name)
}

// TitleFromName is the door the receiving call sites use: it derives a title from a file name and
// applies it, and **it never costs the caller the document**.
//
// The returned bytes are always usable — the titled document on success, the original on any
// failure — so a caller writes `pdf, err = TitleFromName(pdf, name)` and uses `pdf` either way. The
// error is there to be LOGGED, not to be returned to a user.
//
// **This exists because the first version let a metadata write fail the whole operation, at all
// three sites.** `handleOffice` converted a Word document, titled it, and on a title failure
// answered **400 with the error text** — a server-side metadata write reported as the client's bad
// request, discarding a conversion that had succeeded. `handleAssemble` turned it into a 500 and
// `cmdOffice` into a non-zero exit. The repo already had the rule and it was written eight lines
// from a working example: `internal/server/ocr.go`'s `SetLang` call says *"Best-effort: a failure
// here must not fail the OCR itself."*
//
// One door rather than three copies of that judgment (ADR-009) — and the routing guard in
// `titledoor_test.go` accepts either this or `SetTitle`, because a site that calls `SetTitle`
// directly has made the same decision explicitly.
func TitleFromName(pdf []byte, name string) ([]byte, error) {
	title := TitleFromFilename(name)
	if title == "" {
		return pdf, nil
	}
	out, err := SetTitle(pdf, title)
	if err != nil {
		return pdf, fmt.Errorf("pdfops: could not title %q: %w", title, err)
	}
	return out, nil
}
