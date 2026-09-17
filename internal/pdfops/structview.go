package pdfops

import (
	"bytes"
	"errors"
	"math"
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
	// rect is the element's extent on its page — the union of the runs drawn under its MCIDs and its
	// kids' boxes on that page, estimated from baselines as the proposer's outline is (0.85 em up,
	// 0.25 em down). hasRect is false for an element that draws no text, a figure's image among them.
	// pageBox is the page's MediaBox, so a client can place rect on the page it renders (P09.S06a).
	rect    [4]float64
	hasRect bool
	pageBox [4]float64
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
	//
	// # Two indexes, because an MCID is only unique WITHIN a content stream
	//
	// `textAt` is keyed by page, and that is right for the ordinary document, where a page's marked
	// content is in the page's own stream and a kid is a bare integer. It is wrong the moment one page
	// draws two form XObjects that each carry an MCID 0 — the ordinary shape of an n-up — because both
	// texts land under the same key and both elements read both. Measured at P02.S08 before
	// `textByStm` existed: **31 of 61 elements came back with two source pages' text concatenated**,
	// element 27 reading "TitleParagraph number 30…" where its source read "Title".
	//
	// `textByStm` is keyed by the stream the run was actually read from, and **only a kid that names a
	// stream through an MCR's `/Stm` consults it** (see the kid loop below). That is what makes this additive:
	// a document with no `/Stm` anywhere reads through `textAt` exactly as it did before, so the fix
	// cannot move a document it was not written for.
	textAt := map[[3]int]string{}
	rectAt := map[[3]int][4]float64{}
	textByStm := map[[3]int]string{}
	rectByStm := map[[3]int][4]float64{}
	// union folds one run's box into whatever the key already holds. The middle element of every key
	// is the stream: 0 for the page-wide index, the form's object number for the narrow one.
	union := func(m map[[3]int][4]float64, k [3]int, b [4]float64) [4]float64 {
		if seen, ok := m[k]; ok {
			return unionBox(seen, b)
		}
		return b
	}
	for pg := 1; pg <= ctx.PageCount; pg++ {
		pr, perr := readPageRuns(ctx, pg)
		if perr != nil {
			return structureView{}, perr
		}
		for _, r := range pr.runs {
			if r.mcid < 0 {
				continue
			}
			// **The run's OWN box, computed once and never reassigned.** Folding the page-level
			// union back into `box` and then seeding the per-stream map from it put the merged
			// rectangle into the very index that exists to keep the two apart — the text came out
			// right and the geometry did not, which is the half a text assertion cannot see. Caught
			// by the rect clause of `TestACarriedNUpReadsItsOwnText`.
			run := [4]float64{r.x, r.y - 0.25*r.size, r.x + r.width, r.y + 0.85*r.size}
			key := [3]int{pg, 0, r.mcid}
			textAt[key] += r.text
			rectAt[key] = union(rectAt, key, run)
			if r.stm > 0 {
				// Keyed by PAGE as well as stream: one form object drawn on two pages would
				// otherwise union geometry across them, and an element whose `/Pg` resolves to no
				// page (`pg == 0`) would match a real run's key and escape with a box.
				sk := [3]int{pg, r.stm, r.mcid}
				textByStm[sk] += r.text
				rectByStm[sk] = union(rectByStm, sk, run)
			}
		}
	}
	pageBoxes := map[int][4]float64{}
	boxOfPage := func(pg int) [4]float64 {
		if b, ok := pageBoxes[pg]; ok {
			return b
		}
		b := mediaBoxOf(ctx, pg)
		pageBoxes[pg] = b
		return b
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
		type pagedBox struct {
			page int
			box  [4]float64
		}
		var boxes []pagedBox
		for _, k := range e.kids {
			switch k.kind {
			case kidMCID, kidMCR:
				v.marked = true
				pg := pageNr[k.pgObj]
				// A kid that names its stream is read in that stream and **no other**, with no
				// fallback to the page index. A miss is then empty text, which is the honest answer
				// for a document whose `/Stm` names a stream holding no such MCID — and falling back
				// would quietly restore the very cross-stream merge this exists to stop.
				text := textAt[[3]int{pg, 0, k.mcid}]
				box, found := rectAt[[3]int{pg, 0, k.mcid}]
				if k.stm > 0 {
					// **Narrow FIRST, page index as the fallback.** A kid naming a stream the page
					// walk reached is read there and nowhere else. A kid naming one it did not reach
					// — an annotation appearance stream, a form drawn nowhere, one past the nesting
					// ceiling, a dangling `/Stm` — falls back to what this reader has always done,
					// so the change cannot take text away from a document it was not written for.
					if t, ok := textByStm[[3]int{pg, k.stm, k.mcid}]; ok {
						text = t
						box, found = rectByStm[[3]int{pg, k.stm, k.mcid}]
					}
				}
				b.WriteString(text)
				if v.page == 0 {
					v.page = pg
				}
				if found {
					boxes = append(boxes, pagedBox{pg, box})
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
				if kid := view.elements[j]; kid.hasRect {
					boxes = append(boxes, pagedBox{kid.page, kid.rect})
				}
			}
		}
		v.text = b.String()
		// The box is on the element's own page: a paragraph continuing onto the next page is outlined
		// where it starts, which is where the viewer is taken.
		for _, pb := range boxes {
			if pb.page != v.page {
				continue
			}
			if v.hasRect {
				v.rect = unionBox(v.rect, pb.box)
			} else {
				v.rect, v.hasRect = pb.box, true
			}
		}
		if v.page > 0 {
			v.pageBox = boxOfPage(v.page)
		}
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

// unionBox is the smallest box holding both.
func unionBox(a, b [4]float64) [4]float64 {
	return [4]float64{math.Min(a[0], b[0]), math.Min(a[1], b[1]), math.Max(a[2], b[2]), math.Max(a[3], b[3])}
}

// StructureElement is one element of a document's existing structure tree, as the Tags panel shows
// it — `PLAN-accessibility.md` P09.S06a.
//
// The JSON tags are the tree route's field names (`internal/server/tags.go`), held equal by a server test,
// so `nib tag tree --json` prints what the Tags panel reads (P10.S01).
type StructureElement struct {
	// ID is the object number an edit names; 0 for an element written inline.
	ID int `json:"id"`
	// Parent and Kids are indices into the tree's Elements; Parent is -1 for an element under the root.
	Parent int   `json:"parent"`
	Kids   []int `json:"kids"`
	// Kind is `/S` as written; Standard is what the role map resolves it to.
	Kind     string `json:"kind"`
	Standard string `json:"standard"`
	Page     int    `json:"page"`
	Text     string `json:"text"`
	Alt      string `json:"alt"`
	HasAlt   bool   `json:"hasAlt"`
	Scope    string `json:"scope"`
	// Rect is the element's estimated extent on Page in PDF user space, all zero when it draws no text;
	// PageBox is that page's MediaBox, so a client can place Rect on the page it renders.
	Rect    [4]float64 `json:"rect"`
	PageBox [4]float64 `json:"pageBox"`
}

// StructureTree is a document's existing structure tree as reviewable values.
type StructureTree struct {
	// Tagged is false for a document with no structure tree, and Elements is then empty.
	Tagged        bool               `json:"tagged"`
	Unaddressable int                `json:"unaddressable"`
	Elements      []StructureElement `json:"elements"`
}

// ReadStructure reads pdf's existing structure tree. It writes nothing, and a document with no tree is
// an answer — untagged — not an error.
func ReadStructure(pdf []byte) (StructureTree, error) {
	v, err := readStructureView(pdf)
	if errors.Is(err, errNoStructTree) {
		return StructureTree{Elements: []StructureElement{}}, nil
	}
	if err != nil {
		return StructureTree{}, err
	}
	out := StructureTree{Tagged: true, Unaddressable: v.unaddressable, Elements: make([]StructureElement, len(v.elements))}
	for i, e := range v.elements {
		out.Elements[i] = StructureElement{
			ID: e.id, Parent: e.parent, Kids: append([]int{}, e.kids...), Kind: e.kind, Standard: e.standard,
			Page: e.page, Text: e.text, Alt: e.alt, HasAlt: e.hasAlt, Scope: e.scope, Rect: e.rect, PageBox: e.pageBox,
		}
	}
	return out, nil
}
