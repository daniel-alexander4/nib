package pdfops

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/format"
)

// Detecting a stamp the document already carries.
//
// StampPageNumbers and StampWatermark both bake onto whatever they are given, so
// running either twice puts a second set on top of the first — overlapping page
// numbers, a doubled watermark. The review finding that named this assumed the fix
// needed a marker property of Nib's own, on the NibFlags model.
//
// It does not. **pdfcpu already tags its own work**: every watermark it writes goes
// into an optional-content group named "Watermark" (or "Background" when the stamp is
// drawn behind the page rather than on top), and that OCG is how api.RemoveWatermarks
// finds them again later. So "has this been stamped already" is a question the
// document answers by itself, with no property to embed, nothing to keep in sync with
// the undo ring, and no second full rewrite on the stamping path — which is what a
// NibFlags-shaped marker would have cost, since api.AddWatermarks cannot carry a
// property and api.AddProperties is its own read-and-write pass.
//
// **What it cannot do is tell the two kinds apart.** Both of Nib's stamping paths pass
// onTop=true, so page numbers and a watermark land in the SAME "Watermark" group.
// Callers get one bit — "something Nib stamps is already here" — and whatever they put
// on screen has to be honest about not knowing which.
//
// The walk is repeated here rather than called: pdfcpu's own locateOCGs and
// detectStampOCG are unexported. It uses only the exported model/types API, and it
// mirrors their logic deliberately — if pdfcpu ever renames the group, this goes quiet
// rather than wrong, and TestHasStampLayerSeesPdfcpuOwnStamp is what notices.

import (
	"bytes"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// stampOCGNames are the two names pdfcpu gives the layer it stamps into: "Watermark"
// when the stamp is on top (both of Nib's paths) and "Background" when it is behind.
var stampOCGNames = map[string]bool{"Watermark": true, "Background": true}

// HasStampLayer reports whether the PDF already carries a pdfcpu stamp layer — a page
// number run, a watermark, or anything else stamped through the same path, including by
// another tool built on pdfcpu.
//
// It is a FULL PARSE, so it belongs on a path a user action triggers (opening the page
// number or watermark dialog), never in a route reply built for every request. A
// document that cannot be parsed reads as "no stamp": the answer only ever drives a
// warning, and refusing to open a dialog because a probe failed would be worse than the
// doubled stamp it is trying to prevent.
func HasStampLayer(pdf []byte) (bool, error) {
	ctx, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return false, err
	}
	root, err := ctx.Catalog()
	if err != nil {
		return false, err
	}
	o, ok := root.Find("OCProperties")
	if !ok {
		return false, nil
	}
	d, err := ctx.DereferenceDict(o)
	if err != nil || d == nil {
		return false, err
	}
	o, ok = d.Find("OCGs")
	if !ok {
		return false, nil
	}
	arr, err := ctx.DereferenceArray(o)
	if err != nil {
		return false, err
	}
	for _, e := range arr {
		if e == nil {
			continue
		}
		gd, err := ctx.DereferenceDict(e)
		if err != nil || gd == nil {
			continue
		}
		if t := gd.Type(); t == nil || *t != "OCG" {
			continue
		}
		if n := gd.StringEntry("Name"); n != nil && stampOCGNames[*n] {
			return true, nil
		}
	}
	return false, nil
}

// ErrStampTextUnrepresentable is returned when text cannot be baked as written.
//
// pdfcpu's watermark engine has no working escape for its placeholder tokens, so a few
// inputs — text where a run of `%` is followed by one of its placeholder letters — cannot
// be rendered as typed. Nib refuses those rather than baking something else, because on
// the finalize path the result is signed.
var ErrStampTextUnrepresentable = errors.New("this text cannot be stamped as written: pdfcpu's watermark engine has no working escape, so a % it would read as a placeholder — and a run of two or more % — cannot be rendered literally")

// stampText prepares user text for pdfcpu's watermark engine — the ONE place that
// decision is made, because it was made three different ways in four call sites.
//
// # What pdfcpu does with a %, measured against v0.13.0
//
//	"CONFIDENTIAL 100%" -> "CONFIDENTIAL 100"   the % is silently DROPPED
//	"50%P"              -> "503"                %P is the page count
//	"%v"                -> "v0.13.0 dev"        the LIBRARY VERSION, in the document
//	"%"                 -> ""                   empty, and pdfcpu then computes a nil
//	                                            bounding box and panics
//
// Nib bakes these marks onto a document and then, on the finalize path, **signs it**. So a
// dropped % or a substituted page count is not a rendering nit: it is a signed document
// whose visible text is not what the signer typed. `%v` is the sharpest — the signer types
// two characters and certifies a dependency's version string.
//
// # `%%` is not an escape, and believing it was is what this function replaced
//
// Three of the four sites had a policy and all three were wrong. `StampPageNumbers`
// **stripped** the character (an alteration, just a quieter one). `StampFields` and
// `StampWatermark` did nothing. `StampTextLayer` doubled it and its comment claimed that
// "also stops e.g. an OCR'd %P turning into a page count" — measured, it does not:
//
//	"%%P"    -> "%3"
//	"%%%%P"  -> "%%%3"
//
// The doubling emits one `%` and advances a SINGLE character, so the trailing `%` in the
// run pairs with the letter and substitutes anyway. No number of `%` produces a literal
// `%P`; it is unrepresentable through this API.
//
// # So the transform is VERIFIED, not trusted
//
// This escapes and then renders the result through pdfcpu itself, refusing when what comes
// back is not what went in. That is deliberately not a table of placeholder letters: a
// hardcoded `p/P/t/v` is a second copy of pdfcpu's grammar that drifts the first time the
// library adds a token, and this package already learned that lesson in `coverage.go` —
// ask the encoder rather than keeping a table beside it.
//
// A caller gets ("", nil) when there is nothing to draw and should skip; empty text is the
// input that panics.
func stampText(s string) (string, error) {
	if strings.TrimSpace(s) == "" {
		return "", nil
	}
	esc := strings.ReplaceAll(s, "%", "%%")
	if got, _ := format.Text(esc, "", 1, 1); got != s {
		return "", fmt.Errorf("%w (%q would be baked as %q)", ErrStampTextUnrepresentable, s, got)
	}
	return esc, nil
}
