package pdfops

import (
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Optional content that is OFF in the document's default configuration (PLAN-import-export X6, D30): what a reader
// would not see, and so what an export must not carry. Resolved once per walk from `/OCProperties /D` — `/BaseState`,
// `/ON`, `/OFF` — for a group (`/Type /OCG`) and for a membership dictionary (`/OCMD`) by its `/P` policy.
//
// **Declared:** `/AS` (auto-state) and `/Usage /View` are not read, nor is an OCMD's `/VE` expression: a layer those
// would hide is read as visible. Reading text a viewer hides is the unsafe direction, so it is named here and in
// ADR-192, not hidden.

// ocState answers whether an optional-content object (a group or a membership dictionary) is OFF.
type ocState struct {
	xt        *model.XRefTable
	baseOff   bool
	on, off   map[int]bool
	none      bool // no `/OCProperties`: nothing is off
	nestDepth int
}

func newOcState(xt *model.XRefTable) *ocState {
	ls := &ocState{xt: xt, on: map[int]bool{}, off: map[int]bool{}}
	root, err := xt.Catalog()
	if err != nil || root == nil {
		ls.none = true
		return ls
	}
	props, err := xt.DereferenceDict(root["OCProperties"])
	if err != nil || props == nil {
		ls.none = true
		return ls
	}
	d, err := xt.DereferenceDict(props["D"])
	if err != nil || d == nil {
		ls.none = true
		return ls
	}
	if bs := d.NameEntry("BaseState"); bs != nil && *bs == "OFF" {
		ls.baseOff = true
	}
	collect := func(key string, into map[int]bool) {
		arr, err := xt.DereferenceArray(d[key])
		if err != nil {
			return
		}
		for _, o := range arr {
			if ir, ok := o.(types.IndirectRef); ok {
				into[ir.ObjectNumber.Value()] = true
			}
		}
	}
	collect("ON", ls.on)
	collect("OFF", ls.off)
	if !ls.baseOff && len(ls.off) == 0 {
		ls.none = true
	}
	return ls
}

// groupOff is whether the group object number nr is OFF in the default configuration.
func (ls *ocState) groupOff(nr int) bool {
	if ls.none {
		return false
	}
	if ls.baseOff {
		return !ls.on[nr]
	}
	return ls.off[nr]
}

// objectOff is whether o — the value a property list or an XObject's `/OC` holds — names optional content that is OFF.
func (ls *ocState) objectOff(o types.Object) bool {
	if ls == nil || ls.none || o == nil {
		return false
	}
	ir, isRef := o.(types.IndirectRef)
	d, err := ls.xt.DereferenceDict(o)
	if err != nil || d == nil {
		return false
	}
	if t := d.NameEntry("Type"); t != nil && *t == "OCMD" {
		return ls.membershipOff(d)
	}
	if !isRef {
		return false
	}
	return ls.groupOff(ir.ObjectNumber.Value())
}

// membershipOff reads an OCMD: its groups and `/P` policy (AnyOn by default, as the specification says).
func (ls *ocState) membershipOff(d types.Dict) bool {
	var groups []types.Object
	switch v := d["OCGs"].(type) {
	case types.IndirectRef:
		if arr, err := ls.xt.DereferenceArray(v); err == nil && arr != nil {
			groups = arr
		} else {
			groups = []types.Object{v}
		}
	case types.Array:
		groups = v
	}
	if len(groups) == 0 {
		return false
	}
	policy := "AnyOn"
	if p := d.NameEntry("P"); p != nil {
		policy = *p
	}
	nOn, nOff := 0, 0
	for _, g := range groups {
		ir, ok := g.(types.IndirectRef)
		if !ok {
			continue
		}
		if ls.groupOff(ir.ObjectNumber.Value()) {
			nOff++
		} else {
			nOn++
		}
	}
	switch policy {
	case "AllOn":
		return nOff > 0
	case "AnyOff":
		return nOff == 0
	case "AllOff":
		return nOn > 0
	}
	return nOn == 0 // AnyOn: visible when any group is on
}
