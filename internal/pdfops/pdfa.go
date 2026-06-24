package pdfops

import (
	"bytes"
	_ "embed"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// PDF/A is the ISO 19005 archival profile: a self-contained PDF a validator can
// vouch for years from now. PreparePDFA targets PDF/A-2b (ISO 19005-2, level "b"
// = visual appearance, no tagging) — chosen over A-1b because PDF/A-2 permits
// transparency, LZW, and object/xref streams (all of which pdfcpu's writer emits)
// and, unlike A-1, does NOT require the /Info dictionary to be mirrored in XMP,
// so a minimal pdfaid-only XMP packet is conformant.
//
// pdfcpu has zero PDF/A support, so the three conformance pillars are built here
// over its low-level XRefTable/types primitives. Two are mechanical and done:
// the sRGB OutputIntent (embedded ICC) and the identification XMP. The third —
// embedding a font that is referenced but not embedded — is impossible without
// the font program and a remediation subsetter pdfcpu does not offer, so such a
// document is refused rather than silently mis-labelled. Conformance itself
// cannot be validated in-process (no pure-Go PDF/A validator exists), so the
// output is a *candidate*: callers must say "verify with veraPDF", never
// "certified PDF/A".

// srgbICC is the ICC's reference sRGB profile (sRGB2014.icc, freely
// redistributable), embedded as the OutputIntent's DestOutputProfile so that
// DeviceRGB and DeviceGray content becomes device-independent.
//
//go:embed sRGB2014.icc
var srgbICC []byte

// xmpPDFA2B is the minimal conformant XMP packet for a PDF/A-2b file: it carries
// only the mandatory PDF/A identification (pdfaid:part 2, conformance B). Emitting
// nothing else sidesteps the Info↔XMP property-consistency rule that trips up
// PDF/A-1 — PDF/A-2 does not require the document information dictionary to be
// mirrored, so a pdfaid-only packet is both sufficient and safest. The packet is
// stored as an unfiltered stream (a PDF/A requirement, so any tool can read it).
const xmpPDFA2B = "<?xpacket begin=\"\uFEFF\" id=\"W5M0MpCehiHzreSzNTczkc9d\"?>\n" +
	"<x:xmpmeta xmlns:x=\"adobe:ns:meta/\">\n" +
	" <rdf:RDF xmlns:rdf=\"http://www.w3.org/1999/02/22-rdf-syntax-ns#\">\n" +
	"  <rdf:Description rdf:about=\"\" xmlns:pdfaid=\"http://www.aiim.org/pdfa/ns/id/\">\n" +
	"   <pdfaid:part>2</pdfaid:part>\n" +
	"   <pdfaid:conformance>B</pdfaid:conformance>\n" +
	"  </rdf:Description>\n" +
	" </rdf:RDF>\n" +
	"</x:xmpmeta>\n" +
	"<?xpacket end=\"w\"?>"

// PreparePDFA converts pdf into a PDF/A-2b archival candidate. When the document
// can be made conformant it returns the converted bytes (blockers nil). When a
// requirement Nib cannot satisfy is present — non-embedded fonts, or encryption —
// it returns those as human-readable blockers (data nil) so the caller can
// explain why, rather than emit a file that falsely claims PDF/A. err is reserved
// for genuine processing failures.
//
// The conversion strips active content and embedded files (forbidden or
// non-conformant in PDF/A-2) by reusing StripActive, then injects the sRGB
// OutputIntent and the identification XMP. It does not, and cannot, fix every
// possible non-conformance (e.g. DeviceCMYK colour or annotations lacking
// appearance streams), so the result must be verified with veraPDF.
func PreparePDFA(pdf []byte) (data []byte, blockers []string, err error) {
	info, err := api.PDFInfo(bytes.NewReader(pdf), "", nil, true, model.NewDefaultConfiguration())
	if err != nil {
		if isEncryptionErr(err) {
			return nil, []string{pdfaEncryptedMsg}, nil
		}
		return nil, nil, err
	}
	if info.Encrypted {
		return nil, []string{pdfaEncryptedMsg}, nil
	}

	if missing := nonEmbeddedFonts(info); len(missing) > 0 {
		blockers = append(blockers, "these fonts are not embedded, which PDF/A requires and Nib cannot add: "+
			strings.Join(missing, ", ")+". Re-export the document as PDF/A from the application that created it.")
	}
	if len(blockers) > 0 {
		return nil, blockers, nil
	}

	cleaned, err := StripActive(pdf)
	if err != nil {
		return nil, nil, err
	}
	out, err := writeMutated(cleaned, injectPDFAMarkers)
	if err != nil {
		return nil, nil, err
	}
	return out, nil, nil
}

const pdfaEncryptedMsg = "the document is password-protected — remove the password first (Secure → Remove password), then convert"

// nonEmbeddedFonts returns the de-duplicated names of fonts the PDF references
// without embedding. PDF/A requires every font (including the standard-14) to be
// embedded; pdfcpu reports the standard-14 as non-embedded, which is correct here.
func nonEmbeddedFonts(info *pdfcpu.PDFInfo) []string {
	var missing []string
	seen := map[string]bool{}
	for _, f := range info.Fonts {
		if f.Embedded {
			continue
		}
		name := f.Name
		if name == "" {
			name = "(unnamed)"
		}
		if !seen[name] {
			seen[name] = true
			missing = append(missing, name)
		}
	}
	return missing
}

// injectPDFAMarkers adds the sRGB OutputIntent (with embedded ICC profile) and the
// PDF/A identification XMP metadata stream to the document catalog.
func injectPDFAMarkers(ctx *model.Context) error {
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		return err
	}

	// sRGB OutputIntent. The ICC profile stream may be Flate-compressed.
	iccSD, err := xt.NewStreamDictForBuf(srgbICC)
	if err != nil {
		return err
	}
	iccSD.InsertInt("N", 3)
	if err := iccSD.Encode(); err != nil {
		return err
	}
	iccRef, err := xt.IndRefForNewObject(*iccSD)
	if err != nil {
		return err
	}
	oi := types.NewDict()
	oi.InsertName("Type", "OutputIntent")
	oi.InsertName("S", "GTS_PDFA1") // the OutputIntent subtype is GTS_PDFA1 for every PDF/A part
	oi.Insert("OutputConditionIdentifier", types.StringLiteral("sRGB"))
	oi.Insert("Info", types.StringLiteral("sRGB IEC61966-2.1"))
	oi.Insert("DestOutputProfile", *iccRef)
	oiRef, err := xt.IndRefForNewObject(oi)
	if err != nil {
		return err
	}
	root["OutputIntents"] = types.Array{*oiRef}

	// PDF/A identification XMP — must be an UNFILTERED stream (FilterPipeline nil
	// → Encode writes it uncompressed). Overwrites any pre-existing /Metadata.
	xmpSD := &types.StreamDict{Dict: types.NewDict(), Content: []byte(xmpPDFA2B)}
	xmpSD.InsertName("Type", "Metadata")
	xmpSD.InsertName("Subtype", "XML")
	if err := xmpSD.Encode(); err != nil {
		return err
	}
	xmpRef, err := xt.IndRefForNewObject(*xmpSD)
	if err != nil {
		return err
	}
	root["Metadata"] = *xmpRef

	clearImageInterpolate(xt)
	return nil
}

// clearImageInterpolate removes the /Interpolate key from every image XObject.
// PDF/A-2 §6.2.8 requires it to be false if present, and pdfcpu's image import
// sets it true; absence is the default-false the rule wants. (Inline images carry
// the analogous /I in content streams and are out of scope — Nib's own images are
// XObjects, and veraPDF would flag any inline case.)
func clearImageInterpolate(xt *model.XRefTable) {
	for _, e := range xt.Table {
		if e == nil {
			continue
		}
		sd, ok := e.Object.(types.StreamDict)
		if !ok {
			continue
		}
		if n := sd.Dict.NameEntry("Subtype"); n != nil && *n == "Image" {
			delete(sd.Dict, "Interpolate")
		}
	}
}

// isEncryptionErr reports whether err is pdfcpu refusing to read an encrypted PDF
// without the password, so PreparePDFA can return a clear blocker instead.
func isEncryptionErr(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "password") || strings.Contains(s, "encrypt")
}
