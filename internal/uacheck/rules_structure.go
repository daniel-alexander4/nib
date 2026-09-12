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
	if ty := root.NameEntry("Type"); ty != nil && *ty != "StructTreeRoot" {
		return Result{
			Verdict: Fail,
			Why:     fmt.Sprintf("the object the catalog names as /StructTreeRoot has /Type /%s", *ty),
			Where:   "catalog /StructTreeRoot",
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
		// `/K` may legally be a single element rather than an array.
		if single := d.dict(root["K"]); single != nil {
			if _, isElem := single["S"]; isElem {
				return Result{Verdict: Pass}
			}
		}
		return Result{
			Verdict: Fail,
			Why:     "the structure root has no children, so the catalog claims logical structure that holds nothing",
			Where:   "/StructTreeRoot /K",
		}
	}
	return Result{Verdict: Pass}
}
