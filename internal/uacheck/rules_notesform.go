package uacheck

import (
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Notes and the Form element — `PLAN-ua-coverage.md` P03.S06. Each is veraPDF's own property, read from its
// source (veraPDF-validation `GFSENote`, `GFSEForm`) and then measured on veraPDF 1.30.2: `7.18.4 t2` has no
// corpus file, so the fixtures in `rules_notesform_test.go` are its oracle.
//
// **`7.1 t12` (every element carries /P) is deliberately NOT registered.** Measured on veraPDF 1.30.2: an element
// with no /P, with /P null, /P 5, a dangling /P and a /P naming a page all PASS 7.1-12. WHY is not measured, and
// "veraPDF gives an element the parent it was reached from" — the reading this comment first gave — cannot be
// the whole of it: the containment clauses DO read the element's own /P (a TD whose /P names the Document fails
// 7.2-9, `significantParent`). Only the five verdicts are evidence; no rule may borrow the explanation. A clause
// the oracle cannot fail is one nib cannot show agreement on for its failed half, and a rule that always passed would inflate the count while checking nothing (the trap the
// plan names for P03). Declared in the plan, with the fixtures.

func init() {
	register(Rule{Clause: "7.9 t1", Summary: "a Note tag shall have an ID entry", Check: checkNoteIDs})
	register(Rule{Clause: "7.9 t2", Summary: "each Note tag shall have a unique ID", Check: checkNoteIDsUnique})
	register(Rule{Clause: "7.18.4 t2", Summary: "a Form element with no Role attribute shall have one child: an object reference to its widget", Check: checkFormChild})
}

// noteID is a Note's `/ID` as veraPDF reads it (`getStringKey`): a string, or absent.
func (d *Document) noteID(n structNode) (string, bool) {
	if n.dict["ID"] == nil {
		return "", false
	}
	return d.text(n.dict["ID"])
}

// checkNoteIDs evaluates ua1 7.9 t1: every Note carries a non-empty string `/ID`.
func checkNoteIDs(d *Document) Result {
	nodes, unread := d.structNodes()
	notes := 0
	for _, n := range nodes {
		if d.typedAs(n.dict) != "Note" {
			continue
		}
		notes++
		if id, ok := d.noteID(n); !ok || id == "" {
			return Result{Verdict: Fail, Where: nodeWhere(n, d.name(n.dict["S"])),
				Why: "a Note has no /ID, so the reference that points at it has nothing to name"}
		}
	}
	return notesVerdict(notes, unread)
}

// checkNoteIDsUnique evaluates ua1 7.9 t2 across the WHOLE tree, never per subtree: veraPDF adds every Note's ID
// to one set for the document (`StaticContainers.getNoteIDSet`), and the second holder of an ID fails — an empty
// ID included, which is still an ID to that set.
func checkNoteIDsUnique(d *Document) Result {
	nodes, unread := d.structNodes()
	seen := map[string]bool{}
	notes := 0
	for _, n := range nodes {
		if d.typedAs(n.dict) != "Note" {
			continue
		}
		notes++
		id, ok := d.noteID(n)
		if !ok {
			continue
		}
		if seen[id] {
			return Result{Verdict: Fail, Where: nodeWhere(n, d.name(n.dict["S"])),
				Why: fmt.Sprintf("a second Note carries the ID %q, so a reference to it cannot say which note it means", id)}
		}
		seen[id] = true
	}
	return notesVerdict(notes, unread)
}

func notesVerdict(notes int, unread string) Result {
	switch {
	case unread != "":
		return Result{Verdict: CannotCheck, Why: unread}
	case notes == 0:
		return Result{Verdict: NotApplicable, Why: "the document has no Note element"}
	}
	return Result{Verdict: Pass}
}

// checkFormChild evaluates ua1 7.18.4 t2: a Form with no Role attribute (owner `/PrintField`) has exactly one
// child, and it is an object reference to a Widget annotation. veraPDF's children are EVERY entry of the Form's
// own `/K` — elements, marked-content references, bare MCIDs and object references alike (veraPDF-parser
// `TaggedPDFHelper.getChildren`) — so an MCID beside the widget's reference is a second child.
func checkFormChild(d *Document) Result {
	nodes, unread := d.structNodes()
	forms := 0
	for _, n := range nodes {
		if d.typedAs(n.dict) != "Form" {
			continue
		}
		forms++
		if d.attributeOfType(n.dict, "PrintField", "Role", attrName) != nil {
			continue
		}
		if !d.oneWidgetChild(n.dict) {
			return Result{Verdict: Fail, Where: nodeWhere(n, d.name(n.dict["S"])),
				Why: "a Form element with no Role attribute must have exactly one child, an object reference to its widget annotation"}
		}
	}
	switch {
	case unread != "":
		return Result{Verdict: CannotCheck, Why: unread}
	case forms == 0:
		return Result{Verdict: NotApplicable, Why: "the document has no Form element"}
	}
	return Result{Verdict: Pass}
}

// oneWidgetChild is veraPDF's `hasOneInteractiveChild`.
//
// **A named exemption from `elementKid`, the door every other relation reads (ADR-009).** This clause counts
// CHILDREN, not element kids: content (MCIDs, marked-content and object references) counts here and is exactly
// what `elementKid` excludes, and no pass-through element is looked through. Routing it through the door
// would change what it counts.
func (d *Document) oneWidgetChild(form types.Dict) bool {
	k := form["K"]
	if k == nil {
		return false
	}
	entries := []types.Object{k}
	if arr, err := d.Ctx.DereferenceArray(k); err == nil && arr != nil {
		entries = arr
	}
	children := 0
	var only types.Dict
	for _, en := range entries {
		if _, isMCID := d.intValue(en); isMCID {
			children++
			continue
		}
		kid := d.dict(en)
		if kid == nil {
			continue // veraPDF lists nothing that is neither an integer nor a dictionary
		}
		switch d.name(kid["Type"]) {
		case "MCR", "OBJR", "", "StructElem":
			children++
			only = kid
		}
	}
	if children != 1 || only == nil || d.name(only["Type"]) != "OBJR" {
		return false
	}
	annot := d.dict(only["Obj"])
	return annot != nil && d.name(annot["Subtype"]) == "Widget"
}
