package uacheck

import "fmt"

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
