package pdfops

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"nib/internal/pdfread"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The PDF/UA identification, and the rule that nothing carries one nib did not verify — `/pending 492`.
//
// # The defect
//
// `pdfuaid:part` in the catalog's XMP packet claims the WHOLE document conforms to PDF/UA. pdfcpu carries
// the catalog `/Metadata` stream through an ordinary write, so a labelled document nib edited kept the
// claim whatever the edit did. Measured on veraPDF's own PDF/UA-1 corpus: `AddNotes` keeps it and fails
// 7.18.1 t1, `StampWatermark` keeps it and fails 7.21.4.1 t1 — on every one of four labelled files, in
// both the element and the attribute encoding. ADR-031 law 1 names that shape: a conformance assertion
// over content it no longer describes.
//
// # The remedy, and why it is not a check
//
// nib cannot tell whether an edit preserved conformance — its checker covers only part of the 106 rules — so it does
// not ask. Any change drops the identification, including the changes that happened to keep a document
// conformant (`Rotate`, `Optimize`). That is a visible loss in place of a false claim, the trade ADR-031
// already makes for tagging.

// pdfuaidNS is the identification schema's namespace. Matched by URI, never by prefix: a packet is free
// to bind `pdfuaid` to anything, or the schema to another prefix.
const pdfuaidNS = "http://www.aiim.org/pdfua/ns/id/"

// pdfaidNS is the PDF/A identification schema's namespace (ISO 19005), dropped by the same door for the
// same reason (ADR-083, `/pending 641`): `pdfaid:part` claims the whole document conforms, nib verifies
// none of PDF/A's rules, and pdfcpu carries the packet through every write — measured, `Rotate` kept it
// on `PreparePDFA`'s output and on Ghostscript's. Its extension-schema neighbours (`.../pdfa/ns/extension/`,
// `/schema#`, `/property#`) are other URIs, describe a schema and claim nothing, so they stay.
const pdfaidNS = "http://www.aiim.org/pdfa/ns/id/"

// isIdentificationNS reports whether uri is one of the conformance identifications nothing may carry
// through a change nib did not verify.
func isIdentificationNS(uri string) bool { return uri == pdfuaidNS || uri == pdfaidNS }

// errUnparsedClaim means a packet names an identification schema and could not be rewritten.
var errUnparsedClaim = errors.New("the metadata packet names the PDF/UA or PDF/A identification schema and is not well-formed XML")

// withoutUAIdentification returns the packet with every element and attribute in either identification
// namespace (PDF/UA's and PDF/A's, `isIdentificationNS`) removed, and whether anything was.
//
// **Token by token, re-serialised by hand**, because `encoding/xml`'s encoder rewrites namespace
// declarations and escapes whitespace as character references — a packet's padding and prefixes would
// change on every edit, and a claim is not a reason to disturb the rest of someone's metadata. The
// namespace bookkeeping is done here too, per element scope, since `RawToken` reports prefixes only.
func withoutUAIdentification(packet []byte) ([]byte, bool, error) {
	// Well-formedness first, with `Token`: `RawToken` does not check that an end tag matches its start,
	// so the rewriting pass below would re-serialise a broken packet as if it were whole.
	strict := xml.NewDecoder(bytes.NewReader(packet))
	for {
		if _, err := strict.Token(); err == io.EOF {
			break
		} else if err != nil {
			return nil, false, fmt.Errorf("%w: %v", errUnparsedClaim, err)
		}
	}
	dec := xml.NewDecoder(bytes.NewReader(packet))
	dec.Strict = true
	var out bytes.Buffer
	var scopes []map[string]string
	resolve := func(prefix string) string {
		for i := len(scopes) - 1; i >= 0; i-- {
			if uri, ok := scopes[i][prefix]; ok {
				return uri
			}
		}
		return ""
	}
	removed := false
	skipDepth := 0
	for {
		tok, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, false, fmt.Errorf("%w: %v", errUnparsedClaim, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			scope := map[string]string{}
			for _, a := range t.Attr {
				switch {
				case a.Name.Space == "xmlns":
					scope[a.Name.Local] = a.Value
				case a.Name.Space == "" && a.Name.Local == "xmlns":
					scope[""] = a.Value
				}
			}
			scopes = append(scopes, scope)
			if skipDepth > 0 {
				skipDepth++
				continue
			}
			if isIdentificationNS(resolve(t.Name.Space)) {
				skipDepth = 1
				removed = true
				continue
			}
			out.WriteByte('<')
			writeRawName(&out, t.Name)
			for _, a := range t.Attr {
				isDecl := a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns")
				if isDecl && isIdentificationNS(a.Value) {
					continue // the declaration alone claims nothing; it goes with what it declared
				}
				if !isDecl && a.Name.Space != "" && isIdentificationNS(resolve(a.Name.Space)) {
					removed = true
					continue
				}
				out.WriteByte(' ')
				writeRawName(&out, a.Name)
				out.WriteString(`="`)
				out.WriteString(escapeXML(a.Value, true))
				out.WriteByte('"')
			}
			out.WriteByte('>')
		case xml.EndElement:
			if len(scopes) > 0 {
				scopes = scopes[:len(scopes)-1]
			}
			if skipDepth > 0 {
				skipDepth--
				continue
			}
			out.WriteString("</")
			writeRawName(&out, t.Name)
			out.WriteByte('>')
		case xml.CharData:
			if skipDepth == 0 {
				out.WriteString(escapeXML(string(t), false))
			}
		case xml.Comment:
			if skipDepth == 0 {
				out.WriteString("<!--")
				out.Write(t)
				out.WriteString("-->")
			}
		case xml.ProcInst:
			if skipDepth == 0 {
				out.WriteString("<?" + t.Target)
				if len(t.Inst) > 0 {
					out.WriteByte(' ')
					out.Write(t.Inst)
				}
				out.WriteString("?>")
			}
		case xml.Directive:
			if skipDepth == 0 {
				out.WriteString("<!")
				out.Write(t)
				out.WriteByte('>')
			}
		}
	}
	if !removed {
		return packet, false, nil
	}
	return out.Bytes(), true, nil
}

func writeRawName(b *bytes.Buffer, n xml.Name) {
	if n.Space != "" {
		b.WriteString(n.Space)
		b.WriteByte(':')
	}
	b.WriteString(n.Local)
}

// escapeXML escapes only what XML requires, so whitespace — an XMP packet's padding — survives as itself.
func escapeXML(s string, attr bool) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	if attr {
		r = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	}
	return r.Replace(s)
}

// dropUAIdentification removes the conformance identifications — PDF/UA's and, since ADR-083, PDF/A's —
// from a parsed document's catalog packet, and reports whether the document carried one. The name is
// PDF/UA's because that one came first; it is the one door for both.
//
// **A packet that names the schema and cannot be parsed loses the whole `/Metadata` stream.** A claim nib
// cannot edit is still a claim, and keeping it would be the defect this exists to end; a title lost with
// it is a visible loss. The key goes through `dropKey` (`/pending 739`): pdfcpu's `DeleteDictEntry`
// refused on a dangling reference anywhere under the stream's dictionary, and that refusal failed the
// WHOLE rewrite — a rotation, a save, a signature — for a reason the user could not act on.
func dropUAIdentification(ctx *model.Context) (bool, error) {
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		return false, err
	}
	obj, ok := root["Metadata"]
	if !ok {
		return false, nil
	}
	sd, _, err := xt.DereferenceStreamDict(obj)
	if err != nil || sd == nil {
		return false, nil
	}
	if err := sd.Decode(); err != nil {
		// Not a silent retention, measured (`/pending 503`): a caller reaches here only after a read that
		// decodes the catalog's `/Metadata` itself and refuses the whole document when it cannot — `/Filter
		// /Crypt` fails the read with "Invalid filter", a stream that is not the Flate it declares with
		// "zlib: invalid header". There were two callers when that was measured and there are five; measured
		// again for the reads they come through (`/pending 771`): `pdfread.ReadOptimized`, `inspectionRead`
		// and the merge door's `pdfread.Validated` each refuse it. So no edit reaches a packet this line
		// would skip.
		return false, nil
	}
	if !bytes.Contains(sd.Content, []byte(pdfuaidNS)) && !bytes.Contains(sd.Content, []byte(pdfaidNS)) {
		return false, nil
	}
	clean, removed, err := withoutUAIdentification(sd.Content)
	if errors.Is(err, errUnparsedClaim) {
		dropKey(xt, root, "Metadata")
		return true, nil
	}
	if err != nil || !removed {
		return false, err
	}
	sd.Content = clean
	if err := sd.Encode(); err != nil {
		return true, err
	}
	if ir, isRef := obj.(types.IndirectRef); isRef {
		if entry, found := xt.FindTableEntryForIndRef(&ir); found {
			entry.Object = *sd
			return true, nil
		}
	}
	root["Metadata"] = *sd
	return true, nil
}

// withoutUAClaim drops the claim from bytes this package already wrote. Its one caller today
// (/pending 822, `grep 'withoutUAClaim('`): `rewriteOrDropClaim`, when `writeMutated` could not
// write. The merges no longer use it — `mergeOnce` drops
// inside its write (merge.go) — and NUp takes `withoutUAClaimOrOrphanedForm`. It costs a parse,
// measured at 43 ms against `Rotate`'s 55 ms on a 190 KB document, and a rewrite only when there was
// a claim to drop.
func withoutUAClaim(pdf []byte) ([]byte, error) {
	out, _, err := dropUAIdentificationBytes(pdf)
	return out, err
}

// withoutUAClaimOrOrphanedForm is `withoutUAClaim` for NUp's COMPOSITION, where the source catalog is
// carried onto pages it never described (/pending 573).
//
// **One parse, because the composition's tail is already paying for one.**
// `dropUAIdentificationBytes` reads the document unconditionally and writes only when there was a
// claim to drop; the form prune wants the same read and the same write. Giving it a pass of its own
// would put a second full parse-and-write on every n-up — against `api.NUp`'s own measured 147 ms on
// a 22-page document — to correct a catalog the first pass already has open. That is ADR-032's rule
// applied one key over: the correction goes *inside the rewrite the change already performs*.
//
// **NUp is its one caller.** A merge keeps the FIRST document's catalog and that document's pages, so
// its widgets come through with them and its form is consistent; it is a composition that destroys
// pages while keeping the catalog that produces the orphan. SplitPage, the other composing door, does
// not need it: it reassembles through `splice`, whose page selection takes a dropped page's fields
// with it — measured, not inferred, by `TestASplitPageDoesNotLeaveAFormClaimingFieldsNothingDraws`
// (1 field against 1 widget after splitting the page that held the other).
func withoutUAClaimOrOrphanedForm(pdf []byte) ([]byte, error) {
	ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	had, err := dropUAIdentification(ctx)
	if err != nil {
		return nil, err
	}
	pruned, err := pruneOrphanedAcroForm(ctx)
	if err != nil {
		return nil, err
	}
	if !had && !pruned {
		return pdf, nil // the common case: nothing to correct, and no write
	}
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// rewriteOrDropClaim is the tail for a best-effort correction of bytes pdfcpu has just written — a stamp's
// optional-content configuration, an embedded face's CIDSet (`/pending 503`).
//
// The correction may fail and the operation must still succeed. But the bytes it would have fallen back to
// are pdfcpu's own output, and pdfcpu carries a PDF/UA identification through unchanged: `StampImages` and
// the OCR text layer returned a labelled document still claiming conformance whenever the correction
// failed. So a failed correction still drops the claim, in a rewrite with nothing else in it. Only bytes
// pdfcpu cannot read come back as they were — and then nothing in this package could have edited them.
//
// It is one write, never two: the drop-only rewrite runs only when the first did not produce output.
func rewriteOrDropClaim(pdf []byte, fn func(*model.Context) error) []byte {
	if out, err := writeMutated(pdf, fn); err == nil {
		return out
	}
	if out, err := withoutUAClaim(pdf); err == nil {
		return out
	}
	return pdf
}

// DropUAIdentificationUnlessSigned is the door for bytes that change a document OUTSIDE this package —
// a browser's edits posted back to be saved, a document about to be signed. The caller supplies whether
// the document already carries a signature (`sign.HasSignatureBlob`; this package does not import sign).
// It drops a PDF/A identification too (ADR-083): signing a PDF/A file is a change nib cannot verify kept
// it conformant, exactly as for PDF/UA.
//
// **A signed document is returned unchanged, and that is a declared gap, not an oversight.** Dropping the
// claim is a full rewrite, and a full rewrite of a signed document destroys the signature it carries — the
// one thing nib must never do to evidence as a side effect of tidying metadata. So a labelled, signed
// document keeps its identification here; `/pending 492` records it.
//
// **A document pdfcpu cannot read is returned unchanged too**, with the error: the save or the signature
// the user asked for goes ahead, and a metadata step never costs them either.
func DropUAIdentificationUnlessSigned(pdf []byte, signed bool) ([]byte, error) {
	if signed {
		return pdf, nil
	}
	out, _, err := dropUAIdentificationBytes(pdf)
	if err != nil {
		return pdf, err
	}
	return out, nil
}

// dropUAIdentificationBytes returns pdf without its PDF/UA or PDF/A identification, and whether it carried one. A
// document that carried none comes back as the SAME bytes, unwritten — a caller holding a signed document
// that carries no claim must not pay a rewrite for asking.
//
// It is the door for the paths that change a document without running an operation in this package:
// bytes a browser edited and posted back, and a document about to be signed.
//
// It asks first through `inspectionRead` — this runs on every save, Save As and finalize, and the full pass's
// per-page resource step is quadratic on a flat page tree (`/pending 763`) — and pays the full read only for the
// document that has a claim to drop, because that read is the one it writes back out. The probe's own context is
// edited by the asking and then discarded.
func dropUAIdentificationBytes(pdf []byte) ([]byte, bool, error) {
	probe, err := inspectionRead(pdf)
	if err != nil {
		return nil, false, err
	}
	if had, err := dropUAIdentification(probe); err != nil || !had {
		return pdf, had, err
	}
	ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		return nil, false, err
	}
	had, err := dropUAIdentification(ctx)
	if err != nil || !had {
		return pdf, had, err
	}
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		return nil, true, err
	}
	return out.Bytes(), true, nil
}
