package testpdf

import (
	"bytes"
	"encoding/xml"
	"errors"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The PDF/UA identification, for tests of the rule that nothing carries one nib did not verify
// (`/pending 492`). One fixture and one reader for every package that asks, so the population they
// test is the same document read the same way.

const uaNS = "http://www.aiim.org/pdfua/ns/id/"

// WithUAIdentification returns pdf with `pdfuaid:part 1` added to its catalog XMP packet, written
// DIRECTLY through pdfcpu — not through nib's own write path, which removes exactly this. pdf must already
// carry a packet (a titled document does).
func WithUAIdentification(pdf []byte) ([]byte, error) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
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
	sd.Content = []byte(strings.Replace(s, "</rdf:RDF>",
		`<rdf:Description rdf:about="" xmlns:pdfuaid="`+uaNS+`"><pdfuaid:part>1</pdfuaid:part></rdf:Description></rdf:RDF>`, 1))
	if err := sd.Encode(); err != nil {
		return nil, err
	}
	entry, found := ctx.XRefTable.FindTableEntryForIndRef(&ir)
	if !found {
		return nil, errors.New("testpdf: no xref entry for /Metadata")
	}
	entry.Object = *sd
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// ClaimsUA reports whether pdf's catalog packet carries an element or attribute IN the identification
// namespace — the claim. A PDF/A extension-schema block names the namespace as text and claims nothing,
// so the string alone is not the question (measured on veraPDF's corpus).
func ClaimsUA(pdf []byte) (bool, error) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
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
func PacketClaimsUA(packet string) bool {
	dec := xml.NewDecoder(strings.NewReader(packet))
	for {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Space == uaNS {
			return true
		}
		for _, a := range se.Attr {
			if a.Name.Space == uaNS {
				return true
			}
		}
	}
}
