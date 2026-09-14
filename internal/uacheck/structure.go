package uacheck

import (
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Reading the structure tree — `PLAN-accessibility.md` P07.S03.
//
// # Why the checker walks the tree itself instead of borrowing `pdfops`' model
//
// `internal/pdfops` has a typed tree model (`readStructTree`, `parentTreeEntries`) and it is the
// model every tagging door WRITES through. A checker that read documents with the writer's own model
// would agree with the writer by construction: a defect in how the model reads a tree would be a
// defect the checker shares, and the two would certify each other. `pdfops`' own test suite already
// knows this — `countElementsIndependently` exists "so the model's count is compared against
// something that does not share its code".
//
// So this is a second, deliberately independent reading, and law 5 (veraPDF over the corpus) is what
// validates it. That is not ADR-009's two-opinions defect: the rule "how to build a tree" lives in
// `pdfops`, and the rule "what a conforming tree is" lives here — two different rules, each with one
// door.

// maxWalkDepth bounds every recursive read, because a malicious or broken document can make a
// structure tree, a number tree or a form XObject refer to itself.
const maxWalkDepth = 64

// parentTree resolves the catalog's `/StructTreeRoot /ParentTree` into key → value, walking a nested
// number tree (`/Kids`) as well as a flat `/Nums`.
//
// The writer (`pdfops.setParentTreeSlot`) REFUSES nested number trees rather than rebalancing them.
// A reader cannot refuse: real producers write them, and a checker that could not read one would
// report a correctly tagged document as broken.
func (d *Document) parentTree() map[int]types.Object {
	if d.pt != nil {
		return d.pt
	}
	d.pt = map[int]types.Object{}
	root := d.dict(d.Catalog["StructTreeRoot"])
	if root == nil {
		return d.pt
	}
	seen := map[int]bool{}
	var walk func(o types.Object, depth int)
	walk = func(o types.Object, depth int) {
		if depth > maxWalkDepth {
			return
		}
		if ir, ok := o.(types.IndirectRef); ok {
			if seen[ir.ObjectNumber.Value()] {
				return
			}
			seen[ir.ObjectNumber.Value()] = true
		}
		node := d.dict(o)
		if node == nil {
			return
		}
		if nums, err := d.Ctx.DereferenceArray(node["Nums"]); err == nil {
			for i := 0; i+1 < len(nums); i += 2 {
				if k, ok := nums[i].(types.Integer); ok {
					d.pt[k.Value()] = nums[i+1]
				}
			}
		}
		if kids, err := d.Ctx.DereferenceArray(node["Kids"]); err == nil {
			for _, k := range kids {
				walk(k, depth+1)
			}
		}
	}
	walk(root["ParentTree"], 0)
	return d.pt
}

// standardType resolves an element's `/S` through the tree's `/RoleMap` to a standard structure
// type.
//
// A producer may call a form element `/MyFormField` and map it to `/Form`; veraPDF resolves the map
// and so must this, or a correctly tagged form from another producer fails 7.18.4. The hop count is
// bounded because a role map can map a name to itself.
func (d *Document) standardType(elem types.Dict) string {
	s := elem.NameEntry("S")
	if s == nil {
		return ""
	}
	name := *s
	root := d.dict(d.Catalog["StructTreeRoot"])
	if root == nil {
		return name
	}
	roleMap := d.dict(root["RoleMap"])
	for hop := 0; hop < 10 && roleMap != nil; hop++ {
		mapped, ok := roleMap[name].(types.Name)
		if !ok || string(mapped) == name {
			break
		}
		name = string(mapped)
	}
	return name
}

// langOf returns the natural language declared by elem or its nearest ancestor that declares one,
// walking `/P` up to the structure tree root.
//
// Language inherits down the tree (ISO 32000-1 §14.9.2), which is what lets one `/Lang` on a
// `Document` element cover every paragraph beneath it.
func (d *Document) langOf(elem types.Dict) string {
	for depth := 0; elem != nil && depth < maxWalkDepth; depth++ {
		if ty := elem.NameEntry("Type"); ty != nil && *ty == "StructTreeRoot" {
			return ""
		}
		if s, ok := elem["Lang"].(types.StringLiteral); ok && len(s) > 0 {
			if dec, err := types.StringLiteralToString(s); err == nil && dec != "" {
				return dec
			}
			return string(s)
		}
		elem = d.dict(elem["P"])
	}
	return ""
}

// resourcesOf returns a page's `/Resources`, climbing `/Parent` for the inherited case.
//
// Resources are inheritable through the page tree, and a document whose pages share one resource
// dictionary on their `/Pages` node is ordinary. Without the climb every `Do` and every named
// property list on such a page would look unresolvable.
func (d *Document) resourcesOf(page types.Dict) types.Dict {
	for depth := 0; page != nil && depth < maxWalkDepth; depth++ {
		if r := d.dict(page["Resources"]); r != nil {
			return r
		}
		page = d.dict(page["Parent"])
	}
	return nil
}
