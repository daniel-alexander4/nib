package testpdf

import (
	"encoding/xml"
	"errors"
	"io"
	"nib/internal/pdfread"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The PDF/UA identification, for tests of the rule that nothing carries one nib did not verify
// (`/pending 492`). One fixture and one reader for every package that asks, so the population they
// test is the same document read the same way.

const uaNS = "http://www.aiim.org/pdfua/ns/id/"

// pdfaNS is the PDF/A identification's namespace, dropped by the same rule (ADR-083).
const pdfaNS = "http://www.aiim.org/pdfa/ns/id/"

// WithUAIdentification returns pdf with `pdfuaid:part 1` added to its catalog XMP packet, written
// DIRECTLY through pdfcpu — not through nib's own write path, which removes exactly this. pdf must already
// carry a packet (a titled document does).
func WithUAIdentification(pdf []byte) ([]byte, error) {
	return withIdentification(pdf, `<rdf:Description rdf:about="" xmlns:pdfuaid="`+uaNS+`"><pdfuaid:part>1</pdfuaid:part></rdf:Description>`)
}

// WithPDFAIdentification is WithUAIdentification for `pdfaid:part 2` / `pdfaid:conformance B` — the claim
// alone, over a document that need not conform, which is exactly the shape the rule is about.
func WithPDFAIdentification(pdf []byte) ([]byte, error) {
	return withIdentification(pdf, `<rdf:Description rdf:about="" xmlns:pdfaid="`+pdfaNS+`"><pdfaid:part>2</pdfaid:part><pdfaid:conformance>B</pdfaid:conformance></rdf:Description>`)
}

func withIdentification(pdf []byte, description string) ([]byte, error) {
	ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		return nil, err
	}
	ir, ok := root["Metadata"].(types.IndirectRef)
	if !ok {
		return nil, errors.New("testpdf: the document has no indirect /Metadata packet to label — title it first")
	}
	sd, _, err := ctx.XRefTable.DereferenceStreamDict(ir)
	if err != nil || sd == nil {
		return nil, errors.New("testpdf: /Metadata does not resolve to a stream")
	}
	if err := sd.Decode(); err != nil {
		return nil, err
	}
	s := string(sd.Content)
	if !strings.Contains(s, "</rdf:RDF>") {
		return nil, errors.New("testpdf: the packet has no rdf:RDF to add a Description to")
	}
	sd.Content = []byte(strings.Replace(s, "</rdf:RDF>", description+"</rdf:RDF>", 1))
	if err := sd.Encode(); err != nil {
		return nil, err
	}
	entry, found := ctx.XRefTable.FindTableEntryForIndRef(&ir)
	if !found {
		return nil, errors.New("testpdf: no xref entry for /Metadata")
	}
	entry.Object = *sd
	out, err := pdfread.Write(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ClaimsUA reports whether pdf's catalog packet carries an element or attribute IN the identification
// namespace — the claim. A PDF/A extension-schema block names the namespace as text and claims nothing,
// so the string alone is not the question (measured on veraPDF's corpus).
func ClaimsUA(pdf []byte) (bool, error) {
	ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		return false, err
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		return false, err
	}
	o, ok := root["Metadata"]
	if !ok {
		return false, nil
	}
	sd, _, err := ctx.XRefTable.DereferenceStreamDict(o)
	if err != nil || sd == nil {
		return false, nil
	}
	if err := sd.Decode(); err != nil {
		return false, err
	}
	return PacketClaimsUA(string(sd.Content)), nil
}

// PacketClaimsUA is ClaimsUA over a packet already in hand.
//
// **It fails CLOSED** (`/pending 709` R2-11). It is the oracle for "nothing carries an identification
// it did not verify", so a packet the decoder rejects part-way — an undeclared entity, a stray `&` —
// must not read as "no claim": a reader more lenient than Go's decoder still sees the identification
// after the point this one stopped. On a decode error the answer falls back to whether the namespace
// is named at all, which over-reports (a PDF/A extension schema names it and claims nothing) and so can
// fail a test, never pass one.
func PacketClaimsUA(packet string) bool { return packetClaims(packet, uaNS) }

// PacketClaimsPDFA is PacketClaimsUA for the PDF/A identification, failing closed the same way.
func PacketClaimsPDFA(packet string) bool { return packetClaims(packet, pdfaNS) }

func packetClaims(packet, ns string) bool {
	dec := xml.NewDecoder(strings.NewReader(packet))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return false
		}
		if err != nil {
			return strings.Contains(packet, ns)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Space == ns {
			return true
		}
		for _, a := range se.Attr {
			if a.Name.Space == ns {
				return true
			}
		}
	}
}
