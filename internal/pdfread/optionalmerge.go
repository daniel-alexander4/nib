package pdfread

import (
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The optional-content properties, carried through a MERGE — `/pending 559`, ADR-131; `/pending 870`, ADR-153. The mirror of
// `optionalcarry.go` (ADR-043), which carries them through a page selection.
//
// # A merge kept only the first document's, so it REVEALED every later document's hidden layers
//
// A merge keeps the first document's catalog and frees each later one's. A later document's pages come
// across whole — the `/OC /L0 BDC … EMC` bracket in the content, the group named in `/Resources
// /Properties` — and the one thing that said the group was switched off, its catalog's `/OCProperties /D
// /OFF`, did not. Measured 2026-10-11 with ADR-043's own fixture (a 300×200pt black box inside a group
// listed in `/OFF`), the page rasterised at 40 dpi before and after:
//
//	                         the document    Append(host, it)    InsertPDF(host, it)    Append(it, it) page 4
//	  Ghostscript 10.02.1    86 dark px      18,902              18,902                 18,902
//	  poppler pdftoppm        6 dark px      18,598              18,598                 18,598
//
// The last column is a host that HAS `/OCProperties`: its own layer stayed hidden and the second
// document's was drawn, because the second's group is in nobody's list.
//
// # The union: the later document's groups join the host's, in the state their own document gave them
//
// `UnionOptionalContent` runs after `MergeXRefTables` has renumbered the source's
// objects in place — so the source's `/OCProperties`, held from before the merge, already names its
// groups by their numbers in the merged document. Where the host has none, the source's is adopted
// whole. Where it has one:
//
//   - the source's `/OCGs` are appended to the host's;
//   - each source group is given, in the host's default configuration `/D` and in every alternate one in
//     the host's `/Configs`, the state the SOURCE's `/D` gave it: a group that was off is listed in
//     `/OFF`, and one that was on is listed in `/ON` wherever that configuration's `/BaseState` is not
//     `/ON` — the base state is the host's and applies to every group no list names, so a host that says
//     "everything off unless listed" would otherwise hide a later document's visible layers;
//   - the source `/D`'s `/Order`, `/Locked` and `/RBGroups` are appended to the host `/D`'s, and its `/AS` to
//     every host configuration's. Each names only the source's own groups, so nothing of the host's is
//     restated by them.
//
// **The host's own state is never touched**: nothing is removed from a host list, no host group changes
// list, and the host's `/BaseState`, `/Name`, `/Intent` and `/ListMode` stand.
//
// # The source's alternate configurations come too, with the host's groups as the host's default has them
//
// An alternate configuration (`/Configs`) is a named state of the whole document that a reader can switch to.
// The source's were dropped (ADR-131's declared gap); ADR-153 carries them, by the rule above read the other way
// round: each is appended to the host's `/Configs`, and every HOST group is given, in it, the state the host's
// `/D` gave it — off groups in its `/OFF`, on groups in its `/ON` wherever its `/BaseState` is not `/ON` — with
// the host `/D`'s `/AS` appended. So switching to a merged-in configuration shows of the source what the source's
// author chose for it, and of the host exactly what the host's default shows: nothing either document hid by
// default is shown by a configuration its author never saw. The source's lists in it are its own and are not
// restated.
//
// A usage application (`/AS`) can switch a group off by itself — a group no list names, hidden on screen by its
// usage — so the source `/D`'s `/AS` goes to EVERY host configuration, not to the host's `/D` alone: in a host
// alternate without it, such a group was shown.
//
// # What it does not carry, named
//
// A group that only an alternate configuration of its own document hid is shown under the other document's
// configurations, as it was shown under its own default. Two documents' alternates are not paired by name.
//
// # It lives here because BOTH merge loops call it (ADR-009)
//
// `pdfops.mergeOnce` is `Append`, `Combine`, and `InsertPDF` and the split tiles through `splice`; after a
// splice the page selection carries the merged dictionary under ADR-043's own reachability guard.
// `MergeRaw` in this package is the other loop, and `pdfops.RedactPages` assembles its raster pages and
// its runs of untouched pages through it — so with page 1 redacted the first segment was the raster,
// which has no layers, and a layer hidden on page 2 was drawn in the redacted document. One rule, one
// function, called from each loop at the same point: after `MergeXRefTables`, with the source's
// dictionary held from before it.

// SourceOptionalContent is a merge source's `/OCProperties` dictionary, read in its own context before
// the merge, or nil. It is the map the merge then renumbers in place.
func SourceOptionalContent(src *model.Context) types.Dict {
	cat, err := src.XRefTable.Catalog()
	if err != nil {
		return nil
	}
	return derefDict(src.XRefTable, cat["OCProperties"])
}

// UnionOptionalContent joins a merged-in document's optional-content properties to the host's, after
// the merge has renumbered them.
func UnionOptionalContent(ctx *model.Context, src types.Dict) error {
	if src == nil {
		return nil
	}
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		return err
	}
	host := derefDict(xt, root["OCProperties"])
	if host == nil {
		root["OCProperties"] = src
		return nil
	}
	groups := derefArray(xt, src["OCGs"])
	if len(groups) == 0 {
		return nil
	}

	// Each document's groups in the state its own default configuration gives them — the host's read BEFORE
	// anything of the source's joins it.
	srcD := derefDict(xt, src["D"])
	hostD := derefDict(xt, host["D"])
	if hostD == nil {
		hostD = types.Dict{}
		host["D"] = hostD
	}
	srcOn, srcOff := defaultStates(xt, srcD, groups)
	hostOn, hostOff := defaultStates(xt, hostD, derefArray(xt, host["OCGs"]))
	hostAS := derefArray(xt, hostD["AS"])
	srcAS := derefArray(xt, srcD["AS"])
	host["OCGs"] = joined(xt, host["OCGs"], groups)

	for _, c := range append([]types.Object{hostD}, derefArray(xt, host["Configs"])...) {
		if d := derefDict(xt, c); d != nil {
			giveStates(xt, d, srcOn, srcOff, srcAS)
		}
	}
	for _, k := range []string{"Order", "Locked", "RBGroups"} {
		if more := derefArray(xt, srcD[k]); len(more) > 0 {
			hostD[k] = joined(xt, hostD[k], more)
		}
	}
	// The source's alternates, AFTER the loop above: they already say what the source's groups do.
	var alternates types.Array
	for _, c := range derefArray(xt, src["Configs"]) {
		if d := derefDict(xt, c); d != nil {
			giveStates(xt, d, hostOn, hostOff, hostAS)
			alternates = append(alternates, c)
		}
	}
	if len(alternates) > 0 {
		host["Configs"] = joined(xt, host["Configs"], alternates)
	}
	return nil
}

// defaultStates sorts groups by the state the default configuration d gives each, as a reader reads it: off when
// listed in `/OFF`, or under `/BaseState /OFF` and not listed in `/ON`.
func defaultStates(xt *model.XRefTable, d types.Dict, groups types.Array) (on, off types.Array) {
	listed := func(key string) map[int]bool {
		in := map[int]bool{}
		for _, o := range derefArray(xt, d[key]) {
			if r, ok := o.(types.IndirectRef); ok {
				in[r.ObjectNumber.Value()] = true
			}
		}
		return in
	}
	inOn, inOff := listed("ON"), listed("OFF")
	baseOff := nameVal(d, "BaseState") == "OFF"
	for _, g := range groups {
		r, ok := g.(types.IndirectRef)
		if !ok {
			continue
		}
		nr := r.ObjectNumber.Value()
		// `/OFF` is applied after `/ON` (ISO 32000-1 Table 101), so a group in both is off.
		if inOff[nr] || (baseOff && !inOn[nr]) {
			off = append(off, g)
		} else {
			on = append(on, g)
		}
	}
	return on, off
}

// giveStates writes another document's groups into the configuration c in the states their own default gave
// them: off groups into `/OFF`, on groups into `/ON` wherever c's base state would not already show them, and
// that default's usage applications after c's own.
func giveStates(xt *model.XRefTable, c types.Dict, on, off, as types.Array) {
	if len(off) > 0 {
		c["OFF"] = joined(xt, c["OFF"], off)
	}
	if len(on) > 0 && nameVal(c, "BaseState") != "" && nameVal(c, "BaseState") != "ON" {
		c["ON"] = joined(xt, c["ON"], on)
	}
	if len(as) > 0 {
		c["AS"] = joined(xt, c["AS"], as)
	}
}

// joined is the array at have followed by more, as a new array.
func joined(xt *model.XRefTable, have types.Object, more types.Array) types.Array {
	return append(append(types.Array{}, derefArray(xt, have)...), more...)
}

func derefDict(xt *model.XRefTable, o types.Object) types.Dict {
	if o == nil {
		return nil
	}
	d, _ := xt.DereferenceDict(o)
	return d
}

func derefArray(xt *model.XRefTable, o types.Object) types.Array {
	if o == nil {
		return nil
	}
	a, _ := xt.DereferenceArray(o)
	return a
}

func nameVal(d types.Dict, key string) string {
	if n := d.NameEntry(key); n != nil {
		return *n
	}
	return ""
}
