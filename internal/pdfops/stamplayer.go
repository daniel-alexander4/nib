package pdfops

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
