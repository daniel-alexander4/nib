package pdfops

import (
	"bytes"
	"encoding/xml"
	"errors"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The PDF/UA identification nib writes — `/pending 486`, option B, ADR-033.
//
// nib's checker covers 70 of the 106 rules veraPDF evaluates, so a report of "every clause passes" cannot
// support a conformance claim, and nib never writes one on its checker's say-so. What it CAN support is a
// claim about its own output: a Markdown conversion, tagged from its own AST, titled, declared in a
// language someone asserted, is veraPDF-compliant across every construct mdpdf emits — measured, and held
// by `TestNibsOwnMarkdownConversionConformsAcrossEveryConstruct`. That door, and only that door, labels.
//
// **Every refusal is its own error**, because each is a different reason a user's document went out
// without the label, and a lumped "not labelled" reads the same whichever one it was.
var (
	ErrUALanguageNotAsserted = errors.New("the language was not asserted by anyone — a pre-filled default is a guess, and a conformance claim cannot rest on one")
	ErrUAUntagged            = errors.New("the document is not tagged, or its tag tree describes nothing in it")
	ErrUANoLanguage          = errors.New("the document declares no language")
	ErrUANoTitle             = errors.New("the document has no dc:title, or does not ask viewers to display it")
	ErrUANoPacket            = errors.New("the document has no metadata packet nib wrote to add the identification to")
)

const uaIdentificationXML = `<rdf:Description rdf:about="" xmlns:pdfuaid="` + pdfuaidNS + `"><pdfuaid:part>1</pdfuaid:part></rdf:Description>`

// LabelUA writes `pdfuaid:part 1` into a document nib just converted from Markdown, or refuses with the
// reason. langAsserted is the caller's fact: the person chose the language (`nib office --lang`, or a
// Document language the user picked) rather than nib pre-filling one.
//
// **Callers are the two Markdown conversion doors and nothing else.** What makes the claim true is the
// conversion's provenance, which the bytes cannot show — so the rule is kept at the call sites and the
// census over every other operation (`TestNoOperationCarriesAnIdentificationItDidNotVerify`) names this
// one as its only exemption. It must be the LAST write: `SetTitle` rebuilds the packet, and every change
// after this one drops the claim again (ADR-032), which is exactly right.
func LabelUA(pdf []byte, langAsserted bool) ([]byte, error) {
	if !langAsserted {
		return nil, ErrUALanguageNotAsserted
	}
	if s := inspectTags(pdf); !s.supportsAClaim() {
		return nil, ErrUAUntagged
	}
	return writeMutated(pdf, func(ctx *model.Context) error {
		xt := ctx.XRefTable
		root, err := xt.Catalog()
		if err != nil {
			return err
		}
		if readLang(xt, root["Lang"]) == "" {
			return ErrUANoLanguage
		}
		if ctx.ViewerPref == nil || ctx.ViewerPref.DisplayDocTitle == nil || !*ctx.ViewerPref.DisplayDocTitle {
			return ErrUANoTitle
		}
		obj, ok := root["Metadata"]
		if !ok {
			return ErrUANoPacket
		}
		sd, _, err := xt.DereferenceStreamDict(obj)
		if err != nil || sd == nil || sd.Decode() != nil {
			return ErrUANoPacket
		}
		packet := string(sd.Content)
		if !packetHasTitle(packet) {
			return ErrUANoTitle
		}
		// The packet is the one `SetTitle` builds, which binds `rdf:`. A packet shaped otherwise is not one
		// nib wrote, and guessing where a Description goes in it is how a claim lands somewhere no reader
		// looks.
		at := strings.LastIndex(packet, "</rdf:RDF>")
		if at < 0 {
			return ErrUANoPacket
		}
		sd.Content = []byte(packet[:at] + uaIdentificationXML + packet[at:])
		if err := sd.Encode(); err != nil {
			return err
		}
		if ir, isRef := obj.(types.IndirectRef); isRef {
			entry, found := xt.FindTableEntryForIndRef(&ir)
			if !found {
				return ErrUANoPacket
			}
			entry.Object = *sd
			return nil
		}
		root["Metadata"] = *sd
		return nil
	})
}

// packetHasTitle reports whether a packet carries a non-empty `dc:title`, read by namespace.
func packetHasTitle(packet string) bool {
	const nsDC = "http://purl.org/dc/elements/1.1/"
	dec := xml.NewDecoder(bytes.NewReader([]byte(packet)))
	inTitle := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space == nsDC && t.Name.Local == "title" {
				inTitle++
			} else if inTitle > 0 {
				inTitle++
			}
		case xml.EndElement:
			if inTitle > 0 {
				inTitle--
			}
		case xml.CharData:
			if inTitle > 0 && strings.TrimSpace(string(t)) != "" {
				return true
			}
		}
	}
}
