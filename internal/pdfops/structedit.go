package pdfops

import (
	"errors"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The structure editor's dictionary edits — `PLAN-accessibility.md` P09.S02.
//
// Four corrections a person makes to a tree someone else wrote, none of which touches a content stream:
// an element's type, its place among its parent's kids or under another parent, its alternate
// description, and a header cell's scope. Marking content as an artifact is the one edit that rewrites
// page content; it is applied through the same batch and lives in `structartifact.go` (S03).
//
// # Why the dictionary edits never touch the ParentTree
//
// A ParentTree slot names the element that OWNS a marked-content id, and none of these edits changes
// who owns what: a moved element takes its MCIDs with it. What a move changes is the two directions the
// tree is walked — the parent's `/K` and the element's `/P` — and both are written together.
//
// # What is never written through
//
// An attribute object may be shared between elements (it is an indirect object; LibreOffice writes
// them direct, other producers need not). A scope edit writes a NEW attribute value onto the edited
// element, copying the Table attribute object it changes and keeping every other attribute object as
// it was, so a correction to one header cell cannot change another's.
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
)

// structEdit is one correction to an existing tree.
type structEdit struct {
	kind editKind
	// elem is the object number of the element edited — `viewElement.id`.
	elem int
	// value is the new `/S` (editRetype), the alternate description (editAlt; "" removes it), or the
	// scope (editScope: Row, Column, Both; "" removes it).
	value string
	// parent is editMove's new parent, by object number; 0 keeps the element under its current one.
	parent int
	// index is editMove's position among the new parent's element kids; negative or past the end
	// appends.
	index int
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
	// Kind is retype, move, alt, scope or artifact.
	Kind string
	// Element is the edited element's object number.
	Element int
	// Value is the new type (retype), the alternate description (alt; "" removes it), or the scope
	// (scope: Row, Column, Both; "" removes it).
	Value string
	// Parent is a move's new parent by object number; 0 keeps the current one. Index is the position
	// among that parent's element kids; negative appends.
	Parent, Index int
}

// structEditKinds maps a StructureEdit's Kind onto the edit it names.
var structEditKinds = map[string]editKind{
	"retype": editRetype, "move": editMove, "alt": editAlt, "scope": editScope, "artifact": editArtifact,
}

// EditStructure applies edits, in order and as one batch, to pdf's existing structure tree. An element
// the tree does not have — or a document that has no tree any more — is ErrTagsStale; an edit that is
// malformed on its own terms is ErrTagsReview.
func EditStructure(pdf []byte, edits []StructureEdit) ([]byte, error) {
	internal := make([]structEdit, len(edits))
	for i, e := range edits {
		k, ok := structEditKinds[e.Kind]
		if !ok {
			return nil, fmt.Errorf("%w: %q is not an edit — retype, move, alt, scope or artifact", ErrTagsReview, e.Kind)
		}
		internal[i] = structEdit{kind: k, elem: e.Element, value: e.Value, parent: e.Parent, index: e.Index}
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
		live := map[int]bool{}
		for pg := 1; pg <= ctx.PageCount; pg++ {
			if ir, e := ctx.PageDictIndRef(pg); e == nil && ir != nil {
				live[ir.ObjectNumber.Value()] = true
			}
		}
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
		if a := withTableScope(ctx, e.dict["A"], ed.value); a != nil {
			e.dict["A"] = a
		} else {
			delete(e.dict, "A")
		}
	case editMove:
		return moveElement(ctx, tree, e, ed)
	case editArtifact:
		return artifactElement(ctx, tree, e)
	default:
		return fmt.Errorf("%w: unknown edit", ErrTagsReview)
	}
	return nil
}

// withTableScope is attrs with the Table attribute object's `/Scope` set to scope ("" removes it). The
// result is a new value: the Table attribute object is copied, and every other entry is kept as it was
// written, references included.
func withTableScope(ctx *model.Context, attrs types.Object, scope string) types.Object {
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
		if scope == "" {
			delete(cp, "Scope")
		} else {
			cp["Scope"] = types.Name(scope)
		}
		entries[i] = cp
		found = true
	}
	if !found && scope != "" {
		entries = append(entries, types.Dict{"O": types.Name("Table"), "Scope": types.Name(scope)})
	}
	switch len(entries) {
	case 0:
		return nil
	case 1:
		return entries[0]
	}
	return entries
}

// moveElement takes e out of its parent's `/K` and puts it into the new parent's at ed.index, and
// points its `/P` at the new parent.
func moveElement(ctx *model.Context, tree *structTree, e *structElem, ed structEdit) error {
	to := e.parent
	if ed.parent != 0 {
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
	toKids, setTo := kidsArray(ctx, holder)
	at := len(toKids)
	if ed.index >= 0 {
		seen := 0
		for i, en := range toKids {
			if !isElementEntry(ctx, en) {
				continue
			}
			if seen == ed.index {
				at = i
				break
			}
			seen++
		}
	}
	placed := append(types.Array{}, toKids[:at]...)
	placed = append(placed, *ref)
	setTo(append(placed, toKids[at:]...))

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

// removeFromParent takes e out of its parent's `/K` — the root's, for a top-level element — and returns
// the reference that listed it. Shared by a move and an artifact edit, the two ways an element leaves
// where it is.
func removeFromParent(ctx *model.Context, tree *structTree, e *structElem) (*types.IndirectRef, error) {
	holder := tree.root
	if e.parent != nil {
		holder = e.parent.dict
	}
	kids, set := kidsArray(ctx, holder)
	var ref *types.IndirectRef
	kept := kids[:0:0]
	for _, en := range kids {
		if ir, ok := en.(types.IndirectRef); ok && ir.ObjectNumber.Value() == e.objNr && ref == nil {
			r := ir
			ref = &r
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
