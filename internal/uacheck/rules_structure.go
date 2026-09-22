package uacheck

import (
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The first rule, implemented to prove the shape — `PLAN-accessibility.md` P07.S01.
//
// `7.1 t11` is chosen because nib already has everything it needs to answer it and has measured the
// answer against veraPDF on real documents throughout P01–P06: a document either has a
// `/StructTreeRoot` whose tree reaches content, or it does not. The rest of the structure rules are
// P07.S03's.

func init() {
	register(Rule{
		Clause:  "7.1 t11",
		Summary: "logical structure shall be rooted in the document catalog's StructTreeRoot",
		Check:   checkStructTreeRoot,
	})
	register(Rule{
		Clause:  "7.1 t6",
		Summary: "a circular mapping shall not exist",
		Check:   checkRoleMapCycle,
	})
	register(Rule{
		Clause:  "7.1 t5",
		Summary: "every non-standard structure type shall be mapped, directly or through other names, to a standard type",
		Check:   checkNonStandardTypeIsMapped,
	})
	register(Rule{
		Clause:  "7.1 t7",
		Summary: "the standard structure types shall not be remapped",
		Check:   checkStandardTypeNotRemapped,
	})
}

// checkNonStandardTypeIsMapped evaluates ua1 7.1 t5 (P03.S01).
//
// **Its subject is an element nib cannot type as a standard type, and its failure is narrower than
// that.** veraPDF's object is `SENonStandard` — every element whose role map does not lead to a standard
// type — and it fails only the one whose OWN `/S` is non-standard and whose chain dead-ends at another
// non-standard name. Measured on veraPDF 1.30.2 over the corpus and six fixtures of nib's own:
//
//   - `/Standard → /p` fails (7.1-t05-fail-a); `/Alpha → /Zed` fails; `/Standard → ∅` fails (fail-c).
//   - A CYCLE is a subject that passes — `/Standard → /Text body → /Standard` (7.1-t05-fail-d, whose name is
//     the specification's numbering: veraPDF fails it on 7.1-6, not here) and `/TR → /Zed → /TR`.
//   - A STANDARD `/S` sent to a dead end is a subject that passes — `/Document → /Book`, 7.1-t07-fail-a,
//     which is 7.1 t7's failure.
//
// So the verdict is per element, and a fact about the role map alone is not a failure: a map entry no
// element uses is never asked.
func checkNonStandardTypeIsMapped(d *Document) Result {
	nodes, unread := d.structNodes()
	subjects := 0
	for _, n := range nodes {
		own := d.name(n.dict["S"])
		std, unresolved := d.standardType(n.dict)
		if own == "" || (unresolved == "" && standardStructureTypes[std]) {
			continue
		}
		subjects++
		// A loop is 7.1 t6's failure, and an element the typing walk found on one is always circular in the raw
		// walk too — the raw walk takes the same steps and never stops early — so one test covers both.
		if standardStructureTypes[own] || d.roleMapCircular(n.dict) {
			continue
		}
		why := fmt.Sprintf("/%s is not a standard structure type, and the role map does not lead it to one", own)
		if std != own {
			why = fmt.Sprintf("/%s is not a standard structure type, and the role map leads it only as far as /%s, "+
				"which is not one either", own, std)
		}
		return Result{Verdict: Fail, Why: why, Where: nodeWhere(n, own)}
	}
	if unread != "" {
		return Result{Verdict: CannotCheck, Why: unread}
	}
	if subjects == 0 {
		return Result{
			Verdict: NotApplicable,
			Why:     "every structure element types as a standard structure type, so none has a mapping to ask about",
		}
	}
	return Result{Verdict: Pass}
}

// checkStandardTypeNotRemapped evaluates ua1 7.1 t7 (P03.S01).
//
// veraPDF asks it of EVERY structure element (`PDStructElem`) and fails the one whose `/S` is a standard
// type the role map sends somewhere else — `/Document → /Book` (7.1-t07-fail-a), and `/TR → /TD` in a
// fixture of nib's own, so a remap onto another standard type is still a remap. A self-map is not one:
// `/LI → /LI` passes here (7.1-t06-fail-a) and fails 7.1 t6 instead. A remap of a standard type no
// element uses passes, measured — the clause is about elements, not about the role map's contents.
func checkStandardTypeNotRemapped(d *Document) Result {
	nodes, unread := d.structNodes()
	var roleMap types.Dict
	if root := d.dict(d.Catalog["StructTreeRoot"]); root != nil {
		roleMap = d.dict(root["RoleMap"])
	}
	for _, n := range nodes {
		own := d.name(n.dict["S"])
		if !standardStructureTypes[own] || roleMap == nil {
			continue
		}
		if to := d.name(roleMap[own]); to != "" && to != own {
			return Result{
				Verdict: Fail,
				Why:     fmt.Sprintf("the role map sends the standard type /%s to /%s", own, to),
				Where:   nodeWhere(n, own),
			}
		}
	}
	if unread != "" {
		return Result{Verdict: CannotCheck, Why: unread}
	}
	if len(nodes) == 0 {
		return Result{
			Verdict: NotApplicable,
			Why:     "the document has no structure elements, so no element's standard type could be remapped",
		}
	}
	return Result{Verdict: Pass}
}

// checkStructTreeRoot evaluates ua1 7.1 t11.
//
// **`NotApplicable` is not available to this rule, and saying so is the point.** Every PDF/UA
// document must have logical structure — there is no document for which the clause has no subject.
// So the verdicts here are pass, fail, or cannot-check, and a rule that reached for
// `NotApplicable` would be excusing the absence the clause exists to find.
func checkStructTreeRoot(d *Document) Result {
	raw, present := d.Catalog["StructTreeRoot"]
	if !present {
		return Result{
			Verdict: Fail,
			Why:     "the document catalog has no /StructTreeRoot, so the document carries no logical structure at all",
			Where:   "catalog",
		}
	}
	root := d.dict(raw)
	if root == nil {
		return Result{
			Verdict: Fail,
			Why:     "the catalog's /StructTreeRoot does not resolve to a dictionary, so the structure it names is not reachable",
			Where:   "catalog /StructTreeRoot",
		}
	}
	if ty := d.name(root["Type"]); ty != "" && ty != "StructTreeRoot" {
		return Result{
			Verdict: Fail,
			Why:     fmt.Sprintf("the object the catalog names as /StructTreeRoot has /Type /%s", ty),
			Where:   "catalog /StructTreeRoot",
		}
	}
	// `/K` may legally be a single element rather than an array. **Asked before the array read**
	// (`/pending 496`): an array read of a dictionary is an error, so this branch sat below a `CannotCheck`
	// that every single-element root reached first.
	if single := d.dict(root["K"]); single != nil {
		if d.name(single["S"]) != "" {
			return Result{Verdict: Pass}
		}
		return Result{
			Verdict: CannotCheck,
			Why:     "the structure root's /K is a single dictionary that is not a structure element",
			Where:   "/StructTreeRoot /K",
		}
	}
	// **A root with no kids is the state ADR-031 law 1 is about**, and it is a fail rather than a
	// pass-on-a-technicality: the catalog claims structure and nothing hangs off it, which is what
	// `tagState.orphaned()` reports and what P01's phase review caught being rated `carried`.
	kids, kerr := d.Ctx.DereferenceArray(root["K"])
	if kerr != nil {
		return Result{
			Verdict: CannotCheck,
			Why:     fmt.Sprintf("the structure root's /K could not be resolved: %v", kerr),
			Where:   "/StructTreeRoot /K",
		}
	}
	if len(kids) == 0 {
		return Result{
			Verdict: Fail,
			Why:     "the structure root has no children, so the catalog claims logical structure that holds nothing",
			Where:   "/StructTreeRoot /K",
		}
	}
	return Result{Verdict: Pass}
}

// checkRoleMapCycle is the circular-role-map rule — `/pending 548`, the residue `/pending 507` left.
//
// 507 gave `standardType` a cycle guard, so nib HELD the fact that a document's `/RoleMap` sends a name
// around a loop and reported it only as `CannotCheck` — *"nib cannot type this element"* — over a document
// that is in fact non-conformant. This is the verdict that was missing. It is the twentieth of the 106
// rules veraPDF evaluates, and it is cheap because the detection was already written and already exercised.
//
// # The clause number the item asked for was wrong, and veraPDF is what said so
//
// `/pending 548`, `structure.go`'s own header and `docs/red-proofs.md` all called this **7.1 t5**, from the
// corpus file that carries the case being named `7.1 General/7.1-t05-fail-d.pdf`. The corpus's file naming
// is the specification's test numbering and it is NOT veraPDF's rule numbering. Measured, veraPDF 1.30.2
// over those six files:
//
//	7.1-t05-fail-a  7.1-5 failed 0/1   7.1-6 passed 14/0
//	7.1-t05-fail-b  7.1-5 failed 0/2   7.1-6 passed  4/0
//	7.1-t05-fail-c  7.1-5 failed 0/1   7.1-6 passed  3/0
//	7.1-t05-fail-d  7.1-5 passed 2/0   7.1-6 FAILED  2/2
//	7.1-t05-pass-a  7.1-5 passed 0/0   7.1-6 passed 14/0
//	7.1-t05-pass-b  7.1-5 passed 0/0   7.1-6 passed  4/0
//
// veraPDF's `7.1-5` is `isNotMappedToStandardType == false` over `SENonStandard` — *"all non-standard
// structure types shall be mapped to the nearest functionally equivalent standard type"* — and fail-d
// PASSES it, because its types are mapped; that they are mapped in a circle is a different clause.
// **`7.1-6` is `circularMappingExist != true`, and that is this rule.** A rule registered as `7.1 t5` would
// have been compared against the wrong veraPDF verdict on every one of the 297 corpus files.
//
// # The subject is the ELEMENT, not the role map — measured, not read off the prose
//
// *"A circular mapping shall not exist"* reads as a statement about the dictionary, which would make this a
// document-scoped rule with one verdict per file. It is not: veraPDF's object is `PDStructElem`, and
// fail-d's counts are **2 passed and 2 failed over its four elements** — the two whose `/S` lies on the
// loop fail, the `Document` and the `P` that do not lie on it pass.
//
// So **a cycle no element uses is not a failure**, which is the shape a document-scoped reading would get
// wrong in the direction that matters (a false `Fail` on every file a producer left a dead private mapping
// in). `TestACircularRoleMapNoElementUsesIsNotAFailure` pins it, and the oracle corpus measures it standing
// through a document carrying that exact mapping.
func checkRoleMapCycle(d *Document) Result {
	nodes, unread := d.structNodes()
	// **The search runs BEFORE the reach guard, and the order is the decision.** A cycle nib has already
	// seen is settled, and an unread tail below the depth bound cannot take it back; answering
	// `CannotCheck` there would point law 4's third verdict at something nib did establish. The reverse
	// is not true, which is why the guard is still here: an element past the bound may be the one on the
	// loop, so "no cycle among the elements nib read" is not "no cycle".
	for _, n := range nodes {
		if !d.roleMapCircular(n.dict) {
			continue
		}
		std, unresolved := d.standardType(n.dict)
		why := "this element's type is on a circular role mapping: " + unresolved
		if unresolved == "" {
			// The self-map. It resolves — a reader recognises the type before it ever consults the map —
			// so this branch is the one a rule reading `standardType`'s verdict alone cannot reach.
			why = fmt.Sprintf("the role map sends /%s to itself, which is a circular mapping; the element still "+
				"types as /%s, because a conforming reader recognises that type before it consults the map", std, std)
		}
		return Result{Verdict: Fail, Why: why, Where: nodeWhere(n, d.name(n.dict["S"]))}
	}
	if unread != "" {
		return Result{Verdict: CannotCheck, Why: unread}
	}
	if len(nodes) == 0 {
		// No element, no subject — veraPDF reports 0 passed and 0 failed checks over such a document, and
		// the missing structure is 7.1 t11's to report, not this rule's.
		return Result{
			Verdict: NotApplicable,
			Why:     "the document has no structure elements, so no element's type is resolved through the role map at all",
		}
	}
	return Result{Verdict: Pass}
}
