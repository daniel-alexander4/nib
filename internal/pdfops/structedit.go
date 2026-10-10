package pdfops

import (
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The structure editor's dictionary edits — `PLAN-accessibility.md` P09.S02.
//
// The corrections a person makes to a tree someone else wrote, none of which touches a content stream:
// an element's type, its place among its parent's kids or under another parent, its alternate
// description, a header cell's scope, and a table cell's spans and the header cells it names (ADR-119).
// Marking content as an artifact is the one edit that rewrites page content; it is applied through the
// same batch and lives in `structartifact.go` (S03). Creating an element and deleting one are applied
// through the same batch too and live in `structcreate.go` (ADR-124). Tagging a region of a page is the
// other edit that rewrites page content — it brackets what it takes — and lives in `structregion.go` (ADR-125).
//
// # Why the dictionary edits never touch the ParentTree
//
// A ParentTree slot names the element that OWNS a marked-content id, and none of these edits changes
// who owns what: a moved element takes its MCIDs with it. What a move changes is the two directions the
// tree is walked — the parent's `/K` and the element's `/P` — and both are written together. (A delete is
// the one edit that does change an owner — the deleted element's content passes to its parent — and it
// re-points the slots itself.)
//
// # What is never written through
//
// An attribute object may be shared between elements (it is an indirect object; LibreOffice writes
// them direct, other producers need not). An edit to a table attribute — scope, a span, the headers —
// writes a NEW attribute value onto the edited element (`withTableAttribute`), copying the Table attribute
// object it changes and keeping every other attribute object as it was, so a correction to one cell cannot
// change another's.
//
// # What it refuses to leave behind
//
// The consistency invariants are checked before and after. A defect the tree already had is not this
// edit's to fix and does not block it; a defect the edit ADDS refuses the whole batch.

// editKind is what an edit changes.
type editKind int

const (
	editRetype editKind = iota + 1
	editMove
	editAlt
	editScope
	editArtifact
	editColSpan
	editRowSpan
	editHeaders
	editCreate
	editDelete
	editRegion
)

// structEdit is one correction to an existing tree.
type structEdit struct {
	kind editKind
	// elem is the object number of the element edited — `viewElement.id`. editCreate names none: 0.
	elem int
	// value is the new `/S` (editRetype, editCreate), the alternate description (editAlt; "" removes it), the
	// scope (editScope: Row, Column, Both; "" removes it), or a span (editColSpan, editRowSpan: a whole
	// number of at least 1; "" removes it).
	value string
	// headers is editHeaders' header cells, by object number; none removes the cell's `/Headers`.
	headers []int
	// parent is editMove's new parent, by object number; 0 keeps the element under its current one and
	// `rootParent` names the structure tree root. For editCreate it is the new element's parent, and 0 names
	// the root as `rootParent` does — a create has no current parent to keep.
	parent int
	// index is the position among the parent's element kids an editMove or editCreate places its element
	// at; negative or past the end appends (`placeAmongElements`).
	index int
	// page, rect, pieces and alt are editRegion's (ADR-125): the page, the region as fractions of the displayed
	// page — or, in its place, the rectangles of the pieces to take — and the new element's description.
	page   int
	rect   [4]float64
	pieces [][4]float64
	alt    string
}

// standardStructTypes are the standard structure types of ISO 32000-1 §14.8.4, the types a retype may
// choose. A document's own custom types mean something only through its RoleMap, so a correction names
// the standard type it means.
var standardStructTypes = map[string]bool{
	"Document": true, "Part": true, "Art": true, "Sect": true, "Div": true, "BlockQuote": true,
	"Caption": true, "TOC": true, "TOCI": true, "Index": true, "NonStruct": true, "Private": true,
	"P": true, "H": true, "H1": true, "H2": true, "H3": true, "H4": true, "H5": true, "H6": true,
	"L": true, "LI": true, "Lbl": true, "LBody": true,
	"Table": true, "TR": true, "TH": true, "TD": true, "THead": true, "TBody": true, "TFoot": true,
	"Span": true, "Quote": true, "Note": true, "Reference": true, "BibEntry": true, "Code": true,
	"Link": true, "Annot": true, "Ruby": true, "RB": true, "RT": true, "RP": true,
	"Warichu": true, "WT": true, "WP": true, "Figure": true, "Formula": true, "Form": true,
}

// rootParent is the `Parent` that names the structure tree root. Not 0: a move's 0 has always meant "the
// parent it already has", and the editor has sent it with every reorder since P09.S06.
const rootParent = -1

// staleEdit is an edit naming an element the tree no longer has. It is ErrTagsStale to `errors.Is`, so
// the route answers it as it answers a stale review, with a sentence about a tree rather than a
// proposal.
type staleEdit struct{ elem int }

func (e staleEdit) Error() string {
	if e.elem == 0 {
		return "pdfops: this document has no structure tree any more — read it again"
	}
	return fmt.Sprintf("pdfops: element %d is not in this document's structure tree any more — read the tree again", e.elem)
}

func (e staleEdit) Is(target error) bool { return target == ErrTagsStale }

// tableScopes are the values `/Scope` may take.
var tableScopes = map[string]bool{"Row": true, "Column": true, "Both": true}

// StructureEdit is one correction to an existing structure tree, as a reviewer sends it —
// `PLAN-accessibility.md` P09.S04.
type StructureEdit struct {
	// Kind is retype, move, alt, scope, colspan, rowspan, headers, artifact, create, delete or region.
	Kind string
	// Element is the edited element's object number. A create and a region name none: 0.
	Element int
	// Value is the new type (retype, create), the alternate description (alt; "" removes it), the scope
	// (scope: Row, Column, Both; "" removes it), or how many columns or rows a cell spans (colspan,
	// rowspan: a whole number of at least 1; "" removes it).
	Value string
	// Parent is a move's new parent by object number; 0 keeps the current one and -1 is the structure tree
	// root. A create's Parent is the new element's parent, and 0 or -1 is the root. Index is the position
	// among that parent's element kids; negative appends.
	Parent, Index int
	// Headers is a headers edit's header cells — the `TH` elements of the same table that head this
	// cell — by object number. None removes the cell's `/Headers`.
	Headers []int
	// Page, Rect, Pieces and Alt are a region's (ADR-125). The new element — of type Value, placed by Parent
	// and Index as a create's — owns every piece of page Page no element owns whose centre is inside Rect:
	// left, top, right, bottom as fractions of the page as displayed (ADR-088). Pieces, when given, replaces
	// Rect: the element takes what lies INSIDE any of those rectangles — each the rectangle of a piece
	// `ReadUntagged` listed. Alt is its description, required for a Figure.
	Page   int
	Rect   [4]float64
	Pieces [][4]float64
	Alt    string
}

// structEditKinds maps a StructureEdit's Kind onto the edit it names.
var structEditKinds = map[string]editKind{
	"retype": editRetype, "move": editMove, "alt": editAlt, "scope": editScope, "artifact": editArtifact,
	"colspan": editColSpan, "rowspan": editRowSpan, "headers": editHeaders,
	"create": editCreate, "delete": editDelete, "region": editRegion,
}

// EditStructure applies edits, in order and as one batch, to pdf's existing structure tree. An element
// the tree does not have — or a document that has no tree any more — is ErrTagsStale; an edit that is
// malformed on its own terms is ErrTagsReview.
func EditStructure(pdf []byte, edits []StructureEdit) ([]byte, error) {
	internal := make([]structEdit, len(edits))
	for i, e := range edits {
		k, ok := structEditKinds[e.Kind]
		if !ok {
			return nil, fmt.Errorf("%w: %q is not an edit — retype, move, alt, scope, colspan, rowspan, headers, artifact, create, delete or region", ErrTagsReview, e.Kind)
		}
		internal[i] = structEdit{kind: k, elem: e.Element, value: e.Value, parent: e.Parent, index: e.Index, headers: e.Headers,
			page: e.Page, rect: e.Rect, pieces: e.Pieces, alt: e.Alt}
	}
	out, err := applyStructEdits(pdf, internal)
	if errors.Is(err, errNoStructTree) {
		// The reviewer was shown a tree; a document without one is a document that changed since. Not a
		// `%w` of ErrTagsStale, whose own sentence is about a proposal.
		return nil, staleEdit{}
	}
	return out, err
}

// applyStructEdits applies edits, in order, to pdf's structure tree. An element the tree does not have
// is ErrTagsStale; an edit that is malformed on its own terms is ErrTagsReview.
func applyStructEdits(pdf []byte, edits []structEdit) ([]byte, error) {
	if len(edits) == 0 {
		return nil, fmt.Errorf("%w: there is no edit to apply", ErrTagsReview)
	}
	return writeMutated(pdf, func(ctx *model.Context) error {
		live := livePageObjects(ctx)
		tree, err := readStructTree(ctx, live)
		if err != nil {
			return err
		}
		already := map[string]bool{}
		for _, d := range checkStructConsistency(ctx, tree) {
			already[d.key] = true
		}
		for _, ed := range edits {
			// Re-read before each edit: a move changes the parent links the next edit resolves through.
			if tree, err = readStructTree(ctx, live); err != nil {
				return err
			}
			if err := applyStructEdit(ctx, tree, ed); err != nil {
				return err
			}
		}
		if tree, err = readStructTree(ctx, live); err != nil {
			return err
		}
		for _, d := range checkStructConsistency(ctx, tree) {
			if !already[d.key] {
				return fmt.Errorf("pdfops: the edit would leave the structure tree contradicting itself: %s", d.what)
			}
		}
		return nil
	})
}

func applyStructEdit(ctx *model.Context, tree *structTree, ed structEdit) error {
	if ed.kind == editCreate {
		// Before the element lookup: a create is the one edit that names no element the tree has.
		return createElement(ctx, tree, ed)
	}
	if ed.kind == editRegion {
		return regionElement(ctx, tree, ed)
	}
	if ed.elem <= 0 {
		return fmt.Errorf("%w: an element written inline has no object number, so an edit cannot name it", ErrTagsReview)
	}
	e := tree.byObj[ed.elem]
	if e == nil {
		return staleEdit{ed.elem}
	}
	switch ed.kind {
	case editRetype:
		if !standardStructTypes[ed.value] {
			return fmt.Errorf("%w: %q is not a standard structure type", ErrTagsReview, ed.value)
		}
		e.dict["S"] = types.Name(ed.value)
	case editAlt:
		if ed.value == "" {
			delete(e.dict, "Alt")
			return nil
		}
		esc, err := types.EscapedUTF16String(ed.value)
		if err != nil {
			return err
		}
		e.dict["Alt"] = types.StringLiteral(*esc)
	case editScope:
		if standardRole(tree, e.kind) != "TH" {
			return fmt.Errorf("%w: only a header cell (TH) has a scope, and element %d is a %s", ErrTagsReview, ed.elem, e.kind)
		}
		if ed.value != "" && !tableScopes[ed.value] {
			return fmt.Errorf("%w: %q is not a scope — Row, Column or Both", ErrTagsReview, ed.value)
		}
		var scope types.Object
		if ed.value != "" {
			scope = types.Name(ed.value)
		}
		setTableAttribute(ctx, e, "Scope", scope)
	case editColSpan, editRowSpan:
		key, what := "ColSpan", "columns"
		if ed.kind == editRowSpan {
			key, what = "RowSpan", "rows"
		}
		if role := standardRole(tree, e.kind); role != "TH" && role != "TD" {
			return fmt.Errorf("%w: only a table cell (TH or TD) spans %s, and element %d is a %s", ErrTagsReview, what, ed.elem, e.kind)
		}
		var span types.Object
		if ed.value != "" {
			n, err := strconv.Atoi(ed.value)
			if err != nil || n < 1 || n > math.MaxInt32 {
				return fmt.Errorf("%w: %q is not a number of %s a cell can span — a whole number, 1 or more", ErrTagsReview, ed.value, what)
			}
			span = types.Integer(n)
		}
		setTableAttribute(ctx, e, key, span)
	case editHeaders:
		return setCellHeaders(ctx, tree, e, ed)
	case editMove:
		return moveElement(ctx, tree, e, ed)
	case editArtifact:
		return artifactElement(ctx, tree, e)
	case editDelete:
		return deleteElement(ctx, tree, e)
	default:
		return fmt.Errorf("%w: unknown edit", ErrTagsReview)
	}
	return nil
}

// setTableAttribute sets e's Table attribute key to value, or removes it when value is nil.
func setTableAttribute(ctx *model.Context, e *structElem, key string, value types.Object) {
	if a := withTableAttribute(ctx, e.dict["A"], key, value); a != nil {
		e.dict["A"] = a
	} else {
		delete(e.dict, "A")
	}
}

// withTableAttribute is attrs with key set to value in the Table attribute object (a nil value removes
// it) — THE writer of an `/O /Table` attribute: Scope, ColSpan, RowSpan, Headers (ADR-119; ADR-009). The
// result is a new value: the Table attribute object is copied, and every other entry is kept as it was
// written, references included.
func withTableAttribute(ctx *model.Context, attrs types.Object, key string, value types.Object) types.Object {
	var entries types.Array
	if attrs != nil {
		if o, err := ctx.Dereference(attrs); err == nil && o != nil {
			if arr, isArr := o.(types.Array); isArr {
				entries = append(types.Array{}, arr...)
			} else {
				entries = types.Array{attrs}
			}
		}
	}
	found := false
	for i, en := range entries {
		d, err := ctx.DereferenceDict(en)
		if err != nil || d == nil {
			continue
		}
		if owner := d.NameEntry("O"); owner == nil || *owner != "Table" {
			continue
		}
		cp := types.Dict{}
		for k, v := range d {
			cp[k] = v
		}
		if value == nil {
			delete(cp, key)
		} else {
			cp[key] = value
		}
		entries[i] = cp
		found = true
	}
	if !found && value != nil {
		entries = append(entries, types.Dict{"O": types.Name("Table"), key: value})
	}
	switch len(entries) {
	case 0:
		return nil
	case 1:
		return entries[0]
	}
	return entries
}

// tableOf is the nearest Table element holding e, or nil.
func tableOf(tree *structTree, e *structElem) *structElem {
	for p := e.parent; p != nil; p = p.parent {
		if standardRole(tree, p.kind) == "Table" {
			return p
		}
	}
	return nil
}

// setCellHeaders writes a cell's `/Headers`: the `/ID` of each header cell the edit names (ADR-119).
//
// **The edit names ELEMENTS and nib writes the identifiers.** `/Headers` holds byte strings that must each
// equal the `/ID` of a `TH` in the same table (ua1 7.5 t2), so a person asked to type one could only get it
// wrong. A header cell that has no `/ID` is given one no element of the tree carries and entered in the
// root's `/IDTree`, which ISO 32000-1 Table 322 requires once any element has an identifier; a header cell
// that has one keeps it, and the bytes written into `/Headers` are that very object.
func setCellHeaders(ctx *model.Context, tree *structTree, e *structElem, ed structEdit) error {
	if role := standardRole(tree, e.kind); role != "TH" && role != "TD" {
		return fmt.Errorf("%w: only a table cell (TH or TD) names header cells, and element %d is a %s", ErrTagsReview, ed.elem, e.kind)
	}
	if len(ed.headers) == 0 {
		setTableAttribute(ctx, e, "Headers", nil)
		return nil
	}
	table := tableOf(tree, e)
	if table == nil {
		return fmt.Errorf("%w: element %d is in no table, so no header cell heads it", ErrTagsReview, ed.elem)
	}
	var taken map[string]bool // every identifier the tree's elements carry, read once and only when one is allocated
	ids := types.Array{}
	named := map[int]bool{}
	for _, h := range ed.headers {
		if named[h] {
			continue
		}
		named[h] = true
		th := tree.byObj[h]
		if h <= 0 || th == nil {
			return staleEdit{h}
		}
		if th == e || standardRole(tree, th.kind) != "TH" || tableOf(tree, th) != table {
			return fmt.Errorf("%w: element %d is not another header cell (TH) of the table element %d is in", ErrTagsReview, h, ed.elem)
		}
		if _, id, has := elementID(ctx, th.dict); has {
			ids = append(ids, id)
			continue
		}
		if taken == nil {
			taken = map[string]bool{}
			for _, other := range tree.elems {
				if s, _, ok := elementID(ctx, other.dict); ok {
					taken[s] = true
				}
			}
		}
		name := fmt.Sprintf("nib-%d", h)
		for n := 2; taken[name]; n++ {
			name = fmt.Sprintf("nib-%d-%d", h, n)
		}
		taken[name] = true
		id := types.StringLiteral(name)
		th.dict["ID"] = id
		gen := 0
		if en, ok := ctx.XRefTable.Table[h]; ok && en != nil && en.Generation != nil {
			gen = *en.Generation
		}
		if err := setIDTreeEntry(ctx, tree, name, id, *types.NewIndirectRef(h, gen)); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	setTableAttribute(ctx, e, "Headers", ids)
	return nil
}

// moveElement takes e out of its parent's `/K` and puts it into the new parent's at ed.index, and
// points its `/P` at the new parent.
func moveElement(ctx *model.Context, tree *structTree, e *structElem, ed structEdit) error {
	to := e.parent
	switch {
	case ed.parent == rootParent:
		to = nil
	case ed.parent != 0:
		if to = tree.byObj[ed.parent]; to == nil {
			return staleEdit{ed.parent}
		}
	}
	for p := to; p != nil; p = p.parent {
		if p == e {
			return fmt.Errorf("%w: element %d cannot be moved inside itself", ErrTagsReview, ed.elem)
		}
	}
	ref, err := removeFromParent(ctx, tree, e)
	if err != nil {
		return err
	}
	holder := tree.root
	if to != nil {
		holder = to.dict
	}
	placeAmongElements(ctx, holder, *ref, ed.index)

	if to == nil {
		rootRef, err := structTreeRootRef(ctx)
		if err != nil {
			return err
		}
		e.dict["P"] = *rootRef
		return nil
	}
	if to.objNr == 0 {
		// The parent is written inline, so no reference can name it — and only a reorder under the parent
		// the element already has reaches here, since `ed.parent` names a new parent by object number. The
		// element's `/P` already says what a reference can; `0 0 R` names the xref free-list head
		// (`/pending 503`).
		return nil
	}
	gen := 0
	if en, ok := ctx.XRefTable.Table[to.objNr]; ok && en != nil && en.Generation != nil {
		gen = *en.Generation
	}
	e.dict["P"] = *types.NewIndirectRef(to.objNr, gen)
	return nil
}

// placeAmongElements puts ref into holder's `/K` — an element's, or the root's — as its index-th ELEMENT
// kid: before the element that holds that place now, with marked content and object references between
// elements not counted. A negative index, or one past the last element, appends. It is the one placement
// rule (ADR-009): a move and a create both place through it.
func placeAmongElements(ctx *model.Context, holder types.Dict, ref types.IndirectRef, index int) {
	kids, set := kidsArray(ctx, holder)
	at := len(kids)
	if index >= 0 {
		seen := 0
		for i, en := range kids {
			if !isElementEntry(ctx, en) {
				continue
			}
			if seen == index {
				at = i
				break
			}
			seen++
		}
	}
	placed := append(types.Array{}, kids[:at]...)
	placed = append(placed, ref)
	set(append(placed, kids[at:]...))
}

// removeFromParent takes e out of its parent's `/K` — the root's, for a top-level element — and returns
// the reference that listed it. Shared by a move and an artifact edit, two of the three ways an element
// leaves where it is; the third, a delete, leaves its kids in its place (`replaceInParent`).
func removeFromParent(ctx *model.Context, tree *structTree, e *structElem) (*types.IndirectRef, error) {
	return replaceInParent(ctx, tree, e, nil)
}

// replaceInParent puts with where e is listed in its parent's `/K` — the root's, for a top-level element
// — in e's place and in order, and returns the reference that listed e. With nothing, e is simply taken
// out. It is the one way an element leaves its parent's `/K` (ADR-009).
func replaceInParent(ctx *model.Context, tree *structTree, e *structElem, with types.Array) (*types.IndirectRef, error) {
	holder := tree.root
	if e.parent != nil {
		holder = e.parent.dict
	}
	kids, set := kidsArray(ctx, holder)
	var ref *types.IndirectRef
	kept := make(types.Array, 0, len(kids)+len(with))
	for _, en := range kids {
		if ir, ok := en.(types.IndirectRef); ok && ir.ObjectNumber.Value() == e.objNr && ref == nil {
			r := ir
			ref = &r
			kept = append(kept, with...)
			continue
		}
		kept = append(kept, en)
	}
	if ref == nil {
		return nil, fmt.Errorf("pdfops: element %d is not listed in its parent's /K, so it cannot be taken out of it", e.objNr)
	}
	set(kept)
	return ref, nil
}

// kidsArray is holder's `/K` as an array, and the function that writes an array back where `/K` lives —
// into an indirect array object in place, or onto the holder. Every form `/K` may take is normalised,
// the same forms `appendToElementKids` and `appendToRootKids` accept.
func kidsArray(ctx *model.Context, holder types.Dict) (types.Array, func(types.Array)) {
	onHolder := func(a types.Array) { holder["K"] = a }
	k, has := holder["K"]
	if !has || k == nil {
		return nil, onHolder
	}
	if ind, isInd := k.(types.IndirectRef); isInd {
		if arr, err := ctx.DereferenceArray(k); err == nil && arr != nil {
			if en, found := ctx.XRefTable.FindTableEntryForIndRef(&ind); found && en != nil {
				return append(types.Array{}, arr...), func(a types.Array) { en.Object = a }
			}
		}
	}
	if arr, isArr := k.(types.Array); isArr {
		return append(types.Array{}, arr...), onHolder
	}
	return types.Array{k}, onHolder
}

// isElementEntry reports whether a `/K` entry is a structure element rather than marked content or an
// object reference.
func isElementEntry(ctx *model.Context, en types.Object) bool {
	d, err := ctx.DereferenceDict(en)
	if err != nil || d == nil {
		return false
	}
	if ty := d.NameEntry("Type"); ty != nil && (*ty == "MCR" || *ty == "OBJR") {
		return false
	}
	return d.NameEntry("S") != nil
}
