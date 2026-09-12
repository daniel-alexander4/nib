package pdfops

import (
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The structure tree as a typed model — `PLAN-accessibility.md` P05.S02, D8.
//
// # Why a model and not more dictionary walking
//
// D8: *"pdfcpu has no builder, so nib gets one: a typed tree that parses an existing
// `/StructTreeRoot`, mutates it, and writes it back, with `/ParentTree`, `/StructParents` and MCIDs
// maintained as invariants of the model rather than by each caller."*
//
// This slice is the READ half. Nothing here mutates anything.
//
// # What a real tree contains, measured rather than sketched
//
// A LibreOffice HTML conversion, 36 elements, is the reference this was built against:
//
//   - root keys `Type`, `K`, `ParentTree`, **`RoleMap`** — and **no `/ParentTreeNextKey`**, so
//     nothing may depend on that key existing;
//   - **fifteen** distinct element types, four of them role-mapped custom names (`Heading 1`,
//     `Text body`, `Table Contents`, `Table Heading`) beside the standard `H2`, `L`, `LI`, `Lbl`,
//     `LBody`, `Table`, `TR`, `TH`, `TD`, `Link`, `Document`;
//   - `/K` entries of three kinds in one document: 36 indirect references, **23 integer MCIDs**, and
//     an **`OBJR`** dictionary for the `<a href>`'s annotation;
//   - `/A` attributes on 23 of the 36 elements.
//
// A model that handles only "elements with element children" describes none of that.

// kidKind classifies one entry of a `/K` array, which is the only place a structure tree branches.
type kidKind int

const (
	// kidElement is a child structure element, whether reached through an indirect reference or
	// written inline as a dictionary. Both are legal and both appear in the wild.
	kidElement kidKind = iota
	// kidMCID is an integer: a marked-content sequence on the element's own `/Pg`.
	kidMCID
	// kidMCR is a `/Type /MCR` dictionary — a marked-content reference naming its own page, used
	// when an element's content is not on the page its parent named.
	kidMCR
	// kidOBJR is a `/Type /OBJR` dictionary — a reference to a whole object, in practice an
	// annotation. Measured: one per `<a href>` in a LibreOffice conversion.
	kidOBJR
)

// structKid is one entry of an element's `/K`.
type structKid struct {
	kind kidKind
	elem *structElem // kidElement only
	mcid int         // kidMCID and kidMCR
	// pgObj is the object number of the page this kid's content is on: from the MCR's own `/Pg`
	// where it has one, otherwise inherited from the element. Zero when nothing named a page.
	pgObj int
	// obj is the referenced object for kidOBJR.
	obj int
	raw types.Object
}

// structElem is one `/StructElem`.
type structElem struct {
	// objNr is the element's object number, or 0 for an element written inline inside its parent's
	// `/K`. **Identity is the object number**, never the dictionary's content — see the note on
	// readStructTree's visited set.
	objNr int
	dict  types.Dict
	// kind is `/S` verbatim. It may be a role-mapped custom name, so it is NOT interpreted here;
	// `roleMap` is carried alongside so a caller can resolve one without a second parse.
	kind string
	// pgObj is the object number of `/Pg`, or 0. `pgLive` says whether that page is still in the
	// page tree — an element pointing at a page that was removed is what `orphaned()` is about.
	pgObj  int
	pgLive bool
	kids   []structKid
	parent *structElem
	// attrs is `/A` verbatim, present on most elements of a real document and never inspected here.
	attrs types.Object
}

// structTree is a parsed `/StructTreeRoot`.
type structTree struct {
	root types.Dict
	// roleMap maps a document's custom structure names to standard ones. Carried because an element
	// typed `Preformatted Text` means nothing without it.
	roleMap map[string]string
	// elems is every element reachable from the root, in discovery order.
	elems []*structElem
	// byObj indexes the elements that have an object number.
	byObj map[int]*structElem
}

// elements returns how many structure elements the tree actually contains.
func (t *structTree) elements() int { return len(t.elems) }

// anchored returns how many elements name a page that is still in the page tree.
func (t *structTree) anchored() int {
	n := 0
	for _, e := range t.elems {
		if e.pgLive {
			n++
		}
	}
	return n
}

// describedPages returns the object numbers of pages some element points at.
func (t *structTree) describedPages() map[int]bool {
	out := map[int]bool{}
	for _, e := range t.elems {
		if e.pgLive {
			out[e.pgObj] = true
		}
		for _, k := range e.kids {
			if k.pgObj != 0 {
				out[k.pgObj] = true
			}
		}
	}
	return out
}

// errNoStructTree is returned when the catalog has no `/StructTreeRoot`. It is not a failure: most
// documents have no tree, and a caller asking about one needs to tell "absent" from "malformed".
var errNoStructTree = fmt.Errorf("pdfops: the document has no structure tree")

// readStructTree parses the document's structure tree.
//
// # Identity is the object number, and that is a correction
//
// `inspectTags` keyed its visited set on `d.String()` — the dictionary's CONTENT — so two elements
// with byte-identical dictionaries were walked once and counted once. Measured on a fixture with
// two identical `<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R >>` siblings: it reported ONE.
// Real documents produce identical siblings easily.
//
// Keying on the object number is correct and is also why elements written INLINE need care: they
// have no object number, so they cannot be in the visited set at all. That is safe — an inline
// dictionary is reachable from exactly one place by construction, so it cannot be revisited.
//
// # Refusing beats partially parsing
//
// A `/K` entry this cannot classify is an ERROR. The alternative — skip it and carry on — produces a
// model describing fewer children than the document has, and every count taken from it is quietly
// wrong. That is the failure mode the whole slice exists to avoid, so it cannot be the failure mode
// of the parser itself.
func readStructTree(ctx *model.Context, livePages map[int]bool) (*structTree, error) {
	cat, err := ctx.XRefTable.Catalog()
	if err != nil {
		return nil, err
	}
	st, ok := cat["StructTreeRoot"]
	if !ok {
		return nil, errNoStructTree
	}
	root, err := ctx.DereferenceDict(st)
	if err != nil || root == nil {
		return nil, fmt.Errorf("pdfops: /StructTreeRoot does not resolve to a dictionary: %w", err)
	}
	t := &structTree{root: root, roleMap: map[string]string{}, byObj: map[int]*structElem{}}

	if rm, rerr := ctx.DereferenceDict(root["RoleMap"]); rerr == nil && rm != nil {
		for k, v := range rm {
			if n, isName := v.(types.Name); isName {
				t.roleMap[k] = n.Value()
			}
		}
	}

	visited := map[int]bool{}
	kids, err := t.readKids(ctx, root["K"], nil, 0, livePages, visited, 0)
	if err != nil {
		return nil, err
	}
	// The root's own `/K` holds the top-level elements; anything else there is a document nib
	// cannot represent rather than one it should guess about.
	for _, k := range kids {
		if k.kind != kidElement {
			return nil, fmt.Errorf("pdfops: /StructTreeRoot's /K holds a %s, which is only legal "+
				"under an element", k.kindName())
		}
	}
	return t, nil
}

// maxStructDepth bounds recursion, and it is a second line of defence rather than the working one.
//
// The visited set stops every cycle that can actually be built: a cycle needs an element to be
// reached twice, which needs an indirect reference, which the set catches. **Measured — raising this
// bound to 100,000 leaves every test green**, because with the set in place nothing ever recurses
// deep enough to reach it. It is not dead code and it is not tested coverage either, and saying so
// beats implying the latter.
//
// It stays because the failure it guards is unrecoverable — `/pending 454` is this repo's standing
// lesson about walking a PDF without a bound — and because the set is one edit away from being
// weakened by someone who has this comment and not that history.
const maxStructDepth = 200

func (t *structTree) readKids(ctx *model.Context, o types.Object, parent *structElem,
	inheritPg int, livePages map[int]bool, visited map[int]bool, depth int) ([]structKid, error) {

	if o == nil {
		return nil, nil
	}
	if depth > maxStructDepth {
		return nil, fmt.Errorf("pdfops: the structure tree nests deeper than %d levels — it is "+
			"cyclic or malformed", maxStructDepth)
	}
	// `/K` may be a single entry rather than an array; normalise to a list.
	var entries []types.Object
	if arr, aerr := ctx.DereferenceArray(o); aerr == nil && arr != nil {
		entries = arr
	} else {
		entries = []types.Object{o}
	}

	var out []structKid
	for _, raw := range entries {
		k, err := t.readKid(ctx, raw, parent, inheritPg, livePages, visited, depth)
		if err != nil {
			return nil, err
		}
		if k != nil {
			out = append(out, *k)
		}
	}
	return out, nil
}

func (t *structTree) readKid(ctx *model.Context, raw types.Object, parent *structElem,
	inheritPg int, livePages map[int]bool, visited map[int]bool, depth int) (*structKid, error) {

	// An integer is an MCID on the element's own page.
	if n, ok := raw.(types.Integer); ok {
		return &structKid{kind: kidMCID, mcid: n.Value(), pgObj: inheritPg, raw: raw}, nil
	}

	objNr := 0
	if ind, ok := raw.(types.IndirectRef); ok {
		objNr = ind.ObjectNumber.Value()
		if visited[objNr] {
			// Already walked: a tree whose elements point back at their parents is ordinary.
			return nil, nil
		}
	}
	d, err := ctx.DereferenceDict(raw)
	if err != nil || d == nil {
		return nil, fmt.Errorf("pdfops: a /K entry is neither an integer nor a dictionary (%T) — "+
			"this tree holds something the model cannot represent", raw)
	}

	ty := ""
	if n := d.NameEntry("Type"); n != nil {
		ty = *n
	}
	switch ty {
	case "MCR":
		pg := inheritPg
		if ind, ok := d["Pg"].(types.IndirectRef); ok {
			pg = ind.ObjectNumber.Value()
		}
		mcid := 0
		if n := d.IntEntry("MCID"); n != nil {
			mcid = *n
		}
		return &structKid{kind: kidMCR, mcid: mcid, pgObj: pg, raw: raw}, nil
	case "OBJR":
		obj := 0
		if ind, ok := d["Obj"].(types.IndirectRef); ok {
			obj = ind.ObjectNumber.Value()
		}
		pg := inheritPg
		if ind, ok := d["Pg"].(types.IndirectRef); ok {
			pg = ind.ObjectNumber.Value()
		}
		return &structKid{kind: kidOBJR, obj: obj, pgObj: pg, raw: raw}, nil
	case "StructElem", "":
		// `/Type` is OPTIONAL on a structure element (ISO 32000-1 table 323), so an untyped
		// dictionary under a `/K` is an element. Refusing it would reject documents that are legal
		// and common; the `/S` check below is what actually identifies one.
	default:
		return nil, fmt.Errorf("pdfops: a /K entry has /Type /%s, which the model cannot "+
			"represent — it is neither a structure element, an MCR nor an OBJR", ty)
	}

	if objNr != 0 {
		visited[objNr] = true
	}
	e := &structElem{objNr: objNr, dict: d, parent: parent, attrs: d["A"]}
	if n := d.NameEntry("S"); n != nil {
		e.kind = *n
	} else {
		return nil, fmt.Errorf("pdfops: a /K entry is a dictionary with no /S — it is not a " +
			"structure element and the model cannot say what it is")
	}
	e.pgObj = inheritPg
	if ind, ok := d["Pg"].(types.IndirectRef); ok {
		e.pgObj = ind.ObjectNumber.Value()
	}
	e.pgLive = e.pgObj != 0 && livePages[e.pgObj]

	t.elems = append(t.elems, e)
	if objNr != 0 {
		t.byObj[objNr] = e
	}
	kids, err := t.readKids(ctx, d["K"], e, e.pgObj, livePages, visited, depth+1)
	if err != nil {
		return nil, err
	}
	e.kids = kids
	return &structKid{kind: kidElement, elem: e, pgObj: e.pgObj, raw: raw}, nil
}

// kindName is for error messages.
func (k structKid) kindName() string {
	switch k.kind {
	case kidElement:
		return "structure element"
	case kidMCID:
		return "marked-content id"
	case kidMCR:
		return "marked-content reference"
	case kidOBJR:
		return "object reference"
	}
	return "unknown kid"
}
