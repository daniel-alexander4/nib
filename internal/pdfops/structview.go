package pdfops

import (
	"bytes"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The structure editor's read door — `PLAN-accessibility.md` P09.S01.
//
// An existing tree, whoever wrote it, read into values a reviewer can act on: every element in the
// order the tree holds it, its type as written and as the document's RoleMap resolves it, the text
// drawn under it, its alternate description, and a header cell's scope. It writes nothing.
//
// # What it reads text through
//
// The run reader, by MCID — the answer P08.S04's truth reader and the proposer read, so an element's
// text here is the text the proposer would have grouped. Runs inside an `/Artifact` carry no MCID and
// belong to no element.
//
// # What it cannot address, said rather than dropped
//
// An element written inline inside its parent's `/K` has no object number, and an edit names elements
// by object number. Such an element is still in the view — its text and kids are real — and it is
// counted in `unaddressable`, so a tree that cannot be fully edited says so.

// structureView is a document's structure tree as reviewable values.
type structureView struct {
	// elements is every element, in the tree's own order: a parent before its kids, kids in `/K` order.
	elements []viewElement
	// unaddressable counts the elements written inline, which an edit cannot name.
	unaddressable int
}

// viewElement is one structure element as a reviewer sees it.
type viewElement struct {
	// id is the element's object number — what an edit names. 0 for an element written inline.
	id int
	// parent is the index of the element holding this one, or -1 for one directly under the root.
	parent int
	// kind is `/S` as written; standard is what the RoleMap resolves it to.
	kind     string
	standard string
	// page is the 1-based page the element or its first content sits on; 0 when nothing names one.
	page int
	// marked says the element owns marked content directly, not only through its kids.
	marked bool
	// text is the runs drawn under the element's own MCIDs and its kids', in `/K` order.
	text   string
	alt    string
	hasAlt bool
	// scope is a `TH`'s `/Scope` from its Table attribute object — Row, Column or Both — and "" when it
	// declares none.
	scope string
	// kids are the indices of the element's element kids, in `/K` order.
	kids []int
}

// readStructureView reads pdf's structure tree as reviewable values. A document with no tree is
// errNoStructTree.
func readStructureView(pdf []byte) (structureView, error) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return structureView{}, err
	}
	live, pageNr := map[int]bool{}, map[int]int{}
	for pg := 1; pg <= ctx.PageCount; pg++ {
		if ir, e := ctx.PageDictIndRef(pg); e == nil && ir != nil {
			live[ir.ObjectNumber.Value()] = true
			pageNr[ir.ObjectNumber.Value()] = pg
		}
	}
	tree, err := readStructTree(ctx, live)
	if err != nil {
		return structureView{}, err
	}
	// ADR-009 exemption (grouping_test.go): this reads runs to match MCIDs to text and groups nothing.
	textAt := map[[2]int]string{}
	for pg := 1; pg <= ctx.PageCount; pg++ {
		pr, perr := readPageRuns(ctx, pg)
		if perr != nil {
			return structureView{}, perr
		}
		for _, r := range pr.runs {
			if r.mcid >= 0 {
				textAt[[2]int{pg, r.mcid}] += r.text
			}
		}
	}

	index := make(map[*structElem]int, len(tree.elems))
	for i, e := range tree.elems {
		index[e] = i
	}
	view := structureView{elements: make([]viewElement, len(tree.elems))}
	// The model lists a parent before its kids, so walking it backwards finds every kid's text already
	// read — one pass, not a re-walk of each subtree.
	for i := len(tree.elems) - 1; i >= 0; i-- {
		e := tree.elems[i]
		v := viewElement{id: e.objNr, parent: -1, kind: e.kind, standard: standardRole(tree, e.kind), page: pageNr[e.pgObj]}
		if e.parent != nil {
			v.parent = index[e.parent]
		}
		var b strings.Builder
		for _, k := range e.kids {
			switch k.kind {
			case kidMCID, kidMCR:
				v.marked = true
				b.WriteString(textAt[[2]int{pageNr[k.pgObj], k.mcid}])
				if v.page == 0 {
					v.page = pageNr[k.pgObj]
				}
			case kidElement:
				if k.elem == nil {
					continue
				}
				j := index[k.elem]
				v.kids = append(v.kids, j)
				b.WriteString(view.elements[j].text)
				if v.page == 0 {
					v.page = view.elements[j].page
				}
			}
		}
		v.text = b.String()
		if o, ok := e.dict["Alt"]; ok {
			if s, aerr := ctx.XRefTable.DereferenceStringOrHexLiteral(o, model.V10, nil); aerr == nil {
				v.alt, v.hasAlt = s, true
			}
		}
		v.scope = tableScope(ctx, e.attrs)
		if e.objNr == 0 {
			view.unaddressable++
		}
		view.elements[i] = v
	}
	return view, nil
}

// tableScope is `/Scope` from the Table attribute object among attrs, which may be one attribute
// object or an array of them (with revision numbers between, which are skipped).
func tableScope(ctx *model.Context, attrs types.Object) string {
	if attrs == nil {
		return ""
	}
	o, err := ctx.Dereference(attrs)
	if err != nil || o == nil {
		return ""
	}
	objs := []types.Object{o}
	if arr, ok := o.(types.Array); ok {
		objs = arr
	}
	for _, a := range objs {
		d, derr := ctx.DereferenceDict(a)
		if derr != nil || d == nil {
			continue
		}
		if owner := d.NameEntry("O"); owner == nil || *owner != "Table" {
			continue
		}
		if s := d.NameEntry("Scope"); s != nil {
			return *s
		}
	}
	return ""
}

// standardRole resolves a structure type through the document's role map.
func standardRole(tree *structTree, kind string) string {
	for i := 0; i < 10; i++ {
		next, ok := tree.roleMap[kind]
		if !ok || next == kind {
			return kind
		}
		kind = next
	}
	return kind
}
